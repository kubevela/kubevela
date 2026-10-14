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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// These specs pin the security properties the package documents: the
// namespace allowlist holds on every read, Secret data and credential
// shaped content never leave, and outputs are bounded.
var _ = Describe("security properties", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	Context("namespace allowlist", func() {
		It("applies to every application, workflow, resource and log read without revealing existence", func() {
			h = startHarness(Options{Namespaces: []string{"team-b"}}, managedFixtures()...)
			for _, tool := range []string{"get_application", "get_workflow", "get_managed_resource", "get_logs", "explain_application"} {
				res := h.call(tool, map[string]any{"name": "web", "namespace": "team-a"}, nil)
				Expect(errorText(res)).To(ContainSubstring("outside this server's allowed namespaces"), tool)
				Expect(errorText(res)).NotTo(ContainSubstring("not found"), tool)
			}
			for _, tool := range []string{"get_application", "get_workflow"} {
				res := h.call(tool, map[string]any{"name": "ghost", "namespace": "team-a"}, nil)
				Expect(errorText(res)).NotTo(ContainSubstring("not found"), tool+" must not distinguish missing from forbidden")
			}
		})

		It("applies to definition lookups outside the system namespace", func() {
			h = startHarness(Options{Namespaces: []string{"team-b"}}, platformFixtures()...)
			for _, tool := range []string{"describe_definition", "explain_definition"} {
				res := h.call(tool, map[string]any{"name": "team-policy", "namespace": "team-a"}, nil)
				Expect(errorText(res)).To(ContainSubstring("outside this server's allowed namespaces"), tool)
				var out map[string]any
				res = h.call(tool, map[string]any{"name": "webservice", "namespace": "vela-system"}, &out)
				Expect(res.IsError).To(BeFalse(), tool+" system namespace stays readable")
			}
			res := h.call("list_definitions", map[string]any{"namespace": "team-a"}, nil)
			Expect(errorText(res)).To(ContainSubstring("outside this server's allowed namespaces"))
		})
	})

	Context("namespace allowlist on managed objects", func() {
		It("withholds resources an allowed Application manages in other namespaces and never reads them", func() {
			objs := managedFixtures()
			for _, o := range objs {
				if rt, ok := o.(*v1beta1.ResourceTracker); ok {
					rt.Spec.ManagedResources = append(rt.Spec.ManagedResources,
						v1beta1.ManagedResource{
							ClusterObjectReference: oamcommon.ClusterObjectReference{ObjectReference: corev1.ObjectReference{APIVersion: "v1", Kind: "ConfigMap", Namespace: "kube-system", Name: "cluster-info"}},
							OAMObjectReference:     oamcommon.OAMObjectReference{Component: "web"},
						},
						v1beta1.ManagedResource{
							ClusterObjectReference: oamcommon.ClusterObjectReference{ObjectReference: corev1.ObjectReference{APIVersion: "v1", Kind: "Namespace", Name: "team-b"}},
							OAMObjectReference:     oamcommon.OAMObjectReference{Component: "web"},
						})
				}
			}
			objs = append(objs, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cluster-info", Namespace: "kube-system"}, Data: map[string]string{"ca": "very-private"}}, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team-b"}})

			// Unrestricted: everything the tracker records is visible.
			h = startHarness(Options{}, objs...)
			var all GetManagedResourceOutput
			h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a"}, &all)
			Expect(all.Resources).To(HaveLen(4))
			h.close()

			// Restricted to the Application's namespace: foreign and
			// cluster-scoped objects are withheld and counted.
			h = startHarness(Options{Namespaces: []string{"team-a"}}, objs...)
			var out GetManagedResourceOutput
			res := h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a"}, &out)
			Expect(res.IsError).To(BeFalse())
			Expect(out.Resources).To(HaveLen(2))
			for _, r := range out.Resources {
				Expect(r.Namespace).To(Equal("team-a"))
			}
			Expect(out.Meta.Warnings).To(ContainElement("2 managed resources are outside this server's allowed namespaces and were withheld"))
			Expect(toJSON(out)).NotTo(ContainSubstring("very-private"))

			res = h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a", "kind": "ConfigMap", "resourceName": "cluster-info"}, nil)
			Expect(errorText(res)).To(ContainSubstring("not managed by application"), "a withheld object reads as unmanaged, not as forbidden")

			var expl ExplainApplicationOutput
			h.call("explain_application", map[string]any{"name": "web", "namespace": "team-a"}, &expl)
			Expect(expl.Unknowns).To(ContainElement(ContainSubstring("2 managed resources are outside")))
			Expect(toJSON(expl)).NotTo(ContainSubstring("kube-system"))
		})
	})

	Context("remote cluster identity", func() {
		It("warns when a result includes a cluster the gateway does not forward identity to", func() {
			objs := managedFixtures()
			for _, o := range objs {
				if rt, ok := o.(*v1beta1.ResourceTracker); ok {
					rt.Spec.ManagedResources[0].Cluster = "prod-east"
				}
			}
			gateway := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "kubevela-cluster-gateway", Namespace: "vela-system"},
				Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name: "gateway", Image: "oamdev/cluster-gateway:v1.9.0",
					Args: []string{"--feature-gates=APIPriorityAndFairness=false,ClientIdentityPenetration=false"},
				}}}}},
			}
			h = startHarness(Options{}, append(objs, gateway)...)
			var out GetManagedResourceOutput
			h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a"}, &out)
			Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("not your identity")))
			var info GetPlatformInfoOutput
			h.call("get_platform_info", nil, &info)
			Expect(info.Gateway.Found).To(BeTrue())
			Expect(*info.Gateway.IdentityPenetration).To(BeFalse())
			h.close()

			gateway.Spec.Template.Spec.Containers[0].Args = []string{"--feature-gates=ClientIdentityPenetration=true"}
			h = startHarness(Options{}, append(managedFixtures(), gateway)...)
			var local GetManagedResourceOutput
			h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a"}, &local)
			Expect(local.Meta.Warnings).NotTo(ContainElement(ContainSubstring("identity")))
		})
	})

	Context("sensitive content", func() {
		It("covers the credential shapes common in logs and objects", func() {
			cases := map[string]string{
				"MYSQL_ROOT_PASSWORD=abc":                               "MYSQL_ROOT_PASSWORD=" + redactedValue,
				"client_secret=abc&scope=x":                             "client_secret=" + redactedValue + "&scope=x",
				`{"password": "x", "user": "u"}`:                        `{"password": "` + redactedValue + `", "user": "u"}`,
				"GET /cb?access_token=abc&state=1":                      "GET /cb?access_token=" + redactedValue + "&state=1",
				"dsn postgres://app:hunter2@db:5432/x":                  "dsn postgres://app:" + redactedValue + "@db:5432/x",
				"https://hooks.slack.com/services/T000/B000/XXXX":       "https://hooks.slack.com/services/" + redactedValue,
				"token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123 used":         "token ghp_" + redactedValue + " used",
				"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl": "jwt " + redactedValue,
				"glpat-ABCDEFGHIJKLMNOPQRSTUV":                          "glpat-" + redactedValue,
				"xoxb-123456789012-abcdef":                              "xoxb-" + redactedValue,
				"image: registry.example.com/team/app:1.2.3":            "image: registry.example.com/team/app:1.2.3",
				"tokenCount=5": "tokenCount=" + redactedValue,
			}
			for in, want := range cases {
				Expect(redactLine(in)).To(Equal(want), in)
			}
		})

		It("strips control, bidi and tag characters from text", func() {
			Expect(redactLine("ok\x1b[31m red\x1b[0m \u202eevil\u202c \U000E0041tag\x00")).To(Equal("ok red evil tag"))
			Expect(text(strings.Repeat("a", maxTextLen+10))).To(HaveSuffix(truncMark))
		})

		It("redacts Application annotations and returns ConfigMap keys only", func() {
			objs := managedFixtures()
			for _, o := range objs {
				if app, ok := o.(*v1beta1.Application); ok {
					app.Annotations = map[string]string{"kubectl.kubernetes.io/last-applied-configuration": `{"password":"from-annotation"}`, "owner": "team-a"}
				}
				if rt, ok := o.(*v1beta1.ResourceTracker); ok {
					rt.Spec.ManagedResources = append(rt.Spec.ManagedResources, v1beta1.ManagedResource{
						ClusterObjectReference: oamcommon.ClusterObjectReference{ObjectReference: corev1.ObjectReference{APIVersion: "v1", Kind: "ConfigMap", Namespace: "team-a", Name: "web-config"}},
						OAMObjectReference:     oamcommon.OAMObjectReference{Component: "web"},
					})
				}
			}
			objs = append(objs, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "web-config", Namespace: "team-a"}, Data: map[string]string{"app.yaml": "db: postgres://u:p@db", "mode": "prod"}})
			h = startHarness(Options{}, objs...)
			var app GetApplicationOutput
			h.call("get_application", map[string]any{"name": "web", "namespace": "team-a"}, &app)
			Expect(app.Annotations).To(Equal(map[string]string{"owner": "team-a"}))
			var cm GetManagedResourceOutput
			h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a", "kind": "ConfigMap"}, &cm)
			Expect(cm.Resources).To(HaveLen(1))
			payload := toJSON(cm)
			Expect(payload).NotTo(ContainSubstring("postgres"))
			Expect(payload).NotTo(ContainSubstring("prod"))
			Expect(cm.Resources[0].Object["dataKeys"]).To(ConsistOf("app.yaml", "mode"))
			Expect(cm.Resources[0].ObjectNote).To(ContainSubstring("only their keys"))
		})

		It("strips embedded-object annotations and env secrets from live objects", func() {
			objs := managedFixtures()
			for _, o := range objs {
				if d, ok := o.(*appsv1.Deployment); ok && d.Name == "web" {
					d.Annotations = map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"password":"from-last-applied"}}`,
						"team": "a",
					}
					d.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
						{Name: "DB_PASSWORD", Value: "hunter2"},
						{Name: "LOG_LEVEL", Value: "debug"},
						{Name: "URL", Value: "https://x?api_key=abc123"},
					}
				}
			}
			h = startHarness(Options{}, objs...)
			var out GetManagedResourceOutput
			h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a", "kind": "Deployment", "resourceName": "web"}, &out)
			payload := toJSON(out)
			Expect(payload).NotTo(ContainSubstring("from-last-applied"))
			Expect(payload).NotTo(ContainSubstring("last-applied-configuration"))
			Expect(payload).NotTo(ContainSubstring("hunter2"))
			Expect(payload).NotTo(ContainSubstring("abc123"))
			Expect(payload).To(ContainSubstring(`"team":"a"`))
			Expect(payload).To(ContainSubstring("debug"))
		})

		It("redacts credential-shaped text inside error messages", func() {
			h = startHarness(Options{}, managedFixtures()...)
			res := h.call("get_logs", map[string]any{"name": "web", "namespace": "team-a", "pod": "token=verysecret"}, nil)
			text := errorText(res)
			Expect(text).NotTo(ContainSubstring("verysecret"))
			Expect(text).To(ContainSubstring("not managed by application"))
		})

		It("redacts env pairs and string leaves in maps", func() {
			in := map[string]any{
				"env":   []any{map[string]any{"name": "API_TOKEN", "value": "t"}, map[string]any{"name": "PORT", "value": "80"}},
				"notes": "contact admin, Authorization: Bearer abc",
			}
			out := redactMap(in).(map[string]any)
			env := out["env"].([]any)
			Expect(env[0]).To(HaveKeyWithValue("value", redactedValue))
			Expect(env[1]).To(HaveKeyWithValue("value", "80"))
			Expect(out["notes"]).To(Equal("contact admin, Authorization: Bearer " + redactedValue))
		})
	})

	Context("bounds", func() {
		It("caps application and definition lists and says so", func() {
			old := maxListItems
			maxListItems = 1
			DeferCleanup(func() { maxListItems = old })
			h = startHarness(Options{}, appFixtures()...)
			var apps ListApplicationsOutput
			h.call("list_applications", nil, &apps)
			Expect(apps.Applications).To(HaveLen(1))
			Expect(apps.Meta.Truncated).To(BeTrue())
			Expect(apps.Meta.Warnings).To(ContainElement(ContainSubstring("more than 1 applications")))
			var defs ListDefinitionsOutput
			h.call("list_definitions", nil, &defs)
			Expect(defs.Definitions).To(HaveLen(1))
			Expect(defs.Meta.Truncated).To(BeTrue())
		})

		It("withholds a live object over the size limit", func() {
			old := maxObjectBytes
			maxObjectBytes = 64
			DeferCleanup(func() { maxObjectBytes = old })
			h = startHarness(Options{}, managedFixtures()...)
			var out GetManagedResourceOutput
			h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a", "kind": "Deployment", "resourceName": "web"}, &out)
			Expect(out.Resources).To(HaveLen(1))
			Expect(out.Resources[0].Object).To(BeEmpty())
			Expect(out.Resources[0].ObjectNote).To(ContainSubstring("over the 64 byte limit"))
			Expect(out.Meta.Truncated).To(BeTrue())
			Expect(out.Resources[0].Tree).NotTo(BeEmpty(), "the tree is still returned")
		})

		It("clamps tail lines to the maximum", func() {
			opts := logOptions(GetLogsInput{TailLines: 1 << 20, SinceSeconds: -5})
			Expect(*opts.TailLines).To(Equal(maxTailLines))
			Expect(opts.SinceSeconds).To(BeNil())
		})
	})

	It("registers no write tools whatever the flags say until they are implemented", func() {
		h = startHarness(Options{AllowWrite: true, AllowResume: true}, platformFixtures()...)
		for _, t := range h.srv.Tools() {
			Expect(t.Class).To(Equal(ToolClassRead), t.Name)
			Expect(strings.HasPrefix(t.Name, "apply_") || strings.HasPrefix(t.Name, "resume_")).To(BeFalse(), t.Name)
		}
	})
})
