package projects

import (
	"fmt"
	"io"
	"log"
	_ "os"
	"sync"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"github.com/xanzy/go-gitlab"
)

const gitlabCIFile = ".gitlab-ci.yml"

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

	p, _, err := s.gitlabClient.Projects.GetProject(projectID, nil)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	includes, err := s.TraverseCIFileFromProject(p)
	if err != nil {
		return fmt.Errorf("failed to traverse CI file from project: %w", err)
	}

	g := graphviz.New()
	graph, err := g.Graph()
	if err != nil {
		return fmt.Errorf("failed to create graph: %w", err)
	}

	rootInclude := Include{
		Project:  p.Name,
		Ref:      p.DefaultBranch,
		Files:    nil,
		Children: includes,
	}

	rootNode, err := graph.CreateNode(p.Name)
	if err != nil {
		return fmt.Errorf("failed to create rootNode: %s", err)
	}

	populateGraph(graph, rootInclude, rootNode)

	if err := g.RenderFilename(graph, graphviz.PNG, "./graph.png"); err != nil {
		return fmt.Errorf("failed to create graph: %w", err)
	}
	return nil
}

//func (s *Source) GatherProjectData(input GatherProjectDataInput) error {
//
//	projectsChan := make(chan *gitlab.Project, 200)
//
//	if err := os.MkdirAll(input.OutPath, os.ModePerm); err != nil {
//		return fmt.Errorf("failed to create output dir %s: %w", input.OutPath, err)
//	}
//
//	if input.GroupID == 0 {
//		fmt.Println("iterating over all projects")
//		go s.iterateAllProjects(projectsChan)
//	} else {
//		fmt.Println("iterating over group")
//		go s.iterateProjectsFromGroup(projectsChan, input.GroupID)
//	}
//
//	workWG := sync.WaitGroup{}
//	for i := 0; i < 100; i++ {
//		workWG.Add(1)
//		go func() {
//			defer workWG.Done()
//			for p := range projectsChan {
//
//				includes, err := s.TraverseCIFileFromProject(p)
//				if err != nil {
//					continue
//				}
//
//				g := graphviz.New()
//				graph, err := g.Graph()
//				if err != nil {
//					fmt.Printf("failed to create graph: %s", err)
//				}
//
//				populateGraph(graph, nil, includes)
//
//				if err := g.RenderFilename(graph, graphviz.PNG, "./graph.png"); err != nil {
//					log.Printf("failed to create graph: %s", err)
//				}
//			}
//		}()
//	}
//
//	workWG.Wait()
//	return nil
//}

func populateGraph(graph *cgraph.Graph, currentInclude Include, parentNode *cgraph.Node) {
	//for _, f := range currentInclude.Files {
	//	fNode, err := graph.CreateNode(f)
	//	if err != nil {
	//		log.Printf("failed to create file node: %s", err)
	//	}
	//
	//	fNode.SetFillColor("lightgreen")
	//	fNode.SetStyle("dotted")
	//
	//	fEdge, err := graph.CreateEdge("file", parentNode, fNode)
	//	if err != nil {
	//		log.Printf("failed to create file edge: %s", err)
	//	}
	//	fEdge.SetLabel("uses")
	//}

	for _, cI := range currentInclude.Children {
		if cI.Project == "" {
			continue
		}

		childIncludeNode, err := graph.CreateNode(fmt.Sprintf("%s:%s", cI.Project, cI.Ref))
		if err != nil {
			log.Printf("failed to create child include node: %s", err)
		}

		childIncludeNode.SetFillColor("lightred")

		cEdge, err := graph.CreateEdge("include", parentNode, childIncludeNode)
		if err != nil {
			log.Printf("failed to create child include edge: %s", err)
		}
		cEdge.SetLabel("includes")

		populateGraph(graph, cI, parentNode)
	}
}

func (s *Source) getCIFile(project gitlab.Project) ([]byte, error) {
	file, resp, err := s.gitlabClient.RepositoryFiles.GetRawFile(project.ID, gitlabCIFile, &gitlab.GetRawFileOptions{Ref: &project.DefaultBranch})
	if err != nil {
		return nil, fmt.Errorf("failed to get ci file from project %d: %w", project.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("got non 2xx response from Gitlab: %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("got non 2xx response from GitLab: %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return file, nil
}

func (s *Source) iterateProjectsFromGroup(resultChan chan<- *gitlab.Project, groupID int) {
	for nextPage := 1; nextPage != 0; {
		projects, resp, err := s.gitlabClient.Groups.ListGroupProjects(groupID, &gitlab.ListGroupProjectsOptions{
			IncludeSubgroups: gitlab.Bool(true),
			ListOptions:      gitlab.ListOptions{Page: nextPage},
		})
		if err != nil {
			log.Fatalf("failed to get projects from gitlab: %s", err)
		}

		for _, p := range projects {
			resultChan <- p
		}

		nextPage = resp.NextPage
	}

	close(resultChan)
}

type listProjectOutput struct {
	projects       []*gitlab.Project
	gitlabResponse *gitlab.Response
}

func (s *Source) iterateAllProjects(resultChan chan<- *gitlab.Project) {
	wg := sync.WaitGroup{}
	for nextPage := 1; nextPage != 0; {
		wg.Add(1)
		go func(page int) {
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
