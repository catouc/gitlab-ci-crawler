package crawler

import (
	"fmt"
	"log"
	"strings"

	"github.com/deichindianer/gitlab-ci-crawler/internal/gitlab"
	"gopkg.in/yaml.v3"
)

type Template struct {
	Includes []RemoteInclude `yaml:"include"`
}

type RemoteInclude struct {
	Project  string      `yaml:"project"`
	Ref      string      `yaml:"ref"`
	Files    StringArray `yaml:"file"`
	Local    string      `yaml:"local"`
	Template string      `yaml:"template"`
	Children []RemoteInclude
}

type LocalIncludetemplate struct {
	Includes string `yaml:"include"`
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

func (c *Crawler) parseIncludes(file []byte, project gitlab.Project) ([]RemoteInclude, error) {
	var parsed Template
	if err := yaml.Unmarshal(file, &parsed); err != nil {
		return nil, fmt.Errorf("failed to marshal file into include format: %w", err)
	}

	var parsedIncludes []RemoteInclude

	for _, parsedInclude := range parsed.Includes {

		switch {
		case parsedInclude.Project != "":
			if parsedInclude.Ref == "" {
				log.Printf("setting ref for %s:%s to `main` because no ref was specified", parsedInclude.Project, strings.Join(parsedInclude.Files, ","))
				parsedInclude.Ref = "main" // this is not really right but less ambiguous than `""`

			}
		case parsedInclude.Local != "":
			parsedInclude.Project = project.PathWithNamespace
			parsedInclude.Ref = project.DefaultBranch
			parsedInclude.Files = []string{parsedInclude.Local}
		case parsedInclude.Template != "":
			parsedInclude.Project = project.PathWithNamespace
			parsedInclude.Ref = project.DefaultBranch
			parsedInclude.Files = []string{parsedInclude.Template}
		default:
			log.Printf("weird include: %+v", parsedInclude)
		}

		parsedIncludes = append(parsedIncludes, parsedInclude)
	}

	return parsedIncludes, nil
}
