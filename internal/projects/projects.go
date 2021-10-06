package projects

import (
	"fmt"
	"log"
	"strings"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"github.com/xanzy/go-gitlab"
)

const gitlabCIFile = ".gitlab-ci.yml"

var Debug bool

type Source struct {
	gitlabClient *gitlab.Client
	neo4jDriver  neo4j.Driver
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

	return &Source{
		gitlabClient: gitlabClient,
		neo4jDriver:  driver,
	}, nil
}

func (s *Source) PlotDependencyTree(projectID int) error {

	if Debug {
		log.Printf("DEBUG: Plotting tree for project %d", projectID)
	}

	defer s.neo4jDriver.Close()

	session := s.neo4jDriver.NewSession(neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeWrite,
	})
	defer session.Close()

	// Get the project and empty graph
	g := graphviz.New()
	graph, err := g.Graph()
	if err != nil {
		return fmt.Errorf("failed to create graph: %w", err)
	}

	p, _, err := s.gitlabClient.Projects.GetProject(projectID, nil)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}
	log.Printf("processing project at %s", p.WebURL)

	rootNodeName := fmt.Sprintf("Project:%s:%s", p.Name, gitlabCIFile)
	rootNode, err := graph.CreateNode(rootNodeName)
	if err != nil {
		log.Printf("failed to create node %s", err)
	}

	if err := createNode(session, "CREATE (p:Project {name: $name})", map[string]interface{}{"name": fmt.Sprintf("%s:.gitlab-ci.yml", p.Name)}); err != nil {
		return fmt.Errorf("failed to write neo4j transaction: %w", err)
	}

	// Parse the pipeline yaml file and start the recursion on each of the sub-files
	s.populateGraph(session, graph, rootNode, p.ID, gitlabCIFile)

	// Render the graph
	if err := g.RenderFilename(graph, graphviz.PNG, "./graph.png"); err != nil {
		return fmt.Errorf("failed to create graph: %w", err)
	}
	return nil
}

func createNode(session neo4j.Session, cypher string, params map[string]interface{}) error {
	_, err := session.WriteTransaction(func(transaction neo4j.Transaction) (interface{}, error) {
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

func (s *Source) populateGraph(session neo4j.Session, graph *cgraph.Graph, parentNode *cgraph.Node, projectIDorURL interface{}, fileName string) error {
	var err error

	if Debug {
		log.Printf("DEBUG: Populating graph for parent %s on project %s with file %s", parentNode.Name(), projectIDorURL, fileName)
	}

	// First lets get the file we've been asked to process and get its includes
	p, _, err := s.gitlabClient.Projects.GetProject(projectIDorURL, nil)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	file, err := s.getRawFileFromProject(projectIDorURL, fileName, p.DefaultBranch)
	if err != nil {
		return fmt.Errorf("failed to get file %s: %w", gitlabCIFile, err)
	}

	includes, err := s.parseIncludes(file)
	if err != nil {
		return fmt.Errorf("failed to parse includes for %d: %w", projectIDorURL, err)
	}

	// Now we create the nodes for each of the included projects and included files
	for _, includedProject := range includes {
		log.Printf("found included project %s:%s", includedProject.Project, includedProject.Ref)

		// Create project node
		templateNode, err := graph.CreateNode("Template:" + includedProject.Project + ":" + includedProject.Ref)
		if err != nil {
			log.Printf("failed to create file node: %s", err)
		}
		templateNode.SetFillColor("lightblue")

		if err := createNode(session, "CREATE (t:Template {name: '"+includedProject.Project+":"+includedProject.Ref+"'})", nil); err != nil {
			log.Printf("failed to write neo4j transaction: %s\n", err)
		}

		// Create file nodes
		for _, includedFile := range includedProject.Files {
			log.Printf("found included file %s", includedFile)

			fileNode, err := graph.CreateNode("File:" + includedProject.Project + ":" + includedFile)
			if err != nil {
				log.Printf("failed to create file node: %s", err)
			}
			fileNode.SetFillColor("lightgreen")
			fileNode.SetStyle("dotted")

			if err := createNode(session, "CREATE (f:File {name: $name})", map[string]interface{}{"name": includedProject.Project + ":" + includedFile}); err != nil {
				log.Printf("failed to write neo4j transaction: %s\n", err)
			}

			templateEdge, err := graph.CreateEdge("file", fileNode, templateNode)
			if err != nil {
				log.Printf("failed to create file edge: %s", err)
			}
			templateEdge.SetLabel("owned by")

			if err := createNode(session, "CREATE (f:File {name: $name})", map[string]interface{}{"name": includedProject.Project + ":" + includedFile}); err != nil {
				log.Printf("failed to write neo4j transaction: %s\n", err)
			}

			fileEdge, err := graph.CreateEdge("file", parentNode, fileNode)
			if err != nil {
				log.Printf("failed to create file edge: %s", err)
			}
			fileEdge.SetLabel("includes")

			tCypher := fmt.Sprintf("MATCH (t:Template {name: '%s'})\nMATCH (f:File {name: '%s'})\nCREATE (t)-[rel:OWNED_BY]->(f)", includedProject.Project+":"+includedProject.Ref, includedProject.Project+":"+includedFile)
			if err := createNode(session, tCypher, nil); err != nil {
				log.Printf("failed to write neo4j transaction: %s\n", err)
			}

			fCypher := fmt.Sprintf("MATCH (p:File {name: '%s'})\nMATCH (f:File {name: '%s'})\nCREATE (p)-[rel:INCLUDES]->(f)", strings.TrimPrefix(parentNode.Name(), "File:"), includedProject.Project+":"+includedFile)
			if err := createNode(session, fCypher, nil); err != nil {
				log.Printf("failed to write neo4j transaction: %s\n", err)
			}

			pCypher := fmt.Sprintf("MATCH (p:Project {name: '%s'})\nMATCH (f:File {name: '%s'})\nCREATE (p)-[rel:INCLUDES]->(f)", strings.TrimPrefix(parentNode.Name(), "Project:"), includedProject.Project+":"+includedFile)
			if err := createNode(session, pCypher, nil); err != nil {
				log.Printf("failed to write neo4j transaction: %s\n", err)
			}

			// Now recurse downward to do the same again for this file
			s.populateGraph(session, graph, fileNode, includedProject.Project, includedFile)

		}
	}

	return nil
}
