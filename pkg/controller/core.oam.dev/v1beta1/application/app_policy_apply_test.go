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

func waitForPolicyDef(ctx context.Context, name, ns string) {
	Eventually(func() error {
		pd := &v1beta1.PolicyDefinition{}
		return k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, pd)
	}, "30s", "500ms").Should(Succeed())

	// Additional wait to ensure appfile.LoadTemplate can find it
	time.Sleep(100 * time.Millisecond)
}

// Helper function to create a PolicyDefinition and add it to the index
// This is needed because watch events don't fire in the test environment
func createPolicyDefAndIndex(ctx context.Context, policy *v1beta1.PolicyDefinition) {
	Expect(k8sClient.Create(ctx, policy)).Should(Succeed())
	waitForPolicyDef(ctx, policy.Name, policy.Namespace)
	// Manually add to index (watches don't fire in test environment)
	policyScopeIndex.AddOrUpdate(policy)
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

		// Ensure feature gates are enabled (they should be from suite_test.go, but be defensive)
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableGlobalPolicies=true")).ToNot(HaveOccurred())
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true")).ToNot(HaveOccurred())

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

		// Also clean up vela-system policies to avoid polluting other test suites
		velaSystemPolicyList := &v1beta1.PolicyDefinitionList{}
		_ = k8sClient.List(ctx, velaSystemPolicyList, client.InNamespace("vela-system"))
		for _, policy := range velaSystemPolicyList.Items {
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
output: {
	components: [{
				properties: {
					env: [{
						name: parameter.envName
						value: parameter.envValue
					}]
				}
			}]
	ctx: {
		policyApplied: "add-test-env"
		timestamp: "2024-01-01"
	}
}
`,
					},
				},
			},
		}
		createPolicyDefAndIndex(ctx, policyDef)

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

output: {
	labels: {
			"team": parameter.team
			"environment": parameter.environment
			"managed-by": "kubevela"
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

config: {
  enabled: parameter.applyPolicy
}

output: {
	labels: {
			"should-not-appear": "true"
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

	It("Test Application-scoped policy with spec replace (new output.components API)", func() {
		// Create a PolicyDefinition that replaces components using new output API
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

// New output API - simpler syntax, no type/value wrapper
output: {
	components: [{
		name: parameter.newComponentName
		type: "webservice"
		properties: {
			image: "replaced-image"
		}
	}]
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

config: {
  enabled: true
}

// Use kube.#Get to read a ConfigMap from the cluster
_configmap: kube.#Get & {
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

// Add a label with data from the ConfigMap
output: {
	labels: {
			"from-configmap": _configmap.$returns.data.key1
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
		Expect(app.Labels).Should(HaveKeyWithValue("from-configmap", "value1"))
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

config: {
  enabled: true
}

_configmap: kube.#Get & {
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

// Expose ConfigMap data via ctx so components can access it
output: {
  ctx: {
    config: {
      endpoint: _configmap.$returns.data.apiEndpoint
      region: _configmap.$returns.data.region
    }
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

		// Verify additionalContext is stored in the Go context.
		// Must use the typed key constant — plain string lookup returns nil due to Go context key semantics.
		additionalCtx := monCtx.GetContext().Value(oam.PolicyAdditionalContextKey)
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

	It("Test built-in policies (not in index) are silently skipped", func() {
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-builtins",
				Namespace: namespace,
				Labels: map[string]string{
					"original": "label",
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
						Name: "gc",
						Type: "garbage-collect",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"rules":[{"selector":{"resourceTypes":["Job"]},"strategy":"never"}]}`),
						},
					},
					{
						Name: "apply-once-policy",
						Type: "apply-once",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"enable":true}`),
						},
					},
					{
						Name: "placement",
						Type: "override",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"selector":{"cluster":"local"}}`),
						},
					},
				},
			},
		}

		handler := &AppHandler{
			Client: k8sClient,
		}

		// policyScopeIndex has no entry for apply-once, garbage-collect, override
		// (they are built-ins with no PolicyDefinition CRD).
		// The call must succeed without error and leave the app unmodified.
		monCtx := monitorContext.NewTraceContext(ctx, "test-builtins")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// App should be unmodified — no transforms applied
		Expect(app.Labels).Should(HaveLen(1))
		Expect(app.Labels["original"]).Should(Equal("label"))
		Expect(app.Status.AppliedApplicationPolicies).Should(BeEmpty())
	})

	It("Test policy source field is correctly set for global vs explicit policies", func() {
		// Create a global policy
		globalPolicy := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "global-test-policy",
				Namespace: "vela-system",
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
      "global-policy": "true"
    }
}
`,
					},
				},
			},
		}
		createPolicyDefAndIndex(ctx, globalPolicy)

		// Create an explicit policy in app namespace
		explicitPolicy := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "explicit-test-policy",
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
      "explicit-policy": "true"
    }
}
`,
					},
				},
			},
		}
		createPolicyDefAndIndex(ctx, explicitPolicy)

		// Create Application that references the explicit policy
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy-source",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Policies: []v1beta1.AppPolicy{
					{
						Name: "my-explicit",
						Type: "explicit-test-policy",
					},
				},
				Components: []common.ApplicationComponent{
					{
						Name: "test-comp",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image": "nginx"}`),
						},
					},
				},
			},
		}

		handler := &AppHandler{
			Client: k8sClient,
		}

		monCtx := monitorContext.NewTraceContext(ctx, "test-source-field")
		_, err := handler.ApplyApplicationScopeTransforms(monCtx, app)
		Expect(err).Should(BeNil())

		// Verify status contains both policies with correct source
		Expect(len(app.Status.AppliedApplicationPolicies)).Should(Equal(2))

		// Find the global policy in status
		var globalEntry, explicitEntry *common.AppliedApplicationPolicy
		for i := range app.Status.AppliedApplicationPolicies {
			if app.Status.AppliedApplicationPolicies[i].Name == "global-test-policy" {
				globalEntry = &app.Status.AppliedApplicationPolicies[i]
			}
			if app.Status.AppliedApplicationPolicies[i].Name == "my-explicit" {
				explicitEntry = &app.Status.AppliedApplicationPolicies[i]
			}
		}

		Expect(globalEntry).ShouldNot(BeNil())
		Expect(globalEntry.Source).Should(Equal(PolicySourceGlobal))
		Expect(globalEntry.Applied).Should(BeTrue())

		Expect(explicitEntry).ShouldNot(BeNil())
		Expect(explicitEntry.Source).Should(Equal(PolicySourceExplicit))
		Expect(explicitEntry.Applied).Should(BeTrue())
	})

})
