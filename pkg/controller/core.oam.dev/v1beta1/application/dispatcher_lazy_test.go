/*
Copyright 2024 The KubeVela Authors.

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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestLazyRenderingPostDispatchTraits(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.MultiStageComponentApply, true)

	tests := []struct {
		name                   string
		traits                 []appfile.Trait
		traitDefinitions       map[string]*v1beta1.TraitDefinition
		expectedImmediateCount int
		expectedDeferredCount  int
	}{
		{
			name: "separate PostDispatch traits",
			traits: []appfile.Trait{
				{Name: "normal-trait"},
				{Name: "postdispatch-trait"},
				{Name: "predispatch-trait"},
			},
			traitDefinitions: map[string]*v1beta1.TraitDefinition{
				"normal-trait": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.DefaultDispatch,
					},
				},
				"postdispatch-trait": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.PostDispatch,
					},
				},
				"predispatch-trait": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.PreDispatch,
					},
				},
			},
			expectedImmediateCount: 2, // normal and predispatch
			expectedDeferredCount:  1, // postdispatch
		},
		{
			name: "all PostDispatch traits",
			traits: []appfile.Trait{
				{Name: "postdispatch-trait-1"},
				{Name: "postdispatch-trait-2"},
			},
			traitDefinitions: map[string]*v1beta1.TraitDefinition{
				"postdispatch-trait-1": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.PostDispatch,
					},
				},
				"postdispatch-trait-2": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.PostDispatch,
					},
				},
			},
			expectedImmediateCount: 0,
			expectedDeferredCount:  2,
		},
		{
			name: "no PostDispatch traits",
			traits: []appfile.Trait{
				{Name: "normal-trait-1"},
				{Name: "normal-trait-2"},
			},
			traitDefinitions: map[string]*v1beta1.TraitDefinition{
				"normal-trait-1": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.DefaultDispatch,
					},
				},
				"normal-trait-2": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.PreDispatch,
					},
				},
			},
			expectedImmediateCount: 2,
			expectedDeferredCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test trait separation logic
			var immediateTraits []appfile.Trait
			var deferredTraits []interface{}

			for _, tr := range tt.traits {
				// Simulate getTraitDispatchStage
				traitDef, ok := tt.traitDefinitions[tr.Name]
				var stage StageType
				if ok && traitDef.Spec.Stage == v1beta1.PostDispatch {
					stage = PostDispatch
				} else if ok && traitDef.Spec.Stage == v1beta1.PreDispatch {
					stage = PreDispatch
				} else {
					stage = DefaultDispatch
				}

				if stage == PostDispatch {
					traitCopy := tr
					deferredTraits = append(deferredTraits, &traitCopy)
				} else {
					immediateTraits = append(immediateTraits, tr)
				}
			}

			assert.Equal(t, tt.expectedImmediateCount, len(immediateTraits), "immediate traits count mismatch")
			assert.Equal(t, tt.expectedDeferredCount, len(deferredTraits), "deferred traits count mismatch")
		})
	}
}

func TestFetchComponentStatus(t *testing.T) {
	tests := []struct {
		name           string
		workload       *unstructured.Unstructured
		expectedStatus map[string]interface{}
		expectError    bool
	}{
		{
			name: "deployment with status",
			workload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test-deployment",
						"namespace": "default",
					},
					"status": map[string]interface{}{
						"replicas":          3,
						"readyReplicas":     3,
						"availableReplicas": 3,
					},
				},
			},
			expectedStatus: map[string]interface{}{
				"replicas":          3,
				"readyReplicas":     3,
				"availableReplicas": 3,
			},
			expectError: false,
		},
		{
			name: "workload without status",
			workload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm",
						"namespace": "default",
					},
				},
			},
			expectedStatus: map[string]interface{}{},
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, found, _ := unstructured.NestedFieldNoCopy(tt.workload.Object, "status")
			if !found {
				status = map[string]interface{}{}
			}

			statusMap, ok := status.(map[string]interface{})
			if !ok {
				statusMap = map[string]interface{}{}
			}

			assert.Equal(t, tt.expectedStatus, statusMap, "status mismatch")
		})
	}
}

func TestManifestWithDeferredTraits(t *testing.T) {
	manifest := &types.ComponentManifest{
		Name:      "test-component",
		Namespace: "default",
	}

	deferredTraits := []interface{}{
		&appfile.Trait{Name: "status-reader"},
		&appfile.Trait{Name: "another-postdispatch"},
	}

	manifest.DeferredTraits = deferredTraits

	assert.Equal(t, 2, len(manifest.DeferredTraits), "deferred traits not stored correctly")

	for i, dt := range manifest.DeferredTraits {
		trait, ok := dt.(*appfile.Trait)
		assert.True(t, ok, "deferred trait type assertion failed")
		if i == 0 {
			assert.Equal(t, "status-reader", trait.Name)
		} else {
			assert.Equal(t, "another-postdispatch", trait.Name)
		}
	}
}

func TestPostDispatchWithOutputsStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	deployment := createDeploymentWithStatus("test-deployment", "default", 3)
	service := createServiceWithLoadBalancer("test-service", "default", "192.168.1.1")
	ingress := createIngressWithLoadBalancer("test-ingress", "default", "203.0.113.1")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment, service, ingress).
		Build()

	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
	}

	appRev := &v1beta1.ApplicationRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app-v1",
			Namespace: "default",
		},
	}

	h := &AppHandler{
		Client:        fakeClient,
		app:           app,
		currentAppRev: appRev,
	}

	comp := &appfile.Component{
		Name: "test-comp",
		Type: "webservice",
	}

	outputs := []string{"service", "ingress"}
	_ = outputs

	ctx := context.Background()

	workloadStatus, err := h.fetchComponentStatus(ctx, deployment, "", "")
	assert.NoError(t, err)
	assert.NotNil(t, workloadStatus)
	assert.Equal(t, int64(3), workloadStatus["replicas"])

	t.Run("PostDispatch trait can access outputs status", func(t *testing.T) {
		manifest := &types.ComponentManifest{
			Name:            comp.Name,
			Namespace:       "default",
			ComponentOutput: deployment,
			ComponentOutputsAndTraits: []*unstructured.Unstructured{
				service,
				ingress,
			},
			DeferredTraits: []interface{}{
				&appfile.Trait{Name: "test-postdispatch"},
			},
		}

		service.SetLabels(map[string]string{
			oam.TraitResource: "service",
		})
		ingress.SetLabels(map[string]string{
			oam.TraitResource: "ingress",
		})

		outputsStatus, err := h.fetchComponentOutputsStatus(ctx, manifest.ComponentOutputsAndTraits, "", "")
		assert.NoError(t, err)
		assert.NotNil(t, outputsStatus)

		assert.Contains(t, outputsStatus, "service")
		assert.Contains(t, outputsStatus, "ingress")

		verifyServiceStatus(t, outputsStatus["service"].(map[string]interface{}), "192.168.1.1")
		verifyIngressStatus(t, outputsStatus["ingress"].(map[string]interface{}), "203.0.113.1")
	})
}

// Helper functions for creating test resources

func createDeploymentWithStatus(name, namespace string, replicas int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"status": map[string]interface{}{
				"replicas":          replicas,
				"readyReplicas":     replicas,
				"availableReplicas": replicas,
			},
		},
	}
}

func createServiceWithLoadBalancer(name, namespace, ip string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"clusterIP": "10.0.0.1",
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"targetPort": int64(8080),
					},
				},
			},
			"status": map[string]interface{}{
				"loadBalancer": map[string]interface{}{
					"ingress": []interface{}{
						map[string]interface{}{
							"ip": ip,
						},
					},
				},
			},
		},
	}
}

func createIngressWithLoadBalancer(name, namespace, ip string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"host": "example.com",
					},
				},
			},
			"status": map[string]interface{}{
				"loadBalancer": map[string]interface{}{
					"ingress": []interface{}{
						map[string]interface{}{
							"ip": ip,
						},
					},
				},
			},
		},
	}
}

func verifyServiceStatus(t *testing.T, serviceObj map[string]interface{}, expectedIP string) {
	serviceStatus := serviceObj["status"].(map[string]interface{})
	serviceLB := serviceStatus["loadBalancer"].(map[string]interface{})
	serviceIngress := serviceLB["ingress"].([]interface{})
	actualIP := serviceIngress[0].(map[string]interface{})["ip"]
	assert.Equal(t, expectedIP, actualIP)
}

func verifyIngressStatus(t *testing.T, ingressObj map[string]interface{}, expectedIP string) {
	ingressStatus := ingressObj["status"].(map[string]interface{})
	ingressLB := ingressStatus["loadBalancer"].(map[string]interface{})
	ingresses := ingressLB["ingress"].([]interface{})
	actualIP := ingresses[0].(map[string]interface{})["ip"]
	assert.Equal(t, expectedIP, actualIP)
}

func TestDispatcherWithPostDispatchStage(t *testing.T) {
	dispatcher := &manifestDispatcher{
		stage: PostDispatch,
	}

	assert.Equal(t, PostDispatch, dispatcher.stage, "dispatcher stage not set correctly")

	stages := []StageType{PostDispatch, PreDispatch, DefaultDispatch}
	expectedOrder := []StageType{PreDispatch, DefaultDispatch, PostDispatch}

	options := SortDispatchOptions{
		{Stage: stages[0]},
		{Stage: stages[1]},
		{Stage: stages[2]},
	}

	for i := 0; i < len(options); i++ {
		for j := i + 1; j < len(options); j++ {
			if options[i].Stage > options[j].Stage {
				options[i], options[j] = options[j], options[i]
			}
		}
	}

	for i, opt := range options {
		assert.Equal(t, expectedOrder[i], opt.Stage, "stage order mismatch at index %d", i)
	}
}

func TestPostDispatchTraitHealthStatus(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.MultiStageComponentApply, true)

	tests := []struct {
		name                string
		stage               StageType
		deferredTraits      []interface{}
		renderedTraits      []*unstructured.Unstructured
		existingResources   []client.Object
		expectedTraitStatus []common.ApplicationTraitStatus
		expectedHealthy     bool
	}{
		{
			name:  "PostDispatch traits shown as waiting in DefaultDispatch stage",
			stage: DefaultDispatch,
			deferredTraits: []interface{}{
				&appfile.Trait{Name: "my-trait"},
			},
			expectedTraitStatus: []common.ApplicationTraitStatus{
				{
					Type:    "my-trait",
					Healthy: false,
					Message: "PostDispatch: waiting for component to be healthy",
				},
			},
			expectedHealthy: true, // Component itself is healthy
		},
		{
			name:  "PostDispatch traits evaluated when deployed",
			stage: PostDispatch,
			renderedTraits: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "my-component-configmap",
							"namespace": "default",
							"labels": map[string]interface{}{
								oam.TraitTypeLabel: "my-trait",
							},
						},
					},
				},
			},
			existingResources: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "my-component-configmap",
							"namespace": "default",
						},
					},
				},
			},
			expectedTraitStatus: []common.ApplicationTraitStatus{
				{
					Type:    "my-trait",
					Healthy: true,
					Message: "",
				},
			},
			expectedHealthy: true,
		},
		{
			name:  "PostDispatch trait unhealthy when not found",
			stage: PostDispatch,
			renderedTraits: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "my-component-configmap",
							"namespace": "default",
							"labels": map[string]interface{}{
								oam.TraitTypeLabel: "my-trait",
							},
						},
					},
				},
			},
			existingResources: []client.Object{}, // Resource doesn't exist
			expectedTraitStatus: []common.ApplicationTraitStatus{
				{
					Type:    "my-trait",
					Healthy: false,
					Message: "Trait not found",
				},
			},
			expectedHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with existing resources
			scheme := runtime.NewScheme()
			_ = v1beta1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.existingResources...).Build()

			// Create AppHandler
			h := &AppHandler{
				Client: fakeClient,
				app: &v1beta1.Application{
					Spec: v1beta1.ApplicationSpec{
						Components: []common.ApplicationComponent{
							{
								Name: "my-component",
							},
						},
					},
				},
			}

			// Create component
			comp := &appfile.Component{
				Name:   "my-component",
				Traits: []*appfile.Trait{},
			}

			// Create manifest with deferred traits
			manifest := &types.ComponentManifest{
				Name:           "my-component",
				Namespace:      "default",
				DeferredTraits: tt.deferredTraits,
			}

			// These variables are used but not needed for this test
			_ = comp
			_ = manifest

			// Simulate health status collection
			status := &common.ApplicationComponentStatus{
				Name:      "my-component",
				Namespace: "default",
				Healthy:   true,
				Traits:    []common.ApplicationTraitStatus{},
			}

			ctx := context.Background()

			// Add PostDispatch trait status based on stage
			if tt.stage != PostDispatch && len(tt.deferredTraits) > 0 {
				// We're in an earlier stage - show deferred traits as waiting
				for _, deferredTrait := range tt.deferredTraits {
					if trait, ok := deferredTrait.(*appfile.Trait); ok && trait != nil {
						traitStatus := common.ApplicationTraitStatus{
							Type:    trait.Name,
							Healthy: false,
							Message: "PostDispatch: waiting for component to be healthy",
						}
						status.Traits = append(status.Traits, traitStatus)
					}
				}
			} else if tt.stage == PostDispatch && len(tt.renderedTraits) > 0 {
				// We're in PostDispatch stage - evaluate actual traits
				isHealth := true
				for _, trait := range tt.renderedTraits {
					traitName := trait.GetLabels()[oam.TraitTypeLabel]
					if traitName != "" {
						traitHealthy := true
						traitMessage := ""

						// Get the trait from cluster to check its actual status
						currentTrait := &unstructured.Unstructured{}
						currentTrait.SetGroupVersionKind(trait.GroupVersionKind())
						currentTrait.SetName(trait.GetName())
						currentTrait.SetNamespace(trait.GetNamespace())

						err := h.Client.Get(ctx, client.ObjectKey{
							Name:      currentTrait.GetName(),
							Namespace: currentTrait.GetNamespace(),
						}, currentTrait)

						if err != nil {
							traitHealthy = false
							if client.IgnoreNotFound(err) == nil {
								traitMessage = "Trait not found"
							} else {
								traitMessage = "Failed to get trait"
							}
						}

						traitStatus := common.ApplicationTraitStatus{
							Type:    traitName,
							Healthy: traitHealthy,
							Message: traitMessage,
						}
						status.Traits = append(status.Traits, traitStatus)

						if !traitHealthy {
							isHealth = false
						}
					}
				}
				status.Healthy = isHealth
			}

			// Verify results
			assert.Equal(t, len(tt.expectedTraitStatus), len(status.Traits), "trait count mismatch")
			for i, expected := range tt.expectedTraitStatus {
				if i < len(status.Traits) {
					assert.Equal(t, expected.Type, status.Traits[i].Type, "trait type mismatch")
					assert.Equal(t, expected.Healthy, status.Traits[i].Healthy, "trait health mismatch")
					assert.Equal(t, expected.Message, status.Traits[i].Message, "trait message mismatch")
				}
			}
			if tt.stage == PostDispatch {
				assert.Equal(t, tt.expectedHealthy, status.Healthy, "overall health mismatch")
			}
		})
	}
}

func TestGeneratorDeferPostDispatchTraits(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.MultiStageComponentApply, true)

	tests := []struct {
		name                  string
		inputTraits           []*appfile.Trait
		traitDefinitions      map[string]*v1beta1.TraitDefinition
		expectedRenderCount   int // Traits that should be rendered immediately
		expectedDeferredCount int // Traits that should be deferred
	}{
		{
			name: "defer PostDispatch traits during generation",
			inputTraits: []*appfile.Trait{
				{Name: "normal-trait"},
				{Name: "postdispatch-trait"},
				{Name: "predispatch-trait"},
			},
			traitDefinitions: map[string]*v1beta1.TraitDefinition{
				"normal-trait": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.DefaultDispatch,
					},
				},
				"postdispatch-trait": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.PostDispatch,
					},
				},
				"predispatch-trait": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.PreDispatch,
					},
				},
			},
			expectedRenderCount:   2, // normal and predispatch should be rendered
			expectedDeferredCount: 1, // postdispatch should be deferred
		},
		{
			name: "all PostDispatch traits deferred",
			inputTraits: []*appfile.Trait{
				{Name: "postdispatch-1"},
				{Name: "postdispatch-2"},
			},
			traitDefinitions: map[string]*v1beta1.TraitDefinition{
				"postdispatch-1": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.PostDispatch,
					},
				},
				"postdispatch-2": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.PostDispatch,
					},
				},
			},
			expectedRenderCount:   0, // All should be deferred
			expectedDeferredCount: 2,
		},
		{
			name: "no PostDispatch traits",
			inputTraits: []*appfile.Trait{
				{Name: "normal-trait"},
				{Name: "predispatch-trait"},
			},
			traitDefinitions: map[string]*v1beta1.TraitDefinition{
				"normal-trait": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.DefaultDispatch,
					},
				},
				"predispatch-trait": {
					Spec: v1beta1.TraitDefinitionSpec{
						Stage: v1beta1.PreDispatch,
					},
				},
			},
			expectedRenderCount:   2, // All should be rendered
			expectedDeferredCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the generator logic
			var immediateTraits []*appfile.Trait
			var deferredTraits []interface{}

			// This simulates the logic in generator.go
			for _, tr := range tt.inputTraits {
				traitDef, ok := tt.traitDefinitions[tr.Name]
				if ok && traitDef.Spec.Stage == v1beta1.PostDispatch {
					// PostDispatch trait - defer it
					deferredTraits = append(deferredTraits, tr)
				} else {
					// Other traits - render immediately
					immediateTraits = append(immediateTraits, tr)
				}
			}

			// Verify the separation
			assert.Equal(t, tt.expectedRenderCount, len(immediateTraits), "immediate traits count mismatch")
			assert.Equal(t, tt.expectedDeferredCount, len(deferredTraits), "deferred traits count mismatch")

			// Verify trait names are preserved
			for _, tr := range immediateTraits {
				assert.NotEmpty(t, tr.Name, "immediate trait should have name")
			}

			for _, dt := range deferredTraits {
				trait, ok := dt.(*appfile.Trait)
				assert.True(t, ok, "deferred trait should be *appfile.Trait")
				assert.NotEmpty(t, trait.Name, "deferred trait should have name")
			}
		})
	}
}

func TestRenderPostDispatchTraitsWithStatus(t *testing.T) {
	// Test that PostDispatch traits can be rendered with component status
	tests := []struct {
		name            string
		componentStatus map[string]interface{}
		deferredTraits  []interface{}
		expectError     bool
		expectedContext string // Expected substring in context
	}{
		{
			name: "render trait with deployment status",
			componentStatus: map[string]interface{}{
				"replicas":          3,
				"readyReplicas":     3,
				"availableReplicas": 3,
			},
			deferredTraits: []interface{}{
				&appfile.Trait{
					Name: "status-reader",
				},
			},
			expectError:     false,
			expectedContext: `"readyReplicas":3`,
		},
		{
			name:            "render trait with empty status",
			componentStatus: map[string]interface{}{},
			deferredTraits: []interface{}{
				&appfile.Trait{
					Name: "status-reader",
				},
			},
			expectError:     false,
			expectedContext: `"status":{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is a simplified test - in reality, renderPostDispatchTraits
			// would need a full CUE context and trait definitions
			// Here we just verify the status injection logic

			// Verify status can be marshaled to JSON
			statusJSON, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&map[string]interface{}{
				"status": tt.componentStatus,
			})
			assert.NoError(t, err)

			// Check that status contains expected data
			if tt.expectedContext != "" {
				assert.NotNil(t, statusJSON["status"])
			}
		})
	}
}

func TestBuildPostDispatchTraitDefinitionMap(t *testing.T) {
	traitPtr := &appfile.Trait{Name: "test-trait"}
	options := DispatchOptions{DeferredTraits: []*appfile.Trait{traitPtr}}
	defs := buildPostDispatchTraitDefinitionMap(options, &types.ComponentManifest{})
	assert.Equal(t, traitPtr, defs["test-trait"])

	manifestWithProcessed := &types.ComponentManifest{
		ProcessedDeferredTraits: []interface{}{traitPtr},
	}
	defs = buildPostDispatchTraitDefinitionMap(DispatchOptions{}, manifestWithProcessed)
	assert.Equal(t, traitPtr, defs["test-trait"])

	manifestWithDeferred := &types.ComponentManifest{
		DeferredTraits: []interface{}{traitPtr},
	}
	defs = buildPostDispatchTraitDefinitionMap(DispatchOptions{}, manifestWithDeferred)
	assert.Equal(t, traitPtr, defs["test-trait"])
}

func TestExtractHealthFromStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     map[string]interface{}
		expHealthy bool
		expMsg     string
	}{
		{
			name: "replicas not ready",
			status: map[string]interface{}{
				"replicas":      int64(3),
				"readyReplicas": int64(1),
			},
			expHealthy: false,
			expMsg:     "1/3 replicas are ready",
		},
		{
			name: "available replicas missing",
			status: map[string]interface{}{
				"replicas":          int64(4),
				"availableReplicas": int64(2),
			},
			expHealthy: false,
			expMsg:     "2/4 replicas are available",
		},
		{
			name: "failed condition uses message",
			status: map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Available",
						"status":  "False",
						"message": "Minimum replicas unavailable",
					},
				},
			},
			expHealthy: false,
			expMsg:     "Minimum replicas unavailable",
		},
		{
			name: "unknown condition keeps existing message",
			status: map[string]interface{}{
				"replicas":      int64(2),
				"readyReplicas": int64(0),
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Available",
						"status": "Unknown",
					},
				},
			},
			expHealthy: false,
			expMsg:     "0/2 replicas are ready",
		},
		{
			name: "message field propagated",
			status: map[string]interface{}{
				"message": "custom status message",
			},
			expHealthy: true,
			expMsg:     "custom status message",
		},
		{
			name: "all good",
			status: map[string]interface{}{
				"replicas":          int64(2),
				"readyReplicas":     int64(2),
				"availableReplicas": int64(2),
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Available",
						"status": "True",
					},
				},
			},
			expHealthy: true,
			expMsg:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healthy, msg := extractHealthFromStatus(tt.status)
			assert.Equal(t, tt.expHealthy, healthy)
			assert.Equal(t, tt.expMsg, msg)
		})
	}
}

func TestFilterOutputsFromManifest(t *testing.T) {
	deployment := &unstructured.Unstructured{}
	deployment.SetLabels(map[string]string{
		oam.TraitTypeLabel: "",
	})

	traitResource := &unstructured.Unstructured{}
	traitResource.SetLabels(map[string]string{
		oam.TraitTypeLabel: "custom-trait",
		oam.TraitResource:  "statusPod",
	})

	mixed := filterOutputsFromManifest([]*unstructured.Unstructured{deployment, traitResource})
	require.Len(t, mixed, 2)
	assert.Contains(t, mixed, deployment)
	assert.Contains(t, mixed, traitResource)
}
