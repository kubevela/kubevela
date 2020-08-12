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

	"github.com/spf13/cobra"

	"cuelang.org/go/cue"

	"k8s.io/apimachinery/pkg/runtime"
)

type Source struct {
	RepoName string `json:"repoName"`
}

// Template defines the content of a plugin
type Template struct {
	Name           string         `json:"name"`
	Type           DefinitionType `json:"type"`
	CueTemplate    string         `json:"template,omitempty"`
	Parameters     []Parameter    `json:"parameters,omitempty"`
	DefinitionPath string         `json:"definition"`
	CrdName        string         `json:"crdName,omitempty"`

	//trait only
	AppliesTo []string `json:"appliesTo,omitempty"`

	// Plugin Source
	Source  *Source       `json:"source,omitempty"`
	Install *Installation `json:"install,omitempty"`
}

type Chart struct {
	Repo    string `json:"repo"`
	URl     string `json:"url"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Installation struct {
	Helm []Chart `json:"helm"`
}

type DefinitionType string

const (
	TypeWorkload DefinitionType = "workload"
	TypeTrait    DefinitionType = "trait"
	TypeScope    DefinitionType = "scope"
)

type Parameter struct {
	Name     string      `json:"name"`
	Short    string      `json:"short,omitempty"`
	Required bool        `json:"required,omitempty"`
	Default  interface{} `json:"default,omitempty"`
	Usage    string      `json:"usage,omitempty"`
	Type     cue.Kind    `json:"type,omitempty"`
}

// ConvertTemplateJson2Object convert spec.extension to object
func ConvertTemplateJson2Object(in *runtime.RawExtension) (Template, error) {
	var t Template
	var extension Template
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

func SetFlagBy(cmd *cobra.Command, v Parameter) {
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
		cmd.Flags().Int64P(v.Name, v.Short, vv, v.Usage)
	case cue.StringKind:
		cmd.Flags().StringP(v.Name, v.Short, v.Default.(string), v.Usage)
	case cue.BoolKind:
		cmd.Flags().BoolP(v.Name, v.Short, v.Default.(bool), v.Usage)
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
		cmd.Flags().Float64P(v.Name, v.Short, vv, v.Usage)
	}
	if v.Required && v.Name != "name" {
		cmd.MarkFlagRequired(v.Name)
	}
}
