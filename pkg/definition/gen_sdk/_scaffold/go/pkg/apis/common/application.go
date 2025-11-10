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
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/v1beta1"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/kubevela/vela-go-sdk/pkg/apis"
)

type ApplicationBuilder struct {
	name            string
	namespace       string
	labels          map[string]string
	annotations     map[string]string
	resourceVersion string

	components   []apis.Component
	steps        []apis.WorkflowStep
	policies     []apis.Policy
	workflowMode workflowv1alpha1.WorkflowExecuteMode
}

// SetComponents set components to application, use component name to match, if component name not found, append it
func (a *ApplicationBuilder) SetComponents(components ...apis.Component) apis.TypedApplication {
	for _, addComp := range components {
		found := false
		for i, c := range a.components {
			if c.ComponentName() == addComp.ComponentName() {
				a.components[i] = addComp
				found = true
				break
			}
		}
		if !found {
			a.components = append(a.components, addComp)
		}
	}
	return a
}

// SetWorkflowSteps set workflow steps to application, use step name to match, if step name not found, append it
func (a *ApplicationBuilder) SetWorkflowSteps(steps ...apis.WorkflowStep) apis.TypedApplication {
	for _, addStep := range steps {
		found := false
		for i, s := range a.steps {
			if s.WorkflowStepName() == addStep.WorkflowStepName() {
				a.steps[i] = addStep
				found = true
				break
			}
		}
		if !found {
			a.steps = append(a.steps, addStep)
		}
	}
	return a
}

// SetPolicies set policies to application, use policy name to match, if policy name not found, append it
func (a *ApplicationBuilder) SetPolicies(policies ...apis.Policy) apis.TypedApplication {
	for _, addPolicy := range policies {
		found := false
		for i, p := range a.policies {
			if p.PolicyName() == addPolicy.PolicyName() {
				a.policies[i] = addPolicy
				found = true
				break
			}
		}
		if !found {
			a.policies = append(a.policies, addPolicy)
		}
	}
	return a
}

func (a *ApplicationBuilder) GetComponentByName(name string) apis.Component {
	for _, c := range a.components {
		if c.ComponentName() == name {
			return c
		}
	}
	return nil
}

func (a *ApplicationBuilder) GetComponentsByType(typ string) []apis.Component {
	var result []apis.Component
	for _, c := range a.components {
		if c.DefType() == typ {
			result = append(result, c)
		}
	}
	return result
}

func (a *ApplicationBuilder) GetWorkflowStepByName(name string) apis.WorkflowStep {
	for _, s := range a.steps {
		if s.WorkflowStepName() == name {
			return s
		}
	}
	return nil
}

func (a *ApplicationBuilder) GetWorkflowStepsByType(typ string) []apis.WorkflowStep {
	var result []apis.WorkflowStep
	for _, s := range a.steps {
		if s.DefType() == typ {
			result = append(result, s)
		}
	}
	return result
}

func (a *ApplicationBuilder) GetPolicyByName(name string) apis.Policy {
	for _, p := range a.policies {
		if p.PolicyName() == name {
			return p
		}
	}
	return nil
}

func (a *ApplicationBuilder) GetPoliciesByType(typ string) []apis.Policy {
	var result []apis.Policy
	for _, p := range a.policies {
		if p.DefType() == typ {
			result = append(result, p)
		}
	}
	return result
}

// SetWorkflowMode set the workflow mode of application
func (a *ApplicationBuilder) SetWorkflowMode(steps, subSteps workflowv1alpha1.WorkflowMode) apis.TypedApplication {
	a.workflowMode.Steps = steps
	a.workflowMode.SubSteps = subSteps
	return a
}

func (a *ApplicationBuilder) Name(name string) apis.TypedApplication {
	a.name = name
	return a
}

func (a *ApplicationBuilder) Namespace(namespace string) apis.TypedApplication {
	a.namespace = namespace
	return a
}

func (a *ApplicationBuilder) Labels(labels map[string]string) apis.TypedApplication {
	a.labels = labels
	return a
}

func (a *ApplicationBuilder) Annotations(annotations map[string]string) apis.TypedApplication {
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
func New() apis.TypedApplication {
	app := &ApplicationBuilder{
		components: make([]apis.Component, 0),
		steps:      make([]apis.WorkflowStep, 0),
		policies:   make([]apis.Policy, 0),
	}
	return app
}

func (a *ApplicationBuilder) Build() v1beta1.Application {
	components := make([]common.ApplicationComponent, 0, len(a.components))
	for _, component := range a.components {
		components = append(components, component.Build())
	}
	steps := make([]workflowv1alpha1.WorkflowStep, 0, len(a.steps))
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
			Labels:          a.labels,
			Annotations:     a.annotations,
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

func (a *ApplicationBuilder) ToJSON() (string, error) {
	app := a.Build()
	marshal, err := json.Marshal(app)
	if err != nil {
		return "", err
	}
	return string(marshal), nil
}

// Validate validates the application name/namespace/component/step/policy.
// For component/step/policy, it will validate if the required fields are set.
func (a *ApplicationBuilder) Validate() error {
	if a.name == "" {
		return errors.New("name is required")
	}
	if a.namespace == "" {
		return errors.New("namespace is required")
	}
	for _, c := range a.components {
		if err := c.Validate(); err != nil {
			return err
		}
	}
	for _, s := range a.steps {
		if err := s.Validate(); err != nil {
			return err
		}
	}
	for _, p := range a.policies {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func FromK8sObject(app v1beta1.Application) (apis.TypedApplication, error) {
	a := &ApplicationBuilder{}
	a.Name(app.Name)
	a.Namespace(app.Namespace)
	a.resourceVersion = app.ResourceVersion

	for _, comp := range app.Spec.Components {
		c, err := FromComponent(comp)
		if err != nil {
			return nil, errors.Wrap(err, "convert component from k8s object")
		}
		a.SetComponents(c)
	}
	if app.Spec.Workflow != nil {
		for _, step := range app.Spec.Workflow.Steps {
			s, err := FromWorkflowStep(step)
			if err != nil {
				return nil, errors.Wrap(err, "convert workflow step from k8s object")
			}
			a.SetWorkflowSteps(s)
		}
	}
	for _, policy := range app.Spec.Policies {
		p, err := FromPolicy(policy)
		if err != nil {
			return nil, errors.Wrap(err, "convert policy from k8s object")
		}
		a.SetPolicies(p)
	}
	return a, nil
}

func FromComponent(component common.ApplicationComponent) (apis.Component, error) {
	build, ok := ComponentsBuilders[component.Type]
	if !ok {
		return nil, errors.Errorf("no component type %s registered", component.Type)
	}
	return build(component)
}

func FromWorkflowStep(step workflowv1alpha1.WorkflowStep) (apis.WorkflowStep, error) {
	build, ok := WorkflowStepsBuilders[step.Type]
	if !ok {
		return nil, errors.Errorf("no workflow step type %s registered", step.Type)
	}
	return build(step)
}

func FromPolicy(policy v1beta1.AppPolicy) (apis.Policy, error) {
	build, ok := PoliciesBuilders[policy.Type]
	if !ok {
		return nil, errors.Errorf("no policy type %s registered", policy.Type)
	}
	return build(policy)
}

func FromTrait(trait common.ApplicationTrait) (apis.Trait, error) {
	build, ok := TraitBuilders[trait.Type]
	if !ok {
		return nil, errors.Errorf("no trait type %s registered", trait.Type)
	}
	return build(trait)
}
