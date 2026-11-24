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
	"encoding/json"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/addon/service/api"
	"github.com/oam-dev/kubevela/pkg/crate"
	cratesvc "github.com/oam-dev/kubevela/pkg/crate/service"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// placedInlineCrate has one runtime object and one hub object, so its owned
// Application splits into hub and runtime parts when it gets a placement.
func placedInlineCrate() *crate.Crate {
	parsed, err := crate.ParseCrate(cratesvc.MapFS{
		"_crate.cue":      []byte("crate: \"edge-kit\"\nversion: \"0.1.0\"\nplacement: \"runtime\""),
		"v1/_version.cue": []byte(`apiVersion: "v1"`),
		"auxiliary/defaults.yaml": []byte(
			"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: edge-defaults\n"),
		"auxiliary/hub/admission.yaml": []byte(
			"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: edge-admission\n"),
		"v1/definitions/bucket.yaml": []byte(inlineBucketDefinition),
	})
	Expect(err).NotTo(HaveOccurred())
	return parsed
}

// renderInlineAddon renders an addon that bundles edge-kit inline, as a
// type: addon component with the given placement would ask for it.
func renderInlineAddon(place *v1alpha1.Placement) map[string]interface{} {
	return renderAddon(pkgaddon.InstallPackage{
		AppTemplate:  &v1beta1.Application{},
		InlineCrates: []*crate.Crate{placedInlineCrate()},
	}, place)
}

// renderAddon renders pkg as version 1.0.0 of the addon inline-addon.
func renderAddon(pkg pkgaddon.InstallPackage, place *v1alpha1.Placement) map[string]interface{} {
	pkg.Meta = pkgaddon.Meta{Name: "inline-addon", Version: "1.0.0"}
	r := &rendererImpl{
		cli: fake.NewClientBuilder().Build(),
		findPackagesFn: func(_ context.Context, _ client.Client, _, _ []string) ([]*pkgaddon.WholeAddonPackage, error) {
			return []*pkgaddon.WholeAddonPackage{{InstallPackage: pkg, RegistryName: "fixture"}}, nil
		},
	}
	res, err := r.resolveAndRender(context.Background(), api.AddonRequest{
		Name: "inline-addon", SkipVersionValidate: true, Placement: place,
	})
	Expect(err).NotTo(HaveOccurred())
	return res.Application
}

// ownedCratePolicies returns the policies of the owned Application that the
// inline crate's k8s-objects component carries, keyed by policy name.
func ownedCratePolicies(app map[string]interface{}, componentName string) map[string]map[string]interface{} {
	spec, ok := app["spec"].(map[string]interface{})
	Expect(ok).To(BeTrue())
	comps, ok := spec["components"].([]interface{})
	Expect(ok).To(BeTrue())
	for _, item := range comps {
		comp, ok := item.(map[string]interface{})
		Expect(ok).To(BeTrue())
		if comp["name"] != componentName {
			continue
		}
		props, ok := comp["properties"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		objects, ok := props["objects"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(objects).To(HaveLen(1))
		owned, ok := objects[0].(map[string]interface{})
		Expect(ok).To(BeTrue())
		ownedSpec, ok := owned["spec"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		policies, _ := ownedSpec["policies"].([]interface{})
		out := map[string]map[string]interface{}{}
		for _, p := range policies {
			policy, ok := p.(map[string]interface{})
			Expect(ok).To(BeTrue())
			out[policy["name"].(string)] = policy
		}
		return out
	}
	Fail("no component named " + componentName)
	return nil
}

var _ = Describe("inline crate placement", func() {
	BeforeEach(func() {
		prev := utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCrateComponent)
		Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{string(features.EnableCrateComponent): true})).To(Succeed())
		DeferCleanup(func() {
			Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{string(features.EnableCrateComponent): prev})).To(Succeed())
		})
	})

	It("sends the runtime objects to the clusters of the addon's placement", func() {
		app := renderInlineAddon(&v1alpha1.Placement{Clusters: []string{"spoke-1"}})
		policies := ownedCratePolicies(app, "edge-kit")
		Expect(policies).To(HaveKey("crate-runtime"))
		Expect(policies["crate-runtime"]["type"]).To(Equal(v1alpha1.TopologyPolicyType))
		Expect(policies["crate-runtime"]["properties"]).To(HaveKeyWithValue("clusters", ConsistOf("spoke-1")))
	})

	It("forwards a label selector and allowEmpty too", func() {
		app := renderInlineAddon(&v1alpha1.Placement{ClusterLabelSelector: map[string]string{"tier": "runtime"}, AllowEmpty: true})
		props := ownedCratePolicies(app, "edge-kit")["crate-runtime"]["properties"]
		Expect(props).To(HaveKeyWithValue("clusterLabelSelector", HaveKeyWithValue("tier", "runtime")))
		Expect(props).To(HaveKeyWithValue("allowEmpty", true))
	})

	It("keeps everything on the hub without a placement", func() {
		app := renderInlineAddon(nil)
		Expect(ownedCratePolicies(app, "edge-kit")).NotTo(HaveKey("crate-runtime"))
	})

	// The dispatcher patches the addon Application with no last-applied
	// configuration (suppressLastAppliedConfig), so a field the new render
	// leaves out is never deleted. Removing the placement drops the generated
	// policies, and the old workflow must not survive to name them.
	It("leaves no stale workflow when the placement is removed from an installed addon", func() {
		ctx := context.Background()
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		applicator := apply.NewAPIApplicator(cli)
		applyRender := func(place *v1alpha1.Placement) {
			obj := &unstructured.Unstructured{Object: renderInlineAddon(place)}
			obj.SetName("addon-inline-addon")
			Expect(applicator.Apply(ctx, obj)).To(Succeed())
		}

		applyRender(&v1alpha1.Placement{Clusters: []string{"spoke-1"}})
		placed := &v1beta1.Application{}
		Expect(cli.Get(ctx, client.ObjectKey{Namespace: "vela-system", Name: "addon-inline-addon"}, placed)).To(Succeed())
		Expect(placed.Spec.Workflow).NotTo(BeNil(), "a placed addon with crates gets a generated workflow")
		Expect(placed.Spec.Workflow.Steps).NotTo(BeEmpty())

		applyRender(nil)
		plain := &v1beta1.Application{}
		Expect(cli.Get(ctx, client.ObjectKey{Namespace: "vela-system", Name: "addon-inline-addon"}, plain)).To(Succeed())
		Expect(plain.Spec.Policies).NotTo(ContainElement(HaveField("Name", "addon-placement")))
		Expect(plain.Spec.Workflow).NotTo(BeNil())
		Expect(string(plain.Spec.Workflow.Mode.Steps)).To(Equal("DAG"), "generated steps run as a DAG, as they do with no workflow")
		names := []string{}
		for _, s := range plain.Spec.Workflow.Steps {
			Expect(s.Type).To(Equal("apply-component"), "the default steps, not the placement's deploy steps")
			names = append(names, s.Name)
		}
		components := []string{}
		for _, c := range plain.Spec.Components {
			components = append(components, c.Name)
		}
		Expect(names).To(ConsistOf(components), "one apply-component step per component")
	})

	It("leaves no stale workflow when a new addon version drops its crates", func() {
		ctx := context.Background()
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		applicator := apply.NewAPIApplicator(cli)
		applyApp := func(rendered map[string]interface{}) {
			obj := &unstructured.Unstructured{Object: rendered}
			obj.SetName("addon-inline-addon")
			Expect(applicator.Apply(ctx, obj)).To(Succeed())
		}

		for _, place := range []*v1alpha1.Placement{nil, {Clusters: []string{"spoke-1"}}} {
			applyApp(renderInlineAddon(place))
			applyApp(renderAddon(pkgaddon.InstallPackage{AppTemplate: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{Components: []common2.ApplicationComponent{
					{Name: "marker", Type: "k8s-objects", Properties: rawJSON(map[string]interface{}{"objects": []interface{}{}})},
				}},
			}}, nil))
			app := &v1beta1.Application{}
			Expect(cli.Get(ctx, client.ObjectKey{Namespace: "vela-system", Name: "addon-inline-addon"}, app)).To(Succeed())
			Expect(app.Spec.Workflow).NotTo(BeNil())
			names := []string{}
			for _, s := range app.Spec.Workflow.Steps {
				names = append(names, s.Name)
			}
			Expect(names).To(ConsistOf("marker"), "placement %v: no step names the dropped crate", place)
			Expect(cli.Delete(ctx, app)).To(Succeed())
		}
	})

	It("gives an addon's own workflow the default mode explicitly", func() {
		rendered := renderAddon(pkgaddon.InstallPackage{AppTemplate: &v1beta1.Application{
			Spec: v1beta1.ApplicationSpec{
				Components: []common2.ApplicationComponent{{Name: "marker", Type: "k8s-objects", Properties: rawJSON(map[string]interface{}{"objects": []interface{}{}})}},
				Workflow: &v1beta1.Workflow{Steps: []wfv1alpha1.WorkflowStep{{
					WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "apply", Type: "apply-component", Properties: rawJSON(map[string]interface{}{"component": "marker"})},
				}}},
			},
		}}, nil)
		wf := rendered["spec"].(map[string]interface{})["workflow"].(map[string]interface{})
		Expect(wf["mode"]).To(Equal(map[string]interface{}{"steps": "StepByStep", "subSteps": "DAG"}),
			"the mode the controller uses for steps written in spec, so an earlier render's mode cannot linger")
		Expect(wf["steps"]).To(HaveLen(1))
	})

	Describe("an addon upgrade that changes the workflow", func() {
		var (
			ctx     context.Context
			cli     client.Client
			upgrade func(templates ...*v1beta1.Application) *v1beta1.Application
		)
		marker := common2.ApplicationComponent{Name: "marker", Type: "k8s-objects", Properties: rawJSON(map[string]interface{}{"objects": []interface{}{}})}
		applyMarker := wfv1alpha1.WorkflowStep{
			WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "apply", Type: "apply-component", Properties: rawJSON(map[string]interface{}{"component": "marker"})},
		}
		template := func(wf *v1beta1.Workflow, components ...common2.ApplicationComponent) *v1beta1.Application {
			return &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: components, Workflow: wf}}
		}

		BeforeEach(func() {
			ctx = context.Background()
			cli = fake.NewClientBuilder().WithScheme(common.Scheme).Build()
			applicator := apply.NewAPIApplicator(cli)
			// upgrade applies each template in turn as the next version of the
			// addon, the way the dispatcher does, and returns the Application.
			upgrade = func(templates ...*v1beta1.Application) *v1beta1.Application {
				for _, tmpl := range templates {
					obj := &unstructured.Unstructured{Object: renderAddon(pkgaddon.InstallPackage{AppTemplate: tmpl}, nil)}
					obj.SetName("addon-inline-addon")
					Expect(applicator.Apply(ctx, obj)).To(Succeed())
				}
				app := &v1beta1.Application{}
				Expect(cli.Get(ctx, client.ObjectKey{Namespace: "vela-system", Name: "addon-inline-addon"}, app)).To(Succeed())
				return app
			}
		})

		It("fills in the mode keys the addon's own workflow leaves out", func() {
			app := upgrade(
				template(nil, marker),
				template(&v1beta1.Workflow{
					Mode:  &wfv1alpha1.WorkflowExecuteMode{SubSteps: workflowv1alpha1.WorkflowModeStep},
					Steps: []wfv1alpha1.WorkflowStep{applyMarker},
				}, marker),
			)
			Expect(app.Spec.Workflow.Mode).To(Equal(&wfv1alpha1.WorkflowExecuteMode{
				Steps:    workflowv1alpha1.WorkflowModeStep,
				SubSteps: workflowv1alpha1.WorkflowModeStep,
			}), "steps written in spec run one by one, not as the DAG the earlier render wrote")
		})

		It("removes a workflow ref the next version drops", func() {
			app := upgrade(
				template(&v1beta1.Workflow{Ref: "my-wf"}, marker),
				template(nil, marker),
			)
			Expect(app.Spec.Workflow.Ref).To(BeEmpty())
			Expect(app.Spec.Workflow.Steps).To(HaveLen(1))
		})

		It("removes generated steps and their mode when the next version names a workflow by ref", func() {
			app := upgrade(
				template(nil, marker),
				template(&v1beta1.Workflow{Ref: "my-wf"}, marker),
			)
			Expect(app.Spec.Workflow.Ref).To(Equal("my-wf"))
			Expect(app.Spec.Workflow.Steps).To(BeEmpty())
			Expect(app.Spec.Workflow.Mode).To(BeNil(), "the mode comes from the referenced Workflow")
		})

		It("keeps the mode an addon sets next to its workflow ref", func() {
			app := upgrade(template(&v1beta1.Workflow{
				Ref:  "my-wf",
				Mode: &wfv1alpha1.WorkflowExecuteMode{Steps: workflowv1alpha1.WorkflowModeDAG},
			}, marker))
			Expect(app.Spec.Workflow.Mode).To(Equal(&wfv1alpha1.WorkflowExecuteMode{Steps: workflowv1alpha1.WorkflowModeDAG}))
		})

		It("removes the workflow when the next version has no component", func() {
			app := upgrade(
				template(nil, marker),
				template(nil),
			)
			Expect(app.Spec.Workflow).To(BeNil(), "no step may name the component that is gone")
		})

		It("changes nothing when the same version is applied again", func() {
			for _, tmpl := range []*v1beta1.Application{
				template(nil, marker),
				template(nil),
				template(&v1beta1.Workflow{Ref: "my-wf"}, marker),
				template(&v1beta1.Workflow{Steps: []wfv1alpha1.WorkflowStep{applyMarker}}, marker),
			} {
				first := upgrade(tmpl).Spec
				Expect(upgrade(tmpl).Spec).To(Equal(first))
				Expect(cli.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "vela-system", Name: "addon-inline-addon"}})).To(Succeed())
			}
		})
	})

	// Anything that updates the addon Application through the typed struct (the
	// controller adding its finalizer, for one) drops omitempty fields. A
	// workflow that changes on that round trip is patched back by the next
	// dispatch, so the spec would flip on every pass. A null is the exception:
	// it removes its field, the typed struct has no such field, and writing the
	// null again changes nothing.
	It("writes workflows that survive a round trip through the typed Application", func() {
		for _, place := range []*v1alpha1.Placement{nil, {Clusters: []string{"spoke-1"}}} {
			rendered := renderInlineAddon(place)
			typed := &v1beta1.Application{}
			Expect(runtime.DefaultUnstructuredConverter.FromUnstructured(rendered, typed)).To(Succeed())
			back, err := runtime.DefaultUnstructuredConverter.ToUnstructured(typed)
			Expect(err).NotTo(HaveOccurred())
			written := map[string]interface{}{}
			for key, value := range rendered["spec"].(map[string]interface{})["workflow"].(map[string]interface{}) {
				if value != nil {
					written[key] = value
				}
			}
			want, err := json.Marshal(written)
			Expect(err).NotTo(HaveOccurred())
			got, err := json.Marshal(back["spec"].(map[string]interface{})["workflow"])
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(MatchJSON(want), "placement %v", place)
		}
	})
})
