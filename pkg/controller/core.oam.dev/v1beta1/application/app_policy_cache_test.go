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
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Global Policy Cache", func() {
	namespace := "cache-test"
	var ctx context.Context

	BeforeEach(func() {
		// Set namespace in context for definition lookups
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

		// Clear cache before each test
		applicationPolicyCache.InvalidateAll()

		// Set client for lazy initialization
		policyScopeIndex.client = k8sClient
		policyScopeIndex.initialized = false
	})

	AfterEach(func() {
		// Clean up PolicyDefinitions to avoid test pollution
		policyList := &v1beta1.PolicyDefinitionList{}
		_ = k8sClient.List(ctx, policyList, client.InNamespace(namespace))
		for _, policy := range policyList.Items {
			_ = k8sClient.Delete(ctx, &policy)
		}
	})

	It("Test cache basic operations", func() {
		// Verify cache starts empty
		Expect(applicationPolicyCache.Size()).Should(Equal(0))

		// Test that cache can be cleared
		applicationPolicyCache.InvalidateAll()
		Expect(applicationPolicyCache.Size()).Should(Equal(0))
	})

	It("Test cache stores and retrieves rendered results", func() {
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "component",
						Type: "webservice",
					},
				},
			},
		}

		// Create mock rendered results
		results := []RenderedPolicyResult{
			{
				PolicyName:      "policy1",
				PolicyNamespace: namespace,
				Enabled:         true,
				Transforms: &PolicyOutput{
					Labels: map[string]string{
						"test": "value",
					},
				},
				AdditionalContext: map[string]interface{}{
					"key": "value",
				},
			},
		}

		// Set in cache
		err := applicationPolicyCache.Set(app, results)
		Expect(err).Should(BeNil())

		// Verify cache size
		Expect(applicationPolicyCache.Size()).Should(Equal(1))

		// Get from cache
		cached, hit, err := applicationPolicyCache.Get(app)
		Expect(err).Should(BeNil())
		Expect(hit).Should(BeTrue())
		Expect(cached).Should(HaveLen(1))
		Expect(cached[0].PolicyName).Should(Equal("policy1"))
		Expect(cached[0].Enabled).Should(BeTrue())
	})

	It("Test cache invalidation when app spec changes", func() {
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "component",
						Type: "webservice",
					},
				},
			},
		}

		results := []RenderedPolicyResult{
			{
				PolicyName:      "policy1",
				PolicyNamespace: namespace,
				Enabled:         true,
			},
		}

		// Cache with original spec
		err := applicationPolicyCache.Set(app, results)
		Expect(err).Should(BeNil())

		// Verify cache hit
		cached, hit, err := applicationPolicyCache.Get(app)
		Expect(err).Should(BeNil())
		Expect(hit).Should(BeTrue())
		Expect(cached).Should(HaveLen(1))

		// Modify app spec
		app.Spec.Components = append(app.Spec.Components, common.ApplicationComponent{
			Name: "new-component",
			Type: "worker",
		})

		// Cache should miss (spec hash changed)
		cached, hit, err = applicationPolicyCache.Get(app)
		Expect(err).Should(BeNil())
		Expect(hit).Should(BeFalse())
		Expect(cached).Should(BeNil())
	})

	It("Test cache persists until TTL or spec change", func() {
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "component",
						Type: "webservice",
					},
				},
			},
		}

		results := []RenderedPolicyResult{
			{
				PolicyName:      "policy1",
				PolicyNamespace: namespace,
				Enabled:         true,
			},
		}

		// Cache results
		err := applicationPolicyCache.Set(app, results)
		Expect(err).Should(BeNil())

		// Verify cache hit with same app
		cached, hit, err := applicationPolicyCache.Get(app)
		Expect(err).Should(BeNil())
		Expect(hit).Should(BeTrue())
		Expect(cached).Should(HaveLen(1))

		// Cache should still hit (global policy changes don't invalidate immediately)
		// They will be picked up on next render after 1-min TTL expires
		cached, hit, err = applicationPolicyCache.Get(app)
		Expect(err).Should(BeNil())
		Expect(hit).Should(BeTrue())
		Expect(cached).Should(HaveLen(1))
	})

	It("Test cache stores multiple policies", func() {
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "component",
						Type: "webservice",
					},
				},
			},
		}

		// Create multiple rendered results
		results := []RenderedPolicyResult{
			{
				PolicyName:      "policy1",
				PolicyNamespace: namespace,
				Enabled:         true,
				Transforms: &PolicyOutput{
					Labels: map[string]string{
						"from-policy1": "value1",
					},
				},
			},
			{
				PolicyName:      "policy2",
				PolicyNamespace: namespace,
				Enabled:         true,
				Transforms: &PolicyOutput{
					Labels: map[string]string{
						"from-policy2": "value2",
					},
				},
			},
			{
				PolicyName:      "policy3",
				PolicyNamespace: namespace,
				Enabled:         false,
				SkipReason:      "enabled=false",
			},
		}

		// Cache all results
		err := applicationPolicyCache.Set(app, results)
		Expect(err).Should(BeNil())

		// Get from cache
		cached, hit, err := applicationPolicyCache.Get(app)
		Expect(err).Should(BeNil())
		Expect(hit).Should(BeTrue())
		Expect(cached).Should(HaveLen(3))

		// Verify all policies are cached correctly
		Expect(cached[0].PolicyName).Should(Equal("policy1"))
		Expect(cached[0].Enabled).Should(BeTrue())
		Expect(cached[1].PolicyName).Should(Equal("policy2"))
		Expect(cached[1].Enabled).Should(BeTrue())
		Expect(cached[2].PolicyName).Should(Equal("policy3"))
		Expect(cached[2].Enabled).Should(BeFalse())
		Expect(cached[2].SkipReason).Should(Equal("enabled=false"))
	})

	It("Test cache namespace invalidation", func() {
		app1 := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app1",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{Name: "c1", Type: "webservice"}},
			},
		}

		app2 := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app2",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{Name: "c2", Type: "webservice"}},
			},
		}

		results := []RenderedPolicyResult{
			{PolicyName: "policy", PolicyNamespace: namespace, Enabled: true},
		}

		// Cache for both apps
		err := applicationPolicyCache.Set(app1, results)
		Expect(err).Should(BeNil())
		err = applicationPolicyCache.Set(app2, results)
		Expect(err).Should(BeNil())

		Expect(applicationPolicyCache.Size()).Should(Equal(2))

		// Invalidate namespace
		applicationPolicyCache.InvalidateForNamespace(namespace)

		// Both should be invalidated
		Expect(applicationPolicyCache.Size()).Should(Equal(0))
	})

	It("Test cache cleanup stale entries", func() {
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{Name: "component", Type: "webservice"},
				},
			},
		}

		results := []RenderedPolicyResult{
			{PolicyName: "policy", PolicyNamespace: namespace, Enabled: true},
		}

		// Cache results
		err := applicationPolicyCache.Set(app, results)
		Expect(err).Should(BeNil())

		// Verify cache hit immediately
		cached, hit, err := applicationPolicyCache.Get(app)
		Expect(err).Should(BeNil())
		Expect(hit).Should(BeTrue())
		Expect(cached).Should(HaveLen(1))

		// Note: We can't easily test TTL expiration in unit tests without time manipulation
		// The TTL check happens in Get() and would require waiting 1 minute
		// Integration tests should cover TTL behavior
	})

	It("Test InvalidateApplication removes specific app from cache", func() {
		app1 := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: namespace},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{Name: "c1", Type: "webservice"}},
			},
		}

		app2 := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app2", Namespace: namespace},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{Name: "c2", Type: "webservice"}},
			},
		}

		results := []RenderedPolicyResult{
			{PolicyName: "policy", PolicyNamespace: namespace, Enabled: true},
		}

		// Cache both apps
		err := applicationPolicyCache.Set(app1, results)
		Expect(err).Should(BeNil())
		err = applicationPolicyCache.Set(app2, results)
		Expect(err).Should(BeNil())

		Expect(applicationPolicyCache.Size()).Should(Equal(2))

		// Invalidate only app1
		applicationPolicyCache.InvalidateApplication(namespace, "app1")

		Expect(applicationPolicyCache.Size()).Should(Equal(1))

		// app1 should miss
		_, hit, _ := applicationPolicyCache.Get(app1)
		Expect(hit).Should(BeFalse())

		// app2 should still hit
		_, hit, _ = applicationPolicyCache.Get(app2)
		Expect(hit).Should(BeTrue())
	})

	It("Test ApplicationRevision restoration prevents double revisions", func() {
		// This test verifies the fix for the double ApplicationRevision bug
		// where subsequent reconciliations would create spurious revisions

		// Create a policy that transforms the spec
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "transform-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

output: {
	components: [{
		name: "transformed-component"
		type: "webservice"
		properties: {
			image: "transformed:v1"
		}
	}]
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "transform-policy", namespace)

		// Create Application
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.ApplicationKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-double-revision",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Name: "original-component",
					Type: "webservice",
				}},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		handler := &AppHandler{Client: k8sClient, app: app}
		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// First reconciliation - should create transformed spec
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())
		Expect(app.Spec.Components).Should(HaveLen(1))
		Expect(app.Spec.Components[0].Name).Should(Equal("transformed-component"))

		firstSpec := app.Spec.DeepCopy()

		// Simulate ApplicationRevision being created
		app.Status.LatestRevision = &common.Revision{
			Name:     "test-double-revision-v1",
			Revision: 1,
		}

		// Create a mock ApplicationRevision with the transformed spec
		appRev := &v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-double-revision-v1",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						Spec: *firstSpec,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, appRev)).Should(Succeed())

		// Second reconciliation (simulating status update trigger)
		// This should restore spec from ApplicationRevision, NOT create new transformed spec
		handler2 := &AppHandler{Client: k8sClient, app: app}
		_, err = handler2.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Spec should match the first revision (not create a different spec)
		Expect(app.Spec.Components).Should(HaveLen(1))
		Expect(app.Spec.Components[0].Name).Should(Equal("transformed-component"))
		Expect(app.Spec.Components).Should(Equal(firstSpec.Components))
	})
})
