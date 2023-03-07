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

package apis

import (
	"github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/v1beta1"
)

type TypedApplication interface {
	Name(name string) TypedApplication
	Namespace(namespace string) TypedApplication
	Labels(labels map[string]string) TypedApplication
	Annotations(annotations map[string]string) TypedApplication

	SetWorkflowMode(steps, subSteps common.WorkflowMode) TypedApplication
	SetComponents(components ...Component) TypedApplication
	SetWorkflowSteps(steps ...WorkflowStep) TypedApplication
	SetPolicies(policies ...Policy) TypedApplication

	GetNamespace() string
	GetLabels() map[string]string
	GetAnnotations() map[string]string
	GetComponentByName(name string) Component
	GetComponentsByType(typ string) []Component
	GetWorkflowStepByName(name string) WorkflowStep
	GetWorkflowStepsByType(typ string) []WorkflowStep
	GetPolicyByName(name string) Policy
	GetPoliciesByType(typ string) []Policy

	Build() v1beta1.Application
	ToYAML() (string, error)
	Validate() error
}

type Component interface {
	ComponentName() string
	DefType() string
	Build() common.ApplicationComponent
	GetTrait(typ string) Trait
	GetAllTraits() []Trait
	Validate() error
}

type Trait interface {
	DefType() string
	Build() common.ApplicationTrait
	Validate() error
}

type WorkflowStep interface {
	WorkflowStepName() string
	DefType() string
	Build() v1beta1.WorkflowStep
	Validate() error
}

type Policy interface {
	PolicyName() string
	DefType() string
	Build() v1beta1.AppPolicy
	Validate() error
}

type ComponentBase struct {
	Name      string
	Type      string
	DependsOn []string
	Inputs    common.StepInputs
	Outputs   common.StepOutputs
	Traits    []Trait
}

type TraitBase struct {
	Type string
}

type WorkflowSubStepBase struct {
	Name      string
	Type      string
	Meta      *common.WorkflowStepMeta
	If        string
	Timeout   string
	DependsOn []string
	Inputs    common.StepInputs
	Outputs   common.StepOutputs
}

type WorkflowStepBase struct {
	Name      string
	Type      string
	Meta      *common.WorkflowStepMeta
	SubSteps  []WorkflowStep
	If        string
	Timeout   string
	DependsOn []string
	Inputs    common.StepInputs
	Outputs   common.StepOutputs
}

type PolicyBase struct {
	Name string
	Type string
}
