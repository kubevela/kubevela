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

// Package matchers provides custom Gomega matchers for testing defkit definitions.
package matchers

import (
	"fmt"

	"github.com/onsi/gomega/types"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

// BeDeployment returns a matcher that checks if a Resource is a Deployment.
func BeDeployment() types.GomegaMatcher {
	return BeResourceOfKind("Deployment")
}

// BeService returns a matcher that checks if a Resource is a Service.
func BeService() types.GomegaMatcher {
	return BeResourceOfKind("Service")
}

// BeConfigMap returns a matcher that checks if a Resource is a ConfigMap.
func BeConfigMap() types.GomegaMatcher {
	return BeResourceOfKind("ConfigMap")
}

// BeSecret returns a matcher that checks if a Resource is a Secret.
func BeSecret() types.GomegaMatcher {
	return BeResourceOfKind("Secret")
}

// BeIngress returns a matcher that checks if a Resource is an Ingress.
func BeIngress() types.GomegaMatcher {
	return BeResourceOfKind("Ingress")
}

// BeResourceOfKind returns a matcher that checks if a Resource is of the specified kind.
func BeResourceOfKind(kind string) types.GomegaMatcher {
	return &resourceKindMatcher{expectedKind: kind}
}

type resourceKindMatcher struct {
	expectedKind string
}

func (m *resourceKindMatcher) Match(actual interface{}) (bool, error) {
	resource, ok := actual.(*defkit.Resource)
	if !ok {
		return false, fmt.Errorf("BeResourceOfKind expects a *defkit.Resource, got %T", actual)
	}
	return resource.Kind() == m.expectedKind, nil
}

func (m *resourceKindMatcher) FailureMessage(actual interface{}) string {
	resource := actual.(*defkit.Resource)
	return fmt.Sprintf("Expected resource to be of kind %q, but got %q", m.expectedKind, resource.Kind())
}

func (m *resourceKindMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected resource not to be of kind %q", m.expectedKind)
}

// HaveAPIVersion returns a matcher that checks if a Resource has the specified API version.
func HaveAPIVersion(version string) types.GomegaMatcher {
	return &apiVersionMatcher{expectedVersion: version}
}

type apiVersionMatcher struct {
	expectedVersion string
}

func (m *apiVersionMatcher) Match(actual interface{}) (bool, error) {
	resource, ok := actual.(*defkit.Resource)
	if !ok {
		return false, fmt.Errorf("HaveAPIVersion expects a *defkit.Resource, got %T", actual)
	}
	return resource.APIVersion() == m.expectedVersion, nil
}

func (m *apiVersionMatcher) FailureMessage(actual interface{}) string {
	resource := actual.(*defkit.Resource)
	return fmt.Sprintf("Expected resource to have API version %q, but got %q", m.expectedVersion, resource.APIVersion())
}

func (m *apiVersionMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected resource not to have API version %q", m.expectedVersion)
}

// HaveSetOp returns a matcher that checks if a Resource has a Set operation at the given path.
func HaveSetOp(path string) types.GomegaMatcher {
	return &setOpMatcher{expectedPath: path}
}

type setOpMatcher struct {
	expectedPath string
}

func (m *setOpMatcher) Match(actual interface{}) (bool, error) {
	resource, ok := actual.(*defkit.Resource)
	if !ok {
		return false, fmt.Errorf("HaveSetOp expects a *defkit.Resource, got %T", actual)
	}
	for _, op := range resource.Ops() {
		if setOp, ok := op.(*defkit.SetOp); ok {
			if setOp.Path() == m.expectedPath {
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *setOpMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected resource to have Set operation at path %q", m.expectedPath)
}

func (m *setOpMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected resource not to have Set operation at path %q", m.expectedPath)
}

// HaveOpCount returns a matcher that checks if a Resource has the expected number of operations.
func HaveOpCount(count int) types.GomegaMatcher {
	return &opCountMatcher{expectedCount: count}
}

type opCountMatcher struct {
	expectedCount int
}

func (m *opCountMatcher) Match(actual interface{}) (bool, error) {
	resource, ok := actual.(*defkit.Resource)
	if !ok {
		return false, fmt.Errorf("HaveOpCount expects a *defkit.Resource, got %T", actual)
	}
	return len(resource.Ops()) == m.expectedCount, nil
}

func (m *opCountMatcher) FailureMessage(actual interface{}) string {
	resource := actual.(*defkit.Resource)
	return fmt.Sprintf("Expected resource to have %d operations, but got %d", m.expectedCount, len(resource.Ops()))
}

func (m *opCountMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected resource not to have %d operations", m.expectedCount)
}
