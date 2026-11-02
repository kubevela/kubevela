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

package mcp

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const exposeTemplate = `
outputs: service: {
	apiVersion: "v1"
	kind:       "Service"
	spec: {
		selector: "app.oam.dev/component": context.name
		ports: [for p in parameter.ports {port: p, targetPort: p}]
		type: parameter.type
	}
}
outputs: ingress: {
	apiVersion: "networking.k8s.io/v1"
	kind:       "Ingress"
	spec: rules: [{host: parameter["host"]}]
}
patch: metadata: annotations: "expose.oam.dev/type": parameter.type
parameter: {
	// +usage=Ports to expose
	ports: [...int]
	// +usage=Service type
	type: *"ClusterIP" | "NodePort" | "LoadBalancer"
	host?: string
}
`

func explainFixtures() []client.Object {
	objs := append(platformFixtures(), webApp(), failedApp())
	webservice := objs[1].(*v1beta1.ComponentDefinition)
	webservice.Spec.Workload = oamcommon.WorkloadTypeDescriptor{Type: "deployments.apps", Definition: oamcommon.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}}
	webservice.Spec.Status = &oamcommon.Status{HealthPolicy: "isHealth: context.output.status.readyReplicas == context.output.spec.replicas"}
	webservice.Annotations[types.AnnoDefinitionAlias] = "Web Service"
	for i, rev := range []int64{3, 1, 2} {
		objs = append(objs, &v1beta1.DefinitionRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "webservice-v" + string(rune('0'+rev)),
				Namespace:         types.DefaultKubeVelaNS,
				Labels:            map[string]string{oam.LabelComponentDefinitionName: "webservice"},
				CreationTimestamp: metav1.NewTime(time.Date(2026, 9, 1+i, 0, 0, 0, 0, time.UTC)),
			},
			Spec: v1beta1.DefinitionRevisionSpec{Revision: rev, RevisionHash: "hash" + string(rune('0'+rev)), DefinitionType: oamcommon.ComponentType},
		})
	}
	objs = append(objs, &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "expose", Namespace: types.DefaultKubeVelaNS, Labels: map[string]string{types.LabelDefinitionDeprecated: "true"}},
		Spec: v1beta1.TraitDefinitionSpec{
			AppliesToWorkloads: []string{"deployments.apps", "statefulsets.apps"},
			ConflictsWith:      []string{"gateway"},
			PodDisruptive:      false,
			Stage:              v1beta1.PostDispatch,
			Schematic:          &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: exposeTemplate}},
			Restrictions:       &oamcommon.DefinitionRestrictions{Namespaces: []string{"team-a"}},
		},
	})
	return objs
}

var _ = Describe("explain_definition", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	It("explains a component definition with workload, health, revisions and consumers", func() {
		h = startHarness(Options{}, explainFixtures()...)
		var out ExplainDefinitionOutput
		res := h.call("explain_definition", map[string]any{"name": "webservice"}, &out)
		Expect(res.IsError).To(BeFalse())
		Expect(out.Type).To(Equal(DefinitionTypeComponent))
		Expect(out.Alias).To(Equal("Web Service"))
		Expect(out.Workload).To(Equal(&WorkloadInfo{Type: "deployments.apps", APIVersion: "apps/v1", Kind: "Deployment"}))
		Expect(out.Health.HealthPolicy).To(ContainSubstring("readyReplicas"))
		Expect(out.Produces).To(ConsistOf(ProducedResource{Path: "output", APIVersion: "apps/v1", Kind: "Deployment"}))
		Expect(out.ParameterUsage).To(ConsistOf(
			ParameterUsage{Parameter: "image", Fields: []string{"output.spec.template.spec.containers.image"}},
			ParameterUsage{Parameter: "port", Fields: []string{"output.spec.template.spec.containers.ports.containerPort"}},
		))
		Expect(out.Required).To(ConsistOf("image"))
		Expect(out.Revisions).To(HaveLen(3))
		Expect(out.Revisions[0].Revision).To(Equal(int64(3)))
		Expect(out.Revisions[2].Revision).To(Equal(int64(1)))
		Expect(out.Consumers.Count).To(Equal(1))
		Expect(out.Consumers.Applications).To(ConsistOf("team-a/web"))
		Expect(out.Consumers.NamespacesScanned).To(Equal([]string{"all"}))
		Expect(out.Evidence).To(ContainElement(ContainSubstring("ComponentDefinition vela-system/webservice")))
		Expect(out.InspectionError).To(BeEmpty())
	})

	It("explains a trait definition with behaviour, restrictions and multiple outputs", func() {
		h = startHarness(Options{Namespaces: []string{"team-b"}}, explainFixtures()...)
		var out ExplainDefinitionOutput
		h.call("explain_definition", map[string]any{"name": "expose", "type": "trait"}, &out)
		Expect(out.Deprecated).To(BeTrue())
		Expect(out.Trait.AppliesToWorkloads).To(ConsistOf("deployments.apps", "statefulsets.apps"))
		Expect(out.Trait.ConflictsWith).To(ConsistOf("gateway"))
		Expect(out.Trait.Stage).To(Equal("PostDispatch"))
		Expect(out.RestrictedToNamespaces).To(ConsistOf("team-a"))
		Expect(out.Produces).To(ConsistOf(
			ProducedResource{Path: "outputs.service", APIVersion: "v1", Kind: "Service"},
			ProducedResource{Path: "outputs.ingress", APIVersion: "networking.k8s.io/v1", Kind: "Ingress"},
			ProducedResource{Path: "patch", Kind: "(workload patch)"},
		))
		usage := map[string][]string{}
		for _, u := range out.ParameterUsage {
			usage[u.Parameter] = u.Fields
		}
		Expect(usage["ports"]).To(ConsistOf("outputs.service.spec.ports"))
		Expect(usage["type"]).To(ConsistOf("outputs.service.spec.type", "patch.metadata.annotations.expose.oam.dev/type"))
		Expect(usage["host"]).To(ConsistOf("outputs.ingress.spec.rules.host"))
		Expect(out.Required).To(ConsistOf("ports"))
		Expect(out.Revisions).To(BeEmpty())
		Expect(out.Consumers.Count).To(Equal(0), "team-b has no consumer of expose")
		Expect(out.Consumers.NamespacesScanned).To(Equal([]string{"team-b"}))
	})

	It("counts pinned references as consumers and reports restrictions on a component", func() {
		objs := explainFixtures()
		webservice := objs[1].(*v1beta1.ComponentDefinition)
		webservice.Spec.Restrictions = &oamcommon.DefinitionRestrictions{
			Namespaces:        []string{"team-*"},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "apps"}},
		}
		pinned := webApp()
		pinned.Name = "pinned"
		pinned.Spec.Components[0].Type = "webservice@v2"
		h = startHarness(Options{}, append(objs, pinned)...)
		var out ExplainDefinitionOutput
		h.call("explain_definition", map[string]any{"name": "webservice", "type": "component"}, &out)
		Expect(out.Consumers.Applications).To(ContainElement("team-a/pinned"))
		Expect(out.RestrictedToNamespaces).To(Equal([]string{"team-*"}))
		Expect(out.RestrictedBySelector).To(Equal("tier=apps"))
	})

	It("explains an inherited definition with its metadata and a schema limitation, not an error", func() {
		child := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "webservice-plus", Namespace: types.DefaultKubeVelaNS, Annotations: map[string]string{types.AnnoDefinitionDescription: "webservice with defaults"}},
			Spec: v1beta1.ComponentDefinitionSpec{
				Extends:   "webservice",
				Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "parameter: $super.parameter & {port: *8080 | int}\n"}},
			},
		}
		h = startHarness(Options{}, append(explainFixtures(), child)...)
		var out ExplainDefinitionOutput
		res := h.call("explain_definition", map[string]any{"name": "webservice-plus", "type": "component"}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Extends).To(Equal("webservice"))
		Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("extends another")))
	})

	It("returns the definition's metadata and the inspection error for a template that does not parse", func() {
		broken := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: types.DefaultKubeVelaNS, Annotations: map[string]string{types.AnnoDefinitionDescription: "half written"}},
			Spec:       v1beta1.ComponentDefinitionSpec{Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "output: {"}}},
		}
		h = startHarness(Options{}, append(explainFixtures(), broken)...)
		var out ExplainDefinitionOutput
		res := h.call("explain_definition", map[string]any{"name": "broken", "type": "component"}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Description).To(Equal("half written"))
		Expect(out.InspectionError).NotTo(BeEmpty())
		Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("parameter schema unavailable")))
	})

	It("reports an unknown definition", func() {
		h = startHarness(Options{}, explainFixtures()...)
		res := h.call("explain_definition", map[string]any{"name": "ghost"}, nil)
		Expect(errorText(res)).To(ContainSubstring("not found"))
	})
})

var _ = Describe("template inspection", func() {
	It("lists patchOutputs and surfaces built by expressions, and flags computed top-level fields", func() {
		facts := inspectTemplate(`
patch: base & {spec: replicas: 2}
patchOutputs: ingress: {apiVersion: "networking.k8s.io/v1", kind: "Ingress"}
parameter: {}
`)
		paths := map[string]ProducedResource{}
		for _, p := range facts.Produces {
			paths[p.Path] = p
		}
		Expect(paths).To(HaveKey("patch"), "an expression still patches the workload")
		Expect(paths["patch"].Kind).To(BeEmpty())
		Expect(paths["patchOutputs.ingress"].Kind).To(Equal("Ingress"))
		Expect(facts.Dynamic).To(BeFalse())

		dynamic := inspectTemplate("\"\\(parameter.name)\": {kind: \"ConfigMap\"}\nparameter: name: string\n")
		Expect(dynamic.Err).To(BeEmpty())
		Expect(dynamic.Dynamic).To(BeTrue())
	})

	It("tolerates a template that does not parse", func() {
		facts := inspectTemplate("output: {")
		Expect(facts.Err).NotTo(BeEmpty())
		Expect(facts.Produces).To(BeEmpty())
	})

	It("ignores references inside the parameter block itself", func() {
		facts := inspectTemplate("parameter: {a: string, b: parameter.a}\noutput: kind: \"ConfigMap\"\n")
		Expect(facts.Usage).To(BeEmpty())
		Expect(facts.Produces).To(ConsistOf(ProducedResource{Path: "output", Kind: "ConfigMap"}))
	})
})
