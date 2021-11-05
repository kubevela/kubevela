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

import (
	"encoding/json"
	"reflect"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// WorkflowStepGenerator generator generates workflow steps
type WorkflowStepGenerator interface {
	Generate(app *v1beta1.Application, existingSteps []v1beta1.WorkflowStep) ([]v1beta1.WorkflowStep, error)
}

// ChainWorkflowStepGenerator chains multiple workflow step generators
type ChainWorkflowStepGenerator struct {
	generators []WorkflowStepGenerator
}

// Generate generate workflow steps
func (g *ChainWorkflowStepGenerator) Generate(app *v1beta1.Application, existingSteps []v1beta1.WorkflowStep) (steps []v1beta1.WorkflowStep, err error) {
	steps = existingSteps
	for _, generator := range g.generators {
		steps, err = generator.Generate(app, steps)
		if err != nil {
			return steps, errors.Wrapf(err, "generate step failed in WorkflowStepGenerator %s", reflect.TypeOf(generator).Name())
		}
	}
	return steps, nil
}

// NewChainWorkflowStepGenerator create ChainWorkflowStepGenerator
func NewChainWorkflowStepGenerator(generators ...WorkflowStepGenerator) WorkflowStepGenerator {
	return &ChainWorkflowStepGenerator{generators: generators}
}

// ApplyComponentWorkflowStepGenerator generate apply-component workflow steps for all components in the application
type ApplyComponentWorkflowStepGenerator struct{}

// Generate generate workflow steps
func (g *ApplyComponentWorkflowStepGenerator) Generate(app *v1beta1.Application, existingSteps []v1beta1.WorkflowStep) (steps []v1beta1.WorkflowStep, err error) {
	if len(existingSteps) > 0 {
		return existingSteps, nil
	}
	for _, comp := range app.Spec.Components {
		steps = append(steps, v1beta1.WorkflowStep{
			Name: comp.Name,
			Type: "apply-component",
			Properties: util.Object2RawExtension(map[string]string{
				"component": comp.Name,
			}),
		})
	}
	return
}

// Deploy2EnvWorkflowStepGenerator generate deploy2env workflow steps for all envs in the application
type Deploy2EnvWorkflowStepGenerator struct{}

// Generate generate workflow steps
func (g *Deploy2EnvWorkflowStepGenerator) Generate(app *v1beta1.Application, existingSteps []v1beta1.WorkflowStep) (steps []v1beta1.WorkflowStep, err error) {
	if len(existingSteps) > 0 {
		return existingSteps, nil
	}
	for _, policy := range app.Spec.Policies {
		if policy.Type == v1alpha1.EnvBindingPolicyType && policy.Properties != nil {
			spec := &v1alpha1.EnvBindingSpec{}
			if err = json.Unmarshal(policy.Properties.Raw, spec); err != nil {
				return
			}
			for _, env := range spec.Envs {
				steps = append(steps, v1beta1.WorkflowStep{
					Name: "deploy-" + policy.Name + "-" + env.Name,
					Type: "deploy2env",
					Properties: util.Object2RawExtension(map[string]string{
						"policy": policy.Name,
						"env":    env.Name,
					}),
				})
			}
		}
	}
	return
}
