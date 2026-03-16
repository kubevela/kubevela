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

package matchers

import (
	"fmt"

	"github.com/onsi/gomega/types"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

// named is the minimal interface for extracting a parameter name (used in failure messages).
type named interface {
	Name() string
}

// requiredParam is satisfied by any parameter that can report required status.
type requiredParam interface {
	named
	IsRequired() bool
}

// optionalParam is satisfied by any parameter that can report optional status.
type optionalParam interface {
	named
	IsOptional() bool
}

// defaultParam is satisfied by any parameter that can report its default value.
type defaultParam interface {
	named
	HasDefault() bool
	GetDefault() any
}

// describedParam is satisfied by any parameter that can report its description.
type describedParam interface {
	named
	GetDescription() string
}

// BeRequired returns a matcher that checks if a parameter is required.
func BeRequired() types.GomegaMatcher {
	return &requiredMatcher{}
}

type requiredMatcher struct{}

func (m *requiredMatcher) Match(actual interface{}) (bool, error) {
	param, ok := actual.(requiredParam)
	if !ok {
		return false, fmt.Errorf("BeRequired expects a parameter type, got %T", actual)
	}
	return param.IsRequired(), nil
}

func (m *requiredMatcher) FailureMessage(actual interface{}) string {
	param := actual.(requiredParam)
	return fmt.Sprintf("Expected parameter %q to be required", param.Name())
}

func (m *requiredMatcher) NegatedFailureMessage(actual interface{}) string {
	param := actual.(requiredParam)
	return fmt.Sprintf("Expected parameter %q not to be required", param.Name())
}

// BeOptional returns a matcher that checks if a parameter is optional.
func BeOptional() types.GomegaMatcher {
	return &optionalMatcher{}
}

type optionalMatcher struct{}

func (m *optionalMatcher) Match(actual interface{}) (bool, error) {
	param, ok := actual.(optionalParam)
	if !ok {
		return false, fmt.Errorf("BeOptional expects a parameter type, got %T", actual)
	}
	return param.IsOptional(), nil
}

func (m *optionalMatcher) FailureMessage(actual interface{}) string {
	param := actual.(optionalParam)
	return fmt.Sprintf("Expected parameter %q to be optional", param.Name())
}

func (m *optionalMatcher) NegatedFailureMessage(actual interface{}) string {
	param := actual.(optionalParam)
	return fmt.Sprintf("Expected parameter %q not to be optional", param.Name())
}

// HaveDefaultValue returns a matcher that checks if a parameter has the expected default value.
func HaveDefaultValue(expected any) types.GomegaMatcher {
	return &defaultValueMatcher{expectedValue: expected}
}

type defaultValueMatcher struct {
	expectedValue any
}

func (m *defaultValueMatcher) Match(actual interface{}) (bool, error) {
	param, ok := actual.(defaultParam)
	if !ok {
		return false, fmt.Errorf("HaveDefaultValue expects a parameter type, got %T", actual)
	}
	if !param.HasDefault() {
		return false, nil
	}
	return param.GetDefault() == m.expectedValue, nil
}

func (m *defaultValueMatcher) FailureMessage(actual interface{}) string {
	param := actual.(defaultParam)
	if !param.HasDefault() {
		return fmt.Sprintf("Expected parameter %q to have default value %v, but it has no default", param.Name(), m.expectedValue)
	}
	return fmt.Sprintf("Expected parameter %q to have default value %v, but got %v", param.Name(), m.expectedValue, param.GetDefault())
}

func (m *defaultValueMatcher) NegatedFailureMessage(actual interface{}) string {
	param := actual.(defaultParam)
	return fmt.Sprintf("Expected parameter %q not to have default value %v", param.Name(), m.expectedValue)
}

// HaveDescription returns a matcher that checks if a parameter has the expected description.
func HaveDescription(expected string) types.GomegaMatcher {
	return &descriptionMatcher{expectedDesc: expected}
}

type descriptionMatcher struct {
	expectedDesc string
}

func (m *descriptionMatcher) Match(actual interface{}) (bool, error) {
	param, ok := actual.(describedParam)
	if !ok {
		return false, fmt.Errorf("HaveDescription expects a parameter type, got %T", actual)
	}
	return param.GetDescription() == m.expectedDesc, nil
}

func (m *descriptionMatcher) FailureMessage(actual interface{}) string {
	param := actual.(describedParam)
	return fmt.Sprintf("Expected parameter %q to have description %q, but got %q", param.Name(), m.expectedDesc, param.GetDescription())
}

func (m *descriptionMatcher) NegatedFailureMessage(actual interface{}) string {
	param := actual.(describedParam)
	return fmt.Sprintf("Expected parameter %q not to have description %q", param.Name(), m.expectedDesc)
}

// HaveParamNamed returns a matcher that checks if a ComponentDefinition has a parameter with the given name.
func HaveParamNamed(name string) types.GomegaMatcher {
	return &paramNamedMatcher{expectedName: name}
}

type paramNamedMatcher struct {
	expectedName string
}

func (m *paramNamedMatcher) Match(actual interface{}) (bool, error) {
	comp, ok := actual.(*defkit.ComponentDefinition)
	if !ok {
		return false, fmt.Errorf("HaveParamNamed expects a *defkit.ComponentDefinition, got %T", actual)
	}
	for _, p := range comp.GetParams() {
		if p.Name() == m.expectedName {
			return true, nil
		}
	}
	return false, nil
}

func (m *paramNamedMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected component to have parameter named %q", m.expectedName)
}

func (m *paramNamedMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected component not to have parameter named %q", m.expectedName)
}
