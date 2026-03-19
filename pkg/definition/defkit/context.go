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

// ContextRef represents a reference to a context value.
// Context values are runtime values provided by KubeVela.
type ContextRef struct {
	path string
}

func (c *ContextRef) expr()  {}
func (c *ContextRef) value() {}

// Path returns the CUE path for this context reference.
func (c *ContextRef) Path() string { return c.path }

// VelaContext provides access to KubeVela runtime context values.
// These values are populated at runtime and generate CUE path references.
// Named VelaContext to avoid confusion with Go's context package.
type VelaContext struct{}

// VelaCtx returns a new VelaContext instance for accessing runtime values.
// Usage: vela := defkit.VelaCtx()
//
//	deploy.Set("metadata.name", vela.Name())
func VelaCtx() *VelaContext {
	return &VelaContext{}
}

// Name returns the component/trait name from context.
func (c *VelaContext) Name() *ContextRef {
	return &ContextRef{path: "context.name"}
}

// Namespace returns the application namespace from context.
func (c *VelaContext) Namespace() *ContextRef {
	return &ContextRef{path: "context.namespace"}
}

// AppName returns the application name from context.
func (c *VelaContext) AppName() *ContextRef {
	return &ContextRef{path: "context.appName"}
}

// AppRevision returns the application revision from context.
func (c *VelaContext) AppRevision() *ContextRef {
	return &ContextRef{path: "context.appRevision"}
}

// AppRevisionNum returns the application revision number from context.
func (c *VelaContext) AppRevisionNum() *ContextRef {
	return &ContextRef{path: "context.appRevisionNum"}
}

// ClusterVersion returns the Kubernetes cluster version from context.
func (c *VelaContext) ClusterVersion() *ClusterVersionRef {
	return &ClusterVersionRef{basePath: "context.clusterVersion"}
}

// ClusterVersionRef provides access to cluster version components.
type ClusterVersionRef struct {
	basePath string
}

func (c *ClusterVersionRef) expr()  {}
func (c *ClusterVersionRef) value() {}

// Path returns the base path for cluster version.
func (c *ClusterVersionRef) Path() string { return c.basePath }

// Major returns the major version component.
func (c *ClusterVersionRef) Major() *ContextRef {
	return &ContextRef{path: c.basePath + ".major"}
}

// Minor returns the minor version component.
func (c *ClusterVersionRef) Minor() *ContextRef {
	return &ContextRef{path: c.basePath + ".minor"}
}

// Patch returns the patch version component.
func (c *ClusterVersionRef) Patch() *ContextRef {
	return &ContextRef{path: c.basePath + ".patch"}
}

// GitVersion returns the full git version string.
func (c *ClusterVersionRef) GitVersion() *ContextRef {
	return &ContextRef{path: c.basePath + ".gitVersion"}
}

// Revision returns the component revision from context.
func (c *VelaContext) Revision() *ContextRef {
	return &ContextRef{path: "context.revision"}
}

// Output returns a reference to the primary output resource.
func (c *VelaContext) Output() *ContextRef {
	return &ContextRef{path: "context.output"}
}

// Outputs returns a reference to auxiliary outputs by name.
func (c *VelaContext) Outputs(name string) *ContextRef {
	return &ContextRef{path: "context.outputs." + name}
}

// String returns the path as a placeholder string for template building.
// This is used when building raw values that need context references.
func (c *ContextRef) String() string {
	return "$(" + c.path + ")"
}
