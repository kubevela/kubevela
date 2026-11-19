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
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Application-scoped policy feature gates", func() {
	namespace := "policy-featuregate-test"
	velaSystem := oam.SystemDefinitionNamespace
	var ctx context.Context

	BeforeEach(func() {
		ctx = util.SetNamespaceInCtx(context.Background(), namespace)
		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// Set client for lazy initialization
		policyScopeIndex.client = k8sClient
		policyScopeIndex.initialized = false
	})

	AfterEach(func() {
		// Clean up policies in test namespace
		policyList := &v1beta1.PolicyDefinitionList{}
		_ = k8sClient.List(ctx, policyList, client.InNamespace(namespace))
		for _, policy := range policyList.Items {
			_ = k8sClient.Delete(ctx, &policy)
		}

		// Clean up policies in vela-system
		velaSystemPolicyList := &v1beta1.PolicyDefinitionList{}
		_ = k8sClient.List(ctx, velaSystemPolicyList, client.InNamespace(velaSystem))
		for _, policy := range velaSystemPolicyList.Items {
			_ = k8sClient.Delete(ctx, &policy)
		}

		// Restore both gates to enabled
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableGlobalPolicies=true")).ToNot(HaveOccurred())
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true")).ToNot(HaveOccurred())
	})

	It("should skip explicit Application-scoped policies when EnableApplicationScopedPolicies=false", func() {
		// Disable Application-scoped policy execution (but keep global discovery enabled)
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=false")).ToNot(HaveOccurred())

		// Create Application-scoped PolicyDefinition
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-label-explicit",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

config: {
  enabled: true
}

output: {
	labels: {
			"test": "gate-disabled"
		}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "add-label-explicit", namespace)

		// Create Application with explicit policy
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.ApplicationKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-explicit-gate",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{Name: "my-comp", Type: "webservice"},
				},
				Policies: []v1beta1.AppPolicy{
					{Name: "test-policy", Type: "add-label-explicit"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		handler := &AppHandler{Client: k8sClient, app: app}
		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify policy was NOT applied (label not added)
		Expect(app.Labels).ShouldNot(HaveKey("test"))
		// Verify no policies in status
		Expect(app.Status.AppliedApplicationPolicies).Should(BeEmpty())
	})

	It("should discover but not apply global policies when EnableGlobalPolicies=true but EnableApplicationScopedPolicies=false", func() {
		// Disable Application-scoped policy execution but keep discovery enabled
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=false")).ToNot(HaveOccurred())

		// Create global Application-scoped PolicyDefinition in vela-system
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "global-add-label",
				Namespace: velaSystem,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope:  v1beta1.ApplicationScope,
				Global: true,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

config: {
  enabled: true
}

output: {
	labels: {
			"global": "policy"
		}
}
`,
					},
				},
			},
		}
		velaCtx := util.SetNamespaceInCtx(context.Background(), velaSystem)
		Expect(k8sClient.Create(velaCtx, policyDef)).Should(Succeed())
		waitForPolicyDef(velaCtx, "global-add-label", velaSystem)

		// Clear in-memory cache to ensure fresh discovery
		applicationPolicyCache.InvalidateAll()

		// Create Application
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.ApplicationKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-global-gate",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{Name: "my-comp", Type: "webservice"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		handler := &AppHandler{Client: k8sClient, app: app}
		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify policy was NOT applied (label not added)
		Expect(app.Labels).ShouldNot(HaveKey("global"))
		// Verify no policies in status
		Expect(app.Status.AppliedApplicationPolicies).Should(BeEmpty())
	})

	It("should not discover global policies when EnableGlobalPolicies=false (even if EnableApplicationScopedPolicies=true)", func() {
		// Disable global policy discovery but keep execution enabled
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableGlobalPolicies=false")).ToNot(HaveOccurred())

		// Create global Application-scoped PolicyDefinition
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "global-no-discovery",
				Namespace: velaSystem,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope:  v1beta1.ApplicationScope,
				Global: true,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

config: {
  enabled: true
}

output: {
	labels: {
			"discovered": "false"
		}
}
`,
					},
				},
			},
		}
		velaCtx := util.SetNamespaceInCtx(context.Background(), velaSystem)
		Expect(k8sClient.Create(velaCtx, policyDef)).Should(Succeed())
		waitForPolicyDef(velaCtx, "global-no-discovery", velaSystem)

		// Clear in-memory cache
		applicationPolicyCache.InvalidateAll()

		// Create Application
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.ApplicationKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-no-discovery",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{Name: "my-comp", Type: "webservice"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		handler := &AppHandler{Client: k8sClient, app: app}
		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify policy was NOT applied (not discovered)
		Expect(app.Labels).ShouldNot(HaveKey("discovered"))
		Expect(app.Status.AppliedApplicationPolicies).Should(BeEmpty())
	})

	It("should apply both global and explicit policies when both gates are enabled", func() {
		// Both gates already enabled in BeforeSuite

		// Create global PolicyDefinition
		globalPolicy := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "global-full-func",
				Namespace: velaSystem,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope:    v1beta1.ApplicationScope,
				Global:   true,
				Priority: 100,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

config: {
  enabled: true
}

output: {
	labels: {
			"global": "applied"
		}
}
`,
					},
				},
			},
		}
		velaCtx := util.SetNamespaceInCtx(context.Background(), velaSystem)
		createPolicyDefAndIndex(velaCtx, globalPolicy)

		// Create explicit PolicyDefinition
		explicitPolicy := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "explicit-full-func",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

config: {
  enabled: true
}

output: {
	labels: {
			"explicit": "applied"
		}
}
`,
					},
				},
			},
		}
		createPolicyDefAndIndex(ctx, explicitPolicy)

		// Clear in-memory cache
		applicationPolicyCache.InvalidateAll()

		// Create Application with explicit policy
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.ApplicationKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-full-func",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{Name: "my-comp", Type: "webservice"},
				},
				Policies: []v1beta1.AppPolicy{
					{Name: "explicit-policy", Type: "explicit-full-func"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		handler := &AppHandler{Client: k8sClient, app: app}
		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify BOTH policies were applied
		Expect(app.Labels).Should(HaveKeyWithValue("global", "applied"))
		Expect(app.Labels).Should(HaveKeyWithValue("explicit", "applied"))
		// Verify status shows both policies
		Expect(app.Status.AppliedApplicationPolicies).Should(HaveLen(2))
	})
})
