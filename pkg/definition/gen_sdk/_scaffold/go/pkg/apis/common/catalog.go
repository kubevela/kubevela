package common

import (
	"github.com/chivalryq/vela-go-sdk/pkg/apis"

	"github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/v1beta1"
)

type (
	ComponentConstructor       func(comp common.ApplicationComponent) (apis.Component, error)
	TraitConstructor           func(trait common.ApplicationTrait) (apis.Trait, error)
	WorkflowStepConstructor    func(step v1beta1.WorkflowStep) (apis.WorkflowStep, error)
	WorkflowSubStepConstructor func(step common.WorkflowSubStep) (apis.WorkflowStep, error)
	PolicyConstructor          func(policy v1beta1.AppPolicy) (apis.Policy, error)
)

var (
	ComponentsBuilders       = make(map[string]ComponentConstructor, 0)
	WorkflowStepsBuilders    = make(map[string]WorkflowStepConstructor, 0)
	WorkflowSubStepsBuilders = make(map[string]WorkflowSubStepConstructor, 0)
	PoliciesBuilders         = make(map[string]PolicyConstructor, 0)
	TraitBuilders            = make(map[string]TraitConstructor, 0)
)

func RegisterComponent(_type string, c ComponentConstructor) {
	ComponentsBuilders[_type] = c
}

func RegisterPolicy(_type string, c PolicyConstructor) {
	PoliciesBuilders[_type] = c
}

func RegisterWorkflowStep(_type string, c WorkflowStepConstructor) {
	WorkflowStepsBuilders[_type] = c
}

func RegisterTrait(_type string, c TraitConstructor) {
	TraitBuilders[_type] = c
}

func RegisterWorkflowSubStep(_type string, c WorkflowSubStepConstructor) {
	WorkflowSubStepsBuilders[_type] = c
}
