package projects

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"

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

func (s *Source) GatherProjectData(input GatherProjectDataInput) error {

	projectsChan := make(chan *gitlab.Project, 200)

	if err := os.MkdirAll(input.OutPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output dir %s: %w", input.OutPath, err)
	}

	if input.GroupID == 0 {
		fmt.Println("iterating over all projects")
		go s.iterateAllProjects(projectsChan)
	} else {
		fmt.Println("iterating over group")
		go s.iterateProjectsFromGroup(projectsChan, input.GroupID)
	}

	workWG := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		workWG.Add(1)
		go func() {
			defer workWG.Done()
			for p := range projectsChan {

				includes, err := s.TraverseCIFileFromProject(p)
				if err != nil {
					continue
				}

				//ciFile, err := s.getCIFile(*p)
				//if err != nil {
				//	//log.Printf("failed to get CI file from %d: %s", p.ID, err)
				//	continue
				//}
				//
				//includes, err := s.parseIncludes(ciFile)
				//if err != nil {
				//	log.Printf("failed to parse CI file of %d: %s", p.ID, err)
				//	continue
				//}
				//
				for _, include := range includes {
					for _, f := range include.Files {
						fmt.Printf("project %d has direct include %s/%s on ref %s\n", p.ID, include.Project, f, include.Ref)
					}
					for _, cInclude := range include.Children {
						for _, f := range cInclude.Files {
							fmt.Printf("project %d has indirect include %s/%s on ref %s\n", p.ID, cInclude.Project, f, cInclude.Ref)
						}
					}
				}

			}
		}()
	}

	workWG.Wait()
	return nil
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

	//if err := ioutil.WriteFile(filePath, file, 0644); err != nil {
	//	return fmt.Errorf("failed to dump project to disk: %w", err)
	//}

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
