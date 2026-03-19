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

	"k8s.io/apimachinery/pkg/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	monitorContext "github.com/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test FilterInternalMetadata", func() {
	It("should filter out internal annotations and labels", func() {
		metadata := map[string]string{
			// User metadata - should be kept
			"user.custom/annotation":   "keep",
			"my-label":                 "keep",
			"team":                     "platform",
			"custom.guidewire.dev/foo": "keep",

			// Internal metadata - should be filtered out
			"app.oam.dev/revision":               "filter",
			"oam.dev/resourceTracker":            "filter",
			"kubectl.kubernetes.io/last-applied": "filter",
			"kubernetes.io/service-account":      "filter",
			"k8s.io/cluster-service":             "filter",
			"helm.sh/chart":                      "filter",
			"app.kubernetes.io/managed-by":       "filter",
		}

		filtered := oam.FilterInternalMetadata(metadata)

		// Should keep user metadata
		Expect(filtered).Should(HaveKeyWithValue("user.custom/annotation", "keep"))
		Expect(filtered).Should(HaveKeyWithValue("my-label", "keep"))
		Expect(filtered).Should(HaveKeyWithValue("team", "platform"))
		Expect(filtered).Should(HaveKeyWithValue("custom.guidewire.dev/foo", "keep"))

		// Should filter out internal metadata
		Expect(filtered).ShouldNot(HaveKey("app.oam.dev/revision"))
		Expect(filtered).ShouldNot(HaveKey("oam.dev/resourceTracker"))
		Expect(filtered).ShouldNot(HaveKey("kubectl.kubernetes.io/last-applied"))
		Expect(filtered).ShouldNot(HaveKey("kubernetes.io/service-account"))
		Expect(filtered).ShouldNot(HaveKey("k8s.io/cluster-service"))
		Expect(filtered).ShouldNot(HaveKey("helm.sh/chart"))
		Expect(filtered).ShouldNot(HaveKey("app.kubernetes.io/managed-by"))

		// Should have exactly 4 items
		Expect(len(filtered)).Should(Equal(4))
	})

	It("should return nil for empty input", func() {
		filtered := oam.FilterInternalMetadata(nil)
		Expect(filtered).Should(BeNil())

		filtered = oam.FilterInternalMetadata(map[string]string{})
		Expect(filtered).Should(BeNil())
	})

	It("should return nil when all metadata is internal", func() {
		metadata := map[string]string{
			"app.oam.dev/revision":     "filter",
			"kubernetes.io/managed-by": "filter",
		}

		filtered := oam.FilterInternalMetadata(metadata)
		Expect(filtered).Should(BeNil())
	})

	It("should handle keys without prefixes", func() {
		metadata := map[string]string{
			"simple-key": "keep",
			"another":    "keep",
		}

		filtered := oam.FilterInternalMetadata(metadata)
		Expect(filtered).Should(HaveKeyWithValue("simple-key", "keep"))
		Expect(filtered).Should(HaveKeyWithValue("another", "keep"))
		Expect(len(filtered)).Should(Equal(2))
	})
})

var _ = Describe("Test policy context with explicit fields and filtered metadata", func() {
	namespace := "policy-context-test"
	var ctx context.Context

	BeforeEach(func() {
		ctx = util.SetNamespaceInCtx(context.Background(), namespace)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// Ensure feature gates are enabled (defensive against test ordering)
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableGlobalPolicies=true")).ToNot(HaveOccurred())
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true")).ToNot(HaveOccurred())

		// Set client for lazy initialization
		policyScopeIndex.client = k8sClient
		policyScopeIndex.initialized = false
	})

	It("should expose explicit context fields and filtered metadata to policies", func() {
		// Policy that accesses explicit fields and filtered metadata
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-context-fields",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

output: {
	labels: {
      // Use explicit context fields
      "context-app-name": context.appName
      "context-namespace": context.namespace
      "context-app-revision": context.appRevision
      // Use filtered appLabels
      "user-label-value": *context.appLabels["user-label"] | "not-found"
      // Internal label should NOT be available
      "internal-check": *context.appLabels["app.oam.dev/internal"] | "filtered-correctly"
    }
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-context-app",
				Namespace: namespace,
				Labels: map[string]string{
					"user-label":           "user-value",     // Should be available
					"app.oam.dev/internal": "internal-value", // Should be filtered out
				},
				Annotations: map[string]string{
					"user-annotation":       "user-anno-value", // Should be available
					"kubernetes.io/managed": "internal-anno",   // Should be filtered out
				},
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{Name: "test-comp", Type: "webservice"},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "test-context",
						Type: "test-context-fields",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		handler := &AppHandler{Client: k8sClient, app: app}
		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify explicit fields were accessible
		Expect(app.Labels).Should(HaveKeyWithValue("context-app-name", "test-context-app"))
		Expect(app.Labels).Should(HaveKeyWithValue("context-namespace", namespace))
		// appRevision might be empty in test context, so just check it exists
		Expect(app.Labels).Should(HaveKey("context-app-revision"))

		// Verify filtered user label was accessible
		Expect(app.Labels).Should(HaveKeyWithValue("user-label-value", "user-value"))

		// Verify internal label was filtered out (not accessible to policy)
		Expect(app.Labels).Should(HaveKeyWithValue("internal-check", "filtered-correctly"))
	})
})

var _ = Describe("Test policy context with appComponents, appWorkflow, appPolicies", func() {
	namespace := "policy-app-spec-test"
	var ctx context.Context

	BeforeEach(func() {
		ctx = util.SetNamespaceInCtx(context.Background(), namespace)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// Ensure feature gates are enabled (defensive against test ordering)
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableGlobalPolicies=true")).ToNot(HaveOccurred())
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true")).ToNot(HaveOccurred())

		// Set client for lazy initialization
		policyScopeIndex.client = k8sClient
		policyScopeIndex.initialized = false
	})

	It("should expose appComponents, appWorkflow, appPolicies in policy context", func() {
		// Policy that accesses controlled spec fields
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-spec-fields",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

output: {
	labels: {
      // Access appComponents
      "component-count": "\(len(context.appComponents))"
      "first-component": context.appComponents[0].name
      // Access appWorkflow
      "has-workflow": "\(context.appWorkflow != _|_)"
      // Access appPolicies
      "policy-count": "\(len(context.appPolicies))"
    }
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-spec",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{Name: "web-component", Type: "webservice"},
					{Name: "db-component", Type: "webservice"},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []wfTypesv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
								Name: "deploy",
								Type: "deploy",
							},
						},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{Name: "test-spec", Type: "test-app-spec-fields"},
					{Name: "another-policy", Type: "some-type"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		handler := &AppHandler{Client: k8sClient, app: app}
		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify appComponents accessible
		Expect(app.Labels).Should(HaveKeyWithValue("component-count", "2"))
		Expect(app.Labels).Should(HaveKeyWithValue("first-component", "web-component"))

		// Verify appWorkflow accessible
		Expect(app.Labels).Should(HaveKeyWithValue("has-workflow", "true"))

		// Verify appPolicies accessible
		Expect(app.Labels).Should(HaveKeyWithValue("policy-count", "2"))
	})

	It("Regression: Policy spec modifications preserved across status patch", func() {
		// This test verifies the fix for bug where policy-modified spec was lost
		// during UpdateAppLatestRevisionStatus() because Status().Patch() refreshes
		// the entire app object from API server
		ctx := context.Background()

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-regression-spec-preserve"},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

		// Create Application with original component
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-spec-preserve",
				Namespace: ns.Name,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "original-component",
						Type:       "webservice",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image":"original:latest"}`)},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		// Create handler
		handler, err := NewAppHandler(ctx, reconciler, app)
		Expect(err).Should(BeNil())

		// Simulate policy modification
		app.Spec.Components = []common.ApplicationComponent{
			{
				Name:       "modified-component",
				Type:       "webservice",
				Properties: &runtime.RawExtension{Raw: []byte(`{"image":"modified:latest"}`)},
			},
		}

		// Verify component was modified
		Expect(app.Spec.Components).Should(HaveLen(1))
		Expect(app.Spec.Components[0].Name).Should(Equal("modified-component"))

		// Simulate what happens during reconciliation:
		// 1. Generate revision (captures policy-modified spec)
		// 2. Update status (THIS is where the bug occurred - spec was reset)

		// Save the policy-modified spec
		expectedSpec := app.Spec.DeepCopy()

		// Simulate status update (this internally calls patchStatus which refreshes app)
		// In the bug, this would reset app.Spec to original values from API server
		Expect(handler.UpdateAppLatestRevisionStatus(ctx, func(ctx context.Context, app *v1beta1.Application, phase common.ApplicationPhase) error {
			// Mock status patcher that simulates the merge patch behavior
			// In production, this does Status().Patch() which refreshes the entire object
			freshApp := &v1beta1.Application{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: app.Name, Namespace: app.Namespace}, freshApp)
			if err != nil {
				return err
			}
			// The bug was here: app object gets refreshed from API server
			// Without the fix, app.Spec would be reset to original values
			return nil
		})).Should(Succeed())

		// CRITICAL: Verify spec is still the policy-modified version, not the original
		Expect(app.Spec.Components).Should(HaveLen(1), "Spec should still have policy-modified components")
		Expect(app.Spec.Components[0].Name).Should(Equal("modified-component"), "Component should still be modified-component, not original-component")
		Expect(app.Spec).Should(Equal(*expectedSpec), "Entire spec should be preserved across status patch")
	})

	It("Regression: JSON normalization prevents infinite ApplicationRevisions", func() {
		// This test verifies the fix for bug where component properties (RawExtension)
		// had inconsistent JSON byte representations, causing infinite revision creation
		ctx := context.Background()

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-regression-json-normalize"},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

		// Create two Applications with semantically identical but byte-different properties
		// This simulates what happens when policies regenerate components with different JSON formatting

		app1 := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-json-1",
				Namespace: ns.Name,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "web",
						Type: "webservice",
						// JSON with specific field order
						Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx","port":80}`)},
					},
				},
			},
		}

		app2 := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-json-2",
				Namespace: ns.Name,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "web",
						Type: "webservice",
						// Same JSON but different field order (semantically identical)
						Properties: &runtime.RawExtension{Raw: []byte(`{"port":80,"image":"nginx"}`)},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, app1)).Should(Succeed())
		Expect(k8sClient.Create(ctx, app2)).Should(Succeed())

		// Generate revisions for both apps
		handler1, err := NewAppHandler(ctx, reconciler, app1)
		Expect(err).Should(BeNil())
		handler2, err := NewAppHandler(ctx, reconciler, app2)
		Expect(err).Should(BeNil())

		// The gatherRevisionSpec() function now normalizes JSON
		rev1, hash1, err1 := handler1.gatherRevisionSpec(nil)
		Expect(err1).Should(BeNil())

		rev2, hash2, err2 := handler2.gatherRevisionSpec(nil)
		Expect(err2).Should(BeNil())

		// CRITICAL: Despite different input JSON byte order, normalized properties should be identical
		Expect(rev1.Spec.Application.Spec.Components[0].Properties.Raw).Should(Equal(rev2.Spec.Application.Spec.Components[0].Properties.Raw),
			"Normalized JSON should have identical bytes regardless of input field order")

		// Hashes should be identical (preventing duplicate revisions)
		Expect(hash1).Should(Equal(hash2), "Hash should be identical for semantically identical specs")

		// Deep equality should pass
		equal := DeepEqualRevision(rev1, rev2)
		Expect(equal).Should(BeTrue(), "DeepEqualRevision should return true for normalized revisions")
	})
})

var _ = Describe("Test Application Metadata Update from Policies", func() {
	namespace := "metadata-update-test"
	var ctx context.Context

	BeforeEach(func() {
		ctx = util.SetNamespaceInCtx(context.Background(), namespace)
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// Ensure feature gates are enabled (defensive against test ordering)
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableGlobalPolicies=true")).ToNot(HaveOccurred())
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true")).ToNot(HaveOccurred())

		applicationPolicyCache.InvalidateAll()

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
	})

	It("Test UpdateApplicationMetadata persists labels and annotations", func() {
		// Create a PolicyDefinition that adds labels and annotations
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-metadata",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

output: {
	labels: {
		"policy-added-label": "label-value"
		"team": "platform"
	}
	annotations: {
		"policy-added-annotation": "annotation-value"
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())

		// Create an Application with initial labels/annotations
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: namespace,
				Labels: map[string]string{
					"existing-label": "existing-value",
				},
				Annotations: map[string]string{
					"existing-annotation": "existing-value",
				},
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "test-comp",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image": "nginx"}`),
						},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "add-metadata",
						Type: "add-metadata",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		// Apply policy transforms (in-memory modification)
		handler := &AppHandler{
			Client: k8sClient,
		}
		monCtx := monitorContext.NewTraceContext(ctx, "test-metadata-update")
		monCtx, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify in-memory app has merged labels/annotations
		Expect(app.Labels).Should(HaveLen(3))
		Expect(app.Labels["existing-label"]).Should(Equal("existing-value"))
		Expect(app.Labels["policy-added-label"]).Should(Equal("label-value"))
		Expect(app.Labels["team"]).Should(Equal("platform"))

		Expect(app.Annotations).Should(HaveLen(2))
		Expect(app.Annotations["existing-annotation"]).Should(Equal("existing-value"))
		Expect(app.Annotations["policy-added-annotation"]).Should(Equal("annotation-value"))

		// Update Application metadata to persist changes
		err = handler.UpdateApplicationMetadata(monCtx, app)
		Expect(err).Should(BeNil())

		// Fetch Application from API server to verify persistence
		persistedApp := &v1beta1.Application{}
		err = k8sClient.Get(ctx, client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}, persistedApp)
		Expect(err).Should(BeNil())

		// Verify persisted labels include both original and policy-added
		Expect(persistedApp.Labels).Should(HaveLen(3))
		Expect(persistedApp.Labels["existing-label"]).Should(Equal("existing-value"))
		Expect(persistedApp.Labels["policy-added-label"]).Should(Equal("label-value"))
		Expect(persistedApp.Labels["team"]).Should(Equal("platform"))

		// Verify persisted annotations include both original and policy-added
		Expect(persistedApp.Annotations).Should(HaveLen(2))
		Expect(persistedApp.Annotations["existing-annotation"]).Should(Equal("existing-value"))
		Expect(persistedApp.Annotations["policy-added-annotation"]).Should(Equal("annotation-value"))
	})

	It("Test UpdateApplicationMetadata skips when no metadata changes", func() {
		// Create a PolicyDefinition that only modifies spec (no labels/annotations)
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spec-only",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

output: {
	components: [{
		name: "added-component"
		type: "webservice"
		properties: {
			image: "added-image"
		}
	}]
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-no-metadata",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "original-comp",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image": "nginx"}`),
						},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "spec-only",
						Type: "spec-only",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		handler := &AppHandler{
			Client: k8sClient,
		}
		monCtx := monitorContext.NewTraceContext(ctx, "test-no-metadata")
		monCtx, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Update should succeed but skip (no metadata changes)
		err = handler.UpdateApplicationMetadata(monCtx, app)
		Expect(err).Should(BeNil())
	})

	It("Test UpdateApplicationMetadata is idempotent (no update on second call)", func() {
		// Create a PolicyDefinition that adds metadata
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-labels",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

output: {
	labels: {
		"test-label": "test-value"
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-idempotent",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "test-comp",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image": "nginx"}`),
						},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "add-labels",
						Type: "add-labels",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		handler := &AppHandler{
			Client: k8sClient,
		}

		// First reconciliation: Apply transforms and update metadata
		monCtx := monitorContext.NewTraceContext(ctx, "test-idempotent-1")
		monCtx, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		err = handler.UpdateApplicationMetadata(monCtx, app)
		Expect(err).Should(BeNil())

		// Fetch to verify first update succeeded
		persistedApp := &v1beta1.Application{}
		err = k8sClient.Get(ctx, client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}, persistedApp)
		Expect(err).Should(BeNil())
		Expect(persistedApp.Labels).Should(HaveKey("test-label"))
		firstResourceVersion := persistedApp.ResourceVersion

		// Second reconciliation: Simulate another reconcile loop
		// The Application now already has the labels from the policy
		monCtx2 := monitorContext.NewTraceContext(ctx, "test-idempotent-2")
		monCtx2, err = handler.ApplyApplicationScopeTransforms(monCtx2, persistedApp)
		Expect(err).Should(BeNil())

		// This should detect that labels are already present and skip the update
		err = handler.UpdateApplicationMetadata(monCtx2, persistedApp)
		Expect(err).Should(BeNil())

		// Fetch again to verify NO update occurred (resourceVersion unchanged)
		persistedApp2 := &v1beta1.Application{}
		err = k8sClient.Get(ctx, client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}, persistedApp2)
		Expect(err).Should(BeNil())
		Expect(persistedApp2.ResourceVersion).Should(Equal(firstResourceVersion),
			"ResourceVersion should be unchanged on second call (no update performed)")
	})
})
