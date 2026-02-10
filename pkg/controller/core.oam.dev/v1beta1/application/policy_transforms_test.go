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

package application

import (
	"context"
	"encoding/json"
	"time"

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
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Helper function to wait for PolicyDefinition to be retrievable
func waitForPolicyDef(ctx context.Context, name, ns string) {
	Eventually(func() error {
		pd := &v1beta1.PolicyDefinition{}
		return k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, pd)
	}, "30s", "500ms").Should(Succeed())

	// Additional wait to ensure appfile.LoadTemplate can find it
	time.Sleep(100 * time.Millisecond)
}

var _ = Describe("Test Application-scoped PolicyDefinition transforms", func() {
	namespace := "policy-transform-test"
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
	})

	AfterEach(func() {
		// Clean up PolicyDefinitions to avoid test pollution
		policyList := &v1beta1.PolicyDefinitionList{}
		_ = k8sClient.List(ctx, policyList, client.InNamespace(namespace))
		for _, policy := range policyList.Items {
			_ = k8sClient.Delete(ctx, &policy)
		}
	})

	It("Test Application-scoped policy with spec merge transform", func() {
		// Create a PolicyDefinition with Application scope
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-test-env",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	envName: string
	envValue: string
}

// Add environment variable to the first component
transforms: {
	spec: {
		type: "merge"
		value: {
			components: [{
				properties: {
					env: [{
						name: parameter.envName
						value: parameter.envValue
					}]
				}
			}]
		}
	}
}

additionalContext: {
	policyApplied: "add-test-env"
	timestamp: "2024-01-01"
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "add-test-env", namespace)
		waitForPolicyDef(ctx, "add-test-env", namespace)

		// Create an Application that uses this policy
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "my-component",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image":"nginx"}`),
						},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "test-policy",
						Type: "add-test-env",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"envName":"TEST_VAR","envValue":"test-value"}`),
						},
					},
				},
			},
		}

		// Create handler and apply transforms
		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")
		resultCtx, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify the transform was applied
		Expect(app.Spec.Components).Should(HaveLen(1))
		var props map[string]interface{}
		Expect(json.Unmarshal(app.Spec.Components[0].Properties.Raw, &props)).Should(Succeed())
		Expect(props).Should(HaveKey("env"))
		envs := props["env"].([]interface{})
		Expect(envs).Should(HaveLen(1))
		env := envs[0].(map[string]interface{})
		Expect(env["name"]).Should(Equal("TEST_VAR"))
		Expect(env["value"]).Should(Equal("test-value"))

		// Verify additionalContext was stored
		additionalCtx := getAdditionalContextFromCtx(resultCtx)
		Expect(additionalCtx).ShouldNot(BeNil())
		Expect(additionalCtx["policyApplied"]).Should(Equal("add-test-env"))
		Expect(additionalCtx["timestamp"]).Should(Equal("2024-01-01"))
	})

	It("Test Application-scoped policy with labels merge", func() {
		// Create a PolicyDefinition that adds labels
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
parameter: {
	team: string
	environment: string
}

transforms: {
	labels: {
		type: "merge"
		value: {
			"team": parameter.team
			"environment": parameter.environment
			"managed-by": "kubevela"
		}
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "add-labels", namespace)

		// Create an Application with existing labels
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.ApplicationKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-labels",
				Namespace: namespace,
				Labels: map[string]string{
					"existing": "label",
				},
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "my-component",
						Type: "webservice",
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "label-policy",
						Type: "add-labels",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"team":"platform","environment":"production"}`),
						},
					},
				},
			},
		}

		// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify labels were merged
		Expect(app.Labels).Should(HaveLen(4))
		Expect(app.Labels["existing"]).Should(Equal("label"))
		Expect(app.Labels["team"]).Should(Equal("platform"))
		Expect(app.Labels["environment"]).Should(Equal("production"))
		Expect(app.Labels["managed-by"]).Should(Equal("kubevela"))
	})

	It("Test Application-scoped policy with enabled=false", func() {
		// Create a PolicyDefinition with conditional application
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "conditional-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	applyPolicy: bool
}

enabled: parameter.applyPolicy

transforms: {
	labels: {
		type: "merge"
		value: {
			"should-not-appear": "true"
		}
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "conditional-policy", namespace)

		// Create an Application with applyPolicy=false
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-conditional",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "my-component",
						Type: "webservice",
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "conditional",
						Type: "conditional-policy",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"applyPolicy":false}`),
						},
					},
				},
			},
		}

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify transform was NOT applied
		Expect(app.Labels).ShouldNot(HaveKey("should-not-appear"))
	})

	It("Test Application-scoped policy with spec replace", func() {
		// Create a PolicyDefinition that replaces the entire spec
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "replace-spec",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	newComponentName: string
}

transforms: {
	spec: {
		type: "replace"
		value: {
			components: [{
				name: parameter.newComponentName
				type: "webservice"
				properties: {
					image: "replaced-image"
				}
			}]
		}
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "replace-spec", namespace)

		// Create an Application
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-replace",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "original-component",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image":"original"}`),
						},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "replace-policy",
						Type: "replace-spec",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"newComponentName":"replaced-component"}`),
						},
					},
				},
			},
		}

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify spec was completely replaced
		Expect(app.Spec.Components).Should(HaveLen(1))
		Expect(app.Spec.Components[0].Name).Should(Equal("replaced-component"))
		var props map[string]interface{}
		Expect(json.Unmarshal(app.Spec.Components[0].Properties.Raw, &props)).Should(Succeed())
		Expect(props["image"]).Should(Equal("replaced-image"))
	})

	It("Test non-Application-scoped policy is skipped", func() {
		// Create a regular PolicyDefinition without Application scope
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "regular-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				// No Scope specified - should be treated as regular resource-generating policy
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
output: {
	apiVersion: "v1"
	kind: "ConfigMap"
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "regular-policy", namespace)

		// Create an Application
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-skip",
				Namespace: namespace,
				Labels: map[string]string{
					"original": "value",
				},
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "my-component",
						Type: "webservice",
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "regular",
						Type: "regular-policy",
					},
				},
			},
		}

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify app was not modified
		Expect(app.Labels).Should(HaveLen(1))
		Expect(app.Labels["original"]).Should(Equal("value"))
	})

	It("Test Application-scoped policy with CueX kube.#Get action", func() {
		// Create a test ConfigMap that the policy will read
		testConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-data-cm",
				Namespace: namespace,
			},
			Data: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		}
		Expect(k8sClient.Create(ctx, testConfigMap)).Should(Succeed())

		// Create PolicyDefinition with CueX kube.#Get action
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cuex-read-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
import "vela/kube"

parameter: {}
enabled: true

// Use kube.#Get to read a ConfigMap from the cluster
output: kube.#Get & {
  $params: {
    cluster: ""
    resource: {
      apiVersion: "v1"
      kind: "ConfigMap"
      metadata: {
        name: "test-data-cm"
        namespace: "` + namespace + `"
      }
    }
  }
}

// The ConfigMap data will be available in additionalContext
additionalContext: {
	configMapData: output.$returns.data
}

// Add a label with data from the ConfigMap
transforms: {
	labels: {
		type: "merge"
		value: {
			"from-configmap": output.$returns.data.key1
		}
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "cuex-read-policy", namespace)

		// Create Application that uses the policy
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-cuex",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "my-component",
						Type: "webservice",
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "cuex-reader",
						Type: "cuex-read-policy",
					},
				},
			},
		}

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify the CueX kube.#Get action executed successfully by checking:
		// 1. No error occurred (CueX action executed)
		// 2. The label transform was applied (using data from the ConfigMap)
		Expect(app.Labels).ShouldNot(BeNil())
		Expect(app.Labels["from-configmap"]).Should(Equal("value1"))
	})

	It("Test policy additionalContext is available to components as context.custom", func() {
		// Create a ConfigMap that the policy will read
		testCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-data-cm",
				Namespace: namespace,
			},
			Data: map[string]string{
				"apiEndpoint": "https://api.example.com",
				"region":      "us-west-2",
			},
		}
		Expect(k8sClient.Create(ctx, testCM)).Should(Succeed())

		// Create PolicyDefinition that reads ConfigMap and exposes it via additionalContext
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fetch-config-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
import "vela/kube"

parameter: {}
enabled: true

output: kube.#Get & {
  $params: {
    cluster: ""
    resource: {
      apiVersion: "v1"
      kind: "ConfigMap"
      metadata: {
        name: "policy-data-cm"
        namespace: "` + namespace + `"
      }
    }
  }
}

// Expose ConfigMap data via additionalContext so components can access it
additionalContext: {
  config: {
    endpoint: output.$returns.data.apiEndpoint
    region: output.$returns.data.region
  }
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "fetch-config-policy", namespace)

		// Create ComponentDefinition that uses context.custom
		compDef := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "custom-context-comp",
				Namespace: namespace,
			},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload: common.WorkloadTypeDescriptor{
					Definition: common.WorkloadGVK{
						APIVersion: "v1",
						Kind:       "ConfigMap",
					},
				},
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
import "encoding/json"

output: {
  apiVersion: "v1"
  kind: "ConfigMap"
  metadata: {
    name: context.name
    namespace: context.namespace
  }
  data: {
    // Access policy additionalContext via context.custom
    "from-policy": json.Marshal(context.custom.config)
  }
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, compDef)).Should(Succeed())

		// Create Application with the policy and component
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-custom-context",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Policies: []v1beta1.AppPolicy{
					{
						Name: "fetch-config",
						Type: "fetch-config-policy",
					},
				},
				Components: []common.ApplicationComponent{
					{
						Name: "my-component",
						Type: "custom-context-comp",
					},
				},
			},
		}

		// Apply Application-scoped policy transforms
		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test-custom-context")
		monCtx, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify additionalContext is stored in the Go context
		// This data will be available to components via context.custom when rendering
		const policyAdditionalContextKeyString = "kubevela.oam.dev/policy-additional-context"

		additionalCtx := monCtx.GetContext().Value(policyAdditionalContextKeyString)
		Expect(additionalCtx).ShouldNot(BeNil())

		// Cast and verify the structure
		ctxMap, ok := additionalCtx.(map[string]interface{})
		Expect(ok).Should(BeTrue())
		Expect(ctxMap).Should(HaveKey("config"))

		config, ok := ctxMap["config"].(map[string]interface{})
		Expect(ok).Should(BeTrue())
		Expect(config["endpoint"]).Should(Equal("https://api.example.com"))
		Expect(config["region"]).Should(Equal("us-west-2"))

		// This additionalContext will be extracted by process.NewContext(), wrapped under
		// "custom" key, and made available to component/trait templates as context.custom.config
	})
})

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

		// Clear cache before each test
		globalPolicyCache.InvalidateAll()
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
		Expect(globalPolicyCache.Size()).Should(Equal(0))

		// Test that cache can be cleared
		globalPolicyCache.InvalidateAll()
		Expect(globalPolicyCache.Size()).Should(Equal(0))
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
				Transforms: &PolicyTransforms{
					Labels: &Transform{
						Type: "merge",
						Value: map[string]interface{}{
							"test": "value",
						},
					},
				},
				AdditionalContext: map[string]interface{}{
					"key": "value",
				},
			},
		}

		// Set in cache
		err := globalPolicyCache.Set(app, results, "test-hash")
		Expect(err).Should(BeNil())

		// Verify cache size
		Expect(globalPolicyCache.Size()).Should(Equal(1))

		// Get from cache
		cached, hit, err := globalPolicyCache.Get(app, "test-hash")
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
		err := globalPolicyCache.Set(app, results, "hash1")
		Expect(err).Should(BeNil())

		// Verify cache hit
		cached, hit, err := globalPolicyCache.Get(app, "hash1")
		Expect(err).Should(BeNil())
		Expect(hit).Should(BeTrue())
		Expect(cached).Should(HaveLen(1))

		// Modify app spec
		app.Spec.Components = append(app.Spec.Components, common.ApplicationComponent{
			Name: "new-component",
			Type: "worker",
		})

		// Cache should miss (spec hash changed)
		cached, hit, err = globalPolicyCache.Get(app, "hash1")
		Expect(err).Should(BeNil())
		Expect(hit).Should(BeFalse())
		Expect(cached).Should(BeNil())
	})

	It("Test cache invalidation when global policy hash changes", func() {
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

		// Cache with original policy hash
		err := globalPolicyCache.Set(app, results, "old-policy-hash")
		Expect(err).Should(BeNil())

		// Verify cache hit with same hash
		cached, hit, err := globalPolicyCache.Get(app, "old-policy-hash")
		Expect(err).Should(BeNil())
		Expect(hit).Should(BeTrue())

		// Try to get with different policy hash (policy changed)
		cached, hit, err = globalPolicyCache.Get(app, "new-policy-hash")
		Expect(err).Should(BeNil())
		Expect(hit).Should(BeFalse())
		Expect(cached).Should(BeNil())
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
				Transforms: &PolicyTransforms{
					Labels: &Transform{
						Type: "merge",
						Value: map[string]interface{}{
							"from-policy1": "value1",
						},
					},
				},
			},
			{
				PolicyName:      "policy2",
				PolicyNamespace: namespace,
				Enabled:         true,
				Transforms: &PolicyTransforms{
					Labels: &Transform{
						Type: "merge",
						Value: map[string]interface{}{
							"from-policy2": "value2",
						},
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
		err := globalPolicyCache.Set(app, results, "multi-policy-hash")
		Expect(err).Should(BeNil())

		// Get from cache
		cached, hit, err := globalPolicyCache.Get(app, "multi-policy-hash")
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
		err := globalPolicyCache.Set(app1, results, "hash1")
		Expect(err).Should(BeNil())
		err = globalPolicyCache.Set(app2, results, "hash1")
		Expect(err).Should(BeNil())

		Expect(globalPolicyCache.Size()).Should(Equal(2))

		// Invalidate namespace
		globalPolicyCache.InvalidateForNamespace(namespace)

		// Both should be invalidated
		Expect(globalPolicyCache.Size()).Should(Equal(0))
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
		err := globalPolicyCache.Set(app, results, "hash1")
		Expect(err).Should(BeNil())

		// Verify cache hit immediately
		cached, hit, err := globalPolicyCache.Get(app, "hash1")
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
		err := globalPolicyCache.Set(app1, results, "hash1")
		Expect(err).Should(BeNil())
		err = globalPolicyCache.Set(app2, results, "hash1")
		Expect(err).Should(BeNil())

		Expect(globalPolicyCache.Size()).Should(Equal(2))

		// Invalidate only app1
		globalPolicyCache.InvalidateApplication(namespace, "app1")

		Expect(globalPolicyCache.Size()).Should(Equal(1))

		// app1 should miss
		_, hit, _ := globalPolicyCache.Get(app1, "hash1")
		Expect(hit).Should(BeFalse())

		// app2 should still hit
		_, hit, _ = globalPolicyCache.Get(app2, "hash1")
		Expect(hit).Should(BeTrue())
	})
})

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

		// Clear cache
		globalPolicyCache.InvalidateAll()
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
enabled: true

transforms: {
	labels: {
		type: "merge"
		value: {
			"vela-system-global": "true"
		}
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "vela-system-global", velaSystem)

		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// Discover global policies from vela-system
		policies, err := discoverGlobalPolicies(monCtx, k8sClient, velaSystem)
		Expect(err).Should(BeNil())
		Expect(policies).Should(HaveLen(1))
		Expect(policies[0].Name).Should(Equal("vela-system-global"))
		Expect(policies[0].Spec.Global).Should(BeTrue())
		Expect(policies[0].Spec.Priority).Should(Equal(int32(100)))
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
enabled: true

transforms: {
	labels: {
		type: "merge"
		value: {
			"namespace-global": "true"
		}
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "namespace-global", namespace)

		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// Discover from namespace
		policies, err := discoverGlobalPolicies(monCtx, k8sClient, namespace)
		Expect(err).Should(BeNil())
		Expect(policies).Should(HaveLen(1))
		Expect(policies[0].Name).Should(Equal("namespace-global"))

		// Discover from vela-system (should not include namespace policy)
		velaSystemPolicies, err := discoverGlobalPolicies(monCtx, k8sClient, velaSystem)
		Expect(err).Should(BeNil())
		// Should not include namespace-global policy
		for _, p := range velaSystemPolicies {
			Expect(p.Name).ShouldNot(Equal("namespace-global"))
		}
	})

	It("Test priority ordering - higher priority runs first", func() {
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

		monCtx := monitorContext.NewTraceContext(ctx, "test")
		policies, err := discoverGlobalPolicies(monCtx, k8sClient, namespace)
		Expect(err).Should(BeNil())
		Expect(policies).Should(HaveLen(3))

		// Verify order: high-priority (100), medium-priority (50), low-priority (10)
		Expect(policies[0].Name).Should(Equal("high-priority"))
		Expect(policies[0].Spec.Priority).Should(Equal(int32(100)))
		Expect(policies[1].Name).Should(Equal("medium-priority"))
		Expect(policies[1].Spec.Priority).Should(Equal(int32(50)))
		Expect(policies[2].Name).Should(Equal("low-priority"))
		Expect(policies[2].Spec.Priority).Should(Equal(int32(10)))
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

		monCtx := monitorContext.NewTraceContext(ctx, "test")
		policies, err := discoverGlobalPolicies(monCtx, k8sClient, namespace)
		Expect(err).Should(BeNil())
		Expect(policies).Should(HaveLen(3))

		// Verify alphabetical order: policy-a, policy-b, policy-c
		Expect(policies[0].Name).Should(Equal("policy-a"))
		Expect(policies[1].Name).Should(Equal("policy-b"))
		Expect(policies[2].Name).Should(Equal("policy-c"))
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
enabled: true

transforms: {
	labels: {
		type: "merge"
		value: {
			"should-not-apply": "true"
		}
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
		err := validateNotGlobalPolicy(monCtx, k8sClient, "global-policy", namespace)
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(ContainSubstring("marked as Global"))

		// Validation should pass for regular policy
		err = validateNotGlobalPolicy(monCtx, k8sClient, "regular-policy", namespace)
		Expect(err).Should(BeNil())

		// Validation should pass for non-existent policy
		err = validateNotGlobalPolicy(monCtx, k8sClient, "non-existent", namespace)
		Expect(err).Should(BeNil())
	})
})

var _ = Describe("Test Application-scoped Policy Rendering", func() {
	namespace := "rendering-test"
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
	})

	AfterEach(func() {
		// Clean up PolicyDefinitions to avoid test pollution
		policyList := &v1beta1.PolicyDefinitionList{}
		_ = k8sClient.List(ctx, policyList, client.InNamespace(namespace))
		for _, policy := range policyList.Items {
			_ = k8sClient.Delete(ctx, &policy)
		}
	})

	It("Test annotation transforms with merge", func() {
		// Create a PolicyDefinition that adds annotations
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-annotations",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

transforms: {
	annotations: {
		type: "merge"
		value: {
			"policy.oam.dev/applied": "true"
			"policy.oam.dev/version": "v1"
		}
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "add-annotations", namespace)

		// Create an Application with existing annotations
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-annotations",
				Namespace: namespace,
				Annotations: map[string]string{
					"existing": "annotation",
				},
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "component",
						Type: "webservice",
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "annotation-policy",
						Type: "add-annotations",
					},
				},
			},
		}

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify annotations were merged
		Expect(app.Annotations).Should(HaveLen(3))
		Expect(app.Annotations["existing"]).Should(Equal("annotation"))
		Expect(app.Annotations["policy.oam.dev/applied"]).Should(Equal("true"))
		Expect(app.Annotations["policy.oam.dev/version"]).Should(Equal("v1"))
	})

	It("Test context.application access in CUE template", func() {
		// Create a PolicyDefinition that uses context.application
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "context-aware-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
import "strings"

parameter: {}

// Access application metadata from context
enabled: true

transforms: {
	labels: {
		type: "merge"
		value: {
			"app-name": context.application.metadata.name
			"app-namespace": context.application.metadata.namespace
			"app-name-upper": strings.ToUpper(context.application.metadata.name)
		}
	}
}

additionalContext: {
	originalAppName: context.application.metadata.name
	componentCount: len(context.application.spec.components)
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "context-aware-policy", namespace)

		// Create an Application
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.ApplicationKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-test-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{Name: "component1", Type: "webservice"},
					{Name: "component2", Type: "worker"},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name: "context-policy",
						Type: "context-aware-policy",
					},
				},
			},
		}

		// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")
		resultCtx, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify labels from context
		Expect(app.Labels["app-name"]).Should(Equal("my-test-app"))
		Expect(app.Labels["app-namespace"]).Should(Equal(namespace))
		Expect(app.Labels["app-name-upper"]).Should(Equal("MY-TEST-APP"))

		// Verify additionalContext from context
		additionalCtx := getAdditionalContextFromCtx(resultCtx)
		Expect(additionalCtx).ShouldNot(BeNil())
		Expect(additionalCtx["originalAppName"]).Should(Equal("my-test-app"))
		// CUE's len() returns int64, not float64
		Expect(additionalCtx["componentCount"]).Should(Equal(int64(2)))
	})

	It("Test renderPolicy function extracts all fields correctly", func() {
		// Create a comprehensive PolicyDefinition
		policyDef := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "comprehensive-policy",
				Namespace: namespace,
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	shouldApply: bool
}

enabled: parameter.shouldApply

transforms: {
	labels: {
		type: "merge"
		value: {
			"from-render": "true"
		}
	}
}

additionalContext: {
	rendered: true
	policyName: "comprehensive-policy"
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "comprehensive-policy", namespace)

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

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// Test with enabled=true
		policyRef := v1beta1.AppPolicy{
			Name: "test-policy",
			Type: "comprehensive-policy",
			Properties: &runtime.RawExtension{
				Raw: []byte(`{"shouldApply":true}`),
			},
		}

		result, err := handler.renderPolicy(monCtx, app, policyRef, policyDef)
		Expect(err).Should(BeNil())
		Expect(result.PolicyName).Should(Equal("comprehensive-policy"))
		Expect(result.PolicyNamespace).Should(Equal(namespace))
		Expect(result.Enabled).Should(BeTrue())
		Expect(result.Transforms).ShouldNot(BeNil())
		Expect(result.AdditionalContext).ShouldNot(BeNil())
		Expect(result.AdditionalContext["rendered"]).Should(Equal(true))
		Expect(result.AdditionalContext["policyName"]).Should(Equal("comprehensive-policy"))

		// Test with enabled=false
		policyRefDisabled := v1beta1.AppPolicy{
			Name: "test-policy-disabled",
			Type: "comprehensive-policy",
			Properties: &runtime.RawExtension{
				Raw: []byte(`{"shouldApply":false}`),
			},
		}

		resultDisabled, err := handler.renderPolicy(monCtx, app, policyRefDisabled, policyDef)
		Expect(err).Should(BeNil())
		Expect(resultDisabled.Enabled).Should(BeFalse())
		Expect(resultDisabled.SkipReason).Should(Equal("enabled=false"))
	})

	It("Test applyRenderedPolicyResult applies cached transforms correctly", func() {
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

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// Create a RenderedPolicyResult with transforms
		renderedResult := RenderedPolicyResult{
			PolicyName:      "cached-policy",
			PolicyNamespace: namespace,
			Enabled:         true,
			Transforms: &PolicyTransforms{
				Labels: &Transform{
					Type: "merge",
					Value: map[string]interface{}{
						"cached": "true",
						"source": "rendered-result",
					},
				},
			},
			AdditionalContext: map[string]interface{}{
				"fromCache": true,
				"timestamp": "2024-01-01",
			},
		}

		// Apply the rendered result
		resultCtx, _, err := handler.applyRenderedPolicyResult(monCtx, app, renderedResult, 1, 100)
		Expect(err).Should(BeNil())

		// Verify labels were applied
		Expect(app.Labels["cached"]).Should(Equal("true"))
		Expect(app.Labels["source"]).Should(Equal("rendered-result"))

		// Verify additionalContext was stored
		additionalCtx := getAdditionalContextFromCtx(resultCtx)
		Expect(additionalCtx).ShouldNot(BeNil())
		Expect(additionalCtx["fromCache"]).Should(Equal(true))
		Expect(additionalCtx["timestamp"]).Should(Equal("2024-01-01"))

		// Verify status was recorded
		Expect(app.Status.AppliedApplicationPolicies).Should(HaveLen(1))
		Expect(app.Status.AppliedApplicationPolicies[0].Name).Should(Equal("cached-policy"))
		Expect(app.Status.AppliedApplicationPolicies[0].Applied).Should(BeTrue())
	})

	It("Test applyRenderedPolicyResult skips disabled policies", func() {
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

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// Create a disabled RenderedPolicyResult
		renderedResult := RenderedPolicyResult{
			PolicyName:      "disabled-policy",
			PolicyNamespace: namespace,
			Enabled:         false,
			SkipReason:      "enabled=false",
		}

		// Apply the rendered result
		_, _, err := handler.applyRenderedPolicyResult(monCtx, app, renderedResult, 1, 100)
		Expect(err).Should(BeNil())

		// Verify no labels were applied
		Expect(app.Labels).Should(BeEmpty())

		// Verify status shows skipped
		Expect(app.Status.AppliedApplicationPolicies).Should(HaveLen(1))
		Expect(app.Status.AppliedApplicationPolicies[0].Name).Should(Equal("disabled-policy"))
		Expect(app.Status.AppliedApplicationPolicies[0].Applied).Should(BeFalse())
		Expect(app.Status.AppliedApplicationPolicies[0].Reason).Should(Equal("enabled=false"))
	})

	It("Test applyRenderedPolicyResult tracks label changes in status", func() {
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

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// Create a RenderedPolicyResult with label transforms
		renderedResult := RenderedPolicyResult{
			PolicyName:      "label-policy",
			PolicyNamespace: namespace,
			Enabled:         true,
			Transforms: &PolicyTransforms{
				Labels: &Transform{
					Type: "merge",
					Value: map[string]interface{}{
						"added-by":    "policy",
						"environment": "test",
					},
				},
			},
		}

		// Apply the rendered result
		_, _, err := handler.applyRenderedPolicyResult(monCtx, app, renderedResult, 1, 100)
		Expect(err).Should(BeNil())

		// Verify status tracks summary counts of what labels were added
		Expect(app.Status.AppliedApplicationPolicies).Should(HaveLen(1))
		Expect(app.Status.AppliedApplicationPolicies[0].Applied).Should(BeTrue())
		Expect(app.Status.AppliedApplicationPolicies[0].LabelsCount).Should(Equal(2))
		// Full label details are stored in ConfigMap, status only has counts
		Expect(app.Labels["added-by"]).Should(Equal("policy"))
		Expect(app.Labels["environment"]).Should(Equal("test"))
	})

	It("Test applyRenderedPolicyResult tracks annotation changes in status", func() {
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

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// Create a RenderedPolicyResult with annotation transforms
		renderedResult := RenderedPolicyResult{
			PolicyName:      "annotation-policy",
			PolicyNamespace: namespace,
			Enabled:         true,
			Transforms: &PolicyTransforms{
				Annotations: &Transform{
					Type: "merge",
					Value: map[string]interface{}{
						"policy.oam.dev/applied": "true",
						"policy.oam.dev/version": "v1.0",
					},
				},
			},
		}

		// Apply the rendered result
		_, _, err := handler.applyRenderedPolicyResult(monCtx, app, renderedResult, 1, 100)
		Expect(err).Should(BeNil())

		// Verify status tracks summary counts of what annotations were added
		Expect(app.Status.AppliedApplicationPolicies).Should(HaveLen(1))
		Expect(app.Status.AppliedApplicationPolicies[0].Applied).Should(BeTrue())
		Expect(app.Status.AppliedApplicationPolicies[0].AnnotationsCount).Should(Equal(2))
		// Full annotation details are stored in ConfigMap, status only has counts
		Expect(app.Annotations["policy.oam.dev/applied"]).Should(Equal("true"))
		Expect(app.Annotations["policy.oam.dev/version"]).Should(Equal("v1.0"))
	})

	It("Test applyRenderedPolicyResult tracks additionalContext in status", func() {
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

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// Create a RenderedPolicyResult with additionalContext
		renderedResult := RenderedPolicyResult{
			PolicyName:      "context-policy",
			PolicyNamespace: namespace,
			Enabled:         true,
			AdditionalContext: map[string]interface{}{
				"policyApplied": "context-policy",
				"timestamp":     "2024-01-01",
				"configHash":    "abc123",
			},
		}

		// Apply the rendered result
		_, _, err := handler.applyRenderedPolicyResult(monCtx, app, renderedResult, 1, 100)
		Expect(err).Should(BeNil())

		// Verify status tracks presence of additionalContext (full details in ConfigMap)
		Expect(app.Status.AppliedApplicationPolicies).Should(HaveLen(1))
		Expect(app.Status.AppliedApplicationPolicies[0].Applied).Should(BeTrue())
		Expect(app.Status.AppliedApplicationPolicies[0].HasContext).Should(BeTrue())
		// Full context details are stored in ConfigMap, status only has boolean flag
	})

	It("Test applyRenderedPolicyResult tracks spec modification in status", func() {
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

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// Create a RenderedPolicyResult with spec transform
		renderedResult := RenderedPolicyResult{
			PolicyName:      "spec-policy",
			PolicyNamespace: namespace,
			Enabled:         true,
			Transforms: &PolicyTransforms{
				Spec: &Transform{
					Type:  "merge",
					Value: map[string]interface{}{},
				},
			},
		}

		// Apply the rendered result
		_, _, err := handler.applyRenderedPolicyResult(monCtx, app, renderedResult, 1, 100)
		Expect(err).Should(BeNil())

		// Verify status tracks spec modification
		Expect(app.Status.AppliedApplicationPolicies).Should(HaveLen(1))
		Expect(app.Status.AppliedApplicationPolicies[0].Applied).Should(BeTrue())
		Expect(app.Status.AppliedApplicationPolicies[0].SpecModified).Should(BeTrue())
	})

	It("Test applyRenderedPolicyResult tracks all changes together", func() {
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

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test")

		// Create a comprehensive RenderedPolicyResult
		renderedResult := RenderedPolicyResult{
			PolicyName:      "comprehensive-policy",
			PolicyNamespace: namespace,
			Enabled:         true,
			Transforms: &PolicyTransforms{
				Labels: &Transform{
					Type: "merge",
					Value: map[string]interface{}{
						"team": "platform",
					},
				},
				Annotations: &Transform{
					Type: "merge",
					Value: map[string]interface{}{
						"policy.oam.dev/applied": "true",
					},
				},
				Spec: &Transform{
					Type:  "merge",
					Value: map[string]interface{}{},
				},
			},
			AdditionalContext: map[string]interface{}{
				"applied": true,
			},
		}

		// Apply the rendered result
		_, _, err := handler.applyRenderedPolicyResult(monCtx, app, renderedResult, 1, 100)
		Expect(err).Should(BeNil())

		// Verify status tracks summary counts of all changes
		Expect(app.Status.AppliedApplicationPolicies).Should(HaveLen(1))
		policy := app.Status.AppliedApplicationPolicies[0]
		Expect(policy.Applied).Should(BeTrue())
		Expect(policy.LabelsCount).Should(Equal(1))
		Expect(policy.AnnotationsCount).Should(Equal(1))
		Expect(policy.SpecModified).Should(BeTrue())
		Expect(policy.HasContext).Should(BeTrue())
		// Full details are stored in ConfigMap, status only has counts
		Expect(app.Labels["team"]).Should(Equal("platform"))
		Expect(app.Annotations["policy.oam.dev/applied"]).Should(Equal("true"))
	})

	Context("Test spec diff tracking with ConfigMap storage", func() {
		It("Test spec diff tracking stores diffs in ConfigMap", func() {
			// Create a global policy that modifies spec
			policyDef := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "modify-spec-policy",
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

enabled: true

transforms: {
	spec: {
		type: "merge"
		value: {
			components: [{
				properties: {
					replicas: 3
				}
			}]
		}
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "modify-spec-policy", namespace)

			// Create Application with initial spec
			app := &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-diff-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "main-component",
							Type: "webservice",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"image":"nginx","replicas":1}`),
							},
						},
					},
				},
			}

			// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			handler := &AppHandler{
				Client: k8sClient,
				app:    app,
			}

			monCtx := monitorContext.NewTraceContext(ctx, "test-trace")
			_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
			Expect(err).Should(BeNil())

			// Verify ConfigMap reference is set in status
			Expect(app.Status.ApplicationPoliciesConfigMap).ShouldNot(BeEmpty())
			expectedCMName := "application-policies-" + namespace + "-test-diff-app"
			Expect(app.Status.ApplicationPoliciesConfigMap).Should(Equal(expectedCMName))

			// Verify ConfigMap exists
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      expectedCMName,
				Namespace: namespace,
			}, cm)
			Expect(err).Should(BeNil())

			// Verify sequence-prefixed key exists
			Expect(cm.Data).Should(HaveKey("001-modify-spec-policy"))

			// Verify it's valid JSON
			var diff map[string]interface{}
			err = json.Unmarshal([]byte(cm.Data["001-modify-spec-policy"]), &diff)
			Expect(err).Should(BeNil())

			// Verify OwnerReference points to Application
			Expect(cm.OwnerReferences).Should(HaveLen(1))
			Expect(cm.OwnerReferences[0].Name).Should(Equal("test-diff-app"))
			Expect(cm.OwnerReferences[0].UID).Should(Equal(app.UID))
			Expect(*cm.OwnerReferences[0].Controller).Should(BeTrue())

			// Verify status has summary information (sequence/priority are in ConfigMap)
			Expect(app.Status.AppliedApplicationPolicies).Should(HaveLen(1))
			Expect(app.Status.AppliedApplicationPolicies[0].SpecModified).Should(BeTrue())
			// Sequence and priority are in ConfigMap data, not in status
		})

		It("Test multiple policies create ordered diffs in ConfigMap", func() {
			// Create 3 global policies with different priorities
			policy1 := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-first",
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
enabled: true
transforms: {
	spec: {
		type: "merge"
		value: {
			components: [{
				properties: {
					cpu: "100m"
				}
			}]
		}
	}
}
`,
						},
					},
				},
			}

			policy2 := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-second",
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
enabled: true
transforms: {
	spec: {
		type: "merge"
		value: {
			components: [{
				properties: {
					memory: "256Mi"
				}
			}]
		}
	}
}
`,
						},
					},
				},
			}

			policy3 := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-third",
					Namespace: namespace,
				},
				Spec: v1beta1.PolicyDefinitionSpec{
					Global:   true,
					Priority: 10,
					Scope:    v1beta1.ApplicationScope,
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {}
enabled: true
transforms: {
	spec: {
		type: "merge"
		value: {
			components: [{
				properties: {
					replicas: 5
				}
			}]
		}
	}
}
`,
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, policy1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, policy2)).Should(Succeed())
			Expect(k8sClient.Create(ctx, policy3)).Should(Succeed())

			app := &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-multi-diff-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "main",
							Type: "webservice",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"image":"nginx"}`),
							},
						},
					},
				},
			}

			// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			handler := &AppHandler{
				Client: k8sClient,
				app:    app,
			}

			monCtx := monitorContext.NewTraceContext(ctx, "test-trace")
			_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
			Expect(err).Should(BeNil())

			// Verify ConfigMap has ordered keys
			cm := &corev1.ConfigMap{}
			expectedCMName := "application-policies-" + namespace + "-test-multi-diff-app"
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      expectedCMName,
				Namespace: namespace,
			}, cm)
			Expect(err).Should(BeNil())

			// Verify keys are in execution order (sequence prefix)
			Expect(cm.Data).Should(HaveKey("001-policy-first"))
			Expect(cm.Data).Should(HaveKey("002-policy-second"))
			Expect(cm.Data).Should(HaveKey("003-policy-third"))

			// Verify status records (sequence/priority are in ConfigMap, not status)
			Expect(app.Status.AppliedApplicationPolicies).Should(HaveLen(3))
			Expect(app.Status.AppliedApplicationPolicies[0].Name).Should(Equal("policy-first"))
			Expect(app.Status.AppliedApplicationPolicies[1].Name).Should(Equal("policy-second"))
			Expect(app.Status.AppliedApplicationPolicies[2].Name).Should(Equal("policy-third"))
			// Sequence and priority are stored in ConfigMap JSON data, not in status
		})

		It("Test ConfigMap is not created when no spec modifications", func() {
			// Create policy that only adds labels (no spec change)
			policyDef := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels-only-policy",
					Namespace: namespace,
				},
				Spec: v1beta1.PolicyDefinitionSpec{
					Global: true,
					Scope:  v1beta1.ApplicationScope,
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {}

enabled: true

transforms: {
	labels: {
		type: "merge"
		value: {
			"team": "platform"
		}
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "labels-only-policy", namespace)

			app := &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-no-diff-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "main",
							Type: "webservice",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"image":"nginx"}`),
							},
						},
					},
				},
			}

			// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			handler := &AppHandler{
				Client: k8sClient,
				app:    app,
			}

			monCtx := monitorContext.NewTraceContext(ctx, "test-trace")
			_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
			Expect(err).Should(BeNil())

			// Verify policy was applied
			Expect(app.Status.AppliedApplicationPolicies).Should(HaveLen(1))
			Expect(app.Status.AppliedApplicationPolicies[0].SpecModified).Should(BeFalse())
			Expect(app.Status.AppliedApplicationPolicies[0].LabelsCount).Should(Equal(1))
			Expect(app.Labels["team"]).Should(Equal("platform"))

			// ConfigMap IS created even without spec modifications (stores all transforms)
			Expect(app.Status.ApplicationPoliciesConfigMap).ShouldNot(BeEmpty())
			expectedCMName := "application-policies-" + namespace + "-test-no-diff-app"
			Expect(app.Status.ApplicationPoliciesConfigMap).Should(Equal(expectedCMName))
		})

		It("Test spec diff contains meaningful change information", func() {
			// Create a policy that makes multiple types of changes
			policyDef := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "complex-changes-policy",
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

enabled: true

transforms: {
	spec: {
		type: "merge"
		value: {
			components: [{
				properties: {
					replicas: 3
					cpu: "200m"
					memory: "512Mi"
					env: [{
						name: "LOG_LEVEL"
						value: "debug"
					}]
				}
			}]
		}
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "complex-changes-policy", namespace)

			app := &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-complex-diff-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "main",
							Type: "webservice",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"image":"nginx","replicas":1}`),
							},
						},
					},
				},
			}

			// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			handler := &AppHandler{
				Client: k8sClient,
				app:    app,
			}

			monCtx := monitorContext.NewTraceContext(ctx, "test-trace")
			_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
			Expect(err).Should(BeNil())

			// Get the ConfigMap with diffs
			cm := &corev1.ConfigMap{}
			expectedCMName := "application-policies-" + namespace + "-test-complex-diff-app"
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      expectedCMName,
				Namespace: namespace,
			}, cm)
			Expect(err).Should(BeNil())

			// Verify diff exists
			Expect(cm.Data).Should(HaveKey("001-complex-changes-policy"))
			diffJSON := cm.Data["001-complex-changes-policy"]

			// Parse the diff (JSON Merge Patch format)
			var diff map[string]interface{}
			err = json.Unmarshal([]byte(diffJSON), &diff)
			Expect(err).Should(BeNil())

			// Verify diff is not empty (contains actual changes)
			Expect(diff).ShouldNot(BeEmpty())

			// The diff should contain transforms with spec changes
			Expect(diff).Should(HaveKey("transforms"))
			transforms, ok := diff["transforms"].(map[string]interface{})
			Expect(ok).Should(BeTrue())
			Expect(transforms).Should(HaveKey("spec"))
			spec, ok := transforms["spec"].(map[string]interface{})
			Expect(ok).Should(BeTrue())
			Expect(spec).Should(HaveKey("value"))
			value, ok := spec["value"].(map[string]interface{})
			Expect(ok).Should(BeTrue())
			Expect(value).Should(HaveKey("components"))
		})

		It("Test ConfigMap updates when policies change", func() {
			// Create initial policy
			policyDef := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "updateable-policy",
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
enabled: true
transforms: {
	spec: {
		type: "merge"
		value: {
			components: [{
				properties: {
					replicas: 2
				}
			}]
		}
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "updateable-policy", namespace)

			app := &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-update-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "main",
							Type: "webservice",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"image":"nginx","replicas":1}`),
							},
						},
					},
				},
			}

			// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			handler := &AppHandler{
				Client: k8sClient,
				app:    app,
			}

			// First reconciliation
			monCtx := monitorContext.NewTraceContext(ctx, "test-trace")
			_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
			Expect(err).Should(BeNil())

			// Verify ConfigMap was created
			cm := &corev1.ConfigMap{}
			expectedCMName := "application-policies-" + namespace + "-test-update-app"
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      expectedCMName,
				Namespace: namespace,
			}, cm)
			Expect(err).Should(BeNil())
			Expect(cm.Data).Should(HaveKey("001-updateable-policy"))

			// Parse initial diff
			var initialDiff map[string]interface{}
			err = json.Unmarshal([]byte(cm.Data["001-updateable-policy"]), &initialDiff)
			Expect(err).Should(BeNil())

			// Second reconciliation with same policy should update ConfigMap
			app2 := &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-update-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "main",
							Type: "webservice",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"image":"nginx","replicas":1}`),
							},
						},
					},
				},
			}

			handler2 := &AppHandler{
				Client: k8sClient,
				app:    app2,
			}

			monCtx2 := monitorContext.NewTraceContext(ctx, "test-trace-2")
			_, err = handler2.ApplyApplicationScopeTransforms(monCtx2, app2)
			Expect(err).Should(BeNil())

			// ConfigMap should still exist and be updated
			cm2 := &corev1.ConfigMap{}
			expectedCMName2 := "application-policies-" + namespace + "-test-update-app"
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      expectedCMName2,
				Namespace: namespace,
			}, cm2)
			Expect(err).Should(BeNil())
			Expect(cm2.Data).Should(HaveKey("001-updateable-policy"))
		})
	})

	Context("Test Application hash-based cache invalidation", func() {
		It("Test ConfigMap cache invalidates when Application spec changes", func() {
			// Create a policy
			policyDef := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hash-test-policy",
					Namespace: namespace,
				},
				Spec: v1beta1.PolicyDefinitionSpec{
					Global:          true,
					Priority:        100,
					Scope:           v1beta1.ApplicationScope,
					CacheTTLSeconds: -1, // Never expire based on time
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {}
enabled: true
transforms: {
	labels: {
		type: "merge"
		value: {
			"cached": "true"
		}
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "hash-test-policy", namespace)

			app := &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "hash-test-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
						},
					},
				},
			}

			// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			handler := &AppHandler{
				Client: k8sClient,
				app:    app,
			}

			// First application - creates ConfigMap with hash
			monCtx := monitorContext.NewTraceContext(ctx, "test")
			_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
			Expect(err).Should(BeNil())

			// Get ConfigMap and extract hash
			cmName := "application-policies-" + namespace + "-hash-test-app"
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: cmName, Namespace: namespace}, cm)
			Expect(err).Should(BeNil())

			// Extract original hash from ConfigMap data
			var originalHash string
			for _, value := range cm.Data {
				var record map[string]interface{}
				err := json.Unmarshal([]byte(value), &record)
				Expect(err).Should(BeNil())
				if hash, ok := record["application_hash"].(string); ok {
					originalHash = hash
					break
				}
			}
			Expect(originalHash).ShouldNot(BeEmpty())

			// Modify Application spec - this should invalidate cache
			app.Spec.Components = append(app.Spec.Components, common.ApplicationComponent{
				Name: "comp2",
				Type: "worker",
			})

			// Re-apply policies
			handler2 := &AppHandler{
				Client: k8sClient,
				app:    app,
			}
			monCtx2 := monitorContext.NewTraceContext(ctx, "test2")
			_, err = handler2.ApplyApplicationScopeTransforms(monCtx2, app)
			Expect(err).Should(BeNil())

			// Get updated ConfigMap
			cm2 := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: cmName, Namespace: namespace}, cm2)
			Expect(err).Should(BeNil())

			// Extract new hash - it should be different
			var newHash string
			for _, value := range cm2.Data {
				var record map[string]interface{}
				err := json.Unmarshal([]byte(value), &record)
				Expect(err).Should(BeNil())
				if hash, ok := record["application_hash"].(string); ok {
					newHash = hash
					break
				}
			}
			Expect(newHash).ShouldNot(BeEmpty())
			Expect(newHash).ShouldNot(Equal(originalHash), "Hash should change when spec changes")
		})

		It("Test ConfigMap cache invalidates when Application labels change", func() {
			policyDef := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "label-hash-policy",
					Namespace: namespace,
				},
				Spec: v1beta1.PolicyDefinitionSpec{
					Global:          true,
					Priority:        100,
					Scope:           v1beta1.ApplicationScope,
					CacheTTLSeconds: -1,
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {}
enabled: true
transforms: {
	labels: {
		type: "merge"
		value: {
			"test": "value"
		}
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "label-hash-policy", namespace)

			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "label-hash-app",
					Namespace: namespace,
					UID:       "label-hash-uid",
					Labels: map[string]string{
						"original": "label",
					},
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{Name: "comp", Type: "webservice"}},
				},
			}

			// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			handler := &AppHandler{Client: k8sClient, app: app}
			monCtx := monitorContext.NewTraceContext(ctx, "test")
			_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
			Expect(err).Should(BeNil())

			// Get original hash
			cmName := "application-policies-" + namespace + "-label-hash-app"
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: cmName, Namespace: namespace}, cm)
			Expect(err).Should(BeNil())

			var originalHash string
			for _, value := range cm.Data {
				var record map[string]interface{}
				json.Unmarshal([]byte(value), &record)
				if hash, ok := record["application_hash"].(string); ok {
					originalHash = hash
					break
				}
			}

			// Change Application labels
			app.Labels["new"] = "label"
			handler2 := &AppHandler{Client: k8sClient, app: app}
			monCtx2 := monitorContext.NewTraceContext(ctx, "test2")
			_, err = handler2.ApplyApplicationScopeTransforms(monCtx2, app)
			Expect(err).Should(BeNil())

			// Get new hash
			cm2 := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: cmName, Namespace: namespace}, cm2)
			Expect(err).Should(BeNil())

			var newHash string
			for _, value := range cm2.Data {
				var record map[string]interface{}
				json.Unmarshal([]byte(value), &record)
				if hash, ok := record["application_hash"].(string); ok {
					newHash = hash
					break
				}
			}

			Expect(newHash).ShouldNot(Equal(originalHash), "Hash should change when labels change")
		})
	})

	Context("Test TTL-based caching (cacheTTLSeconds)", func() {
		It("Test policy with cacheTTLSeconds: -1 stores TTL in ConfigMap", func() {
			policyDef := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ttl-never-policy",
					Namespace: namespace,
				},
				Spec: v1beta1.PolicyDefinitionSpec{
					Global:          true,
					Priority:        100,
					Scope:           v1beta1.ApplicationScope,
					CacheTTLSeconds: -1, // Never refresh
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {}
enabled: true
transforms: {
	labels: {
		type: "merge"
		value: {"ttl": "never"}
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "ttl-never-policy", namespace)

			app := &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "ttl-never-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{Name: "comp", Type: "webservice"}},
				},
			}

			// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			handler := &AppHandler{Client: k8sClient, app: app}
			monCtx := monitorContext.NewTraceContext(ctx, "test")
			_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
			Expect(err).Should(BeNil())

			// Verify ConfigMap contains ttl_seconds: -1
			cmName := "application-policies-" + namespace + "-ttl-never-app"
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: cmName, Namespace: namespace}, cm)
			Expect(err).Should(BeNil())

			// Parse and verify TTL
			for _, value := range cm.Data {
				var record map[string]interface{}
				err := json.Unmarshal([]byte(value), &record)
				Expect(err).Should(BeNil())

				ttl, ok := record["ttl_seconds"].(float64)
				Expect(ok).Should(BeTrue(), "ttl_seconds should be present")
				Expect(int32(ttl)).Should(Equal(int32(-1)), "TTL should be -1 (never refresh)")

				// Verify rendered_at timestamp exists
				_, ok = record["rendered_at"].(string)
				Expect(ok).Should(BeTrue(), "rendered_at should be present")
			}
		})

		It("Test policy with cacheTTLSeconds: 60 stores TTL in ConfigMap", func() {
			policyDef := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ttl-60-policy",
					Namespace: namespace,
				},
				Spec: v1beta1.PolicyDefinitionSpec{
					Global:          true,
					Priority:        100,
					Scope:           v1beta1.ApplicationScope,
					CacheTTLSeconds: 60, // Cache for 60 seconds
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {}
enabled: true
transforms: {
	labels: {
		type: "merge"
		value: {"ttl": "60"}
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "ttl-60-policy", namespace)

			app := &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "ttl-60-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{Name: "comp", Type: "webservice"}},
				},
			}

			// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			handler := &AppHandler{Client: k8sClient, app: app}
			monCtx := monitorContext.NewTraceContext(ctx, "test")
			_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
			Expect(err).Should(BeNil())

			// Verify ConfigMap contains ttl_seconds: 60
			cmName := "application-policies-" + namespace + "-ttl-60-app"
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: cmName, Namespace: namespace}, cm)
			Expect(err).Should(BeNil())

			for _, value := range cm.Data {
				var record map[string]interface{}
				err := json.Unmarshal([]byte(value), &record)
				Expect(err).Should(BeNil())

				ttl, ok := record["ttl_seconds"].(float64)
				Expect(ok).Should(BeTrue())
				Expect(int32(ttl)).Should(Equal(int32(60)))
			}
		})

		It("Test policy with cacheTTLSeconds not specified", func() {
			policyDef := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ttl-default-policy",
					Namespace: namespace,
				},
				Spec: v1beta1.PolicyDefinitionSpec{
					Global:   true,
					Priority: 100,
					Scope:    v1beta1.ApplicationScope,
					// CacheTTLSeconds not specified - in tests it's 0, but CRD default is -1 in production
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {}
enabled: true
transforms: {
	labels: {
		type: "merge"
		value: {"ttl": "default"}
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "ttl-default-policy", namespace)

			app := &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "ttl-default-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{Name: "comp", Type: "webservice"}},
				},
			}

			// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			handler := &AppHandler{Client: k8sClient, app: app}
			monCtx := monitorContext.NewTraceContext(ctx, "test")
			_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
			Expect(err).Should(BeNil())

			// Verify ConfigMap contains ttl_seconds: 0 (Go zero value when not specified in tests)
			// Note: CRD default marker will set it to -1 in production when CRDs are regenerated
			cmName := "application-policies-" + namespace + "-ttl-default-app"
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: cmName, Namespace: namespace}, cm)
			Expect(err).Should(BeNil())

			for _, value := range cm.Data {
				var record map[string]interface{}
				err := json.Unmarshal([]byte(value), &record)
				Expect(err).Should(BeNil())

				ttl, ok := record["ttl_seconds"].(float64)
				Expect(ok).Should(BeTrue())
				Expect(int32(ttl)).Should(Equal(int32(0)), "Should be 0 (Go zero value) in tests")
			}
		})
	})

	Context("Test context.prior support", func() {
		It("Test context.prior is available to policy template on second render", func() {
			// Create a policy that uses context.prior
			policyDef := &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prior-context-policy",
					Namespace: namespace,
				},
				Spec: v1beta1.PolicyDefinitionSpec{
					Global:          true,
					Priority:        100,
					Scope:           v1beta1.ApplicationScope,
					CacheTTLSeconds: 0, // Always re-render so we can test prior
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {}
enabled: true

// Check if prior result exists
hasPrior: context.prior != _|_

transforms: {
	labels: {
		type: "merge"
		value: {
			if hasPrior {
				"render-count": "incremental"
			}
			if !hasPrior {
				"render-count": "first"
			}
		}
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policyDef)).Should(Succeed())
		waitForPolicyDef(ctx, "prior-context-policy", namespace)

			app := &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "prior-test-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{Name: "comp", Type: "webservice"}},
				},
			}

			// Create the Application first so it gets a UID (needed for ConfigMap OwnerReference)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			// First render - no prior context
			handler := &AppHandler{Client: k8sClient, app: app}
			monCtx := monitorContext.NewTraceContext(ctx, "test")
			_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
			Expect(err).Should(BeNil())

			// Check that label indicates first render
			Expect(app.Labels["render-count"]).Should(Equal("first"))

			// Store ConfigMap name
			cmName := app.Status.ApplicationPoliciesConfigMap
			Expect(cmName).ShouldNot(BeEmpty())

			// Clear in-memory cache to force re-render (TTL=0 means never cache)
			globalPolicyCache.InvalidateAll()

			// Second render - should have prior context
			app2 := app.DeepCopy()
			app2.Status.ApplicationPoliciesConfigMap = cmName // Preserve ConfigMap reference

			handler2 := &AppHandler{Client: k8sClient, app: app2}
			monCtx2 := monitorContext.NewTraceContext(ctx, "test2")
			_, err = handler2.ApplyApplicationScopeTransforms(monCtx2, app2)
			Expect(err).Should(BeNil())

			// Check that label indicates incremental render (had prior)
			Expect(app2.Labels["render-count"]).Should(Equal("incremental"))
		})
	})
})

var _ = Describe("Test Application-scoped policy feature gates", func() {
	namespace := "policy-featuregate-test"
	velaSystem := oam.SystemDefinitionNamespace
	var ctx context.Context

	BeforeEach(func() {
		ctx = util.SetNamespaceInCtx(context.Background(), namespace)
		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
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
enabled: true
transforms: {
	labels: {
		type: "merge"
		value: {
			"test": "gate-disabled"
		}
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
enabled: true
transforms: {
	labels: {
		type: "merge"
		value: {
			"global": "policy"
		}
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
		globalPolicyCache.InvalidateAll()

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
enabled: true
transforms: {
	labels: {
		type: "merge"
		value: {
			"discovered": "false"
		}
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
		globalPolicyCache.InvalidateAll()

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
enabled: true
transforms: {
	labels: {
		type: "merge"
		value: {
			"global": "applied"
		}
	}
}
`,
					},
				},
			},
		}
		velaCtx := util.SetNamespaceInCtx(context.Background(), velaSystem)
		Expect(k8sClient.Create(velaCtx, globalPolicy)).Should(Succeed())
		waitForPolicyDef(velaCtx, "global-full-func", velaSystem)

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
enabled: true
transforms: {
	labels: {
		type: "merge"
		value: {
			"explicit": "applied"
		}
	}
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, explicitPolicy)).Should(Succeed())
		waitForPolicyDef(ctx, "explicit-full-func", namespace)

		// Clear in-memory cache
		globalPolicyCache.InvalidateAll()

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
