// Package schema defines the data structures that represent a piperun pipeline.
package schema

// Pipeline is the top-level decoded representation of a piperun.hcl file.
type Pipeline struct {
	Version   int                  `json:"version"`
	Stages    map[string]*Stage    `json:"stages"`
	Variables map[string]*Variable `json:"variables"`
	Locals    map[string]string    `json:"locals"`
}

// Stage is a single unit of work in a pipeline.
type Stage struct {
	Name      string     `json:"name"`
	Script    string     `json:"script"`
	DependsOn []string   `json:"depends_on,omitempty"`
	If        string     `json:"if,omitempty"`
	ForEach   string     `json:"for_each,omitempty"`
	Container *Container `json:"container,omitempty"`
	Lifecycle []string   `json:"lifecycle,omitempty"`
	PreHook   *Hook      `json:"pre_hook,omitempty"`
	PostHook  *Hook      `json:"post_hook,omitempty"`
}

// Container describes an optional Docker container context for a stage.
type Container struct {
	Image string `json:"image"`
}

// Hook describes a pre or post hook that runs around a stage.
type Hook struct {
	Script string `json:"script"`
}

// Variable is a pipeline-level input variable.
type Variable struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}
