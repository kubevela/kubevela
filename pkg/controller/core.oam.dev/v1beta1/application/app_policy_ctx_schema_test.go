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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// A policy declares the shape of what it puts in context.custom with a
// top-level `schema: ctx` block, read by admission and checked at render.
var _ = Describe("Test policy context schemas", func() {
	namespace := "policy-ctx-schema"
	var ctx context.Context

	ctxPolicy := func(name, template string) *v1beta1.PolicyDefinition {
		return &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:    true,
				Scope:     v1beta1.ApplicationScope,
				Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
			},
		}
	}

	// ctx-reader labels a ConfigMap with context.custom.team.
	ctxReader := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "ctx-reader", Namespace: namespace},
		Spec: v1beta1.ComponentDefinitionSpec{
			Workload: common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "v1", Kind: "ConfigMap"}},
			Schematic: &common.Schematic{CUE: &common.CUE{Template: `
parameter: {}
output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: labels: team: context.custom.team
	data: k: "v"
}
`}},
		},
	}

	newApp := func(name string) *v1beta1.Application {
		return &v1beta1.Application{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.ApplicationKind},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{
				Name:       name,
				Type:       "ctx-reader",
				Properties: &runtime.RawExtension{Raw: []byte(`{}`)},
			}}},
		}
	}

	getApp := func(key types.NamespacedName) *v1beta1.Application {
		app := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).Should(Succeed())
		return app
	}

	cmTeam := func(name string) string {
		cm := &corev1.ConfigMap{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, cm); err != nil {
			return "<" + err.Error() + ">"
		}
		return cm.Labels["team"]
	}

	// reconcileUntil reconciles until cond holds, reporting the app's state if it never does.
	reconcileUntil := func(key types.NamespacedName, cond func() bool) {
		GinkgoHelper()
		reconcileUntilOK(key, func() error {
			if cond() {
				return nil
			}
			app := getApp(key)
			return fmt.Errorf("phase=%s conditions=%v cm=%q", app.Status.Phase, app.Status.Conditions, cmTeam(key.Name))
		})
	}

	running := func(key types.NamespacedName, team string) func() bool {
		return func() bool {
			return getApp(key).Status.Phase == common.ApplicationRunning && cmTeam(key.Name) == team
		}
	}

	// failsWith reconciles until a failed condition carries every fragment.
	failsWith := func(key types.NamespacedName, fragments ...string) {
		GinkgoHelper()
		reconcileUntilOK(key, func() error {
			var msgs []string
			for _, c := range getApp(key).Status.Conditions {
				if c.Status != corev1.ConditionFalse {
					continue
				}
				msgs = append(msgs, c.Message)
				all := true
				for _, f := range fragments {
					all = all && strings.Contains(c.Message, f)
				}
				if all {
					return nil
				}
			}
			return fmt.Errorf("no failed condition with %q; failed conditions: %q", fragments, msgs)
		})
	}

	// deploy creates an app reading context.custom.team under the given globals.
	deploy := func(name string, policies ...*v1beta1.PolicyDefinition) types.NamespacedName {
		GinkgoHelper()
		for _, p := range policies {
			createPolicyDefAndIndex(ctx, p)
		}
		app := newApp(name)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		return client.ObjectKeyFromObject(app)
	}

	mandatoryGlobals := func() {
		common2.AllowSkipGlobalPolicies = false
		DeferCleanup(func() { common2.AllowSkipGlobalPolicies = true })
	}

	BeforeEach(func() {
		ctx = util.SetNamespaceInCtx(context.Background(), namespace)
		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		velaSystem := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: oam.SystemDefinitionNamespace}}
		Expect(k8sClient.Create(ctx, &velaSystem)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, ctxReader.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableGlobalPolicies=true")).ToNot(HaveOccurred())
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true")).ToNot(HaveOccurred())
		applicationPolicyCache.InvalidateAll()
		policyScopeIndex.client = k8sClient
		policyScopeIndex.initialized = false
	})

	AfterEach(func() {
		apps := &v1beta1.ApplicationList{}
		_ = k8sClient.List(ctx, apps, client.InNamespace(namespace))
		for i := range apps.Items {
			_ = k8sClient.Delete(ctx, &apps.Items[i])
		}
		policies := &v1beta1.PolicyDefinitionList{}
		_ = k8sClient.List(ctx, policies, client.InNamespace(namespace))
		for i := range policies.Items {
			_ = k8sClient.Delete(ctx, &policies.Items[i])
			policyScopeIndex.Delete(policies.Items[i].Name, namespace)
		}
		applicationPolicyCache.InvalidateAll()
	})

	// Also a downgrade check: unchanged, this passes on a controller without
	// schema support, where a template declaring one renders like any other.
	It("a policy declaring a schema renders and supplies its context", func() {
		createPolicyDefAndIndex(ctx, ctxPolicy("ctx-compat", `
schema: ctx: close({team: =~"^[a-z]+$", meshDomain?: string})
parameter: {}
output: ctx: team: "payments"
`))
		app := newApp("ctx-compat")
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)
		reconcileUntil(key, func() bool {
			return getApp(key).Status.Phase == common.ApplicationRunning && cmTeam("ctx-compat") == "payments"
		})
	})

	Context("a policy's output is checked against its schema", func() {
		BeforeEach(mandatoryGlobals)

		It("a value outside the schema fails the render, naming the policy and the key", func() {
			key := deploy("ctx-bad-value", ctxPolicy("ctx-bad-value", `
schema: ctx: team: =~"^[a-z]+$"
output: ctx: team: "Payments"
`))
			failsWith(key, `"ctx-bad-value"`, "schema", "team")
			Expect(cmTeam("ctx-bad-value")).Should(HavePrefix("<"), "nothing is deployed")
		})

		It("a value of the wrong type fails the render", func() {
			key := deploy("ctx-bad-type", ctxPolicy("ctx-bad-type", `
schema: ctx: {team: string, replicas: int}
output: ctx: {team: "a", replicas: "three"}
`))
			failsWith(key, `"ctx-bad-type"`, "replicas")
		})

		It("a required key the output leaves out fails the render", func() {
			key := deploy("ctx-missing", ctxPolicy("ctx-missing", `
schema: ctx: {team: string, costCentre: string}
output: ctx: team: "payments"
`))
			failsWith(key, `"ctx-missing"`, "costCentre")
		})

		It("a schema whose output sets no ctx at all fails when it requires a key", func() {
			key := deploy("ctx-no-output", ctxPolicy("ctx-no-output", `
schema: ctx: team: string
output: labels: a: "b"
`), ctxPolicy("ctx-no-output-supplier", `output: ctx: team: "payments"`))
			failsWith(key, `"ctx-no-output"`, "team")
		})

		It("an optional key may be left out", func() {
			key := deploy("ctx-optional", ctxPolicy("ctx-optional", `
schema: ctx: {team: string, meshDomain?: string}
output: ctx: team: "payments"
`))
			reconcileUntil(key, running(key, "payments"))
		})

		It("a closed schema refuses a key it does not declare", func() {
			key := deploy("ctx-closed", ctxPolicy("ctx-closed", `
schema: ctx: close({team: string})
output: ctx: {team: "payments", region: "eu"}
`))
			failsWith(key, `"ctx-closed"`, "region")
		})

		It("an open schema lets an undeclared key through", func() {
			key := deploy("ctx-open", ctxPolicy("ctx-open", `
schema: ctx: team: string
output: ctx: {team: "payments", region: "eu"}
`))
			reconcileUntil(key, running(key, "payments"))
		})

		It("a nested value is checked", func() {
			key := deploy("ctx-nested", ctxPolicy("ctx-nested", `
schema: ctx: {team: string, org: unit: =~"^u-"}
output: ctx: {team: "payments", org: unit: "x"}
`))
			failsWith(key, `"ctx-nested"`, "unit")
		})

		It("a schema may use the rest of the template at render", func() {
			key := deploy("ctx-ref", ctxPolicy("ctx-ref", `
_name: =~"^[a-z]+$"
schema: ctx: team: _name
output: ctx: team: "payments"
`))
			reconcileUntil(key, running(key, "payments"))
		})

		It("a standard-library constraint is checked", func() {
			key := deploy("ctx-stdlib", ctxPolicy("ctx-stdlib", `
import "strings"
schema: ctx: team: strings.MinRunes(10)
output: ctx: team: "payments"
`))
			failsWith(key, `"ctx-stdlib"`, "team")
		})

		It("a policy without a schema is not checked", func() {
			key := deploy("ctx-schemaless", ctxPolicy("ctx-schemaless", `output: ctx: {team: "payments", anything: 1}`))
			reconcileUntil(key, running(key, "payments"))
		})

		It("a disabled policy is not checked", func() {
			key := deploy("ctx-disabled",
				ctxPolicy("ctx-disabled", `
config: enabled: false
schema: ctx: costCentre: string
output: ctx: team: "never"
`),
				ctxPolicy("ctx-disabled-supplier", `output: ctx: team: "payments"`))
			reconcileUntil(key, running(key, "payments"))
		})

		It("a passing check leaves the value as the policy set it", func() {
			key := deploy("ctx-default", ctxPolicy("ctx-default", `
schema: ctx: team: *"fallback" | string
output: ctx: team: "payments"
`))
			reconcileUntil(key, running(key, "payments"))
		})

		It("a schema default fills the value components receive when the output leaves it out", func() {
			key := deploy("ctx-fill", ctxPolicy("ctx-fill", `
schema: ctx: {team: *"fallback" | string, zone: string}
output: ctx: zone: "eu"
`))
			reconcileUntil(key, running(key, "fallback"))
		})
	})

	It("with skips allowed, a policy failing its schema is skipped and its value never deployed", func() {
		key := deploy("ctx-skip", ctxPolicy("ctx-skip", `
schema: ctx: team: =~"^[a-z]+$"
output: ctx: team: "Payments"
`))
		// Judged and skipped, rather than not yet reached: then the value can
		// never be deployed, with no waiting to see that it was not.
		reconcileUntil(key, func() bool {
			for _, p := range getApp(key).Status.AppliedApplicationPolicies {
				if p.Name == "ctx-skip" && p.Error && !p.Applied && strings.Contains(p.Message, "output.ctx does not match schema.ctx") {
					return true
				}
			}
			return false
		})
		Expect(cmTeam("ctx-skip")).ShouldNot(Equal("Payments"))
	})
})
