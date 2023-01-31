package apis

import (
	"github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/v1beta1"
)

type Application interface {
	Name(name string) Application
	Namespace(namespace string) Application
	Labels(labels map[string]string) Application
	Annotations(annotations map[string]string) Application

	WithComponents(component ...Component) Application
	WithWorkflowSteps(step ...WorkflowStep) Application
	WithPolicies(policy ...Policy) Application
	WithWorkflowMode(steps, subSteps common.WorkflowMode) Application

	SetComponent(component Component) Application
	SetWorkflowStep(step WorkflowStep) Application
	SetPolicy(policy Policy) Application

	GetName() string
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
}

type Component interface {
	ComponentName() string
	DefType() string
	Build() common.ApplicationComponent
	GetTrait(typ string) Trait
}

type Trait interface {
	DefType() string
	Build() common.ApplicationTrait
}

type WorkflowStep interface {
	WorkflowStepName() string
	DefType() string
	Build() v1beta1.WorkflowStep
}

type Policy interface {
	PolicyName() string
	DefType() string
	Build() v1beta1.AppPolicy
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
