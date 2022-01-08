package crawler

import (
	"fmt"
	"log"
	"strings"

	"github.com/deichindianer/gitlab-ci-crawler/internal/gitlab"
	"gopkg.in/yaml.v3"
)

type stringTemplate struct {
	Includes StringArray `yaml:"include"`
}

type remoteTemplate struct {
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

func (c *Crawler) parseIncludes(file []byte) ([]RemoteInclude, error) {
	var remoteParsed remoteTemplate
	errorList := make([]string, 0, 2)

	err := yaml.Unmarshal(file, &remoteParsed)
	if err == nil {
		return remoteParsed.Includes, nil
	}

	errorList = append(errorList, fmt.Sprintf("failed to marshal file into remote include format: %s\n", err))

	var stringParsed stringTemplate
	err = yaml.Unmarshal(file, &stringParsed)
	if err == nil {
		remoteIncludes := make([]RemoteInclude, len(stringParsed.Includes))
		for i, include := range stringParsed.Includes {
			remoteIncludes[i] = RemoteInclude{
				Local: include,
			}
		}
		return remoteIncludes, nil
	}

	errorList = append(errorList, fmt.Sprintf("failed to marshal file into string include format: %s\n", err))

	return nil, fmt.Errorf("parsing error: %s", strings.Join(errorList, ","))
}

func enrichIncludes(rawIncludes []RemoteInclude, project gitlab.Project, defaultRefName string) []RemoteInclude {
	enrichedIncludes := make([]RemoteInclude, len(rawIncludes))

	for i, include := range rawIncludes {
		switch {
		case include.Project != "":
			if include.Ref == "" {
				log.Printf("setting ref for %s:%s to `%s` because no ref was specified", include.Project, strings.Join(include.Files, ","), defaultRefName)
				include.Ref = defaultRefName
			}
		case include.Local != "":
			include.Project = project.PathWithNamespace
			include.Ref = project.DefaultBranch
			include.Files = []string{include.Local}
		case include.Template != "":
			include.Project = project.PathWithNamespace
			include.Ref = project.DefaultBranch
			include.Files = []string{include.Template}
		default:
			log.Printf("weird include: %+v", include)
		}
		enrichedIncludes[i] = include
	}
	return enrichedIncludes
}
