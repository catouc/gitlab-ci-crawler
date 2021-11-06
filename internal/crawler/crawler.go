package crawler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deichindianer/gitlab-ci-crawler/internal/gitlab"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"golang.org/x/time/rate"
)

const gitlabCIFileName = ".gitlab-ci.yml"

type Crawler struct {
	config          Config
	gitlabClient    *gitlab.Client
	neo4jConnection neo4j.Driver
	neo4jSession    neo4j.Session

	projectSetMut sync.RWMutex
	projectSet    map[string]struct{}
}

type Config struct {
	GitlabHost    string `conf:"required,short:g,env:GITLAB_HOST"`
	GitlabToken   string `conf:"required,short:t,env:GITLAB_TOKEN"`
	GitlabMaxRPS  int    `conf:"default:1,short:r,env:GITLAB_MAX_RPS"`
	Neo4jHost     string `conf:"default:bolt://127.0.0.1:7687,flag:neo4j-host,short:n,env:NEO4J_HOST"`
	Neo4jUsername string `conf:"default:neo4j,flag:neo4j-username,short:u,env:NEO4J_USERNAME"`
	Neo4jPassword string `conf:"required,flag:neo4j-password,short:w,env:NEO4J_PASSWORD"`
}

// New creates a new project crawler
// The caller is responsible for closing the neo4j driver and session
// the Crawl func handles this already.
func New(cfg Config) (*Crawler, error) {
	httpClient := &rateLimitedHTTPClient{
		Client: &http.Client{
			Timeout: 5 * time.Second,
		},
		RateLimiter: rate.NewLimiter(rate.Limit(cfg.GitlabMaxRPS), cfg.GitlabMaxRPS),
	}

	gitlabClient := gitlab.NewClient(cfg.GitlabHost, cfg.GitlabToken, httpClient)

	driver, err := neo4j.NewDriver(cfg.Neo4jHost, neo4j.BasicAuth(cfg.Neo4jUsername, cfg.Neo4jPassword, ""))
	if err != nil {
		return nil, fmt.Errorf("failed to create neo4j driver: %w", err)
	}

	session := driver.NewSession(neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeWrite,
	})

	return &Crawler{
		config:          cfg,
		gitlabClient:    gitlabClient,
		neo4jConnection: driver,
		neo4jSession:    session,
		projectSet:      make(map[string]struct{}),
	}, nil
}

// Crawl iterates through every project in the given GitLab host
// and parses the CI file and it's includes into the given Neo4j instance
func (c *Crawler) Crawl(ctx context.Context) error {
	defer c.neo4jSession.Close()
	defer c.neo4jConnection.Close()

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

		projectCypher := fmt.Sprintf("MERGE (p:Project {name: '%s'})", project.PathWithNamespace)
		if err := c.createNode(projectCypher, nil); err != nil {
			return fmt.Errorf("failed to write project to neo4j: %w", err)
		}

		gitlabCIFile, err := c.gitlabClient.GetRawFileFromProject(ctx, project.ID, gitlabCIFileName, project.DefaultBranch)
		if err != nil {
			if errors.Is(err, gitlab.ErrRawFileNotFound) {
				return nil
			}
			return fmt.Errorf("failed to get file %s: %w", gitlabCIFileName, err)
		}

		includes, err := c.parseIncludes(gitlabCIFile, project)
		if err != nil {
			return fmt.Errorf("failed to parse includes for %d: %w", project.ID, err)
		}

		for _, i := range includes {
			if i.Ref == "" {
				log.Printf("Got empty ref for %s:%s", i.Project, strings.Join(i.Files, ","))
			}
			//log.Printf("Iterating over %s-%s\n", i.Project, i.Files)
			if err := c.traverseIncludes(project.PathWithNamespace, i); err != nil {
				return fmt.Errorf("failed to parse include %s: %w", i.Project, err)
			}
		}
		return nil
	}
}

func (c *Crawler) traverseIncludes(parentName string, include RemoteInclude) error {
	c.projectSetMut.Lock()
	c.projectSet[include.Project] = struct{}{}
	c.projectSetMut.Unlock()

	includeCypher := fmt.Sprintf("MERGE (p:Project {name: '%s'})", include.Project)
	if err := c.createNode(includeCypher, nil); err != nil {
		return fmt.Errorf("failed to write project to neo4j: %w", err)
	}

	pCypher := fmt.Sprintf("MATCH (p:Project {name: '%s'})\nMATCH (p2:Project {name: '%s'})\nMERGE (p)-[rel:INCLUDES {ref: '%s', files:'%s'}]->(p2)", parentName, include.Project, include.Ref, strings.Join(include.Files, ","))
	if err := c.createNode(pCypher, nil); err != nil {
		return fmt.Errorf("failed to write neo4j transaction: %w", err)
	}

	for _, ci := range include.Children {
		if err := c.traverseIncludes(ci.Project, ci); err != nil {
			return fmt.Errorf("failed to write child includes for %s: %w", ci.Project, err)
		}
	}

	return nil
}

func (c *Crawler) createNode(cypher string, params map[string]interface{}) error {
	_, err := c.neo4jSession.WriteTransaction(func(transaction neo4j.Transaction) (interface{}, error) {
		result, err := transaction.Run(cypher, params)
		if err != nil {
			return nil, err
		}

		if result.Next() {
			return nil, nil
		}

		return nil, result.Err()
	})
	return err
}
