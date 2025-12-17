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

// TestContextBuilder provides a fluent API for creating mock contexts in tests.
// This enables unit testing of definitions without a Kubernetes cluster.
type TestContextBuilder struct {
	name          string
	namespace     string
	appName       string
	appRevision   string
	params        map[string]any
	clusterMajor  int
	clusterMinor  int
	outputStatus  map[string]any
	outputsStatus map[string]map[string]any
	workload      *Resource
}

// TestContext creates a new test context builder for unit testing definitions.
func TestContext() *TestContextBuilder {
	return &TestContextBuilder{
		name:          "test-component",
		namespace:     "default",
		appName:       "test-app",
		appRevision:   "test-app-v1",
		params:        make(map[string]any),
		clusterMajor:  1,
		clusterMinor:  28,
		outputsStatus: make(map[string]map[string]any),
	}
}

// WithName sets the component name (context.name).
func (t *TestContextBuilder) WithName(name string) *TestContextBuilder {
	t.name = name
	return t
}

// WithNamespace sets the target namespace (context.namespace).
func (t *TestContextBuilder) WithNamespace(namespace string) *TestContextBuilder {
	t.namespace = namespace
	return t
}

// WithAppName sets the application name (context.appName).
func (t *TestContextBuilder) WithAppName(appName string) *TestContextBuilder {
	t.appName = appName
	return t
}

// WithAppRevision sets the application revision (context.appRevision).
func (t *TestContextBuilder) WithAppRevision(revision string) *TestContextBuilder {
	t.appRevision = revision
	return t
}

// WithParam sets a parameter value for the test context.
func (t *TestContextBuilder) WithParam(name string, value any) *TestContextBuilder {
	t.params[name] = value
	return t
}

// WithParams sets multiple parameter values at once.
func (t *TestContextBuilder) WithParams(params map[string]any) *TestContextBuilder {
	for k, v := range params {
		t.params[k] = v
	}
	return t
}

// WithClusterVersion sets the target cluster version (context.clusterVersion).
func (t *TestContextBuilder) WithClusterVersion(major, minor int) *TestContextBuilder {
	t.clusterMajor = major
	t.clusterMinor = minor
	return t
}

// WithOutputStatus sets the output status for health policy testing.
// This simulates runtime status like readyReplicas.
func (t *TestContextBuilder) WithOutputStatus(status map[string]any) *TestContextBuilder {
	t.outputStatus = status
	return t
}

// WithOutputsStatus sets status for a named auxiliary output.
func (t *TestContextBuilder) WithOutputsStatus(name string, status map[string]any) *TestContextBuilder {
	t.outputsStatus[name] = status
	return t
}

// WithWorkload sets a workload resource for trait testing.
func (t *TestContextBuilder) WithWorkload(workload *Resource) *TestContextBuilder {
	t.workload = workload
	return t
}

// Build creates the test context. This is called internally by Render/Validate.
func (t *TestContextBuilder) Build() *TestRuntimeContext {
	return &TestRuntimeContext{
		name:          t.name,
		namespace:     t.namespace,
		appName:       t.appName,
		appRevision:   t.appRevision,
		params:        t.params,
		clusterMajor:  t.clusterMajor,
		clusterMinor:  t.clusterMinor,
		outputStatus:  t.outputStatus,
		outputsStatus: t.outputsStatus,
		workload:      t.workload,
	}
}

// Name returns the component name.
func (t *TestContextBuilder) Name() string { return t.name }

// Namespace returns the namespace.
func (t *TestContextBuilder) Namespace() string { return t.namespace }

// AppName returns the application name.
func (t *TestContextBuilder) AppName() string { return t.appName }

// Params returns all parameter values.
func (t *TestContextBuilder) Params() map[string]any { return t.params }

// ClusterVersion returns major, minor version.
func (t *TestContextBuilder) ClusterVersion() (int, int) { return t.clusterMajor, t.clusterMinor }

// TestRuntimeContext holds the built test context values.
type TestRuntimeContext struct {
	name          string
	namespace     string
	appName       string
	appRevision   string
	params        map[string]any
	clusterMajor  int
	clusterMinor  int
	outputStatus  map[string]any
	outputsStatus map[string]map[string]any
	workload      *Resource
}

// Name returns the component name.
func (c *TestRuntimeContext) Name() string { return c.name }

// Namespace returns the namespace.
func (c *TestRuntimeContext) Namespace() string { return c.namespace }

// AppName returns the application name.
func (c *TestRuntimeContext) AppName() string { return c.appName }

// AppRevision returns the application revision.
func (c *TestRuntimeContext) AppRevision() string { return c.appRevision }

// GetParam returns a parameter value by name.
func (c *TestRuntimeContext) GetParam(name string) (any, bool) {
	v, ok := c.params[name]
	return v, ok
}

// GetParamOr returns a parameter value or a default.
func (c *TestRuntimeContext) GetParamOr(name string, defaultValue any) any {
	if v, ok := c.params[name]; ok {
		return v
	}
	return defaultValue
}

// IsParamSet returns true if the parameter has a value.
func (c *TestRuntimeContext) IsParamSet(name string) bool {
	_, ok := c.params[name]
	return ok
}

// ClusterVersion returns the cluster version.
func (c *TestRuntimeContext) ClusterVersion() (int, int) {
	return c.clusterMajor, c.clusterMinor
}

// ClusterMinor returns just the minor version (common check).
func (c *TestRuntimeContext) ClusterMinor() int {
	return c.clusterMinor
}

// OutputStatus returns the simulated output status.
func (c *TestRuntimeContext) OutputStatus() map[string]any {
	return c.outputStatus
}

// OutputsStatus returns status for a named output.
func (c *TestRuntimeContext) OutputsStatus(name string) map[string]any {
	return c.outputsStatus[name]
}

// Workload returns the workload for trait testing.
func (c *TestRuntimeContext) Workload() *Resource {
	return c.workload
}

// currentTestContext holds the current test context for parameter resolution.
// This is set during Render() execution.
var currentTestContext *TestRuntimeContext

// setCurrentTestContext sets the current test context.
func setCurrentTestContext(ctx *TestRuntimeContext) {
	currentTestContext = ctx
}

// clearCurrentTestContext clears the current test context.
func clearCurrentTestContext() {
	currentTestContext = nil
}

// getCurrentTestContext returns the current test context, if any.
func getCurrentTestContext() *TestRuntimeContext {
	return currentTestContext
}
