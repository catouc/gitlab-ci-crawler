package projects

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"github.com/xanzy/go-gitlab"
)

const gitlabCIFileName = ".gitlab-ci.yml"

var Debug bool

type Source struct {
	gitlabClient    *gitlab.Client
	neo4jConnection neo4j.Driver
	neo4jSession    neo4j.Session
}

type GatherProjectDataInput struct {
	OutPath string
	GroupID int
}

type Config struct {
	GitlabHost    string
	GitlabToken   string
	Neo4jHost     string
	Neo4jUsername string
	Neo4jPassword string
}

type Project struct {
	gitlab.Project
	Includes []Project
}

// New creates a new project crawler thing
// The caller is responsible for closing the neo4j driver!
func New(cfg Config) (*Source, error) {
	gitlabClient, err := gitlab.NewClient(cfg.GitlabToken, gitlab.WithBaseURL(cfg.GitlabHost))
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab client: %w", err)
	}

	driver, err := neo4j.NewDriver(cfg.Neo4jHost, neo4j.BasicAuth(cfg.Neo4jUsername, cfg.Neo4jPassword, ""))
	if err != nil {
		return nil, fmt.Errorf("failed to create neo4j driver: %w", err)
	}

	session := driver.NewSession(neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeWrite,
	})

	return &Source{
		gitlabClient:    gitlabClient,
		neo4jConnection: driver,
		neo4jSession:    session,
	}, nil
}

func (s *Source) RunForestRun() error {
	resultChan := make(chan *gitlab.Project, 200)

	go s.iterateAllProjects(resultChan)

	for p := range resultChan {
		if err := s.ParseProject(p); err != nil {
			log.Printf("failed to parse project: %s", err)
		}
	}

	return nil
}

func (s *Source) createNode(cypher string, params map[string]interface{}) error {
	_, err := s.neo4jSession.WriteTransaction(func(transaction neo4j.Transaction) (interface{}, error) {
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

func (s *Source) ParseProject(project *gitlab.Project) error {

	projectCypher := fmt.Sprintf("MERGE (p:Project {name: '%s'})", project.PathWithNamespace)
	if err := s.createNode(projectCypher, nil); err != nil {
		return fmt.Errorf("failed to write project to neo4j: %w", err)
	}

	gitlabCIFile, err := s.getRawFileFromProject(project.ID, gitlabCIFileName, project.DefaultBranch)
	if err != nil {
		return fmt.Errorf("failed to get file %s: %w", gitlabCIFileName, err)
	}

	includes, err := s.parseIncludes(gitlabCIFile)
	if err != nil {
		return fmt.Errorf("failed to parse includes for %d: %w", project.ID, err)
	}

	for _, i := range includes {
		includeCypher := fmt.Sprintf("MERGE (p:Project {name: '%s'})", i.Project)
		if err := s.createNode(includeCypher, nil); err != nil {
			return fmt.Errorf("failed to write project to neo4j: %w", err)
		}

		pCypher := fmt.Sprintf("MATCH (p:Project {name: '%s'})\nMATCH (p2:Project {name: '%s'})\nMERGE (p)-[rel:INCLUDES {ref: '%s', files:'%s'}]->(p2)", project.PathWithNamespace, i.Project, i.Ref, strings.Join(i.Files, ","))
		if err := s.createNode(pCypher, nil); err != nil {
			return fmt.Errorf("failed to write neo4j transaction: %w", err)
		}
	}

	return nil
}

type listProjectOutput struct {
	projects       []*gitlab.Project
	gitlabResponse *gitlab.Response
}

func (s *Source) iterateAllProjects(resultChan chan<- *gitlab.Project) {
	wg := sync.WaitGroup{}
	for nextPage := 1; nextPage != 0; {
		log.Printf("curr page %d", nextPage)
		wg.Add(1)

		func(page int) {
			pp := s.getProjectsFromPage(page)

			nextPage = pp.gitlabResponse.NextPage
			for _, p := range pp.projects {
				resultChan <- p
			}
			wg.Done()
		}(nextPage)
	}

	wg.Wait()
	close(resultChan)
}

func (s *Source) getProjectsFromPage(page int) listProjectOutput {
	projects, resp, err := s.gitlabClient.Projects.ListProjects(&gitlab.ListProjectsOptions{
		ListOptions: gitlab.ListOptions{
			Page:    page,
			PerPage: 100,
		},
	})
	if err != nil {
		log.Printf("failed to get projects from page %d: %s", page, err)
		return listProjectOutput{}
	}

	result := listProjectOutput{
		projects:       projects,
		gitlabResponse: resp,
	}

	return result
}
