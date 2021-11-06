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

import (
	"fmt"
	"strings"
)

// UIParameter Structured import table simple UI model
type UIParameter struct {
	Sort        uint      `json:"sort"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	Validate    *Validate `json:"validate,omitempty"`
	JSONKey     string    `json:"jsonKey"`
	UIType      string    `json:"uiType"`
	// means only can be read.
	Disable                 *bool          `json:"disable,omitempty"`
	SubParameterGroupOption [][]string     `json:"subParameterGroupOption,omitempty"`
	SubParameters           []*UIParameter `json:"subParameters,omitempty"`
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
}

// Option select option
type Option struct {
	Label string      `json:"label"`
	Value interface{} `json:"value"`
}

// ParseUIParameterFromDefinition cue of parameter in Definitions was analyzed to obtain the form description model.
func ParseUIParameterFromDefinition(definition []byte) ([]*UIParameter, error) {
	var params []*UIParameter

	return params, nil
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
func GetDefaultUIType(apiType string, haveOptions bool, subType string) string {
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

// StringsContain strings contain
func StringsContain(items []string, source string) bool {
	for _, item := range items {
		if item == source {
			return true
		}
	}
	return false
}
