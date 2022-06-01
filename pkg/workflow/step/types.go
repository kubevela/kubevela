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

package step

const (
	// DeployWorkflowStep identifies the step of deploy components in multi-clusters
	DeployWorkflowStep = "deploy"
)

// DeployWorkflowStepSpec the spec of `deploy` WorkflowStep
type DeployWorkflowStepSpec struct {
	// Auto nil/true mean auto deploy, false means additional pre-approve step will be injected before the deploy step
	Auto *bool `json:"auto,omitempty"`
	// Policies specifies the policies to use in the step
	Policies []string `json:"policies,omitempty"`
	// Parallelism allows setting parallelism for the component deploy process
	Parallelism *int `json:"parallelism,omitempty"`
	// IgnoreTerraformComponent default is true, true means this step will apply the components without the terraform workload.
	IgnoreTerraformComponent *bool `json:"ignoreTerraformComponent,omitempty"`
}
