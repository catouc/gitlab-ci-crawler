package projects

import (
	"fmt"
	"log"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"github.com/xanzy/go-gitlab"
)

const gitlabCIFile = ".gitlab-ci.yml"
var Debug bool

type Source struct {
	gitlabClient *gitlab.Client
}

type GatherProjectDataInput struct {
	OutPath string
	GroupID int
}

func New(gitlabHost, gitlabToken string) (*Source, error) {
	gitlabClient, err := gitlab.NewClient(gitlabToken, gitlab.WithBaseURL(gitlabHost))
	if err != nil {
		return nil, err
	}
	return &Source{gitlabClient: gitlabClient}, nil
}

func (s *Source) PlotDependencyTree(projectID int) error {

	if Debug {
		log.Printf("DEBUG: Plotting tree for project %d", projectID)
	}

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

	// Parse the pipeline yaml file and start the recursion on each of the sub-files
	s.populateGraph(graph, rootNode, p.ID, gitlabCIFile)

	// Render the graph
	if err := g.RenderFilename(graph, graphviz.PNG, "./graph.png"); err != nil {
		return fmt.Errorf("failed to create graph: %w", err)
	}
	return nil
}

func (s *Source) populateGraph(graph *cgraph.Graph, parentNode *cgraph.Node, projectIDorURL interface{}, fileName string) error {
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

		// Create file nodes
		for _, includedFile := range includedProject.Files {
			log.Printf("found included file %s", includedFile)	

			fileNode, err := graph.CreateNode("File:" + includedProject.Project + ":" + includedFile)
			if err != nil {
				log.Printf("failed to create file node: %s", err)
			}	
			fileNode.SetFillColor("lightgreen")
			fileNode.SetStyle("dotted")
	
			templateEdge, err := graph.CreateEdge("file", fileNode, templateNode)
			if err != nil {
				log.Printf("failed to create file edge: %s", err)
			}
			templateEdge.SetLabel("owned by")

			fileEdge, err := graph.CreateEdge("file", parentNode, fileNode)
			if err != nil {
				log.Printf("failed to create file edge: %s", err)
			}
			fileEdge.SetLabel("includes")
			
			// Now recurse downward to do the same again for this file
			s.populateGraph(graph, fileNode, includedProject.Project, includedFile)

		}
	}

	return nil
}
