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
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	. "github.com/kubevela/vela-go-sdk/pkg/apis"

	"github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/v1beta1"
)

type ApplicationBuilder struct {
	name            string
	namespace       string
	labels          map[string]string
	annotations     map[string]string
	resourceVersion string

	components   []Component
	steps        []WorkflowStep
	policies     []Policy
	workflowMode v1beta1.WorkflowExecuteMode
}

// SetComponent set component to application, use component name to match
// TODO: behavior when component not found?
// 1. return error
// 2. ignore
// 3. add a new one to application
func (a *ApplicationBuilder) SetComponent(component Component) TypedApplication {
	for i, c := range a.components {
		if c.ComponentName() == component.ComponentName() {
			a.components[i] = component
			return a
		}
	}
	return a
}

func (a *ApplicationBuilder) SetWorkflowStep(step WorkflowStep) TypedApplication {
	for i, s := range a.steps {
		if s.WorkflowStepName() == step.WorkflowStepName() {
			a.steps[i] = step
			return a
		}
	}
	return a
}

func (a *ApplicationBuilder) SetPolicy(policy Policy) TypedApplication {
	for i, p := range a.policies {
		if p.PolicyName() == policy.PolicyName() {
			a.policies[i] = policy
			return a
		}
	}
	return a
}

func (a *ApplicationBuilder) GetComponentByName(name string) Component {
	for _, c := range a.components {
		if c.ComponentName() == name {
			return c
		}
	}
	return nil
}

func (a *ApplicationBuilder) GetComponentsByType(typ string) []Component {
	var result []Component
	for _, c := range a.components {
		if c.DefType() == typ {
			result = append(result, c)
		}
	}
	return result
}

func (a *ApplicationBuilder) GetWorkflowStepByName(name string) WorkflowStep {
	for _, s := range a.steps {
		if s.WorkflowStepName() == name {
			return s
		}
	}
	return nil
}

func (a *ApplicationBuilder) GetWorkflowStepsByType(typ string) []WorkflowStep {
	var result []WorkflowStep
	for _, s := range a.steps {
		if s.DefType() == typ {
			result = append(result, s)
		}
	}
	return result
}

func (a *ApplicationBuilder) GetPolicyByName(name string) Policy {
	for _, p := range a.policies {
		if p.PolicyName() == name {
			return p
		}
	}
	return nil
}

func (a *ApplicationBuilder) GetPoliciesByType(typ string) []Policy {
	var result []Policy
	for _, p := range a.policies {
		if p.DefType() == typ {
			result = append(result, p)
		}
	}
	return result
}

// AddWorkflowSteps append workflow steps to application
func (a *ApplicationBuilder) AddWorkflowSteps(step ...WorkflowStep) TypedApplication {
	a.steps = append(a.steps, step...)
	return a
}

// AddComponents append components to application
func (a *ApplicationBuilder) AddComponents(component ...Component) TypedApplication {
	a.components = append(a.components, component...)
	return a
}

// AddPolicies append policies to application
func (a *ApplicationBuilder) AddPolicies(policy ...Policy) TypedApplication {
	a.policies = append(a.policies, policy...)
	return a
}

// SetWorkflowMode set the workflow mode of application
func (a *ApplicationBuilder) SetWorkflowMode(steps, subSteps common.WorkflowMode) TypedApplication {
	a.workflowMode.Steps = steps
	a.workflowMode.SubSteps = subSteps
	return a
}

func (a *ApplicationBuilder) Name(name string) TypedApplication {
	a.name = name
	return a
}

func (a *ApplicationBuilder) Namespace(namespace string) TypedApplication {
	a.namespace = namespace
	return a
}

func (a *ApplicationBuilder) Labels(labels map[string]string) TypedApplication {
	a.labels = labels
	return a
}

func (a *ApplicationBuilder) Annotations(annotations map[string]string) TypedApplication {
	a.annotations = annotations
	return a
}

func (a *ApplicationBuilder) GetName() string {
	return a.name
}

func (a *ApplicationBuilder) GetNamespace() string {
	return a.namespace
}

func (a *ApplicationBuilder) GetLabels() map[string]string {
	return a.labels
}

func (a *ApplicationBuilder) GetAnnotations() map[string]string {
	return a.annotations
}

// New creates a new application with the given components.
func New() TypedApplication {
	app := &ApplicationBuilder{
		components: make([]Component, 0),
		steps:      make([]WorkflowStep, 0),
		policies:   make([]Policy, 0),
	}
	return app
}

func (a *ApplicationBuilder) Build() v1beta1.Application {
	components := make([]common.ApplicationComponent, 0, len(a.components))
	for _, component := range a.components {
		components = append(components, component.Build())
	}
	steps := make([]v1beta1.WorkflowStep, 0, len(a.steps))
	for _, step := range a.steps {
		steps = append(steps, step.Build())
	}
	policies := make([]v1beta1.AppPolicy, 0)
	for _, policy := range a.policies {
		policies = append(policies, policy.Build())
	}

	res := v1beta1.Application{
		TypeMeta: v1.TypeMeta{
			Kind:       v1beta1.ApplicationKind,
			APIVersion: v1beta1.SchemeGroupVersion.String(),
		},
		ObjectMeta: v1.ObjectMeta{
			Name:            a.name,
			Namespace:       a.namespace,
			ResourceVersion: a.resourceVersion,
		},
		Spec: v1beta1.ApplicationSpec{
			Components: components,
			Workflow: &v1beta1.Workflow{
				Steps: steps,
			},
			Policies: policies,
		},
	}
	return res
}

func (a *ApplicationBuilder) ToYAML() (string, error) {
	app := a.Build()
	marshal, err := yaml.Marshal(app)
	if err != nil {
		return "", err
	}
	return string(marshal), nil
}

func FromK8sObject(app *v1beta1.Application) (TypedApplication, error) {
	a := &ApplicationBuilder{}
	a.Name(app.Name)
	a.Namespace(app.Namespace)
	a.resourceVersion = app.ResourceVersion

	for _, comp := range app.Spec.Components {
		c, err := FromComponent(&comp)
		if err != nil {
			return nil, errors.Wrap(err, "convert component from k8s object")
		}
		a.AddComponents(c)
	}
	if app.Spec.Workflow != nil {
		for _, step := range app.Spec.Workflow.Steps {
			s, err := FromWorkflowStep(&step)
			if err != nil {
				return nil, errors.Wrap(err, "convert workflow step from k8s object")
			}
			a.AddWorkflowSteps(s)
		}
	}
	for _, policy := range app.Spec.Policies {
		p, err := FromPolicy(&policy)
		if err != nil {
			return nil, errors.Wrap(err, "convert policy from k8s object")
		}
		a.AddPolicies(p)
	}
	return a, nil
}

func FromComponent(component *common.ApplicationComponent) (Component, error) {
	build, ok := ComponentsBuilders[component.Type]
	if !ok {
		return nil, errors.Errorf("no component type %s registered", component.Type)
	}
	return build(*component)
}

func FromWorkflowStep(step *v1beta1.WorkflowStep) (WorkflowStep, error) {
	build, ok := WorkflowStepsBuilders[step.Type]
	if !ok {
		return nil, errors.Errorf("no workflow step type %s registered", step.Type)
	}
	return build(*step)
}

func FromPolicy(policy *v1beta1.AppPolicy) (Policy, error) {
	build, ok := PoliciesBuilders[policy.Type]
	if !ok {
		return nil, errors.Errorf("no policy type %s registered", policy.Type)
	}
	return build(*policy)
}

func FromTrait(trait *common.ApplicationTrait) (Trait, error) {
	build, ok := TraitBuilders[trait.Type]
	if !ok {
		return nil, errors.Errorf("no trait type %s registered", trait.Type)
	}
	return build(*trait)
}
