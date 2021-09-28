package projects

import (
	"fmt"
	"io"

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
