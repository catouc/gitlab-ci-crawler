package crawler

import (
	"context"
	"errors"
	"fmt"
	"github.com/deichindianer/gitlab-ci-crawler/internal/storage"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deichindianer/gitlab-ci-crawler/internal/gitlab"
	"golang.org/x/time/rate"
)

const gitlabCIFileName = ".gitlab-ci.yml"

type Crawler struct {
	config       *Config
	gitlabClient *gitlab.Client
	storage      storage.Storage

	projectSetMut sync.RWMutex
	projectSet    map[string]struct{}
}

// New creates a new project crawler
// The caller is responsible for closing the neo4j driver and session
// the Crawl func handles this already.
func New(cfg *Config, store storage.Storage) (*Crawler, error) {
	httpClient := &rateLimitedHTTPClient{
		Client: &http.Client{
			Timeout: 5 * time.Second,
		},
		RateLimiter: rate.NewLimiter(rate.Limit(cfg.GitlabMaxRPS), cfg.GitlabMaxRPS),
	}

	gitlabClient := gitlab.NewClient(cfg.GitlabHost, cfg.GitlabToken, httpClient)

	return &Crawler{
		config:       cfg,
		gitlabClient: gitlabClient,
		storage:      store,
		projectSet:   make(map[string]struct{}),
	}, nil
}

// Crawl iterates through every project in the given GitLab host
// and parses the CI file and it's includes into the given Neo4j instance
func (c *Crawler) Crawl(ctx context.Context) error {

	log.Printf("Starting to crawl %s...\n", c.config.GitlabHost)
	resultChan := make(chan gitlab.Project, 200)

	go func() {
		if err := c.gitlabClient.StreamAllProjects(ctx, 100, resultChan); err != nil {
			log.Println(err)
		}
	}()

	for p := range resultChan {
		_, exists := c.projectSet[p.PathWithNamespace]
		if exists {
			continue
		}

		if err := c.updateProjectInGraph(ctx, p); err != nil {
			log.Printf("failed to parse project: %s", err)
		}
	}

	return nil
}

func (c *Crawler) updateProjectInGraph(ctx context.Context, project gitlab.Project) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		c.projectSetMut.RLock()
		_, exists := c.projectSet[project.PathWithNamespace]
		if exists {
			c.projectSetMut.RUnlock()
			return nil
		}
		c.projectSetMut.RUnlock()
		c.projectSetMut.Lock()
		c.projectSet[project.PathWithNamespace] = struct{}{}
		c.projectSetMut.Unlock()

		if err := c.storage.CreateProjectNode(project.PathWithNamespace); err != nil {
			return fmt.Errorf("failed to write project to neo4j: %w", err)
		}

		gitlabCIFile, err := c.gitlabClient.GetRawFileFromProject(ctx, project.ID, gitlabCIFileName, project.DefaultBranch)
		if err != nil {
			if errors.Is(err, gitlab.ErrRawFileNotFound) {
				return nil
			}
			return fmt.Errorf("failed to get file %s: %w", gitlabCIFileName, err)
		}

		includes, err := c.parseIncludes(gitlabCIFile)
		if err != nil {
			return fmt.Errorf("failed to parse includes for %d: %w", project.ID, err)
		}

		includes = enrichIncludes(includes, project)

		for _, i := range includes {
			if i.Ref == "" {
				log.Printf("Got empty ref for %s:%s", i.Project, strings.Join(i.Files, ","))
			}

			if err := c.traverseIncludes(project.PathWithNamespace, i); err != nil {
				log.Printf("failed to parse include %s: %s", i.Project, err)
			}
		}
		return nil
	}
}

func (c *Crawler) traverseIncludes(parentName string, include RemoteInclude) error {
	c.projectSetMut.Lock()
	c.projectSet[include.Project] = struct{}{}
	c.projectSetMut.Unlock()

	if err := c.storage.CreateProjectNode(include.Project); err != nil {
		return fmt.Errorf("failed to write project to neo4j: %w", err)
	}

	if err := c.storage.CreateIncludeEdge(storage.IncludeEdge{
		SourceProject: parentName,
		TargetProject: include.Project,
		Ref:           include.Ref,
		Files:         include.Files,
	}); err != nil {
		return fmt.Errorf("failed to write neo4j transaction: %w", err)
	}

	for _, ci := range include.Children {
		if err := c.traverseIncludes(ci.Project, ci); err != nil {
			return fmt.Errorf("failed to write child includes for %s: %w", ci.Project, err)
		}
	}

	return nil
}
