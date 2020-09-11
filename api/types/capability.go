/*


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
	"fmt"

	"cuelang.org/go/cue"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
)

type Source struct {
	RepoName  string `json:"repoName"`
	ChartName string `json:"chartName,omitempty"`
}

type CrdInfo struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// Capability defines the content of a capability
type Capability struct {
	Name           string      `json:"name"`
	Type           CapType     `json:"type"`
	CueTemplate    string      `json:"template,omitempty"`
	Parameters     []Parameter `json:"parameters,omitempty"`
	DefinitionPath string      `json:"definition"`
	CrdName        string      `json:"crdName,omitempty"`
	Center         string      `json:"center,omitempty"`
	Status         string      `json:"status,omitempty"`

	//trait only
	AppliesTo []string `json:"appliesTo,omitempty"`

	// Plugin Source
	Source  *Source       `json:"source,omitempty"`
	Install *Installation `json:"install,omitempty"`
	CrdInfo *CrdInfo      `json:"crdInfo,omitempty"`
}

type Chart struct {
	Repo    string `json:"repo"`
	URL     string `json:"url"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Installation struct {
	Helm Chart `json:"helm"`
}

type CapType string

const (
	TypeWorkload CapType = "workload"
	TypeTrait    CapType = "trait"
	TypeScope    CapType = "scope"
)

type Parameter struct {
	Name     string      `json:"name"`
	Short    string      `json:"short,omitempty"`
	Required bool        `json:"required,omitempty"`
	Default  interface{} `json:"default,omitempty"`
	Usage    string      `json:"usage,omitempty"`
	Type     cue.Kind    `json:"type,omitempty"`
}

// ConvertTemplateJSON2Object convert spec.extension to object
func ConvertTemplateJSON2Object(in *runtime.RawExtension) (Capability, error) {
	var t Capability
	var extension Capability
	if in == nil {
		return t, fmt.Errorf("extension field is nil")
	}
	if in.Raw == nil {
		return t, fmt.Errorf("template object is nil")
	}
	err := json.Unmarshal(in.Raw, &extension)
	if err == nil {
		t = extension
	}
	return t, err
}

func SetFlagBy(flags *pflag.FlagSet, v Parameter) {
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
		flags.Int64P(v.Name, v.Short, vv, v.Usage)
	case cue.StringKind:
		flags.StringP(v.Name, v.Short, v.Default.(string), v.Usage)
	case cue.BoolKind:
		flags.BoolP(v.Name, v.Short, v.Default.(bool), v.Usage)
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
		flags.Float64P(v.Name, v.Short, vv, v.Usage)
	}
}
