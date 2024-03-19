package crawler

import (
	"fmt"
)

// How to parse a trigger
/*

is "trigger" in the yaml?

Is it an "trigger:include" trigger or a "trigger:project" one?

For "trigger:project" we look at the "project" and "branch" fields and store that in
a struct.

For "trigger:include" we start having fun is it
* A string?
* A list?

if string sotre local project and include string in a struct

if list...

Go through every item in the list and find out if it's one of these variants

* local
* template
* project + ref(opt.) + file
* artifact + job

*/

// Might be using https://github.com/BurntSushi/go-sumtype if this works out
type Trigger interface {
	include()

	project() string
}

type TriggerProject struct {
	Project string
	Branch string // optional, defaults to default branch of project
}

func (tp TriggerProject) include () {}

func (tp TriggerProject) project () string {
	return tp.Project
}

type TriggerLocalInclude struct {
	Project string // always the local project while parsing the file
	File string
}

func (tli TriggerLocalInclude) include () {}

func (tli TriggerLocalInclude) project() string {
	return tli.Project
}

type TriggerTemplateInclude struct {
	// potentially set Project string to the canonical location
	Project string
	File string
}

func (tti TriggerTemplateInclude) include() {}

func (tti TriggerTemplateInclude) project() string {
	return tti.Project
}

type TriggerRemoteInclude struct {
	Project string
	RemoteFile string
}

type TriggerPojectInclude struct {
	Project string
	Ref string
	File []string
}

func (tpi TriggerPojectInclude) include() {}

func (tpi TriggerPojectInclude) project() string {
	return tpi.Project
}

type TriggerArtifactInclude struct {
	Project string
	ArtifactName string
	JobName string
}

func (tai TriggerArtifactInclude) include() {}

func (tai TriggerArtifactInclude) project() string {
	return tai.Project
}

func (c *Crawler) parseTriggers(projectPathWithNamespace string, file []byte) ([]Trigger, error) {
	parsed, err := c.UnmarshalCIFile(file)
	if err != nil {
		return nil, err
	}

	triggers := make([]Trigger, 0)
	for rawJob := range parsed {
		job, isMap := parsed[rawJob].(map[string]interface{})
		if !isMap {
			c.logger.Debug().
				Str("CIFileKey", rawJob).
				Msg("Skipping job since it's not a map")
			continue
		}

		rawTrigger, triggerExists := job["trigger"]
		if !triggerExists {
			c.logger.Debug().
				Str("CIFileKey", rawJob).
				Msg("Skipping job since it doesn't contain a trigger")
		}

		switch t := rawTrigger.(type) {
		case string:
			triggers = append(triggers,
				TriggerLocalInclude{
					Project: projectPathWithNamespace,
					File: t,
				},
			)
		case map[string]interface{}:
			println(t["include"])
			trigger, err := c.parseTriggerMap(t, projectPathWithNamespace)
			if err != nil {
				c.logger.Warn().
					Err(err).
					Str("CIFileKey", rawJob).
					Msg("could not parse contents of trigger")
			}
			triggers = append(triggers, trigger...)
		default:
			return nil, fmt.Errorf("Trigger is neither a string or a map[string]interface{}, not sure if this is possible")
		}
	}

	return triggers, nil
}

func (c *Crawler) parseTriggerMap(input map[string]interface{}, currentProjectPath string) ([]Trigger, error) {
	// TODO: type switch on input to include dymamic child pipelines
	// https://docs.gitlab.com/ee/ci/pipelines/downstream_pipelines.html#dynamic-child-pipelines
	// This will require the entire parser to be reworked again because the RawTrigger
	// Inlcude field cannot hold multiple values.
	//
	// Example configuration:
	// trigger:
	//   include:
	//     - artifact: some yaml
	//       job: some job name

	// This is a bit jank, I can't come up with a much better solution currently.
	// Basically it attempts to parse the easy way out first and returns a bool whether
	// we early return here.
	projectTrigger, successfullyParsedProjectTrigger := c.attemptParsingTriggerProject(input)
	if successfullyParsedProjectTrigger {
		return []Trigger{projectTrigger}, nil
	}

	includeTriggers, successfullyParsedIncludeTriggers := c.attemptParsingTriggerInclude(input, currentProjectPath)
	if successfullyParsedIncludeTriggers {
		return includeTriggers, nil
	}

	return nil, fmt.Errorf("failed to parse trigger map")
}

func (c *Crawler) attemptParsingTriggerProject (input map[string]interface{}) (Trigger, bool) {
	project := extractFieldFromMap("project", input)
	if project == "" {
		return TriggerProject{}, false
	}

	branch := extractFieldFromMap("branch", input)

	return TriggerProject{Project: project, Branch: branch}, true
}

func (c *Crawler) attemptParsingTriggerInclude(input map[string]interface{}, currentProjectPath string) ([]Trigger, bool) {
	include, exists := input["include"]
	if !exists {
		return nil, false
	}

	switch v := include.(type) {
	case string:
		return []Trigger{TriggerLocalInclude{
			Project: currentProjectPath,
			File: v,
		}}, true
	case []map[string]interface{}:
		returnTriggers := make([]Trigger, len(v))
		for idx, includeTrigger := range v {
			includeTrigger, err := c.parseIncludeTriggerList(includeTrigger, currentProjectPath)
			if err != nil {
				// TODO: log error somewhere
				continue
			}

			returnTriggers[idx] = includeTrigger
		}

		return returnTriggers, true
	default:
		return nil, false
	}
}

func (c *Crawler) parseIncludeTriggerList(input map[string]interface{}, currentProjectPath string) (Trigger, error) {
	local, localExists := input["local"]
	if localExists {
		filePath, ok := local.(string)
		if !ok {
			return nil, fmt.Errorf("found value of %T in a local field, that's not supported", local)
		}
		return TriggerLocalInclude{Project: currentProjectPath, File: filePath}, nil
	}

	template, templateExists := input["template"]
	if templateExists {
		templatePath, ok := template.(string)
		if !ok {
			return nil, fmt.Errorf("found value of %T in a template field, that's not supported", local)
		}
		return TriggerTemplateInclude{Project: "", File: templatePath}, nil
	}

	artifact, artifactExists := input["artifact"]
	job, jobExists := input["job"]
	if artifactExists && jobExists {
		artifactName, artifactOK := artifact.(string)
		if !artifactOK {
			return nil, fmt.Errorf("found artifact that is %T instead of string", artifact)
		}

		jobName, jobOK := job.(string)
		if !jobOK {
			return nil, fmt.Errorf("found job that is %T instead of string", job)
		}

		return TriggerArtifactInclude{
			Project: currentProjectPath,
			ArtifactName: artifactName,
			JobName: jobName,
		}, nil
	}

	project, projectExists := input["project"]
	fileRaw, fileExists := input["file"]
	if projectExists && fileExists {
		returnTrigger := TriggerPojectInclude{}
		switch file := fileRaw.(type) {
		case string:
			returnTrigger.File = []string{file}
		case []string:
			returnTrigger.File = file
		default:
			return nil, fmt.Errorf("got %T instead of a file list", fileRaw)
		}

		projectPath, ok := project.(string)
		if !ok {
			return nil, fmt.Errorf("got %T as project instead of a string", project)
		}

		returnTrigger.Project = projectPath

		// the '_' is just to prevent it from panicking, we don't care about whether
		// this really worked since the null type of "" is fine.
		ref, _ := input["ref"].(string)
		returnTrigger.Ref = ref

		return returnTrigger, nil
	}

	return nil, fmt.Errorf("did not find any valid trigger values")
}

func extractFieldFromMap(fieldName string, in map[string]interface{}) string {
	field, exists := in[fieldName]
	if !exists {
		return ""
	}

	sField, ok := field.(string)
	if !ok {
		return ""
	}

	return sField
}
