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

package utils

// UIParameter Structured import table simple UI model
type UIParameter struct {
	Label       string    `json:"label"`
	Description string    `json:"description"`
	Validate    *Validate `json:"validete,omitempty"`
	JSONKey     string    `json:"jsonKey"`
	UIType      string    `json:"uiType"`
	// means only can be read.
	Disable       bool           `json:"disable"`
	SubParameters []*UIParameter `json:"subParameters,omitempty"`
}

// Validate parameter validate rule
type Validate struct {
	Required     bool        `json:"required,omitempty"`
	Max          int         `json:"max,omitempty"`
	Min          int         `json:"min,omitempty"`
	Regular      string      `json:"regular,omitempty"`
	Options      []*Options  `json:"options,omitempty"`
	DefaultValue interface{} `json:"defaultValue,omitempty"`
}

// Options select option
type Options struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ParseUIParameterFromDefinition cue of parameter in Definitions was analyzed to obtain the form description model.
func ParseUIParameterFromDefinition(definition []byte) ([]*UIParameter, error) {
	var params []*UIParameter

	return params, nil
}
