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

// Package controllers_test contains E2E tests for Application-scoped PolicyDefinitions.
//
// PREREQUISITE: The KubeVela controller must be deployed with the following feature gates enabled:
//
//	--feature-gates=EnableApplicationScopedPolicies=true
//	--feature-gates=EnableGlobalPolicies=true   (for global policy tests only)
//
// When running via Helm (e.g. make e2e-test-local):
//
//	helm upgrade --install kubevela ./charts/vela-core ... \
//	  --set featureGates.enableApplicationScopedPolicies=true \
//	  --set featureGates.enableGlobalPolicies=true
package controllers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	v1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// policyTransformWaitTimeout is the timeout used by all policy transform E2E tests.
const policyTransformWaitTimeout = 60 * time.Second

var _ = Describe("Application Policy Transform Tests", func() {
	ctx := context.Background()
	var namespace string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespace = randomNamespaceName("policy-transforms-e2e")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, 10*time.Second, time.Second).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("Cleaning up namespace")
		Expect(k8sClient.Delete(ctx, &ns)).Should(Succeed())
	})

	It("Test explicit Application-scoped policy adds labels to Application", func() {
		By("Creating an Application-scoped PolicyDefinition that adds labels based on parameters")
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-labels-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	team: string
	environment: string
}
output: {
	labels: {
		"platform.io/team": parameter.team
		"platform.io/environment": parameter.environment
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())

		By("Waiting for PolicyDefinition to be accepted by the cluster")
		Eventually(func(g Gomega) {
			pd := &v1beta1.PolicyDefinition{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "add-labels-policy"}, pd)).Should(Succeed())
		}, 15*time.Second, time.Second).Should(Succeed())

		By("Creating an Application that explicitly references the policy")
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "labeled-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "hello-world",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image":"crccheck/hello-world"}`),
						},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "add-team-labels",
						Type: "add-labels-policy",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"team":"platform-team","environment":"production"}`),
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		appKey := client.ObjectKeyFromObject(app)

		By("Verifying policy transform labels were added to the Application")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Labels).Should(HaveKeyWithValue("platform.io/team", "platform-team"))
			g.Expect(app.Labels).Should(HaveKeyWithValue("platform.io/environment", "production"))
		}, policyTransformWaitTimeout, 3*time.Second).Should(Succeed())
	})

	It("Test global Application-scoped policy automatically applies to all Applications in namespace", func() {
		By("Creating a global PolicyDefinition in the test namespace")
		globalPolicyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "global-compliance-labels",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope:    v1beta1.ApplicationScope,
				Global:   true,
				Priority: 100,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}
output: {
	labels: {
		"compliance.io/managed-by": "kubevela"
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, globalPolicyDef)).Should(Succeed())

		By("Creating an Application that does NOT explicitly reference the policy")
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "auto-labeled-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "hello-world",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image":"crccheck/hello-world"}`),
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		appKey := client.ObjectKeyFromObject(app)

		By("Verifying the global policy label was automatically applied without explicit reference")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Labels).Should(HaveKeyWithValue("compliance.io/managed-by", "kubevela"))
		}, policyTransformWaitTimeout, 3*time.Second).Should(Succeed())
	})

	It("Test policy version context fields are accessible in CUE templates", func() {
		By("Creating a PolicyDefinition that writes version metadata into Application labels")
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "version-context-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
output: {
	labels: {
		"audit.io/policy-name": context.policyType
		"audit.io/policy-instance": context.policyName
		"audit.io/policy-revision": "\(context.policyRevision)"
		if context.policyRevisionName != "" {
			"audit.io/policy-revision-name": context.policyRevisionName
		}
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		pdKey := client.ObjectKeyFromObject(policyDef)

		By("Waiting for PolicyDefinition LatestRevision to be populated by the controller")
		var latestRevisionName string
		Eventually(func(g Gomega) {
			pd := &v1beta1.PolicyDefinition{}
			g.Expect(k8sClient.Get(ctx, pdKey, pd)).Should(Succeed())
			g.Expect(pd.Status.LatestRevision).ShouldNot(BeNil())
			latestRevisionName = pd.Status.LatestRevision.Name
		}, 30*time.Second, time.Second).Should(Succeed())

		By("Creating an Application that uses the version-aware policy")
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "version-context-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "hello-world",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image":"crccheck/hello-world"}`),
						},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "version-check",
						Type: "version-context-policy",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		appKey := client.ObjectKeyFromObject(app)

		By("Verifying version context fields were accessible and written as labels")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			// context.policyType should be the PolicyDefinition's name
			g.Expect(app.Labels).Should(HaveKeyWithValue("audit.io/policy-name", "version-context-policy"))
			// context.policyName should be the instance name from spec.policies[].name
			g.Expect(app.Labels).Should(HaveKeyWithValue("audit.io/policy-instance", "version-check"))
			// context.policyRevision should be >= 1 (not "0")
			g.Expect(app.Labels).Should(HaveKey("audit.io/policy-revision"))
			g.Expect(app.Labels["audit.io/policy-revision"]).ShouldNot(Equal("0"))
			// context.policyRevisionName should match the DefinitionRevision name
			g.Expect(app.Labels).Should(HaveKeyWithValue("audit.io/policy-revision-name", latestRevisionName))
		}, policyTransformWaitTimeout, 3*time.Second).Should(Succeed())
	})

	It("Test policy with annotations output applies both labels and annotations", func() {
		By("Creating a PolicyDefinition that outputs both labels and annotations")
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "labels-and-annotations-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	team: string
}
output: {
	labels: {
		"org.io/team": parameter.team
	}
	annotations: {
		"org.io/managed-by": "kubevela"
		"org.io/policy-applied": "true"
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())

		By("Creating an Application that uses the policy")
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "labeled-annotated-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "hello-world",
						Type:       "webservice",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image":"crccheck/hello-world"}`)},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name:       "add-org-metadata",
						Type:       "labels-and-annotations-policy",
						Properties: &runtime.RawExtension{Raw: []byte(`{"team":"platform"}`)},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		appKey := client.ObjectKeyFromObject(app)

		By("Verifying labels and annotations were both applied")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Labels).Should(HaveKeyWithValue("org.io/team", "platform"))
			g.Expect(app.Annotations).Should(HaveKeyWithValue("org.io/managed-by", "kubevela"))
			g.Expect(app.Annotations).Should(HaveKeyWithValue("org.io/policy-applied", "true"))
		}, policyTransformWaitTimeout, 3*time.Second).Should(Succeed())
	})

	It("Test PolicyDefinition version is pinned in ApplicationRevision across re-reconciliations", func() {
		By("Creating a minimal ComponentDefinition so GenerateAppFile succeeds")
		cd := &v1beta1.ComponentDefinition{}
		cdJSON, err := yaml.YAMLToJSON([]byte(fmt.Sprintf(`
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: simple-worker
  namespace: %s
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
    template: |
      output: {
        apiVersion: "apps/v1"
        kind: "Deployment"
        spec: {
          selector: matchLabels: "app": context.name
          template: {
            metadata: labels: "app": context.name
            spec: containers: [{name: context.name, image: parameter.image}]
          }
        }
      }
      parameter: { image: string }
`, namespace)))
		Expect(err).Should(BeNil())
		Expect(json.Unmarshal(cdJSON, cd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, cd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Creating initial PolicyDefinition (v1) that labels with 'from-v1'")
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "versioned-label-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
output: {
	labels: {
		"versioned.io/label": "from-v1"
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		pdKey := client.ObjectKeyFromObject(policyDef)

		By("Creating Application and waiting for first ApplicationRevision and v1 label")
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "versioned-pinned-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "hello-world",
						Type:       "simple-worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image":"busybox:1"}`)},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name:       "pin-test",
						Type:       "versioned-label-policy",
						Properties: &runtime.RawExtension{Raw: []byte(`{}`)},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		appKey := client.ObjectKeyFromObject(app)

		var firstRevisionName string
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Labels).Should(HaveKeyWithValue("versioned.io/label", "from-v1"))
			g.Expect(app.Status.LatestRevision).ShouldNot(BeNil())
			firstRevisionName = app.Status.LatestRevision.Name
		}, policyTransformWaitTimeout, 3*time.Second).Should(Succeed())
		Expect(firstRevisionName).ShouldNot(BeEmpty())

		By("Updating PolicyDefinition to v2 with different label value")
		Eventually(func(g Gomega) {
			pd := &v1beta1.PolicyDefinition{}
			g.Expect(k8sClient.Get(ctx, pdKey, pd)).Should(Succeed())
			pd.Spec.Schematic.CUE.Template = `
output: {
	labels: {
		"versioned.io/label": "from-v2"
	}
}
`
			g.Expect(k8sClient.Update(ctx, pd)).Should(Succeed())
		}, 15*time.Second, time.Second).Should(Succeed())

		By("Updating Application spec to force a new ApplicationRevision")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			app.Spec.Components[0].Properties = &runtime.RawExtension{
				Raw: []byte(`{"image":"busybox:2"}`),
			}
			g.Expect(k8sClient.Update(ctx, app)).Should(Succeed())
		}, 15*time.Second, time.Second).Should(Succeed())

		By("Waiting for a second ApplicationRevision and verifying the label updated to v2")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.LatestRevision).ShouldNot(BeNil())
			g.Expect(app.Status.LatestRevision.Name).ShouldNot(Equal(firstRevisionName))
			g.Expect(app.Labels).Should(HaveKeyWithValue("versioned.io/label", "from-v2"))
		}, policyTransformWaitTimeout, 3*time.Second).Should(Succeed())
	})
})
