package addon

import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
)

// Addon contains all information represent an addon
type Addon struct {
	AddonMeta

	APISchema *openapi3.Schema     `json:"schema"`
	UISchema  []*utils.UIParameter `json:"uiSchema"`

	// More details about the addon, e.g. README
	Detail         string               `json:"detail,omitempty"`
	Definitions    []AddonElementFile   `json:"definitions"`
	CUEDefinitions []AddonElementFile   `json:"cue_definitions"`
	Parameters     string               `json:"parameters"`
	CUETemplates   []AddonElementFile   `json:"cue_templates"`
	YAMLTemplates  []AddonElementFile   `json:"yaml_templates,omitempty"`
	DefSchemas     []AddonElementFile   `json:"def_schemas,omitempty"`
	AppTemplate    *v1beta1.Application `json:"app_template"`
}

// AddonMeta defines the format for a single addon
type AddonMeta struct {
	Name          string             `json:"name" validate:"required"`
	Version       string             `json:"version"`
	Description   string             `json:"description"`
	Icon          string             `json:"icon"`
	URL           string             `json:"url,omitempty"`
	Tags          []string           `json:"tags,omitempty"`
	DeployTo      *AddonDeployTo     `json:"deployTo,omitempty"`
	Dependencies  []*AddonDependency `json:"dependencies,omitempty"`
	NeedNamespace []string           `json:"needNamespace,omitempty"`
	Invisible     bool               `json:"invisible"`
}

// AddonDeployTo defines where the addon to deploy to
type AddonDeployTo struct {
	ControlPlane   bool `json:"control_plane"`
	RuntimeCluster bool `json:"runtime_cluster"`
}

// AddonDependency defines the other addons it depends on
type AddonDependency struct {
	Name string `json:"name,omitempty"`
}

// AddonElementFile can be addon's definition or addon's component
type AddonElementFile struct {
	Data string
	Name string
}
