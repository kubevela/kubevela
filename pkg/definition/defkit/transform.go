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

package defkit

// TransformFunc is a function that transforms a value.
type TransformFunc func(any) any

// TransformedValue wraps a value with a transformation function.
type TransformedValue struct {
	source    Value
	transform TransformFunc
}

func (t *TransformedValue) expr()  {}
func (t *TransformedValue) value() {}

// Transform creates a transformed value that applies the given function to the source value.
// This is useful for converting parameter values to different formats (e.g., port lists).
func Transform(source Value, fn TransformFunc) *TransformedValue {
	return &TransformedValue{
		source:    source,
		transform: fn,
	}
}

// Source returns the source value being transformed.
func (t *TransformedValue) Source() Value {
	return t.source
}

// TransformFn returns the transformation function.
func (t *TransformedValue) TransformFn() TransformFunc {
	return t.transform
}

// HasExposedPortsCondition checks if a ports parameter has any exposed ports.
type HasExposedPortsCondition struct {
	baseCondition
	ports Value
}

// HasExposedPorts creates a condition that checks if any port has expose=true.
// This is used to conditionally create Service outputs.
func HasExposedPorts(ports Value) Condition {
	return &HasExposedPortsCondition{ports: ports}
}

// Ports returns the ports value being checked.
func (h *HasExposedPortsCondition) Ports() Value {
	return h.ports
}
