/*
Copyright 2025 The KubeVela Authors.

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

// Package defkit provides a Go SDK for authoring KubeVela X-Definitions.
// It enables platform engineers to write ComponentDefinition, TraitDefinition,
// PolicyDefinition, and WorkflowStepDefinition using fluent Go APIs that
// compile transparently to CUE.
package defkit

// Expr represents any expression that can be used in a template.
// This includes parameter references, context references, literals,
// and compound expressions.
type Expr interface {
	// expr is a marker method to identify expression types
	expr()
}

// Condition represents a boolean expression used in conditionals.
// Conditions can be composed using And(), Or(), and Not().
type Condition interface {
	Expr
	// condition is a marker method to identify condition types
	condition()
}

// baseCondition provides common implementation of Expr and Condition marker methods.
// Embed this in condition types to automatically implement the marker interfaces.
type baseCondition struct{}

func (baseCondition) expr()      {}
func (baseCondition) condition() {}

// Value represents a value that can be assigned to a field.
// This includes literals, parameter references, and expressions.
type Value interface {
	Expr
	// value is a marker method to identify value types
	value()
}

// Param represents a parameter definition with its schema and constraints.
// Parameters are created using typed constructors like String(), Int(), etc.
type Param interface {
	Value
	// Name returns the parameter name
	Name() string
	// IsRequired returns true if the parameter is required
	IsRequired() bool
	// IsOptional returns true if the parameter is optional
	IsOptional() bool
	// HasDefault returns true if the parameter has a default value
	HasDefault() bool
	// GetDefault returns the default value, or nil if none
	GetDefault() any
	// GetDescription returns the parameter description
	GetDescription() string
}

// ParamType represents the type of a parameter
type ParamType string

const (
	// ParamTypeString represents a string parameter
	ParamTypeString ParamType = "string"
	// ParamTypeInt represents an integer parameter
	ParamTypeInt ParamType = "int"
	// ParamTypeBool represents a boolean parameter
	ParamTypeBool ParamType = "bool"
	// ParamTypeFloat represents a float/number parameter
	ParamTypeFloat ParamType = "float"
	// ParamTypeArray represents an array parameter
	ParamTypeArray ParamType = "array"
	// ParamTypeMap represents a map parameter
	ParamTypeMap ParamType = "map"
	// ParamTypeStruct represents a struct parameter
	ParamTypeStruct ParamType = "struct"
	// ParamTypeEnum represents an enum parameter
	ParamTypeEnum ParamType = "enum"
	// ParamTypeOneOf represents a discriminated union parameter
	ParamTypeOneOf ParamType = "oneof"
)
