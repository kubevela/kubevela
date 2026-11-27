/*
Copyright 2026 The KubeVela Authors.

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

package service

import (
	"context"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/addon/service/api"
)

// workflowSteps returns each top-level step of a rendered addon Application.
func workflowSteps(app map[string]interface{}) []map[string]interface{} {
	spec, ok := app["spec"].(map[string]interface{})
	Expect(ok).To(BeTrue())
	wf, ok := spec["workflow"].(map[string]interface{})
	Expect(ok).To(BeTrue(), "the Application must have a workflow")
	raw, _ := wf["steps"].([]interface{})
	steps := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		step, ok := item.(map[string]interface{})
		Expect(ok).To(BeTrue())
		steps = append(steps, step)
	}
	return steps
}

// stepNames returns the name of each step, in order.
func stepNames(steps []map[string]interface{}) []string {
	names := make([]string, 0, len(steps))
	for _, s := range steps {
		name, _ := s["name"].(string)
		names = append(names, name)
	}
	return names
}

// appliedComponent returns the component an apply-component step applies.
func appliedComponent(step map[string]interface{}) string {
	Expect(step["type"]).To(Equal("apply-component"))
	props, ok := step["properties"].(map[string]interface{})
	Expect(ok).To(BeTrue())
	name, _ := props["component"].(string)
	return name
}

var _ = Describe("an addon that writes its own workflow", func() {
	marker := common2.ApplicationComponent{Name: "marker", Type: "k8s-objects", Properties: rawJSON(map[string]interface{}{"objects": []interface{}{}})}
	applyMarker := wfv1alpha1.WorkflowStep{
		WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "apply", Type: "apply-component", Properties: rawJSON(map[string]interface{}{"component": "marker"})},
	}
	definition := pkgaddon.ElementFile{Name: "marker-step.yaml", Data: `apiVersion: core.oam.dev/v1beta1
kind: WorkflowStepDefinition
metadata:
  name: marker-step
spec:
  schematic:
    cue:
      template: "parameter: {}"
`}
	withWorkflow := func(wf *v1beta1.Workflow) *v1beta1.Application {
		return &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common2.ApplicationComponent{marker}, Workflow: wf}}
	}

	It("applies the definitions it ships before its own steps", func() {
		app := renderAddon(pkgaddon.InstallPackage{
			AppTemplate: withWorkflow(&v1beta1.Workflow{Steps: []wfv1alpha1.WorkflowStep{applyMarker}}),
			Definitions: []pkgaddon.ElementFile{definition},
		}, nil)

		steps := workflowSteps(app)
		Expect(stepNames(steps)).To(Equal([]string{"addon-definitions", "apply"}))
		Expect(appliedComponent(steps[0])).To(Equal("addon-definitions"))
	})

	It("applies the crates it imports after its own steps", func() {
		app := renderAddon(pkgaddon.InstallPackage{
			AppTemplate: withWorkflow(&v1beta1.Workflow{Steps: []wfv1alpha1.WorkflowStep{applyMarker}}),
			Imports:     []pkgaddon.CrateImport{{Crate: "aws-s3", Enabled: true}},
		}, nil)

		spec := app["spec"].(map[string]interface{})
		var crateName string
		for _, item := range spec["components"].([]interface{}) {
			if comp := item.(map[string]interface{}); comp["type"] == "crate" {
				crateName, _ = comp["name"].(string)
			}
		}
		Expect(crateName).NotTo(BeEmpty())

		steps := workflowSteps(app)
		Expect(stepNames(steps)).To(Equal([]string{"apply", crateName}))
		Expect(appliedComponent(steps[1])).To(Equal(crateName))
	})

	It("names its steps apart from the addon's own", func() {
		own := applyMarker
		own.Name = "addon-definitions"
		app := renderAddon(pkgaddon.InstallPackage{
			AppTemplate: withWorkflow(&v1beta1.Workflow{Steps: []wfv1alpha1.WorkflowStep{own}}),
			Definitions: []pkgaddon.ElementFile{definition},
		}, nil)

		steps := workflowSteps(app)
		Expect(stepNames(steps)).To(Equal([]string{"addon-definitions-2", "addon-definitions"}))
		Expect(appliedComponent(steps[0])).To(Equal("addon-definitions"))
		Expect(appliedComponent(steps[1])).To(Equal("marker"))
	})

	It("makes its own steps wait for its definitions when they run as a DAG", func() {
		app := renderAddon(pkgaddon.InstallPackage{
			AppTemplate: withWorkflow(&v1beta1.Workflow{
				Mode:  &wfv1alpha1.WorkflowExecuteMode{Steps: workflowv1alpha1.WorkflowModeDAG},
				Steps: []wfv1alpha1.WorkflowStep{applyMarker},
			}),
			Definitions: []pkgaddon.ElementFile{definition},
		}, nil)

		steps := workflowSteps(app)
		Expect(stepNames(steps)).To(Equal([]string{"addon-definitions", "apply"}))
		Expect(steps[1]["dependsOn"]).To(ConsistOf("addon-definitions"))
	})

	It("is refused when its workflow is a ref and it ships definitions", func() {
		pkg := pkgaddon.InstallPackage{
			Meta:        pkgaddon.Meta{Name: "ref-addon", Version: "1.0.0"},
			AppTemplate: withWorkflow(&v1beta1.Workflow{Ref: "my-wf"}),
			Definitions: []pkgaddon.ElementFile{definition},
		}
		r := &rendererImpl{
			cli: fake.NewClientBuilder().Build(),
			findPackagesFn: func(_ context.Context, _ client.Client, _, _ []string) ([]*pkgaddon.WholeAddonPackage, error) {
				return []*pkgaddon.WholeAddonPackage{{InstallPackage: pkg, RegistryName: "fixture"}}, nil
			},
		}
		_, err := r.resolveAndRender(context.Background(), api.AddonRequest{Name: "ref-addon", SkipVersionValidate: true})
		Expect(err).To(MatchError(ContainSubstring("addon-definitions")))
	})

	It("leaves a workflow ref alone when it adds nothing to apply", func() {
		app := renderAddon(pkgaddon.InstallPackage{AppTemplate: withWorkflow(&v1beta1.Workflow{Ref: "my-wf"})}, nil)
		spec := app["spec"].(map[string]interface{})
		Expect(spec["workflow"]).To(HaveKeyWithValue("ref", "my-wf"))
	})
})
