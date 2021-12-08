package addon

import (
	"github.com/getkin/kin-openapi/openapi3"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
)

// Addon contains all information represent an addon
type Addon struct {
	Meta

	APISchema *openapi3.Schema     `json:"schema"`
	UISchema  []*utils.UIParameter `json:"uiSchema"`

	// More details about the addon, e.g. README
	Detail         string               `json:"detail,omitempty"`
	Definitions    []ElementFile   `json:"definitions"`
	CUEDefinitions []ElementFile   `json:"cue_definitions"`
	Parameters     string               `json:"parameters"`
	CUETemplates   []ElementFile   `json:"cue_templates"`
	YAMLTemplates  []ElementFile   `json:"yaml_templates,omitempty"`
	DefSchemas     []ElementFile   `json:"def_schemas,omitempty"`
	AppTemplate    *v1beta1.Application `json:"app_template"`
}

// Meta defines the format for a single addon
type Meta struct {
	Name          string        `json:"name" validate:"required"`
	Version       string        `json:"version"`
	Description   string        `json:"description"`
	Icon          string        `json:"icon"`
	URL           string        `json:"url,omitempty"`
	Tags          []string      `json:"tags,omitempty"`
	DeployTo      *DeployTo     `json:"deployTo,omitempty"`
	Dependencies  []*Dependency `json:"dependencies,omitempty"`
	NeedNamespace []string      `json:"needNamespace,omitempty"`
	Invisible     bool          `json:"invisible"`
}

// DeployTo defines where the addon to deploy to
type DeployTo struct {
	ControlPlane   bool `json:"control_plane"`
	RuntimeCluster bool `json:"runtime_cluster"`
}

// Dependency defines the other addons it depends on
type Dependency struct {
	Name string `json:"name,omitempty"`
}

// ElementFile can be addon's definition or addon's component
type ElementFile struct {
	Data string
	Name string
}
