package projects

import (
	"fmt"
	"io"
	"log"
	"strings"

	_ "github.com/goccy/go-graphviz"
	"github.com/xanzy/go-gitlab"
	"gopkg.in/yaml.v3"
)

type Template struct {
	Includes []Include `yaml:"include"`
}

type Include struct {
	Project  string      `yaml:"project"`
	Ref      string      `yaml:"ref"`
	Files    StringArray `yaml:"file"`
	Children []Include
}

type StringArray []string

func (a *StringArray) UnmarshalYAML(value *yaml.Node) error {
	var multi []string
	err := value.Decode(&multi)
	if err != nil {
		var single string
		err := value.Decode(&single)
		if err != nil {
			return err
		}
		*a = []string{single}
	} else {
		*a = multi
	}
	return nil
}

func (s *Source) TraverseCIFileFromProject(project *gitlab.Project) ([]Include, error) {
	file, err := s.getRawFileFromProject(project.ID, gitlabCIFile, project.DefaultBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get file %s: %w", gitlabCIFile, err)
	}

	includes, err := s.parseIncludes(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse includes for %d: %w", project.ID, err)
	}

	allIncludes := s.traverseInclude(includes)
	return allIncludes, nil
}

func (s *Source) traverseInclude(includes []Include) []Include {
	result := make([]Include, len(includes))
	for _, i := range includes {
		children, err := s.getIncludeChildren(i)
		if err != nil {
			log.Printf("failed to get children of include: %s", err)
		}
		i.Children = s.traverseInclude(children)
		result = append(result, i)
	}
	return result
}

func (s *Source) getIncludeChildren(i Include) ([]Include, error) {
	var children []Include

	for _, f := range i.Files {
		trimmedFileName := strings.Trim(strings.Trim(f, "\""), "/")
		file, err := s.getRawFileFromProject(i.Project, trimmedFileName, i.Ref)
		if err != nil {
			return nil, fmt.Errorf("failed to get file %s: %w", f, err)
		}

		includes, err := s.parseIncludes(file)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal file %s: %w", f, err)
		}

		children = append(children, includes...)
	}

	return children, nil
}

func (s *Source) parseIncludes(file []byte) ([]Include, error) {
	var parsed Template
	if err := yaml.Unmarshal(file, &parsed); err != nil {
		return nil, fmt.Errorf("failed to marshal file into include format: %w", err)
	}
	return parsed.Includes, nil
}

func (s *Source) getRawFileFromProject(projectID interface{}, fileName, ref string) ([]byte, error) {
	file, resp, err := s.gitlabClient.RepositoryFiles.GetRawFile(projectID, fileName, &gitlab.GetRawFileOptions{Ref: &ref})
	if err != nil {
		return nil, fmt.Errorf("failed to get %s from project %+v: %w", fileName, projectID, err)
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
