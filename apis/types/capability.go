/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

import (
	"encoding/json"

	"cuelang.org/go/cue"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/pflag"
)

// Source record the source of Capability
type Source struct {
	RepoName  string `json:"repoName"`
	ChartName string `json:"chartName,omitempty"`
}

// CRDInfo record the CRD info of the Capability
type CRDInfo struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// Capability defines the content of a capability
type Capability struct {
	Name           string             `json:"name"`
	Type           CapType            `json:"type"`
	CueTemplate    string             `json:"template,omitempty"`
	CueTemplateURI string             `json:"templateURI,omitempty"`
	Parameters     []Parameter        `json:"parameters,omitempty"`
	CrdName        string             `json:"crdName,omitempty"`
	Center         string             `json:"center,omitempty"`
	Status         string             `json:"status,omitempty"`
	Description    string             `json:"description,omitempty"`
	Category       CapabilityCategory `json:"category,omitempty"`

	// trait only
	AppliesTo []string `json:"appliesTo,omitempty"`

	// Namespace represents it's a system-level or user-level capability.
	Namespace string `json:"namespace,omitempty"`

	// Plugin Source
	Source  *Source       `json:"source,omitempty"`
	Install *Installation `json:"install,omitempty"`
	CrdInfo *CRDInfo      `json:"crdInfo,omitempty"`
}

// Chart defines all necessary information to install a whole chart
type Chart struct {
	Repo      string                 `json:"repo"`
	URL       string                 `json:"url"`
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace,omitempty"`
	Version   string                 `json:"version"`
	Values    map[string]interface{} `json:"values"`
}

// Installation defines the installation method for this Capability, currently only helm is supported
type Installation struct {
	Helm Chart `json:"helm"`
	// TODO(wonderflow) add raw yaml file support for install capability
}

// CapType defines the type of capability
type CapType string

const (
	// TypeComponentDefinition represents OAM ComponentDefinition
	TypeComponentDefinition CapType = "componentDefinition"
	// TypeWorkload represents OAM Workload
	TypeWorkload CapType = "workload"
	// TypeTrait represents OAM Trait
	TypeTrait CapType = "trait"
	// TypeScope represent OAM Scope
	TypeScope CapType = "scope"
)

// CapabilityConfigMapNamePrefix is the prefix for capability ConfigMap name
const CapabilityConfigMapNamePrefix = "schema-"

const (
	// OpenapiV3JSONSchema is the key to store OpenAPI v3 JSON schema in ConfigMap
	OpenapiV3JSONSchema string = "openapi-v3-json-schema"
)

// CapabilityCategory defines the category of a capability
type CapabilityCategory string

// categories of capability schematic
const (
	TerraformCategory CapabilityCategory = "terraform"

	HelmCategory CapabilityCategory = "helm"

	KubeCategory CapabilityCategory = "kube"

	CUECategory CapabilityCategory = "cue"
)

// Parameter defines a parameter for cli from capability template
type Parameter struct {
	Name     string      `json:"name"`
	Short    string      `json:"short,omitempty"`
	Required bool        `json:"required,omitempty"`
	Default  interface{} `json:"default,omitempty"`
	Usage    string      `json:"usage,omitempty"`
	Type     cue.Kind    `json:"type,omitempty"`
	Alias    string      `json:"alias,omitempty"`
	JSONType string      `json:"jsonType,omitempty"`
}

// SetFlagBy set cli flag from Parameter
func SetFlagBy(flags *pflag.FlagSet, v Parameter) {
	name := v.Name
	if v.Alias != "" {
		name = v.Alias
	}
	// nolint:exhaustive
	switch v.Type {
	case cue.IntKind:
		var vv int64
		switch val := v.Default.(type) {
		case int64:
			vv = val
		case json.Number:
			vv, _ = val.Int64()
		case int:
			vv = int64(val)
		case float64:
			vv = int64(val)
		}
		flags.Int64P(name, v.Short, vv, v.Usage)
	case cue.StringKind:
		flags.StringP(name, v.Short, v.Default.(string), v.Usage)
	case cue.BoolKind:
		flags.BoolP(name, v.Short, v.Default.(bool), v.Usage)
	case cue.NumberKind, cue.FloatKind:
		var vv float64
		switch val := v.Default.(type) {
		case int64:
			vv = float64(val)
		case json.Number:
			vv, _ = val.Float64()
		case int:
			vv = float64(val)
		case float64:
			vv = val
		}
		flags.Float64P(name, v.Short, vv, v.Usage)
	default:
		// other types not supported yet
	}
}

// CapabilityCmpOptions will set compare option
var CapabilityCmpOptions = []cmp.Option{
	cmp.Comparer(func(a, b Parameter) bool {
		if a.Name != b.Name || a.Short != b.Short || a.Required != b.Required ||
			a.Usage != b.Usage || a.Type != b.Type {
			return false
		}
		// nolint:exhaustive
		switch a.Type {
		case cue.IntKind:
			var va, vb int64
			switch vala := a.Default.(type) {
			case int64:
				va = vala
			case json.Number:
				va, _ = vala.Int64()
			case int:
				va = int64(vala)
			case float64:
				va = int64(vala)
			}
			switch valb := b.Default.(type) {
			case int64:
				vb = valb
			case json.Number:
				vb, _ = valb.Int64()
			case int:
				vb = int64(valb)
			case float64:
				vb = int64(valb)
			}
			return va == vb
		case cue.StringKind:
			return a.Default.(string) == b.Default.(string)
		case cue.BoolKind:
			return a.Default.(bool) == b.Default.(bool)
		case cue.NumberKind, cue.FloatKind:
			var va, vb float64
			switch vala := a.Default.(type) {
			case int64:
				va = float64(vala)
			case json.Number:
				va, _ = vala.Float64()
			case int:
				va = float64(vala)
			case float64:
				va = vala
			}
			switch valb := b.Default.(type) {
			case int64:
				vb = float64(valb)
			case json.Number:
				vb, _ = valb.Float64()
			case int:
				vb = float64(valb)
			case float64:
				vb = valb
			}
			return va == vb
		default:
			// complex type not supported, will regard them as not changed.
		}
		return true
	})}

// EqualCapability will check whether two capabilities is equal
func EqualCapability(a, b Capability) bool {
	return cmp.Equal(a, b, CapabilityCmpOptions...)
}
