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

package common

import (
	"github.com/kubevela/vela-go-sdk/pkg/apis"

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
