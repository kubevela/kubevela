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

var _ = Describe("Test Global PolicyDefinition Features", func() {
	namespace := "global-policy-test"
	var ctx context.Context
	velaSystem := "vela-system"

	BeforeEach(func() {
		// Set namespace in context for definition lookups
		ctx = util.SetNamespaceInCtx(context.Background(), namespace)

		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		velaSystemNs := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: velaSystem,
			},
		}
		Expect(k8sClient.Create(ctx, &velaSystemNs)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// Ensure feature gates are enabled (defensive against test ordering)
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableGlobalPolicies=true")).ToNot(HaveOccurred())
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true")).ToNot(HaveOccurred())

		// Clear cache
		applicationPolicyCache.InvalidateAll()

		// Set client for lazy initialization
		policyScopeIndex.client = k8sClient
		policyScopeIndex.initialized = false
	})

	AfterEach(func() {
		// Clean up PolicyDefinitions to avoid test pollution in both namespaces
		policyList := &v1beta1.PolicyDefinitionList{}
		_ = k8sClient.List(ctx, policyList, client.InNamespace(namespace))
		for _, policy := range policyList.Items {
			_ = k8sClient.Delete(ctx, &policy)
		}

		// Also clean up vela-system policies
		velaSystemPolicyList := &v1beta1.PolicyDefinitionList{}
		_ = k8sClient.List(ctx, velaSystemPolicyList, client.InNamespace(velaSystem))
		for _, policy := range velaSystemPolicyList.Items {
			_ = k8sClient.Delete(ctx, &policy)
		}
	})

	It("Test global policy discovery from vela-system applies to all namespaces", func() {
		// Create global policy in vela-system
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vela-system-global",
				Namespace: velaSystem,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

config: {
  enabled: true
}

output: {
	labels: {
			"vela-system-global": "true"
		}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "vela-system-global", velaSystem)

		// Force index rebuild so it picks up the newly created policy
		policyScopeIndex.initialized = false
		policies := policyScopeIndex.GetGlobalApplicationPoliciesDeduped(namespace)
		Expect(policies).Should(HaveLen(1))
		Expect(policies[0].Name).Should(Equal("vela-system-global"))
		Expect(policies[0].Global).Should(BeTrue())
		Expect(policies[0].Priority).Should(Equal(int32(100)))
	})

	It("Test global policy discovery from namespace applies only to that namespace", func() {
		// Create global policy in specific namespace
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "namespace-global",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 50,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

config: {
  enabled: true
}

output: {
	labels: {
			"namespace-global": "true"
		}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "namespace-global", namespace)

		// Force index rebuild and query via index (production code path)
		policyScopeIndex.initialized = false
		policies := policyScopeIndex.GetGlobalApplicationPoliciesDeduped(namespace)
		Expect(policies).Should(HaveLen(1))
		Expect(policies[0].Name).Should(Equal("namespace-global"))

		// Query vela-system scope (should not include namespace policy)
		velaSystemPolicies := policyScopeIndex.GetGlobalApplicationPoliciesDeduped(velaSystem)
		// Should not include namespace-global policy
		for _, p := range velaSystemPolicies {
			Expect(p.Name).ShouldNot(Equal("namespace-global"))
		}
	})

	It("Test priority ordering - lower priority value runs first", func() {
		// Create 3 policies with different priorities
		policy1 := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "low-priority",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 10,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: `parameter: {}`},
				},
			},
		}
		policy2 := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "high-priority",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: `parameter: {}`},
				},
			},
		}
		policy3 := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "medium-priority",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 50,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: `parameter: {}`},
				},
			},
		}

		Expect(k8sClient.Create(ctx, policy1)).Should(Succeed())
		Expect(k8sClient.Create(ctx, policy2)).Should(Succeed())
		Expect(k8sClient.Create(ctx, policy3)).Should(Succeed())

		// Force index rebuild and verify ordering via production code path
		policyScopeIndex.initialized = false
		policies := policyScopeIndex.GetGlobalApplicationPoliciesDeduped(namespace)
		Expect(policies).Should(HaveLen(3))

		// Verify order: low-priority value (10) first, then (50), then (100)
		// Lower priority value = runs first (matches Kubernetes admission webhook convention)
		Expect(policies[0].Name).Should(Equal("low-priority"))
		Expect(policies[0].Priority).Should(Equal(int32(10)))
		Expect(policies[1].Name).Should(Equal("medium-priority"))
		Expect(policies[1].Priority).Should(Equal(int32(50)))
		Expect(policies[2].Name).Should(Equal("high-priority"))
		Expect(policies[2].Priority).Should(Equal(int32(100)))
	})

	It("Test alphabetical ordering for same priority", func() {
		// Create 3 policies with same priority
		policy1 := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-c",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 50,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: `parameter: {}`},
				},
			},
		}
		policy2 := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-a",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 50,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: `parameter: {}`},
				},
			},
		}
		policy3 := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-b",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 50,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: `parameter: {}`},
				},
			},
		}

		Expect(k8sClient.Create(ctx, policy1)).Should(Succeed())
		Expect(k8sClient.Create(ctx, policy2)).Should(Succeed())
		Expect(k8sClient.Create(ctx, policy3)).Should(Succeed())

		// Force index rebuild and verify alphabetical ordering via production code path
		policyScopeIndex.initialized = false
		policies := policyScopeIndex.GetGlobalApplicationPoliciesDeduped(namespace)
		Expect(policies).Should(HaveLen(3))

		// Verify alphabetical order: policy-a, policy-b, policy-c
		Expect(policies[0].Name).Should(Equal("policy-a"))
		Expect(policies[1].Name).Should(Equal("policy-b"))
		Expect(policies[2].Name).Should(Equal("policy-c"))
	})

	It("Test explicit policies run in spec declaration order, not alphabetical", func() {
		// Build three policyToRender entries with the same priority (0) but in reverse-alphabetical
		// spec order: "policy-z" declared first, "policy-a" declared last.
		// They must execute in spec order (z → m → a), not alphabetical (a → m → z).
		policiesToRender := []policyToRender{
			{
				policyDef: &v1beta1.PolicyDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "policy-z", Namespace: namespace},
					Spec: v1beta1.PolicyDefinitionSpec{
						Scope: v1beta1.ApplicationScope,
						Schematic: &common.Schematic{CUE: &common.CUE{Template: `
parameter: {}
config: { enabled: true }
output: { labels: { "order": "first" } }
`}},
					},
				},
				policyRef: v1beta1.AppPolicy{Name: "policy-z", Type: "policy-z"},
				priority:  0,
				specOrder: 0,
				source:    PolicySourceExplicit,
			},
			{
				policyDef: &v1beta1.PolicyDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "policy-m", Namespace: namespace},
					Spec: v1beta1.PolicyDefinitionSpec{
						Scope: v1beta1.ApplicationScope,
						Schematic: &common.Schematic{CUE: &common.CUE{Template: `
parameter: {}
config: { enabled: true }
output: { labels: { "order": "second" } }
`}},
					},
				},
				policyRef: v1beta1.AppPolicy{Name: "policy-m", Type: "policy-m"},
				priority:  0,
				specOrder: 1,
				source:    PolicySourceExplicit,
			},
			{
				policyDef: &v1beta1.PolicyDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "policy-a", Namespace: namespace},
					Spec: v1beta1.PolicyDefinitionSpec{
						Scope: v1beta1.ApplicationScope,
						Schematic: &common.Schematic{CUE: &common.CUE{Template: `
parameter: {}
config: { enabled: true }
output: { labels: { "order": "third" } }
`}},
					},
				},
				policyRef: v1beta1.AppPolicy{Name: "policy-a", Type: "policy-a"},
				priority:  0,
				specOrder: 2,
				source:    PolicySourceExplicit,
			},
		}

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: namespace},
			Spec:       v1beta1.ApplicationSpec{},
		}

		h := &AppHandler{Client: k8sClient}
		monCtx := monitorContext.NewTraceContext(ctx, "test")
		results, err := h.renderPoliciesInSequence(monCtx, app, policiesToRender)
		Expect(err).Should(BeNil())

		// All three should be enabled
		Expect(results).Should(HaveLen(3))
		// Must be in spec order: z, m, a — not alphabetical (a, m, z)
		Expect(results[0].PolicyName).Should(Equal("policy-z"))
		Expect(results[1].PolicyName).Should(Equal("policy-m"))
		Expect(results[2].PolicyName).Should(Equal("policy-a"))
	})

	It("Test opt-out annotation prevents global policies", func() {
		// Create global policy
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "global-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global: true,
				Scope:  v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

config: {
  enabled: true
}

output: {
	labels: {
			"should-not-apply": "true"
		}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "global-policy", namespace)

		// Create Application with opt-out annotation
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opt-out-app",
				Namespace: namespace,
				Annotations: map[string]string{
					SkipGlobalPoliciesAnnotation: "true",
				},
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

		// Verify opt-out check
		Expect(shouldSkipGlobalPolicies(app)).Should(BeTrue())

		// Application without annotation
		app2 := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "normal-app",
				Namespace: namespace,
			},
		}
		Expect(shouldSkipGlobalPolicies(app2)).Should(BeFalse())
	})

	It("Test validation prevents explicit reference of global policy", func() {
		// Create global policy
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "global-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global: true,
				Scope:  v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: `parameter: {}`},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "global-policy", namespace)

		// Create non-global policy for comparison
		regularPolicyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "regular-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global: false,
				Scope:  v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: `parameter: {}`},
				},
			},
		}
		Expect(k8sClient.Create(ctx, regularPolicyDef)).Should(Succeed())

		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// Validation should fail for global policy
		err := validateNotGlobalPolicy(monCtx, k8sClient, "global-policy")
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(ContainSubstring("marked as Global"))

		// Validation should pass for regular policy
		err = validateNotGlobalPolicy(monCtx, k8sClient, "regular-policy")
		Expect(err).Should(BeNil())

		// Validation should pass for non-existent policy
		err = validateNotGlobalPolicy(monCtx, k8sClient, "non-existent")
		Expect(err).Should(BeNil())
	})
})
