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
	"context"
	"encoding/json"
	"errors"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/version"
)

const webserviceTemplate = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: template: spec: containers: [{
		image: parameter.image
		ports: [{containerPort: parameter.port}]
	}]
}
parameter: {
	// +usage=Which image would you like to use
	image: string
	// +usage=Which port to expose
	port: *80 | int
}
`

var _ = Describe("vela mcp server", func() {
	var (
		ctx     context.Context
		cancel  context.CancelFunc
		session *sdk.ClientSession
		srv     *Server
	)

	fixtures := func() []client.Object {
		return []client.Object{
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubevela-vela-core",
					Namespace: types.DefaultKubeVelaNS,
					Labels: map[string]string{
						controllerNameLabel: controllerNameValue,
						chartVersionLabel:   "1.11.0",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To[int32](1),
					Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name:  "kubevela",
						Image: "oamdev/vela-core:v1.11.0",
						Args: []string{
							"--feature-gates=AuthenticateApplication=false",
							"--feature-gates=RestrictDefinitionNamespaces=true",
							"--feature-gates=EnableCueValidation=false,ApplyOnce=true",
						},
					}}}},
				},
				Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
			},
			&v1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "webservice",
					Namespace:   types.DefaultKubeVelaNS,
					Annotations: map[string]string{types.AnnoDefinitionDescription: "Describes long-running, scalable, containerized services"},
				},
				Spec: v1beta1.ComponentDefinitionSpec{
					Version:   "2.1.0",
					Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: webserviceTemplate}},
				},
				Status: v1beta1.ComponentDefinitionStatus{LatestRevision: &oamcommon.Revision{Name: "webservice-v3", Revision: 3}},
			},
			&v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "scaler",
					Namespace:   types.DefaultKubeVelaNS,
					Annotations: map[string]string{types.AnnoDefinitionDescription: "Manually scale the workload"},
				},
				Spec: v1beta1.TraitDefinitionSpec{
					Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "patch: spec: replicas: parameter.replicas\nparameter: replicas: *1 | int\n"}},
				},
			},
			&v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "team-policy", Namespace: "team-a"},
				Spec: v1beta1.PolicyDefinitionSpec{
					Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "parameter: {}\n"}},
				},
			},
		}
	}

	start := func(opts Options, objs ...client.Object) {
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(objs...).Build()
		f := velacmd.NewTestFactory(&rest.Config{Host: "https://example.invalid"}, cli)
		if opts.ServerVersion == nil {
			opts.ServerVersion = func(*rest.Config) (string, error) { return "v1.31.4", nil }
		}
		var err error
		srv, err = NewServer(f, opts)
		Expect(err).NotTo(HaveOccurred())

		clientTransport, serverTransport := sdk.NewInMemoryTransports()
		go func() { _ = srv.Serve(ctx, serverTransport) }()
		c := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "0"}, nil)
		session, err = c.Connect(ctx, clientTransport, nil)
		Expect(err).NotTo(HaveOccurred())
	}

	call := func(name string, args map[string]any, into any) *sdk.CallToolResult {
		res, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		Expect(err).NotTo(HaveOccurred())
		if into != nil && !res.IsError {
			raw, err := json.Marshal(res.StructuredContent)
			Expect(err).NotTo(HaveOccurred())
			Expect(json.Unmarshal(raw, into)).To(Succeed())
		}
		return res
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		version.VelaVersion = "v1.11.0"
	})

	AfterEach(func() {
		if session != nil {
			_ = session.Close()
		}
		cancel()
	})

	Context("registry", func() {
		It("registers only read tools by default and annotates them read-only", func() {
			start(Options{}, fixtures()...)
			for _, t := range srv.Tools() {
				Expect(t.Class).To(Equal(ToolClassRead))
			}
			res, err := session.ListTools(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			names := map[string]*sdk.Tool{}
			for _, t := range res.Tools {
				names[t.Name] = t
			}
			Expect(names).To(HaveKey("get_platform_info"))
			Expect(names).To(HaveKey("list_definitions"))
			Expect(names).To(HaveKey("describe_definition"))
			Expect(names).NotTo(HaveKey("apply_application"))
			Expect(names).NotTo(HaveKey("resume_workflow"))
			Expect(names["list_definitions"].Annotations.ReadOnlyHint).To(BeTrue())
		})

		It("refuses an unsupported transport", func() {
			_, err := NewServer(velacmd.NewTestFactory(nil, nil), Options{Transport: "http"})
			Expect(err).To(MatchError(ContainSubstring("not supported")))
		})
	})

	Context("get_platform_info", func() {
		It("reports versions, gates and controller health from the cluster", func() {
			start(Options{}, fixtures()...)
			var out GetPlatformInfoOutput
			res := call("get_platform_info", nil, &out)
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
			start(Options{ServerVersion: func(*rest.Config) (string, error) { return "", errors.New("boom") }}, fixtures()[1:]...)
			var out GetPlatformInfoOutput
			res := call("get_platform_info", nil, &out)
			Expect(res.IsError).To(BeFalse())
			Expect(out.VelaCore).To(Equal(unknownVersion))
			Expect(out.Kubernetes).To(HaveKeyWithValue("local", unknownVersion))
			Expect(out.Controller.Found).To(BeFalse())
			Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("no vela-core deployment")))
			Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("kubernetes version: boom")))
		})

		It("falls back to the image tag when the chart label is missing and flags skew", func() {
			objs := fixtures()
			deploy := objs[0].(*appsv1.Deployment)
			delete(deploy.Labels, chartVersionLabel)
			deploy.Spec.Template.Spec.Containers[0].Image = "oamdev/vela-core:v1.10.4"
			start(Options{}, objs...)
			var out GetPlatformInfoOutput
			call("get_platform_info", nil, &out)
			Expect(out.VelaCore).To(Equal("v1.10.4"))
			Expect(out.Warnings).To(ContainElement(ContainSubstring("version skew")))
		})
	})

	Context("list_definitions", func() {
		It("lists every type across namespaces, sorted", func() {
			start(Options{}, fixtures()...)
			var out ListDefinitionsOutput
			call("list_definitions", nil, &out)
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
			start(Options{}, fixtures()...)
			var out ListDefinitionsOutput
			call("list_definitions", map[string]any{"type": "trait"}, &out)
			Expect(out.Definitions).To(HaveLen(1))
			Expect(out.Definitions[0].Name).To(Equal("scaler"))

			res := call("list_definitions", map[string]any{"type": "gadget"}, nil)
			Expect(res.IsError).To(BeTrue())
		})

		It("respects the namespace restriction while always including the system namespace", func() {
			start(Options{Namespaces: []string{"team-a"}}, fixtures()...)
			var out ListDefinitionsOutput
			call("list_definitions", nil, &out)
			names := []string{}
			for _, d := range out.Definitions {
				names = append(names, d.Namespace+"/"+d.Name)
			}
			Expect(names).To(ConsistOf("team-a/team-policy", "vela-system/webservice", "vela-system/scaler"))
		})
	})

	Context("describe_definition", func() {
		It("returns the parameter schema with required fields and defaults", func() {
			start(Options{}, fixtures()...)
			var out DescribeDefinitionOutput
			res := call("describe_definition", map[string]any{"name": "webservice"}, &out)
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
			start(Options{}, fixtures()...)
			var out DescribeDefinitionOutput
			call("describe_definition", map[string]any{"name": "team-policy", "namespace": "team-a", "type": "policy"}, &out)
			Expect(out.Namespace).To(Equal("team-a"))

			res := call("describe_definition", map[string]any{"name": "nope"}, nil)
			Expect(res.IsError).To(BeTrue())
			Expect(res.Content).NotTo(BeEmpty())
		})
	})
})
