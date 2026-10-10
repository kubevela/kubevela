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

package helm_test

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// ============================================================================
// Helmchart Auth -- Secret-referenced authentication tests for GWCP-98771.
// Every scenario here assumes the auth-test registries (deployed in BeforeSuite
// from test/e2e-framework/testdata/auth/manifests/) are running and the test chart
// has been pushed to each.
// ============================================================================
var _ = Describe("Helmchart Auth", Label("core-helm", "helm-auth"), func() {

	BeforeEach(func() {
		if os.Getenv("KUBEVELA_E2E_AUTH") != "1" {
			Skip("auth-test registries not deployed (set KUBEVELA_E2E_AUTH=1 to enable)")
		}
	})

	const (
		chartMuseumURL       = "https://chartmuseum.kubevela-auth-test.svc.cluster.local:8080"
		chartMuseumBearerURL = "https://chartmuseum-bearer.kubevela-auth-test.svc.cluster.local"
		ociRegistrySource    = "oci://zot.kubevela-auth-test.svc.cluster.local:5000/charts/podinfo"
		ociRegistryHost      = "zot.kubevela-auth-test.svc.cluster.local:5000"
		authTestUser         = "test-user"
		authTestPass         = "test-pass"
		authBearerToken      = "kubevela-auth-test-token"
	)

	// createSecretInline creates a Secret of the given type with stringData in the
	// helmTestContext's namespace. Returns an error if creation fails; tests should
	// Expect Succeed().
	createSecretInline := func(h *helmTestContext, name string, secretType corev1.SecretType, stringData map[string]string) error {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: h.Namespace},
			Type:       secretType,
			StringData: stringData,
		}
		return k8sClient.Create(h.Ctx, s)
	}

	createSecretInNamespace := func(h *helmTestContext, name, ns string, secretType corev1.SecretType, stringData map[string]string) error {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Type:       secretType,
			StringData: stringData,
		}
		return k8sClient.Create(h.Ctx, s)
	}

	// chartWithRepoAuth builds a chart props map with a repo-based source and an auth secretRef.
	chartWithRepoAuth := func(repoURL, secretName string, secretNs ...string) map[string]interface{} {
		secretRefBlock := map[string]interface{}{"name": secretName}
		if len(secretNs) > 0 && secretNs[0] != "" {
			secretRefBlock["namespace"] = secretNs[0]
		}
		return map[string]interface{}{
			"source":  "podinfo",
			"repoURL": repoURL,
			"version": "1.0.0",
			"auth":    map[string]interface{}{"secretRef": secretRefBlock},
		}
	}

	// chartWithURLAuth builds a chart props map with a direct .tgz URL and auth.
	chartWithURLAuth := func(url, secretName string) map[string]interface{} {
		return map[string]interface{}{
			"source":  url,
			"version": "1.0.0",
			"auth":    map[string]interface{}{"secretRef": map[string]interface{}{"name": secretName}},
		}
	}

	// chartWithOCIAuth builds a chart props map with an oci:// source and auth.
	chartWithOCIAuth := func(secretName string) map[string]interface{} {
		return map[string]interface{}{
			"source":  ociRegistrySource,
			"version": "1.0.0",
			"auth":    map[string]interface{}{"secretRef": map[string]interface{}{"name": secretName}},
		}
	}

	// buildPodinfoComponentForAuth is the auth-block analog of buildPodinfoComponent.
	// It accepts a chart props map (assembled by chartWithRepoAuth/chartWithURLAuth/
	// chartWithOCIAuth) and embeds it under "chart".
	buildPodinfoComponentForAuth := func(h *helmTestContext, releaseName string, chartProps map[string]interface{}) common2.ApplicationComponent {
		merged := map[string]interface{}{
			"chart": chartProps,
			"release": map[string]interface{}{
				"name":      releaseName,
				"namespace": h.Namespace,
			},
			"options": map[string]interface{}{
				"createNamespace": true,
				"skipTests":       true,
			},
		}
		raw, err := json.Marshal(merged)
		Expect(err).ShouldNot(HaveOccurred())
		return common2.ApplicationComponent{
			Name:       "podinfo",
			Type:       "helmchart",
			Properties: &runtime.RawExtension{Raw: raw},
		}
	}

	deployAuthAppSuccess := func(h *helmTestContext, prefix, releaseName string, chartProps map[string]interface{}) {
		comp := buildPodinfoComponentForAuth(h, releaseName, chartProps)
		h.App = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prefix + "-" + helmTestSuffix(),
				Namespace: h.AppNamespace,
			},
			Spec: v1beta1.ApplicationSpec{Components: []common2.ApplicationComponent{comp}},
		}
		Expect(k8sClient.Create(h.Ctx, h.App)).Should(Succeed())
		h.AppKey = client.ObjectKeyFromObject(h.App)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
			g.Expect(h.App.Status.Phase).Should(Equal(common2.ApplicationRunning))
		}, 180*time.Second, 5*time.Second).Should(Succeed())
	}

	// deployAuthAppExpectFailure accepts either of two failure paths:
	//   1) The validating webhook denies the Create() with the auth error in
	//      its message (the webhook does a dry-run render that exercises the
	//      resolver; resolver errors surface at admission time).
	//   2) Create() succeeds and the workflow later reaches Phase="failed"
	//      with the error in a step message.
	// Both are valid: the resolver runs in both contexts and surfaces the
	// same verbatim, RFC-grounded message.
	deployAuthAppExpectFailure := func(h *helmTestContext, prefix, releaseName string, chartProps map[string]interface{}, errSubstring string) {
		comp := buildPodinfoComponentForAuth(h, releaseName, chartProps)
		h.App = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prefix + "-" + helmTestSuffix(),
				Namespace: h.AppNamespace,
			},
			Spec: v1beta1.ApplicationSpec{Components: []common2.ApplicationComponent{comp}},
		}
		createErr := k8sClient.Create(h.Ctx, h.App)
		if createErr != nil {
			Expect(createErr.Error()).To(ContainSubstring(errSubstring),
				"webhook denial did not contain expected substring %q: %v", errSubstring, createErr)
			return
		}
		h.AppKey = client.ObjectKeyFromObject(h.App)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
			g.Expect(h.App.Status.Workflow).ToNot(BeNil())
			g.Expect(string(h.App.Status.Workflow.Phase)).To(Equal("failed"))
			var found bool
			for _, step := range h.App.Status.Workflow.Steps {
				if strings.Contains(step.Message, errSubstring) {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(),
				"no workflow step contained %q; workflow=%+v", errSubstring, h.App.Status.Workflow)
		}, 180*time.Second, 5*time.Second).Should(Succeed())
	}

	// ----------- Positive paths ------------

	// OCI plain-HTTP via Opaque Secret (insecurePlainHTTP opts in).
	// The dispatcher branches for kubernetes.io/basic-auth and
	// kubernetes.io/dockerconfigjson are exhaustively covered by
	// auth_test.go unit tests; both produce the same HTTPOption shape
	// the resolver returns for the Opaque (basic) path here, so an
	// extra OCI e2e against typed Secrets is duplicative coverage.
	Context("OCI / Opaque (basic) with insecurePlainHTTP", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("pulls and installs the chart over plain HTTP", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"username":          authTestUser,
				"password":          authTestPass,
				"insecurePlainHTTP": "true",
			})).To(Succeed())
			deployAuthAppSuccess(h, "auth-oci-opaque", "podinfo", chartWithOCIAuth("creds"))
		})
	})

	Context("HTTPS Helm-repo / kubernetes.io/basic-auth typed Secret", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("pulls and installs the chart", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeBasicAuth, map[string]string{
				"username": authTestUser, "password": authTestPass,
			})).To(Succeed())
			deployAuthAppSuccess(h, "auth-http-basic-typed", "podinfo", chartWithRepoAuth(chartMuseumURL, "creds"))
		})
	})

	Context("HTTPS Helm-repo / Opaque (basic)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("pulls and installs the chart", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"username": authTestUser, "password": authTestPass,
			})).To(Succeed())
			deployAuthAppSuccess(h, "auth-http-basic-opaque", "podinfo", chartWithRepoAuth(chartMuseumURL, "creds"))
		})
	})

	Context("HTTPS Helm-repo / Bearer token via nginx-fronted ChartMuseum", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("pulls and installs the chart with Authorization: Bearer", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"token": authBearerToken,
			})).To(Succeed())
			deployAuthAppSuccess(h, "auth-http-bearer", "podinfo", chartWithRepoAuth(chartMuseumBearerURL, "creds"))
		})
	})

	Context("HTTPS Helm-repo / Opaque (basic + insecureSkipTLS)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("pulls and installs the chart with TLS verification disabled", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"username": authTestUser, "password": authTestPass, "insecureSkipTLS": "true",
			})).To(Succeed())
			deployAuthAppSuccess(h, "auth-http-skip-tls", "podinfo", chartWithRepoAuth(chartMuseumURL, "creds"))
		})
	})

	Context("URL direct .tgz / Opaque (basic)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("pulls and installs the chart from a direct .tgz URL", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"username": authTestUser, "password": authTestPass,
			})).To(Succeed())
			url := chartMuseumURL + "/charts/podinfo-1.0.0.tgz"
			deployAuthAppSuccess(h, "auth-url-direct", "podinfo", chartWithURLAuth(url, "creds"))
		})
	})

	Context("secretRef.namespace omitted (defaults to release namespace)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("resolves the Secret from the release namespace", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"username": authTestUser, "password": authTestPass,
			})).To(Succeed())
			// No secretRef.namespace; resolver defaults to release namespace.
			deployAuthAppSuccess(h, "auth-ns-omitted", "podinfo", chartWithRepoAuth(chartMuseumURL, "creds"))
		})
	})

	Context("secretRef.namespace explicitly set to Application namespace", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
			h.AppNamespace = randomNamespaceName("helm-auth-app")
			appNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: h.AppNamespace}}
			Expect(k8sClient.Create(h.Ctx, appNamespace)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(h.Ctx, appNamespace,
					client.PropagationPolicy(metav1.DeletePropagationForeground)))).To(Succeed())
			})
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("resolves the Secret when secretRef.namespace == Application namespace", func() {
			Expect(h.AppNamespace).NotTo(Equal(h.Namespace))
			Expect(createSecretInNamespace(h, "creds", h.AppNamespace, corev1.SecretTypeOpaque, map[string]string{
				"username": authTestUser, "password": authTestPass,
			})).To(Succeed())
			deployAuthAppSuccess(h, "auth-ns-app", "podinfo", chartWithRepoAuth(chartMuseumURL, "creds", h.AppNamespace))
		})
	})

	// ----------- Negative paths ------------

	Context("Missing Secret", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("fails with the not-found error", func() {
			deployAuthAppExpectFailure(h, "auth-missing", "podinfo",
				chartWithRepoAuth(chartMuseumURL, "nonexistent-secret"),
				`not found: it MUST exist in the release namespace`)
		})
	})

	Context("Wrong Secret type", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("rejects an unsupported Secret type", func() {
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "dummy-sa", Namespace: h.Namespace},
			}
			Expect(k8sClient.Create(h.Ctx, sa)).To(Succeed())
			s := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "creds", Namespace: h.Namespace,
					Annotations: map[string]string{"kubernetes.io/service-account.name": "dummy-sa"},
				},
				Type: corev1.SecretTypeServiceAccountToken,
			}
			Expect(k8sClient.Create(h.Ctx, s)).To(Succeed())
			deployAuthAppExpectFailure(h, "auth-wrong-type", "podinfo",
				chartWithRepoAuth(chartMuseumURL, "creds"),
				`MUST be one of kubernetes.io/basic-auth, kubernetes.io/dockerconfigjson, kubernetes.io/tls, or Opaque`)
		})
	})

	Context("Cross-namespace Secret rejected", Ordered, func() {
		h := newHelmTestContext()
		otherNS := ""
		BeforeAll(func() {
			h.CreateNamespace()
			otherNS = "auth-cross-ns-other-" + helmTestSuffix()
			Expect(k8sClient.Create(h.Ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: otherNS}})).To(Succeed())
			Expect(createSecretInNamespace(h, "creds", otherNS, corev1.SecretTypeOpaque, map[string]string{
				"username": authTestUser, "password": authTestPass,
			})).To(Succeed())
		})
		AfterAll(func() {
			_ = k8sClient.Delete(h.Ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: otherNS}})
			h.Cleanup()
		})

		It("rejects a Secret reference outside release-ns and app-ns", func() {
			deployAuthAppExpectFailure(h, "auth-cross-ns", "podinfo",
				chartWithRepoAuth(chartMuseumURL, "creds", otherNS),
				`namespace MUST equal the release namespace`)
		})
	})

	Context("Opaque mixed credentials (basic + token)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("rejects Opaque Secret with both basic-auth keys and a token", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"username": authTestUser, "password": authTestPass, "token": "some.bearer.token",
			})).To(Succeed())
			deployAuthAppExpectFailure(h, "auth-mixed", "podinfo",
				chartWithRepoAuth(chartMuseumURL, "creds"),
				`at most one credential method MUST be configured per Secret (RFC 6750 §2)`)
		})
	})

	Context("Bearer token over plain http://", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("rejects Bearer over plain HTTP per RFC 6750 §2", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"token": authBearerToken,
			})).To(Succeed())
			deployAuthAppExpectFailure(h, "auth-bearer-http", "podinfo",
				chartWithRepoAuth("http://chartmuseum.kubevela-auth-test.svc.cluster.local:8080", "creds"),
				`bearer tokens MUST be sent only over HTTPS or OCI (RFC 6750 §2)`)
		})
	})

	Context("Bearer token + insecureSkipTLS", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("rejects Bearer combined with TLS verification disabled per RFC 6750 §2", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"token": authBearerToken, "insecureSkipTLS": "true",
			})).To(Succeed())
			deployAuthAppExpectFailure(h, "auth-bearer-insecure", "podinfo",
				chartWithRepoAuth(chartMuseumURL, "creds"),
				`bearer tokens MUST NOT be sent with TLS verification disabled (RFC 6750 §2)`)
		})
	})

	Context("User-supplied Bearer on OCI source", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("rejects user-supplied Bearer on OCI sources", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"token": authBearerToken,
			})).To(Succeed())
			deployAuthAppExpectFailure(h, "auth-bearer-oci", "podinfo",
				chartWithOCIAuth("creds"),
				`user-supplied bearer tokens MUST NOT be used with OCI sources`)
		})
	})

	Context("Username containing ':'", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("rejects per RFC 7617 §2", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"username": "user:colon", "password": authTestPass,
			})).To(Succeed())
			deployAuthAppExpectFailure(h, "auth-colon", "podinfo",
				chartWithRepoAuth(chartMuseumURL, "creds"),
				`username MUST NOT contain ':' (RFC 7617 §2)`)
		})
	})

	Context("Bearer token charset violation", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
		})
		AfterAll(func() {
			h.Cleanup()
		})

		It("rejects per RFC 6750 §2.1", func() {
			Expect(createSecretInline(h, "creds", corev1.SecretTypeOpaque, map[string]string{
				"token": "bad token with spaces",
			})).To(Succeed())
			deployAuthAppExpectFailure(h, "auth-token-charset", "podinfo",
				chartWithRepoAuth(chartMuseumBearerURL, "creds"),
				`b64token charset (RFC 6750 §2.1)`)
		})
	})
})
