package crawler

import (
	"context"
	"errors"
	"fmt"
	"github.com/catouc/gitlab-ci-crawler/internal/gitlab"
	"github.com/catouc/gitlab-ci-crawler/internal/storage"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
	"net/http"
	"strings"
)

const gitlabCIFileName = ".gitlab-ci.yml"

type Crawler struct {
	config       *Config
	gitlabClient *gitlab.Client
	storage      storage.Storage
	logger       zerolog.Logger
	nWorkers     int
}

// New creates a new project crawler
// The caller is responsible for closing the neo4j driver and session
// the Crawl func handles this already.
func New(cfg *Config, logger zerolog.Logger, store storage.Storage) (*Crawler, error) {
	retryClient := retryablehttp.NewClient()

	retryClient.RetryMax = cfg.HTTPClientMaxRetry
	retryClient.RetryWaitMax = cfg.HTTPClientMaxRetryWait
	retryClient.RetryWaitMin = cfg.HTTPClientMinRetryWait
	retryClient.HTTPClient = &http.Client{Timeout: cfg.HTTPClientTimeout}

	httpClient := &rateLimitedHTTPClient{
		Client:      retryClient.StandardClient(),
		RateLimiter: rate.NewLimiter(rate.Limit(cfg.GitlabMaxRPS), cfg.GitlabMaxRPS),
	}

	gitlabClient := gitlab.NewClient(cfg.GitlabHost, cfg.GitlabToken, httpClient, logger)

	return &Crawler{
		config:       cfg,
		gitlabClient: gitlabClient,
		storage:      store,
		logger:       logger,
		nWorkers:     cfg.NumberOfWorkers,
	}, nil
}

// Crawl iterates through every project in the given GitLab host
// and parses the CI file, and it's includes into the given Neo4j instance
func (c *Crawler) Crawl(ctx context.Context) error {
	if c.config.StorageCleanup {
		c.logger.Info().Msg("Cleanup storage...")
		err := c.storage.RemoveAll(ctx)
		if err != nil {
			return err
		}
	}

	c.logger.Info().Msg("Starting to crawl...")
	resultChan := make(chan gitlab.Project, 200)

	var streamOK bool

	go func() {
		defer close(resultChan)

		if err := c.gitlabClient.StreamAllProjects(ctx, 100, resultChan); err != nil {
			c.logger.Err(err).Msg("stopping crawler: error in project stream")
			return
		}

		streamOK = true
	}()

	errs, ctx := errgroup.WithContext(ctx)

	for i := 0; i < c.nWorkers; i++ {
		errs.Go(
			func() error {
				err := c.updateProjectInGraphWorker(ctx, resultChan)
				return err
			})
	}
	err := errs.Wait()
	if err != nil {
		return err
	}

	if !streamOK {
		return errors.New("stream failed")
	}

	c.logger.Info().Msg("stopped crawling")
	return nil
}

func (c *Crawler) updateProjectInGraphWorker(ctx context.Context, projects chan gitlab.Project) error {
	for p := range projects {
		if err := c.updateProjectInGraph(ctx, p); err != nil {
			c.logger.Err(err).
				Str("ProjectPath", p.PathWithNamespace).
				Int("ProjectID", p.ID).
				Msg("failed to parse project")
			return err
		}
	}
	return nil
}

func (c *Crawler) updateProjectInGraph(ctx context.Context, project gitlab.Project) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		if err := c.storage.CreateProjectNode(ctx, project.PathWithNamespace); err != nil {
			return fmt.Errorf("failed to write project to neo4j: %w", err)
		}

		if len(project.DefaultBranch) == 0 {
			c.logger.Debug().
				Str("Project", project.PathWithNamespace).
				Msg("Project has no DefaultBranch")

			return nil
		}

		err := c.handleIncludes(ctx, project, gitlabCIFileName, make(map[string]bool))
		if err != nil {
			c.logger.Error().
				Err(err).
				Str("Project", project.PathWithNamespace).
				Msg("failed to handle all includes")
		}
		return nil
	}
}

func (c *Crawler) handleIncludes(ctx context.Context, project gitlab.Project, filePath string, cycleDetectionMap map[string]bool) error {
	if cycleDetectionMap[project.PathWithNamespace + "--" + filePath] {
		projectsVisited := make([]string, 0, len(cycleDetectionMap))
		for k := range cycleDetectionMap {
			projectsVisited = append(projectsVisited, k)
		}
		return errors.New("cycle detected, this should not be possible, the projects visited are: " + strings.Join(projectsVisited[:], ","))
	}
	cycleDetectionMap[project.PathWithNamespace + "--" + filePath] = true

	gitlabCIFile, err := c.gitlabClient.GetRawFileFromProject(ctx, project.ID, filePath, project.DefaultBranch)
	if err != nil {
		if errors.Is(err, gitlab.ErrRawFileNotFound) {
			return nil
		}
		return fmt.Errorf("failed to get file %s: %w", filePath, err)
	}

	triggers, err := c.parseTriggers(gitlabCIFile)
	if err != nil {
		return fmt.Errorf("failed to parse triggers: %w", err)
	}

	triggers = c.enrichTriggers(triggers, project.PathWithNamespace)

	for _, trigger := range triggers {
		c.logger.Debug().Dict("trigger", zerolog.Dict().
			Str("Project", trigger.Project).
			Str("SourceProject", project.PathWithNamespace),
		).Msg("")
		err := c.storage.CreateTriggerEdge(ctx, storage.Edge{
			SourceProject: project.PathWithNamespace,
			TargetProject: trigger.Project,
			Ref: trigger.Branch,
		})
		if err != nil {
			c.logger.Err(err).
				Str("Project", project.PathWithNamespace).
				Msg("failed to create trigger edge")
		}
	}

	includes, err := c.parseIncludes(gitlabCIFile)
	if err != nil {
		return fmt.Errorf("failed to parse includes: %w", err)
	}

	includes = c.enrichIncludes(
		includes,
		project.DefaultBranch,
		project.PathWithNamespace,
		c.config.DefaultRefName,
	)

	for _, i := range includes {
		if i.Ref == "" {
			c.logger.Warn().
				Str("Project", i.Project).
				Str("Files", strings.Join(i.Files, ",")).
				Msg("Got empty ref")
		}

		if err = c.traverseIncludes(ctx, project.PathWithNamespace, i); err != nil {
			c.logger.Err(err).
				Str("Project", i.Project).
				Msg("failed to parse include")
		}

		p, err := c.gitlabClient.GetProjectFromPath(ctx, i.Project)
		if err != nil {
			return err
		}

		for _, f := range i.Files {
			err = c.handleIncludes(ctx, p, f, cycleDetectionMap)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Crawler) traverseIncludes(ctx context.Context, parentName string, include RemoteInclude) error {

	if err := c.storage.CreateProjectNode(ctx, include.Project); err != nil {
		return fmt.Errorf("failed to write project to neo4j: %w", err)
	}

	if err := c.storage.CreateIncludeEdge(ctx, storage.Edge{
		SourceProject: parentName,
		TargetProject: include.Project,
		Ref:           include.Ref,
		Files:         include.Files,
	}); err != nil {
		return fmt.Errorf("failed to write neo4j transaction: %w", err)
	}

	return nil
}
