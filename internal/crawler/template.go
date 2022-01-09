package crawler

import (
	"fmt"
	"strings"

	"github.com/deichindianer/gitlab-ci-crawler/internal/gitlab"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

type RemoteInclude struct {
	Project  string      `yaml:"project"`
	Ref      string      `yaml:"ref"`
	Files    StringArray `yaml:"file"`
	Local    string      `yaml:"local"`
	Remote   string      `yaml:"remote"`
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
	var parsed map[string]interface{}

	err := yaml.Unmarshal(file, &parsed)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal includes: %s", err)
	}

	rawIncludes, exist := parsed["include"]
	if !exist {
		return []RemoteInclude{}, nil
	}

	includes, ok := rawIncludes.([]interface{})
	if !ok {
		return []RemoteInclude{}, fmt.Errorf("failed to assert include slice to interface{}")
	}

	rIncludes := make([]RemoteInclude, 0, len(includes))
	for _, include := range includes {
		switch i := include.(type) {
		case string:
			rIncludes = append(rIncludes, RemoteInclude{Local: i})
		case map[string]interface{}:
			ri, err := c.parseIncludeMap(i)
			if err != nil {
				c.logger.Err(err).Msg("failed to parse include map data into RemoteInclude")
				continue
			}
			rIncludes = append(rIncludes, ri)
		}
	}

	return rIncludes, nil
}

// parseIncludeMap takes a map or a string taken from the includes out of a gitlab-ci.yml
// file and tries to parse them into the RemoteInclude struct.
// Early exits are if `local`, `remote` or `template` are called.
func (c *Crawler) parseIncludeMap(input map[string]interface{}) (RemoteInclude, error) {
	const (
		localIncludeKey    = "local"
		remoteIncludeKey   = "remote"
		templateIncludeKey = "template"
	)

	for _, s := range []string{localIncludeKey, remoteIncludeKey, templateIncludeKey} {
		val, ok := input[s]
		if !ok {
			continue
		}

		sVal, ok := val.(string)
		if !ok {
			c.logger.Warn().
				Interface("Value", val).
				Msg("`Value` did not assert to string, this is bad and should be reported as an issue")
			continue
		}

		switch s {
		case localIncludeKey:
			return RemoteInclude{Local: sVal}, nil
		case remoteIncludeKey:
			return RemoteInclude{Remote: sVal}, nil
		case templateIncludeKey:
			return RemoteInclude{Template: sVal}, nil
		}
	}

	project, exists := input["project"]
	if !exists {
		return RemoteInclude{}, fmt.Errorf("failed to get valid include, missing `project` key")
	}

	sProject, ok := project.(string)
	if !ok {
		return RemoteInclude{}, fmt.Errorf("failed to convert %+v(%T) into string", project, project)
	}

	file, exists := input["file"]
	if !exists {
		return RemoteInclude{}, fmt.Errorf("failed to get valid include, missing `file` key")
	}

	sFiles := make([]string, 0)
	switch f := file.(type) {
	case string:
		sFiles = append(sFiles, f)
	case []interface{}:
		for _, fVal := range f {
			fString, ok := fVal.(string)
			if !ok {
				c.logger.Debug().
					Interface("Value", fVal).
					Msg("failed to parse `Value` into string, skipping")
				continue
			}
			sFiles = append(sFiles, fString)
		}
	default:
		return RemoteInclude{}, fmt.Errorf("failed to conver %+v(%T) to either string or []string", file, file)
	}

	ref := input["ref"]

	sRef, ok := ref.(string)
	if !ok {
		c.logger.Debug().
			Interface("Value", ref).
			Str("Project", sProject).
			Msg("failed to parse `Value` into string, skipping ref for `Project`")
	}

	return RemoteInclude{
		Project: sProject,
		Files:   sFiles,
		Ref:     sRef,
	}, nil
}

func (c *Crawler) enrichIncludes(rawIncludes []RemoteInclude, project gitlab.Project, defaultRefName string) []RemoteInclude {
	enrichedIncludes := make([]RemoteInclude, len(rawIncludes))

	for i, include := range rawIncludes {

		switch {
		case include.Project != "":
			if include.Ref == "" {
				c.logger.Debug().
					Dict("include", zerolog.Dict().
						Str("Project", include.Project).
						Str("Files", strings.Join(include.Files, ",")).
						Str("Ref", defaultRefName)).
					Str("DefaultRefName", defaultRefName).
					Msg("Setting include ref to DefaultRefName because it was not set")
				include.Ref = defaultRefName
			}
		case include.Local != "":
			include.Project = project.PathWithNamespace
			include.Ref = project.DefaultBranch
			include.Files = []string{include.Local}
		case include.Remote != "":
			// TODO: implement
		case include.Template != "":
			include.Project = project.PathWithNamespace
			include.Ref = project.DefaultBranch
			include.Files = []string{include.Template}
		default:
			c.logger.Warn().
				Dict("include", zerolog.Dict().
					Str("Project", include.Project).
					Str("Files", strings.Join(include.Files, ",")).
					Str("Ref", include.Ref).
					Str("Local", include.Local).
					Str("Template", include.Template)).
				Msg("could not parse include")
		}
		enrichedIncludes[i] = include
	}
	return enrichedIncludes
}
