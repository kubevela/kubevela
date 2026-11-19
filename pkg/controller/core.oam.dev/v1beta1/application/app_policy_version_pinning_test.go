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

package application

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test PolicyDefinition Version Pinning in ApplicationRevision", func() {
	namespace := "policy-version-pinning-test"
	var ctx context.Context

	BeforeEach(func() {
		ctx = util.SetNamespaceInCtx(context.Background(), namespace)
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// Enable Application-scoped policies feature
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true")).Should(Succeed())

		// Set client for lazy initialization
		policyScopeIndex.client = k8sClient
		policyScopeIndex.initialized = false
	})

	AfterEach(func() {
		// Clean up
		policyList := &v1beta1.PolicyDefinitionList{}
		_ = k8sClient.List(ctx, policyList, client.InNamespace(namespace))
		for _, policy := range policyList.Items {
			_ = k8sClient.Delete(ctx, &policy)
		}

		appList := &v1beta1.ApplicationList{}
		_ = k8sClient.List(ctx, appList, client.InNamespace(namespace))
		for _, app := range appList.Items {
			_ = k8sClient.Delete(ctx, &app)
		}

		appRevList := &v1beta1.ApplicationRevisionList{}
		_ = k8sClient.List(ctx, appRevList, client.InNamespace(namespace))
		for _, appRev := range appRevList.Items {
			_ = k8sClient.Delete(ctx, &appRev)
		}

		defRevList := &v1beta1.DefinitionRevisionList{}
		_ = k8sClient.List(ctx, defRevList, client.InNamespace(namespace))
		for _, defRev := range defRevList.Items {
			_ = k8sClient.Delete(ctx, &defRev)
		}
	})

	It("should store PolicyDefinitions in ApplicationRevision on new revision", func() {
		// Create a PolicyDefinition v1
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
output: {
	labels: {
		"policy-version": "v1"
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "test-policy", namespace)

		// Create DefinitionRevision for the policy
		defRev := &v1beta1.DefinitionRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy-v1",
				Namespace: namespace,
			},
			Spec: v1beta1.DefinitionRevisionSpec{
				DefinitionType: common.PolicyType,
				Revision:       1,
				RevisionHash:   "hash-v1",
			},
		}
		Expect(k8sClient.Create(ctx, defRev)).Should(Succeed())

		// Update PolicyDefinition with LatestRevision
		Eventually(func() error {
			pd := &v1beta1.PolicyDefinition{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-policy", Namespace: namespace}, pd); err != nil {
				return err
			}
			pd.Status.LatestRevision = &common.Revision{
				Name:         "test-policy-v1",
				Revision:     1,
				RevisionHash: "hash-v1",
			}
			return k8sClient.Status().Update(ctx, pd)
		}).Should(Succeed())

		// Create Application using the policy
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "test-comp",
						Type: "webservice",
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "test-policy",
						Type: "test-policy",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		// Create handler and process
		handler := &AppHandler{
			Client:        k8sClient,
			app:           app,
			isNewRevision: true,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test-version-pinning")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify ApplicationRevision would contain PolicyDefinitions
		Expect(handler.applicationScopedPolicyDefs).Should(HaveKey("test-policy"))
		Expect(handler.policyVersions).Should(HaveKey("test-policy"))
		Expect(handler.policyVersions["test-policy"].DefinitionRevisionName).Should(Equal("test-policy-v1"))
		Expect(handler.policyVersions["test-policy"].Revision).Should(Equal(int64(1)))
		Expect(handler.policyVersions["test-policy"].RevisionHash).Should(Equal("hash-v1"))
	})

	It("should use stored PolicyDefinitions from ApplicationRevision on re-reconciliation", func() {
		// Create PolicyDefinition v1
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "version-test-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
output: {
	labels: {
		"from-policy": "v1"
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "version-test-policy", namespace)

		// Create DefinitionRevision v1
		defRev1 := &v1beta1.DefinitionRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "version-test-policy-v1",
				Namespace: namespace,
			},
			Spec: v1beta1.DefinitionRevisionSpec{
				DefinitionType: common.PolicyType,
				Revision:       1,
				RevisionHash:   "abc123",
			},
		}
		Expect(k8sClient.Create(ctx, defRev1)).Should(Succeed())

		// Create ApplicationRevision with stored v1
		appRev := &v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-v1",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						Spec: v1beta1.ApplicationSpec{
							Components: []common.ApplicationComponent{
								{
									Name: "test-comp",
									Type: "webservice",
								},
							},
						},
					},
					PolicyDefinitions: map[string]v1beta1.PolicyDefinition{
						"version-test-policy": *policyDef.DeepCopy(),
					},
					PolicyVersions: map[string]v1beta1.PolicyVersionMetadata{
						"version-test-policy": {
							DefinitionRevisionName: "version-test-policy-v1",
							Revision:               1,
							RevisionHash:           "abc123",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, appRev)).Should(Succeed())

		// Now update PolicyDefinition to v2 (simulating a new version in the cluster)
		Eventually(func() error {
			pd := &v1beta1.PolicyDefinition{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "version-test-policy", Namespace: namespace}, pd); err != nil {
				return err
			}
			pd.Spec.Schematic.CUE.Template = `
output: {
	labels: {
		"from-policy": "v2-updated"
	}
}
`
			pd.Status.LatestRevision = &common.Revision{
				Name:         "version-test-policy-v2",
				Revision:     2,
				RevisionHash: "def456",
			}
			if err := k8sClient.Update(ctx, pd); err != nil {
				return err
			}
			return k8sClient.Status().Update(ctx, pd)
		}).Should(Succeed())

		// Create Application
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-version",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "test-comp",
						Type: "webservice",
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "version-test-policy",
						Type: "version-test-policy",
					},
				},
			},
			Status: common.AppStatus{
				LatestRevision: &common.Revision{
					Name:         "test-app-v1",
					RevisionHash: "rev-hash-1",
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		// Process with isNewRevision=false (re-reconciliation)
		handler := &AppHandler{
			Client:        k8sClient,
			app:           app,
			isNewRevision: false, // Key: re-reconciling existing revision
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test-version-pinning-reuse")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify it used v1 from ApplicationRevision, NOT v2 from cluster
		Expect(handler.policyVersions).Should(HaveKey("version-test-policy"))
		Expect(handler.policyVersions["version-test-policy"].DefinitionRevisionName).Should(Equal("version-test-policy-v1"))
		Expect(handler.policyVersions["version-test-policy"].Revision).Should(Equal(int64(1)))
		Expect(handler.policyVersions["version-test-policy"].RevisionHash).Should(Equal("abc123"))
	})

	It("should invalidate cache when ApplicationRevision hash changes", func() {
		// Create PolicyDefinition
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cache-test-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
output: {
	labels: {
		"cache-test": "value"
	}
}
`,
					},
				},
			},
			Status: v1beta1.PolicyDefinitionStatus{
				LatestRevision: &common.Revision{
					Name:         "cache-test-policy-v1",
					Revision:     1,
					RevisionHash: "cache-hash-1",
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "cache-test-policy", namespace)
		policyDef.Status = v1beta1.PolicyDefinitionStatus{
			LatestRevision: &common.Revision{
				Name:         "cache-test-policy-v1",
				Revision:     1,
				RevisionHash: "cache-hash-1",
			},
		}
		Expect(k8sClient.Status().Update(ctx, policyDef)).Should(Succeed())

		// Create Application
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cache-test-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "test-comp",
						Type: "webservice",
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "cache-test-policy",
						Type: "cache-test-policy",
					},
				},
			},
			Status: common.AppStatus{
				LatestRevision: &common.Revision{
					Name:         "cache-test-app-v1",
					RevisionHash: "rev-hash-1",
				},
			},
		}

		// First request - should populate cache
		handler1 := &AppHandler{
			Client:        k8sClient,
			app:           app,
			isNewRevision: true,
		}

		monCtx1 := monitorContext.NewTraceContext(ctx, "test-cache-1")
		_, err := handler1.ApplyApplicationScopeTransforms(monCtx1, app)
		Expect(err).Should(BeNil())

		// Second request with same revision - should hit cache
		handler2 := &AppHandler{
			Client:        k8sClient,
			app:           app,
			isNewRevision: false,
		}

		monCtx2 := monitorContext.NewTraceContext(ctx, "test-cache-2")
		_, err = handler2.ApplyApplicationScopeTransforms(monCtx2, app)
		Expect(err).Should(BeNil())

		// Third request with DIFFERENT revision hash - should miss cache
		appWithNewRev := app.DeepCopy()
		appWithNewRev.Status.LatestRevision.RevisionHash = "rev-hash-2" // Changed!

		handler3 := &AppHandler{
			Client:        k8sClient,
			app:           appWithNewRev,
			isNewRevision: false,
		}

		monCtx3 := monitorContext.NewTraceContext(ctx, "test-cache-3")
		_, err = handler3.ApplyApplicationScopeTransforms(monCtx3, appWithNewRev)
		Expect(err).Should(BeNil())

		// Verify maps were populated (proves it rendered, not just returned cached)
		Expect(handler3.applicationScopedPolicyDefs).Should(HaveKey("cache-test-policy"))
		Expect(handler3.policyVersions).Should(HaveKey("cache-test-policy"))
		Expect(handler3.policyVersions["cache-test-policy"].DefinitionRevisionName).Should(Equal("cache-test-policy-v1"))
	})

	It("should pin policy version across reconciliations until Application spec changes", func() {
		// Step 1: Create initial PolicyDefinition v1
		policyDefV1 := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lifecycle-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
output: {
	labels: {
		"policy-version": "v1"
		"test-label": "from-v1"
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDefV1)).Should(Succeed())
		waitForPolicyDef(ctx, "lifecycle-policy", namespace)

		// Create DefinitionRevision v1
		defRevV1 := &v1beta1.DefinitionRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lifecycle-policy-v1",
				Namespace: namespace,
			},
			Spec: v1beta1.DefinitionRevisionSpec{
				DefinitionType: common.PolicyType,
				Revision:       1,
				RevisionHash:   "hash-v1",
			},
		}
		Expect(k8sClient.Create(ctx, defRevV1)).Should(Succeed())

		// Update PolicyDefinition status with v1 revision info
		Eventually(func() error {
			pd := &v1beta1.PolicyDefinition{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "lifecycle-policy", Namespace: namespace}, pd); err != nil {
				return err
			}
			pd.Status.LatestRevision = &common.Revision{
				Name:         "lifecycle-policy-v1",
				Revision:     1,
				RevisionHash: "hash-v1",
			}
			return k8sClient.Status().Update(ctx, pd)
		}).Should(Succeed())

		// Step 2: Initial Application deployment (creates ApplicationRevision-1 with v1)
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lifecycle-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "test-comp",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image": "nginx:1.0"}`),
						},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "lifecycle-policy",
						Type: "lifecycle-policy",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		// First reconciliation: isNewRevision=true (creating first ApplicationRevision)
		handler1 := &AppHandler{
			Client:        k8sClient,
			app:           app,
			isNewRevision: true,
		}

		monCtx1 := monitorContext.NewTraceContext(ctx, "lifecycle-initial")
		_, err := handler1.ApplyApplicationScopeTransforms(monCtx1, app)
		Expect(err).Should(BeNil())

		// Verify: Should have discovered v1 from cluster
		Expect(handler1.policyVersions).Should(HaveKey("lifecycle-policy"))
		Expect(handler1.policyVersions["lifecycle-policy"].DefinitionRevisionName).Should(Equal("lifecycle-policy-v1"))
		Expect(handler1.policyVersions["lifecycle-policy"].Revision).Should(Equal(int64(1)))
		Expect(handler1.policyVersions["lifecycle-policy"].RevisionHash).Should(Equal("hash-v1"))

		// Verify: Application labels should have v1 values
		Expect(app.Labels).Should(HaveKeyWithValue("policy-version", "v1"))
		Expect(app.Labels).Should(HaveKeyWithValue("test-label", "from-v1"))

		// Create ApplicationRevision-1 with stored PolicyDefinitions
		// Convert pointer map to value map for ApplicationRevision storage
		policyDefs := make(map[string]v1beta1.PolicyDefinition)
		for k, v := range handler1.applicationScopedPolicyDefs {
			policyDefs[k] = *v
		}

		appRev1 := &v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lifecycle-app-v1",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						Spec: app.Spec,
					},
					PolicyDefinitions: policyDefs,
					PolicyVersions:    handler1.policyVersions,
				},
			},
		}
		Expect(k8sClient.Create(ctx, appRev1)).Should(Succeed())

		// Update Application status to reference ApplicationRevision-1
		Eventually(func() error {
			appCopy := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "lifecycle-app", Namespace: namespace}, appCopy); err != nil {
				return err
			}
			appCopy.Status.LatestRevision = &common.Revision{
				Name:         "lifecycle-app-v1",
				RevisionHash: "app-rev-hash-1",
			}
			return k8sClient.Status().Update(ctx, appCopy)
		}).Should(Succeed())

		// Step 3: Re-reconcile without spec change (should reuse ApplicationRevision-1 with v1)
		// Fetch updated Application
		appRecon1 := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "lifecycle-app", Namespace: namespace}, appRecon1)).Should(Succeed())

		handler2 := &AppHandler{
			Client:        k8sClient,
			app:           appRecon1,
			isNewRevision: false, // Key: reusing existing revision
		}

		monCtx2 := monitorContext.NewTraceContext(ctx, "lifecycle-recon-1")
		_, err = handler2.ApplyApplicationScopeTransforms(monCtx2, appRecon1)
		Expect(err).Should(BeNil())

		// Verify: Should still use v1 (loaded from ApplicationRevision-1)
		Expect(handler2.policyVersions).Should(HaveKey("lifecycle-policy"))
		Expect(handler2.policyVersions["lifecycle-policy"].DefinitionRevisionName).Should(Equal("lifecycle-policy-v1"))
		Expect(handler2.policyVersions["lifecycle-policy"].Revision).Should(Equal(int64(1)))
		Expect(handler2.policyVersions["lifecycle-policy"].RevisionHash).Should(Equal("hash-v1"))

		// Step 4: Update PolicyDefinition in cluster to v2
		Eventually(func() error {
			pd := &v1beta1.PolicyDefinition{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "lifecycle-policy", Namespace: namespace}, pd); err != nil {
				return err
			}
			// Update template to v2
			pd.Spec.Schematic.CUE.Template = `
output: {
	labels: {
		"policy-version": "v2"
		"test-label": "from-v2-updated"
	}
}
`
			return k8sClient.Update(ctx, pd)
		}).Should(Succeed())

		// Create DefinitionRevision v2
		defRevV2 := &v1beta1.DefinitionRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lifecycle-policy-v2",
				Namespace: namespace,
			},
			Spec: v1beta1.DefinitionRevisionSpec{
				DefinitionType: common.PolicyType,
				Revision:       2,
				RevisionHash:   "hash-v2",
			},
		}
		Expect(k8sClient.Create(ctx, defRevV2)).Should(Succeed())

		// Update PolicyDefinition status to point to v2
		Eventually(func() error {
			pd := &v1beta1.PolicyDefinition{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "lifecycle-policy", Namespace: namespace}, pd); err != nil {
				return err
			}
			pd.Status.LatestRevision = &common.Revision{
				Name:         "lifecycle-policy-v2",
				Revision:     2,
				RevisionHash: "hash-v2",
			}
			return k8sClient.Status().Update(ctx, pd)
		}).Should(Succeed())

		// Step 5: Re-reconcile again (still no spec change) - should STILL use v1 (version pinning!)
		appRecon2 := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "lifecycle-app", Namespace: namespace}, appRecon2)).Should(Succeed())

		handler3 := &AppHandler{
			Client:        k8sClient,
			app:           appRecon2,
			isNewRevision: false, // Still reusing ApplicationRevision-1
		}

		monCtx3 := monitorContext.NewTraceContext(ctx, "lifecycle-recon-2-after-policy-update")
		_, err = handler3.ApplyApplicationScopeTransforms(monCtx3, appRecon2)
		Expect(err).Should(BeNil())

		// Verify: Should STILL use v1 from ApplicationRevision-1 (version pinning!)
		// This is the key assertion - proves version pinning works
		Expect(handler3.policyVersions).Should(HaveKey("lifecycle-policy"))
		Expect(handler3.policyVersions["lifecycle-policy"].DefinitionRevisionName).Should(Equal("lifecycle-policy-v1"))
		Expect(handler3.policyVersions["lifecycle-policy"].Revision).Should(Equal(int64(1)))
		Expect(handler3.policyVersions["lifecycle-policy"].RevisionHash).Should(Equal("hash-v1"))

		// Step 6: Update Application spec (triggers new ApplicationRevision-2)
		Eventually(func() error {
			appUpdate := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "lifecycle-app", Namespace: namespace}, appUpdate); err != nil {
				return err
			}
			// Change component image - this is a spec change
			appUpdate.Spec.Components[0].Properties = &runtime.RawExtension{
				Raw: []byte(`{"image": "nginx:2.0"}`), // Changed!
			}
			return k8sClient.Update(ctx, appUpdate)
		}).Should(Succeed())

		// Fetch updated Application
		appNewRev := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "lifecycle-app", Namespace: namespace}, appNewRev)).Should(Succeed())

		// Process with isNewRevision=true (creating ApplicationRevision-2)
		handler4 := &AppHandler{
			Client:        k8sClient,
			app:           appNewRev,
			isNewRevision: true, // New revision!
		}

		monCtx4 := monitorContext.NewTraceContext(ctx, "lifecycle-new-revision")
		_, err = handler4.ApplyApplicationScopeTransforms(monCtx4, appNewRev)
		Expect(err).Should(BeNil())

		// Verify: Should NOW discover and use v2 from cluster
		Expect(handler4.policyVersions).Should(HaveKey("lifecycle-policy"))
		Expect(handler4.policyVersions["lifecycle-policy"].DefinitionRevisionName).Should(Equal("lifecycle-policy-v2"))
		Expect(handler4.policyVersions["lifecycle-policy"].Revision).Should(Equal(int64(2)))
		Expect(handler4.policyVersions["lifecycle-policy"].RevisionHash).Should(Equal("hash-v2"))

		// Verify: Application labels should now have v2 values
		Expect(appNewRev.Labels).Should(HaveKeyWithValue("policy-version", "v2"))
		Expect(appNewRev.Labels).Should(HaveKeyWithValue("test-label", "from-v2-updated"))
	})

	It("Test policy version context fields are accessible in CUE templates", func() {
		// Create PolicyDefinition with version metadata
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "version-test-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
output: {
	labels: {
		"policy.oam.dev/name": context.policyName
		"policy.oam.dev/revision": "\(context.policyRevision)"
		if context.policyRevisionName != "" {
			"policy.oam.dev/revision-name": context.policyRevisionName
		}
		if context.policyRevisionHash != "" {
			"policy.oam.dev/revision-hash": context.policyRevisionHash
		}
	}
}
`,
					},
				},
			},
			Status: v1beta1.PolicyDefinitionStatus{
				LatestRevision: &common.Revision{
					Name:         "version-test-policy-v2",
					Revision:     2,
					RevisionHash: "abc123def456",
				},
			},
		}

		createPolicyDefAndIndex(ctx, policyDef)

		// Update status separately (status is a subresource and not set on Create)
		policyDef.Status = v1beta1.PolicyDefinitionStatus{
			LatestRevision: &common.Revision{
				Name:         "version-test-policy-v2",
				Revision:     2,
				RevisionHash: "abc123def456",
			},
		}
		Expect(k8sClient.Status().Update(ctx, policyDef)).Should(Succeed())

		// Create Application using this policy
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "comp1",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image":"nginx"}`),
						},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "version-test",
						Type: "version-test-policy",
					},
				},
			},
		}

		// Create handler
		handler, err := NewAppHandler(ctx, reconciler, app)
		Expect(err).Should(BeNil())
		handler.isNewRevision = true

		// Enable feature gate
		utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true")
		defer utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=false")

		// Apply policies
		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err = handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify version metadata is in labels
		Expect(app.Labels).Should(HaveKeyWithValue("policy.oam.dev/name", "version-test"))
		Expect(app.Labels).Should(HaveKeyWithValue("policy.oam.dev/revision", "2"))
		Expect(app.Labels).Should(HaveKeyWithValue("policy.oam.dev/revision-name", "version-test-policy-v2"))
		Expect(app.Labels).Should(HaveKeyWithValue("policy.oam.dev/revision-hash", "abc123def456"))

		// Verify handler tracked version metadata
		Expect(handler.policyVersions).Should(HaveKey("version-test"))
		versionMeta := handler.policyVersions["version-test"]
		Expect(versionMeta.DefinitionRevisionName).Should(Equal("version-test-policy-v2"))
		Expect(versionMeta.Revision).Should(Equal(int64(2)))
		Expect(versionMeta.RevisionHash).Should(Equal("abc123def456"))
	})
})
