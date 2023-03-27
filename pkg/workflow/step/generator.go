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
	"context"
	"encoding/json"
	"reflect"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wftypes "github.com/kubevela/workflow/pkg/types"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// WorkflowStepGenerator generator generates workflow steps
type WorkflowStepGenerator interface {
	Generate(app *v1beta1.Application, existingSteps []workflowv1alpha1.WorkflowStep) ([]workflowv1alpha1.WorkflowStep, error)
}

// ChainWorkflowStepGenerator chains multiple workflow step generators
type ChainWorkflowStepGenerator struct {
	generators []WorkflowStepGenerator
}

// Generate generate workflow steps
func (g *ChainWorkflowStepGenerator) Generate(app *v1beta1.Application, existingSteps []workflowv1alpha1.WorkflowStep) (steps []workflowv1alpha1.WorkflowStep, err error) {
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

// RefWorkflowStepGenerator generate workflow steps from ref workflow
type RefWorkflowStepGenerator struct {
	context.Context
	client.Client
}

// Generate generate workflow steps
func (g *RefWorkflowStepGenerator) Generate(app *v1beta1.Application, existingSteps []workflowv1alpha1.WorkflowStep) (steps []workflowv1alpha1.WorkflowStep, err error) {
	if app.Spec.Workflow == nil || app.Spec.Workflow.Ref == "" {
		return existingSteps, nil
	}
	if app.Spec.Workflow.Steps != nil {
		return nil, errors.Errorf("cannot set steps and ref in workflow at the same time")
	}
	wf := &workflowv1alpha1.Workflow{}
	if err = g.Client.Get(g.Context, types.NamespacedName{Namespace: app.GetNamespace(), Name: app.Spec.Workflow.Ref}, wf); err != nil {
		return
	}
	return wf.Steps, nil
}

// ApplyComponentWorkflowStepGenerator generate apply-component workflow steps for all components in the application
type ApplyComponentWorkflowStepGenerator struct{}

// Generate generate workflow steps
func (g *ApplyComponentWorkflowStepGenerator) Generate(app *v1beta1.Application, existingSteps []workflowv1alpha1.WorkflowStep) (steps []workflowv1alpha1.WorkflowStep, err error) {
	if len(existingSteps) > 0 {
		return existingSteps, nil
	}
	for _, comp := range app.Spec.Components {
		steps = append(steps, workflowv1alpha1.WorkflowStep{
			WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
				Name: comp.Name,
				Type: wftypes.WorkflowStepTypeApplyComponent,
				Properties: util.Object2RawExtension(map[string]string{
					"component": comp.Name,
				}),
			},
		})
	}
	return
}

// Deploy2EnvWorkflowStepGenerator generate deploy2env workflow steps for all envs in the application
type Deploy2EnvWorkflowStepGenerator struct{}

// Generate generate workflow steps
func (g *Deploy2EnvWorkflowStepGenerator) Generate(app *v1beta1.Application, existingSteps []workflowv1alpha1.WorkflowStep) (steps []workflowv1alpha1.WorkflowStep, err error) {
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
				steps = append(steps, workflowv1alpha1.WorkflowStep{
					WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
						Name: "deploy-" + policy.Name + "-" + env.Name,
						Type: "deploy2env",
						Properties: util.Object2RawExtension(map[string]string{
							"policy": policy.Name,
							"env":    env.Name,
						}),
					},
				})
			}
		}
	}
	return
}

// DeployWorkflowStepGenerator generate deploy workflow steps for all topology & override in the application
type DeployWorkflowStepGenerator struct{}

// Generate generate workflow steps
func (g *DeployWorkflowStepGenerator) Generate(app *v1beta1.Application, existingSteps []workflowv1alpha1.WorkflowStep) (steps []workflowv1alpha1.WorkflowStep, err error) {
	if len(existingSteps) > 0 {
		return existingSteps, nil
	}
	var topologies []string
	var overrides []string
	for _, policy := range app.Spec.Policies {
		switch policy.Type {
		case v1alpha1.TopologyPolicyType:
			topologies = append(topologies, policy.Name)
		case v1alpha1.OverridePolicyType:
			overrides = append(overrides, policy.Name)
		}
	}
	for _, topology := range topologies {
		steps = append(steps, workflowv1alpha1.WorkflowStep{
			WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
				Name: "deploy-" + topology,
				Type: "deploy",
				Properties: util.Object2RawExtension(map[string]interface{}{
					"policies": append(overrides, topology),
				}),
			},
		})
	}
	if len(topologies) == 0 {
		containsRefObjects := false
		for _, comp := range app.Spec.Components {
			if comp.Type == v1alpha1.RefObjectsComponentType {
				containsRefObjects = true
				break
			}
		}
		if containsRefObjects || len(overrides) > 0 {
			steps = append(steps, workflowv1alpha1.WorkflowStep{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "deploy",
					Type:       "deploy",
					Properties: util.Object2RawExtension(map[string]interface{}{"policies": append([]string{}, overrides...)}),
				},
			})
		}
	}
	return steps, nil
}

// IsBuiltinWorkflowStepType checks if workflow step type is builtin type
func IsBuiltinWorkflowStepType(wfType string) bool {
	for _, _type := range []string{
		wftypes.WorkflowStepTypeSuspend,
		wftypes.WorkflowStepTypeApplyComponent,
		wftypes.WorkflowStepTypeBuiltinApplyComponent,
		wftypes.WorkflowStepTypeStepGroup,
	} {
		if _type == wfType {
			return true
		}
	}
	return false
}
