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

package schema

import (
	"fmt"
	"strings"

	"github.com/oam-dev/kubevela/pkg/utils"
)

// UISchema ui schema
type UISchema []*UIParameter

// Validate check the ui schema
func (u UISchema) Validate() error {
	for _, p := range u {
		// check the conditions
		for _, c := range p.Conditions {
			if err := c.Validate(); err != nil {
				return err
			}
		}
		//TODO: other fields check
	}
	return nil
}

// UIParameter Structured import table simple UI model
type UIParameter struct {
	Sort        uint      `json:"sort"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	Validate    *Validate `json:"validate,omitempty"`
	JSONKey     string    `json:"jsonKey"`
	UIType      string    `json:"uiType"`
	Style       *Style    `json:"style,omitempty"`
	// means disable parameter in ui
	Disable *bool `json:"disable,omitempty"`
	// Conditions: control whether fields are enabled or disabled by certain conditions.
	// Rules:
	// if all conditions are not matching, the parameter will be disabled
	// if there are no conditions, and disable==false the parameter will be enabled.
	// if one disable action condition is matched, the parameter will be disabled.
	// if all enable actions conditions are matched, the parameter will be enabled.
	// +optional
	Conditions              []Condition    `json:"conditions,omitempty"`
	SubParameterGroupOption []GroupOption  `json:"subParameterGroupOption,omitempty"`
	SubParameters           []*UIParameter `json:"subParameters,omitempty"`
	AdditionalParameter     *UIParameter   `json:"additionalParameter,omitempty"`
	Additional              *bool          `json:"additional,omitempty"`
}

// Condition control whether fields are enabled or disabled by certain conditions.
type Condition struct {
	// JSONKey specifies the path of the field, support the peer and subordinate fields.
	JSONKey string `json:"jsonKey"`
	// Op options includes `==` 、`!=` and `in`, default is `==`
	// +optional
	Op string `json:"op,omitempty"`
	// Value specifies the prospective value.
	Value interface{} `json:"value"`
	// Action options includes `enable` or `disable`, default is `enable`
	// +optional
	Action string `json:"action,omitempty"`
}

// Validate check the validity of condition
func (c Condition) Validate() error {
	if c.JSONKey == "" {
		return fmt.Errorf("the json key of the condition can not be empty")
	}
	if c.Action != "enable" && c.Action != "disable" && c.Action != "" {
		return fmt.Errorf("the action of the condition only supports enable, disable or leave it empty")
	}
	if c.Op != "" && !utils.StringsContain([]string{"==", "!=", "in"}, c.Op) {
		return fmt.Errorf("the op of the condition must be `==` 、`!=` and `in`")
	}
	return nil
}

// Style ui style
type Style struct {
	// ColSpan the width of a responsive layout
	ColSpan int `json:"colSpan"`
}

// GroupOption define multiple data structure composition options.
type GroupOption struct {
	Label string   `json:"label"`
	Keys  []string `json:"keys"`
}

// Validate parameter validate rule
type Validate struct {
	Required     bool        `json:"required,omitempty"`
	Max          *float64    `json:"max,omitempty"`
	MaxLength    *uint64     `json:"maxLength,omitempty"`
	Min          *float64    `json:"min,omitempty"`
	MinLength    uint64      `json:"minLength,omitempty"`
	Pattern      string      `json:"pattern,omitempty"`
	Options      []Option    `json:"options,omitempty"`
	DefaultValue interface{} `json:"defaultValue,omitempty"`
	// the parameter cannot be changed twice.
	Immutable bool `json:"immutable"`
}

// Option select option
type Option struct {
	Label string      `json:"label"`
	Value interface{} `json:"value"`
}

// FirstUpper Sets the first letter of the string to upper.
func FirstUpper(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// FirstLower  Sets the first letter of the string to lowercase.
func FirstLower(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// GetDefaultUIType Set the default mapping for API Schema Type
func GetDefaultUIType(apiType string, haveOptions bool, subType string, haveSub bool) string {
	switch apiType {
	case "string":
		if haveOptions {
			return "Select"
		}
		return "Input"
	case "number", "integer":
		return "Number"
	case "boolean":
		return "Switch"
	case "array":
		if subType == "string" {
			return "Strings"
		}
		if subType == "number" || subType == "integer" {
			return "Numbers"
		}
		return "Structs"
	case "object":
		if haveSub {
			return "Group"
		}
		return "KV"
	default:
		return "Input"
	}
}

// RenderLabel render option label
func RenderLabel(source interface{}) string {
	switch v := source.(type) {
	case int:
		return fmt.Sprintf("%d", v)
	case string:
		return FirstUpper(v)
	default:
		return FirstUpper(fmt.Sprintf("%v", v))
	}
}
