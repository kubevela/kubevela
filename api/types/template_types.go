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

// Template defines the content of a plugin
type Template struct {
	Name           string         `json:"name"`
	Type           DefinitionType `json:"type"`
	Template       string         `json:"template,omitempty"`
	Parameters     []Parameter    `json:"parameters,omitempty"`
	DefinitionPath string         `json:"definition"`
}

type DefinitionType string

const (
	TypeWorkload DefinitionType = "workload"
	TypeTrait    DefinitionType = "trait"
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
		cmd.Flags().Int64P(v.Name, v.Short, v.Default.(int64), v.Usage)
	case cue.StringKind:
		cmd.Flags().StringP(v.Name, v.Short, v.Default.(string), v.Usage)
	case cue.BoolKind:
		cmd.Flags().BoolP(v.Name, v.Short, v.Default.(bool), v.Usage)
	case cue.NumberKind, cue.FloatKind:
		cmd.Flags().Float64P(v.Name, v.Short, v.Default.(float64), v.Usage)
	}
	if v.Required && v.Name != "name" {
		cmd.MarkFlagRequired(v.Name)
	}
}
