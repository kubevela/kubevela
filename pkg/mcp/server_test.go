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
	"errors"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/rest"

	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/version"
)

var _ = Describe("vela mcp server", func() {
	var h *harness

	BeforeEach(func() {
		version.VelaVersion = "v1.11.0"
	})

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	Context("registry", func() {
		It("registers only read tools by default and annotates them read-only", func() {
			h = startHarness(Options{}, platformFixtures()...)
			for _, t := range h.srv.Tools() {
				Expect(t.Class).To(Equal(ToolClassRead))
			}
			res, err := h.session.ListTools(h.ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			names := map[string]*sdk.Tool{}
			for _, t := range res.Tools {
				names[t.Name] = t
			}
			for _, name := range []string{"get_platform_info", "list_definitions", "describe_definition",
				"list_applications", "get_application", "get_workflow", "get_managed_resource", "get_logs"} {
				Expect(names).To(HaveKey(name))
				Expect(names[name].Annotations.ReadOnlyHint).To(BeTrue(), name)
			}
			Expect(names).NotTo(HaveKey("apply_application"))
			Expect(names).NotTo(HaveKey("resume_workflow"))
		})

		It("refuses an unsupported transport", func() {
			_, err := NewServer(velacmd.NewTestFactory(nil, nil), Options{Transport: "http"})
			Expect(err).To(MatchError(ContainSubstring("not supported")))
		})
	})

	Context("get_platform_info", func() {
		It("reports versions, gates and controller health from the cluster", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out GetPlatformInfoOutput
			res := h.call("get_platform_info", nil, &out)
			Expect(res.IsError).To(BeFalse())
			Expect(out.VelaCLI).To(Equal("v1.11.0"))
			Expect(out.VelaCore).To(Equal("1.11.0"))
			Expect(out.Kubernetes).To(HaveKeyWithValue("local", "v1.31.4"))
			Expect(out.FeatureGates).To(HaveKeyWithValue("AuthenticateApplication", false))
			Expect(out.FeatureGates).To(HaveKeyWithValue("RestrictDefinitionNamespaces", true))
			Expect(out.FeatureGates).To(HaveKeyWithValue("ApplyOnce", true))
			Expect(out.Controller.Found).To(BeTrue())
			Expect(out.Controller.ReadyReplicas).To(Equal(int32(1)))
			Expect(out.Warnings).To(BeEmpty())
			Expect(out.Meta.Source).To(Equal(SourceLive))
			Expect(out.Meta.VelaCore).To(Equal("1.11.0"))
			Expect(out.Meta.ObservedAt).NotTo(BeZero())
		})

		It("warns on minor version skew and degrades without a controller", func() {
			version.VelaVersion = "v1.12.0"
			h = startHarness(Options{ServerVersion: func(*rest.Config) (string, error) { return "", errors.New("boom") }}, platformFixtures()[1:]...)
			var out GetPlatformInfoOutput
			res := h.call("get_platform_info", nil, &out)
			Expect(res.IsError).To(BeFalse())
			Expect(out.VelaCore).To(Equal(unknownVersion))
			Expect(out.Kubernetes).To(HaveKeyWithValue("local", unknownVersion))
			Expect(out.Controller.Found).To(BeFalse())
			Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("no vela-core deployment")))
			Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("kubernetes version: boom")))
		})

		It("falls back to the image tag when the chart label is missing and flags skew", func() {
			objs := platformFixtures()
			deploy := objs[0].(*appsv1.Deployment)
			delete(deploy.Labels, chartVersionLabel)
			deploy.Spec.Template.Spec.Containers[0].Image = "oamdev/vela-core:v1.10.4"
			h = startHarness(Options{}, objs...)
			var out GetPlatformInfoOutput
			h.call("get_platform_info", nil, &out)
			Expect(out.VelaCore).To(Equal("v1.10.4"))
			Expect(out.Warnings).To(ContainElement(ContainSubstring("version skew")))
		})
	})

	Context("list_definitions", func() {
		It("lists every type across namespaces, sorted", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out ListDefinitionsOutput
			h.call("list_definitions", nil, &out)
			Expect(out.Definitions).To(HaveLen(3))
			Expect(out.Definitions[0].Type).To(Equal(DefinitionTypeComponent))
			Expect(out.Definitions[0].Name).To(Equal("webservice"))
			Expect(out.Definitions[0].Version).To(Equal("2.1.0"))
			Expect(out.Definitions[0].LatestRevision).To(Equal("webservice-v3"))
			Expect(out.Definitions[0].Description).To(ContainSubstring("long-running"))
			Expect(out.Definitions[1].Type).To(Equal(DefinitionTypePolicy))
			Expect(out.Definitions[1].Namespace).To(Equal("team-a"))
			Expect(out.Definitions[2].Type).To(Equal(DefinitionTypeTrait))
		})

		It("filters by type and rejects unknown types", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out ListDefinitionsOutput
			h.call("list_definitions", map[string]any{"type": "trait"}, &out)
			Expect(out.Definitions).To(HaveLen(1))
			Expect(out.Definitions[0].Name).To(Equal("scaler"))

			res := h.call("list_definitions", map[string]any{"type": "gadget"}, nil)
			Expect(res.IsError).To(BeTrue())
		})

		It("respects the namespace restriction while always including the system namespace", func() {
			h = startHarness(Options{Namespaces: []string{"team-a"}}, platformFixtures()...)
			var out ListDefinitionsOutput
			h.call("list_definitions", nil, &out)
			names := []string{}
			for _, d := range out.Definitions {
				names = append(names, d.Namespace+"/"+d.Name)
			}
			Expect(names).To(ConsistOf("team-a/team-policy", "vela-system/webservice", "vela-system/scaler"))
		})
	})

	Context("describe_definition", func() {
		It("returns the parameter schema with required fields and defaults", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out DescribeDefinitionOutput
			res := h.call("describe_definition", map[string]any{"name": "webservice"}, &out)
			Expect(res.IsError).To(BeFalse())
			Expect(out.Type).To(Equal(DefinitionTypeComponent))
			Expect(out.LatestRevision).To(Equal("webservice-v3"))
			Expect(out.Required).To(ConsistOf("image"))
			props, ok := out.Parameters["properties"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(props).To(HaveKey("image"))
			port, ok := props["port"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(port["default"]).To(BeEquivalentTo(80))
			Expect(port["description"]).To(Equal("Which port to expose"))
		})

		It("finds a definition by type in a user namespace and reports unknown names", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out DescribeDefinitionOutput
			h.call("describe_definition", map[string]any{"name": "team-policy", "namespace": "team-a", "type": "policy"}, &out)
			Expect(out.Namespace).To(Equal("team-a"))

			res := h.call("describe_definition", map[string]any{"name": "nope"}, nil)
			Expect(errorText(res)).To(ContainSubstring("not found"))
		})
	})
})
