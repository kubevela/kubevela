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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// consumerApp declares one webservice component.
func consumerApp(name string) *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a"},
		Spec: v1beta1.ApplicationSpec{Components: []oamcommon.ApplicationComponent{
			{Name: name, Type: "webservice", Properties: raw(`{"image":"nginx:1.27"}`)},
		}},
	}
}

// proposedWebservice is the cluster's webservice template with one extra
// rendered label, as a definition object in YAML.
func proposedWebservice(template string) string {
	return fmt.Sprintf(`apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webservice
  namespace: vela-system
spec:
  schematic:
    cue:
      template: |
%s`, indent(template, "        "))
}

func indent(s, prefix string) string {
	out := ""
	for _, line := range splitLines(s) {
		out += prefix + line + "\n"
	}
	return out
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

const labeledWebserviceTemplate = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: labels: tier: "frontend"
	spec: template: spec: containers: [{
		image: parameter.image
		ports: [{containerPort: parameter.port}]
	}]
}
parameter: {
	image: string
	port: *80 | int
}
`

const breakingWebserviceTemplate = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: labels: tier: parameter.tier
	spec: template: spec: containers: [{
		image: parameter.image
	}]
}
parameter: {
	image: string
	tier:  string
}
`

var _ = Describe("diff_definition_impact", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	fixtures := func() []client.Object {
		return append(platformFixtures(), consumerApp("shop"), consumerApp("blog"),
			&v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "team-a"},
				Spec: v1beta1.ApplicationSpec{Components: []oamcommon.ApplicationComponent{
					{Name: "other", Type: "worker", Properties: raw(`{"image":"nginx"}`)},
				}},
			})
	}

	It("lists consumers without rendering when no proposed copy is given", func() {
		h = startHarness(Options{}, fixtures()...)
		var out DefinitionImpactOutput
		res := h.call("diff_definition_impact", map[string]any{"name": "webservice"}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Consumers.Count).To(Equal(2))
		Expect(out.Consumers.Applications).To(ConsistOf("team-a/blog", "team-a/shop"))
		Expect(out.Impacts).To(BeEmpty())
		Expect(out.Evidence).To(ContainElement(ContainSubstring("without rendering")))
	})

	It("renders every consumer against the proposed copy and diffs each", func() {
		h = startHarness(Options{}, fixtures()...)
		var out DefinitionImpactOutput
		res := h.call("diff_definition_impact", map[string]any{
			"name":       "webservice",
			"definition": proposedWebservice(labeledWebserviceTemplate),
		}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.DefinitionChanged).To(BeTrue())
		Expect(out.Impacts).To(HaveLen(2))
		for _, impact := range out.Impacts {
			Expect(impact.RenderError).To(BeEmpty())
			Expect(impact.Unchanged).To(BeFalse())
			Expect(impact.ChangeSet).NotTo(BeNil())
			Expect(impact.ChangeSet.Resources).To(HaveLen(1))
			Expect(impact.ChangeSet.Resources[0].Action).To(Equal(ActionModify))
			Expect(toJSON(impact.ChangeSet.Resources[0].Fields)).To(ContainSubstring("metadata.labels.tier"))
			Expect(impact.Risk).NotTo(BeNil())
		}
		Expect(out.Meta.Source).To(Equal(SourceRendered))
	})

	It("reports a consumer the proposed copy no longer renders as that application's error", func() {
		h = startHarness(Options{}, fixtures()...)
		var out DefinitionImpactOutput
		res := h.call("diff_definition_impact", map[string]any{
			"name":       "webservice",
			"definition": proposedWebservice(breakingWebserviceTemplate),
		}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Impacts).To(HaveLen(2))
		for _, impact := range out.Impacts {
			Expect(impact.RenderError).NotTo(BeEmpty(), impact.Name)
			Expect(impact.ChangeSet).To(BeNil())
		}
	})

	It("says when the proposed copy is identical and every impact is unchanged", func() {
		h = startHarness(Options{}, fixtures()...)
		var out DefinitionImpactOutput
		res := h.call("diff_definition_impact", map[string]any{
			"name":       "webservice",
			"definition": proposedWebservice(webserviceTemplate),
		}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Impacts).To(HaveLen(2))
		for _, impact := range out.Impacts {
			Expect(impact.Unchanged).To(BeTrue(), impact.Name)
		}
	})

	It("bounds how many consumers it renders and says so", func() {
		old := maxImpactApps
		maxImpactApps = 1
		DeferCleanup(func() { maxImpactApps = old })
		h = startHarness(Options{}, fixtures()...)
		var out DefinitionImpactOutput
		res := h.call("diff_definition_impact", map[string]any{
			"name":       "webservice",
			"definition": proposedWebservice(labeledWebserviceTemplate),
		}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Consumers.Count).To(Equal(2))
		Expect(out.Impacts).To(HaveLen(1))
		Expect(out.Meta.Truncated).To(BeTrue())
	})

	It("refuses a proposed copy whose name or kind does not match", func() {
		h = startHarness(Options{}, fixtures()...)
		res := h.call("diff_definition_impact", map[string]any{
			"name":       "worker",
			"definition": proposedWebservice(labeledWebserviceTemplate),
		}, nil)
		Expect(errorText(res)).To(ContainSubstring("named"))
	})

	It("scans only the allowed namespaces under a restriction", func() {
		h = startHarness(Options{Namespaces: []string{"team-b"}}, fixtures()...)
		var out DefinitionImpactOutput
		res := h.call("diff_definition_impact", map[string]any{"name": "webservice"}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Consumers.Count).To(BeZero())
		Expect(out.Consumers.NamespacesScanned).To(Equal([]string{"team-b"}))
	})
})
