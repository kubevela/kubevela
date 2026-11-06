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
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/yaml"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// endpointFixtures adds to managedFixtures a ClusterIP Service, a NodePort
// Service and an Ingress the web component manages, and a node whose
// external address a NodePort is reached on.
func endpointFixtures() []client.Object {
	objs := managedFixtures()
	track := func(apiVersion, kind, name string) v1beta1.ManagedResource {
		return v1beta1.ManagedResource{
			ClusterObjectReference: oamcommon.ClusterObjectReference{ObjectReference: corev1.ObjectReference{APIVersion: apiVersion, Kind: kind, Namespace: "team-a", Name: name}},
			OAMObjectReference:     oamcommon.OAMObjectReference{Component: "web"},
		}
	}
	for _, o := range objs {
		if rt, ok := o.(*v1beta1.ResourceTracker); ok {
			rt.Spec.ManagedResources = append(rt.Spec.ManagedResources,
				track("v1", "Service", "web"),
				track("v1", "Service", "web-np"),
				track("networking.k8s.io/v1", "Ingress", "web"),
			)
		}
	}
	prefix := networkingv1.PathTypePrefix
	return append(objs,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-a"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Ports: []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP}}},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web-np", Namespace: "team-a"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Ports: []corev1.ServicePort{{Name: "http", Port: 8080, NodePort: 30080, Protocol: corev1.ProtocolTCP}}},
		},
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-a"},
			Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
				Host: "web.example.com",
				IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
					Path:     "/",
					PathType: &prefix,
					Backend:  networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: "web", Port: networkingv1.ServiceBackendPort{Number: 80}}},
				}}}},
			}}},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeExternalIP, Address: "203.0.113.7"}}},
		},
	)
}

var _ = Describe("what the tools return", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	getApp := func(name, ns string) GetApplicationOutput {
		var out GetApplicationOutput
		res := h.call("get_application", map[string]any{"name": name, "namespace": ns}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		return out
	}
	urls := func(eps []Endpoint) []string {
		out := []string{}
		for _, e := range eps {
			out = append(out, e.URL)
		}
		return out
	}

	Context("get_application", func() {
		It("returns endpoints read live from the Services and Ingresses it manages", func() {
			h = startHarness(Options{}, endpointFixtures()...)
			out := getApp("web", "team-a")
			Expect(urls(out.Endpoints)).To(ContainElements(
				"http://web.example.com",
				"http://203.0.113.7:30080",
				"http://web.team-a",
			))
			for _, e := range out.Endpoints {
				Expect(e.Component).To(Equal("web"))
				Expect(e.Cluster).To(Equal("local"))
				Expect(e.Source).NotTo(BeEmpty())
				if e.URL == "http://web.team-a" {
					Expect(e.Internal).To(BeTrue(), "a ClusterIP Service name is only reachable in the cluster")
					Expect(e.Source).To(Equal("Service team-a/web"))
				}
			}
			Expect(out.Endpoints[len(out.Endpoints)-1].Internal).To(BeTrue(), "external addresses come first")
			Expect(out.Evidence).To(ContainElement(ContainSubstring("endpoints from the Services, Ingresses and HTTPRoutes")))
		})

		It("returns the same correlation handles as explain_application", func() {
			h = startHarness(Options{}, explainAppFixtures()...)
			out := getApp("web", "team-a")
			var explained ExplainApplicationOutput
			h.call("explain_application", map[string]any{"name": "web", "namespace": "team-a"}, &explained)
			out.Handles.Window.To, explained.Handles.Window.To = out.Handles.Window.From, explained.Handles.Window.From
			Expect(out.Handles).To(Equal(explained.Handles))
			Expect(out.Handles.Pods).To(HaveLen(2))
			Expect(out.Handles.LabelSelector).To(Equal("app.oam.dev/name=web,app.oam.dev/namespace=team-a"))
		})

		It("under an allowlist does not read nodes for a NodePort address and says so", func() {
			h = startHarness(Options{Namespaces: []string{"team-a"}}, endpointFixtures()...)
			out := getApp("web", "team-a")
			Expect(urls(out.Endpoints)).To(ContainElements("http://web.example.com", "http://web.team-a"))
			Expect(urls(out.Endpoints)).NotTo(ContainElement(ContainSubstring("203.0.113.7")))
			Expect(out.Unknowns).To(ContainElement(ContainSubstring("Service team-a/web-np port 30080: no node address")))
			Expect(out.Unknowns).To(ContainElement(SatisfyAll(ContainSubstring("outside the allowed namespaces"), ContainSubstring("Node list"))))
		})

		It("reports an endpoint read RBAC forbids instead of dropping it silently", func() {
			forbidNodes := &interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*corev1.NodeList); ok {
					return apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "", errors.New("cannot list nodes"))
				}
				return c.List(ctx, list, opts...)
			}}
			h = startHarnessIntercepted(Options{}, nil, forbidNodes, endpointFixtures()...)
			out := getApp("web", "team-a")
			Expect(urls(out.Endpoints)).To(ContainElement("http://web.example.com"))
			Expect(out.Unknowns).To(ContainElement(SatisfyAll(ContainSubstring("could not be read"), ContainSubstring("Node list"), ContainSubstring("forbidden"))))
			Expect(out.Unknowns).To(ContainElement(ContainSubstring("web-np port 30080: no node address")))
		})

		It("still answers from recorded status when the trackers cannot be listed", func() {
			failTrackers := &interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				// Forbidden falls back to vela-prism's ApplicationResourceTracker,
				// so an outage is what reaches the caller.
				if _, ok := list.(*v1beta1.ResourceTrackerList); ok {
					return apierrors.NewServiceUnavailable("etcd is unavailable")
				}
				return c.List(ctx, list, opts...)
			}}
			h = startHarnessIntercepted(Options{}, nil, failTrackers, endpointFixtures()...)
			out := getApp("web", "team-a")
			Expect(out.Endpoints).To(BeEmpty())
			Expect(out.Unknowns).To(ContainElement(SatisfyAll(ContainSubstring("managed resources"), ContainSubstring("etcd is unavailable"))))
			Expect(out.Status.Phase).NotTo(BeEmpty(), "the recorded status is still returned")
			Expect(out.Status.Services).NotTo(BeEmpty())
		})

		It("names a managed resource it could not read instead of dropping it", func() {
			failDeployment := &interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if u, ok := obj.(*unstructured.Unstructured); ok && u.GetKind() == "Deployment" && key.Name == "web" {
					return apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, key.Name, errors.New("denied"))
				}
				return c.Get(ctx, key, obj, opts...)
			}}
			h = startHarnessIntercepted(Options{}, nil, failDeployment, endpointFixtures()...)
			out := getApp("web", "team-a")
			Expect(out.Unknowns).To(ContainElement(SatisfyAll(
				ContainSubstring("could not be read"), ContainSubstring("Deployment team-a/web"), ContainSubstring("pods and endpoints"))))
			Expect(urls(out.Endpoints)).To(ContainElement("http://web.team-a"), "the resources that could be read are still answered")
		})

		It("lists external addresses before internal ones across components", func() {
			objs := endpointFixtures()
			for _, o := range objs {
				if rt, ok := o.(*v1beta1.ResourceTracker); ok {
					rt.Spec.ManagedResources = append(rt.Spec.ManagedResources, v1beta1.ManagedResource{
						ClusterObjectReference: oamcommon.ClusterObjectReference{ObjectReference: corev1.ObjectReference{APIVersion: "v1", Kind: "Service", Namespace: "team-a", Name: "aaa"}},
						OAMObjectReference:     oamcommon.OAMObjectReference{Component: "aaa"},
					})
				}
			}
			objs = append(objs, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "aaa", Namespace: "team-a"},
				Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Ports: []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}}},
			})
			h = startHarness(Options{}, objs...)
			eps := getApp("web", "team-a").Endpoints
			seenInternal := false
			for _, e := range eps {
				if e.Internal {
					seenInternal = true
				}
				Expect(seenInternal && !e.Internal).To(BeFalse(), "external %s comes after an internal address in %v", e.URL, urls(eps))
			}
			Expect(eps[0].Internal).To(BeFalse())
		})

		It("does not call a hostless Ingress a NodePort", func() {
			objs := endpointFixtures()
			for _, o := range objs {
				if ing, ok := o.(*networkingv1.Ingress); ok {
					ing.Spec.Rules[0].Host = ""
				}
			}
			h = startHarness(Options{}, objs...)
			out := getApp("web", "team-a")
			Expect(out.Unknowns).To(ContainElement(ContainSubstring("Ingress team-a/web sets no host")))
			Expect(out.Unknowns).NotTo(ContainElement(SatisfyAll(ContainSubstring("Ingress"), ContainSubstring("NodePort"))))
		})
	})

	Context("get_logs", func() {
		It("returns handles for the pods it read", func() {
			logs := func(context.Context, *rest.Config, string, string, string, *corev1.PodLogOptions) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("ready\n")), nil
			}
			h = startHarness(Options{PodLogs: logs}, managedFixtures()...)
			var out GetLogsOutput
			res := h.call("get_logs", map[string]any{"name": "web", "namespace": "team-a"}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Handles.Application).To(Equal("web"))
			Expect(out.Handles.Pods).To(ConsistOf(
				PodHandle{Cluster: "local", Namespace: "team-a", Name: "web-abc-1", Component: "web"},
				PodHandle{Cluster: "local", Namespace: "team-a", Name: "web-abc-2", Component: "web"},
			))
			Expect(out.Handles.Window.Basis).NotTo(BeEmpty())
		})

		It("names only the pod it read when a pod is selected", func() {
			logs := func(context.Context, *rest.Config, string, string, string, *corev1.PodLogOptions) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("ready\n")), nil
			}
			h = startHarness(Options{PodLogs: logs}, managedFixtures()...)
			var out GetLogsOutput
			res := h.call("get_logs", map[string]any{"name": "web", "namespace": "team-a", "pod": "web-abc-2"}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Handles.Pods).To(Equal([]PodHandle{{Cluster: "local", Namespace: "team-a", Name: "web-abc-2", Component: "web"}}))
			Expect(out.Handles.Clusters).To(Equal([]string{"local"}))
		})

		It("says the answer is incomplete when some cluster and namespace pairs are not traced", func() {
			objs := managedFixtures()
			allowed := []string{"team-a"}
			for i := 0; i < maxTreeScopes; i++ {
				ns := fmt.Sprintf("extra-%d", i)
				allowed = append(allowed, ns)
				for _, o := range objs {
					if rt, ok := o.(*v1beta1.ResourceTracker); ok {
						rt.Spec.ManagedResources = append(rt.Spec.ManagedResources, v1beta1.ManagedResource{
							ClusterObjectReference: oamcommon.ClusterObjectReference{ObjectReference: corev1.ObjectReference{APIVersion: "v1", Kind: "ConfigMap", Namespace: ns, Name: "cfg"}},
							OAMObjectReference:     oamcommon.OAMObjectReference{Component: "web"},
						})
					}
				}
				objs = append(objs, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: ns}})
			}
			logs := func(context.Context, *rest.Config, string, string, string, *corev1.PodLogOptions) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("ready\n")), nil
			}
			h = startHarness(Options{PodLogs: logs, Namespaces: allowed}, objs...)
			var out GetLogsOutput
			res := h.call("get_logs", map[string]any{"name": "web", "namespace": "team-a"}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Pods).NotTo(BeEmpty(), "pods in the traced scope are still read")
			Expect(out.Meta.Truncated).To(BeTrue())
			Expect(out.Meta.Warnings).To(ContainElement(SatisfyAll(ContainSubstring("not traced"), ContainSubstring("Narrow by cluster or component"))))
		})
	})

	Context("get_platform_info", func() {
		It("lists enabled addons and says where served data comes from", func() {
			h = startHarness(Options{}, clusterFixtures()...)
			var out GetPlatformInfoOutput
			res := h.call("get_platform_info", nil, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.ServedData).To(Equal("builtin"))
			names := map[string]string{}
			for _, a := range out.Addons {
				names[a.Name] = a.Version
			}
			Expect(names).To(HaveKeyWithValue("velaux", "v1.11.0"))
			Expect(names).To(HaveKeyWithValue("fluxcd", "2.4.0"))
		})
	})

	Context("describe_definition", func() {
		It("returns a usage example generated from the schema", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out DescribeDefinitionOutput
			res := h.call("describe_definition", map[string]any{"name": "webservice", "type": "component"}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Example).NotTo(BeNil())
			Expect(out.Example.Placeholders).To(Equal([]string{"image"}), "port has a default, so only image needs a value")
			var app map[string]any
			Expect(yaml.Unmarshal([]byte(out.Example.YAML), &app)).To(Succeed())
			Expect(app["kind"]).To(Equal("Application"))
			comp := app["spec"].(map[string]any)["components"].([]any)[0].(map[string]any)
			Expect(comp["type"]).To(Equal("webservice"))
			Expect(comp["properties"]).To(Equal(map[string]any{"image": "<image>"}))
		})

		It("shows a trait as the fragment that goes under a component", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out DescribeDefinitionOutput
			h.call("describe_definition", map[string]any{"name": "scaler", "type": "trait"}, &out)
			Expect(out.Example).NotTo(BeNil())
			Expect(out.Example.YAML).To(HavePrefix("# goes under a component"))
			Expect(out.Example.YAML).To(ContainSubstring("type: scaler"))
			Expect(out.Example.Placeholders).To(BeEmpty())
		})

		It("flags a required map with no named fields as a placeholder", func() {
			sch := openapi3.NewObjectSchema().WithProperty("http", openapi3.NewObjectSchema().WithAdditionalProperties(openapi3.NewIntegerSchema()))
			sch.Required = []string{"http"}
			ex, err := definitionExample(DefinitionTypeTrait, "gateway", sch)
			Expect(err).NotTo(HaveOccurred())
			Expect(ex.Placeholders).To(Equal([]string{"http"}))
		})
	})

	Context("definitionExample constraints", func() {
		It("emits minItems elements with indexed placeholders", func() {
			sch := openapi3.NewObjectSchema().WithProperty("hosts", openapi3.NewArraySchema().WithItems(openapi3.NewStringSchema()).WithMinItems(2))
			sch.Required = []string{"hosts"}
			ex, err := definitionExample(DefinitionTypeTrait, "t", sch)
			Expect(err).NotTo(HaveOccurred())
			Expect(ex.Placeholders).To(Equal([]string{"hosts[0]", "hosts[1]"}))
		})

		It("fills the first branch's required fields of a struct disjunction", func() {
			sch := openapi3.NewObjectSchema().
				WithProperty("type", openapi3.NewSchema()).
				WithProperty("claimName", openapi3.NewStringSchema()).
				WithProperty("medium", openapi3.NewStringSchema())
			sch.OneOf = openapi3.SchemaRefs{
				openapi3.NewSchemaRef("", &openapi3.Schema{Required: []string{"type", "claimName"}}),
				openapi3.NewSchemaRef("", &openapi3.Schema{Required: []string{"type", "medium"}}),
			}
			ex, err := definitionExample(DefinitionTypeTrait, "volume", sch)
			Expect(err).NotTo(HaveOccurred())
			Expect(ex.Placeholders).To(ConsistOf("type", "claimName"))
		})

		It("picks numbers inside exclusive and max-only bounds", func() {
			zero, three, ten := float64(0), float64(3), float64(10)
			cases := []struct {
				sch  *openapi3.Schema
				want any
			}{
				{&openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}, Min: &three, ExclusiveMin: true}, int64(4)},
				{&openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}, Max: &zero}, int64(0)},
				{&openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}, Max: &zero, ExclusiveMax: true}, int64(-1)},
				{&openapi3.Schema{Type: &openapi3.Types{openapi3.TypeNumber}, Min: &zero, ExclusiveMin: true, Max: &three}, 1.5},
				{&openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}, Min: &three, Max: &ten}, int64(3)},
			}
			for _, c := range cases {
				var ph []string
				Expect(exampleValue(c.sch, "n", &ph, 0)).To(Equal(c.want))
			}
		})

		It("adds optional properties up to minProperties", func() {
			sch := openapi3.NewObjectSchema().
				WithProperty("a", openapi3.NewStringSchema()).
				WithProperty("b", openapi3.NewStringSchema()).
				WithProperty("c", openapi3.NewStringSchema())
			sch.MinProps = 2
			var ph []string
			Expect(exampleObject(sch, "", &ph, 0)).To(HaveLen(2))
			Expect(ph).To(Equal([]string{"a", "b"}))
		})
	})

	Context("definitionExample", func() {
		It("fills nested required fields, arrays, enums and numeric minimums", func() {
			min := float64(3)
			item := openapi3.NewObjectSchema().WithProperty("name", openapi3.NewStringSchema())
			item.Required = []string{"name"}
			sch := openapi3.NewObjectSchema().
				WithProperty("env", openapi3.NewArraySchema().WithItems(item)).
				WithProperty("tier", openapi3.NewStringSchema().WithEnum("gold", "silver")).
				WithProperty("replicas", &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}, Min: &min}).
				WithProperty("debug", openapi3.NewBoolSchema()).
				WithProperty("optional", openapi3.NewStringSchema())
			sch.Required = []string{"env", "tier", "replicas", "debug"}
			ex, err := definitionExample(DefinitionTypePolicy, "spread", sch)
			Expect(err).NotTo(HaveOccurred())
			Expect(ex.Placeholders).To(ConsistOf("env[0].name", "tier", "replicas", "debug"))
			var doc map[string]any
			Expect(yaml.Unmarshal([]byte(ex.YAML), &doc)).To(Succeed())
			props := doc["policies"].([]any)[0].(map[string]any)["properties"].(map[string]any)
			Expect(props).To(Equal(map[string]any{
				"env":      []any{map[string]any{"name": "<env[0].name>"}},
				"tier":     "gold",
				"replicas": float64(3),
				"debug":    false,
			}))
		})
	})
})
