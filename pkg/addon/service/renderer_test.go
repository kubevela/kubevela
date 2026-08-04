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

package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/addon/service/api"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// fakeClientWithRegistry returns a fake client with no addon registry configured,
// so any addon resolution fails as "not found".
func fakeClientWithRegistry(t *testing.T) client.Client {
	t.Helper()
	return fake.NewClientBuilder().Build()
}

func TestSetAddonRegistryLabel(t *testing.T) {
	app := &v1beta1.Application{}
	setAddonRegistryLabel(app, "ecr")
	assert.Equal(t, "ecr", app.Labels[oam.LabelAddonRegistry])

	app.Labels["preserved"] = "true"
	setAddonRegistryLabel(app, "another")
	assert.Equal(t, "another", app.Labels[oam.LabelAddonRegistry])
	assert.Equal(t, "true", app.Labels["preserved"])
}

func TestRenderAddonNotFound(t *testing.T) {
	r := &rendererImpl{cli: fakeClientWithRegistry(t)}
	_, err := r.RenderAddon(context.Background(), api.AddonRequest{Name: "does-not-exist"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestValidateSystemRequirementsNilPasses(t *testing.T) {
	cli := fakeClientWithRegistry(t)
	err := pkgaddon.ValidateSystemRequirements(context.Background(), nil, cli, nil)
	require.NoError(t, err)
}

func TestRenderAddonSkipVersionValidateStillResolves(t *testing.T) {
	r := &rendererImpl{cli: fakeClientWithRegistry(t)}
	_, err := r.RenderAddon(context.Background(), api.AddonRequest{Name: "does-not-exist", SkipVersionValidate: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRenderAddonCachesByKey(t *testing.T) {
	r := &rendererImpl{cli: fakeClientWithRegistry(t)}
	var calls int
	r.resolveFn = func(_ context.Context, req api.AddonRequest) (*api.AddonResult, error) {
		calls++
		return &api.AddonResult{ResolvedVersion: req.Version}, nil
	}

	reqA := api.AddonRequest{Name: "example", Version: "1.0.0", Properties: map[string]interface{}{"replicas": 1}}

	res1, err := r.RenderAddon(context.Background(), reqA)
	require.NoError(t, err)
	res2, err := r.RenderAddon(context.Background(), reqA)
	require.NoError(t, err)
	assert.Equal(t, 1, calls, "identical requests must resolve exactly once")
	assert.Same(t, res1, res2, "cache hit must return the same result pointer")

	// Different version and different properties each produce a new key.
	_, err = r.RenderAddon(context.Background(), api.AddonRequest{Name: "example", Version: "2.0.0", Properties: map[string]interface{}{"replicas": 1}})
	require.NoError(t, err)
	_, err = r.RenderAddon(context.Background(), api.AddonRequest{Name: "example", Version: "1.0.0", Properties: map[string]interface{}{"replicas": 2}})
	require.NoError(t, err)
	assert.Equal(t, 3, calls, "distinct requests must each resolve")
}

func TestRenderAddonDoesNotCacheLatest(t *testing.T) {
	r := &rendererImpl{cli: fakeClientWithRegistry(t)}
	var calls int
	r.resolveFn = func(_ context.Context, _ api.AddonRequest) (*api.AddonResult, error) {
		calls++
		return &api.AddonResult{ResolvedVersion: fmt.Sprintf("3.0.%d", calls+1)}, nil
	}

	req := api.AddonRequest{Name: "example"}
	first, err := r.RenderAddon(context.Background(), req)
	require.NoError(t, err)
	second, err := r.RenderAddon(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 2, calls, "latest must be resolved for every request")
	assert.Equal(t, "3.0.2", first.ResolvedVersion)
	assert.Equal(t, "3.0.3", second.ResolvedVersion)
}

func TestHashPropertiesStable(t *testing.T) {
	a := map[string]interface{}{"b": 2, "a": 1, "nested": map[string]interface{}{"x": "y"}}
	b := map[string]interface{}{"a": 1, "nested": map[string]interface{}{"x": "y"}, "b": 2}
	assert.Equal(t, hashProperties(a), hashProperties(b), "same map (any order) must hash equally")

	c := map[string]interface{}{"a": 1, "b": 3}
	assert.NotEqual(t, hashProperties(a), hashProperties(c), "different maps must hash differently")

	assert.Equal(t, hashProperties(nil), hashProperties(nil), "nil must hash stably")
}

func TestSanitizeManifest(t *testing.T) {
	testCases := map[string]struct {
		in   map[string]interface{}
		want map[string]interface{}
	}{
		"strips root status and top-level creationTimestamp": {
			in: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name":              "flux-system",
					"creationTimestamp": nil,
				},
				"status": map[string]interface{}{"phase": "Active"},
			},
			want: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": "flux-system",
				},
			},
		},
		"strips creationTimestamp in nested objects and arrays": {
			in: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":              "pod",
							"creationTimestamp": nil,
						},
					},
					"objects": []interface{}{
						map[string]interface{}{
							"metadata": map[string]interface{}{
								"name":              "cm",
								"creationTimestamp": nil,
							},
						},
					},
				},
			},
			want: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"name": "pod",
						},
					},
					"objects": []interface{}{
						map[string]interface{}{
							"metadata": map[string]interface{}{
								"name": "cm",
							},
						},
					},
				},
			},
		},
		"leaves a manifest without status or creationTimestamp unchanged": {
			in: map[string]interface{}{
				"kind":     "ConfigMap",
				"metadata": map[string]interface{}{"name": "keep"},
			},
			want: map[string]interface{}{
				"kind":     "ConfigMap",
				"metadata": map[string]interface{}{"name": "keep"},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			sanitizeManifest(tc.in)
			assert.Equal(t, tc.want, tc.in)
		})
	}
}

func TestSuppressLastAppliedConfig(t *testing.T) {
	testCases := map[string]struct {
		in   map[string]interface{}
		want map[string]interface{}
	}{
		"adds an annotations map when metadata has none": {
			in: map[string]interface{}{
				"metadata": map[string]interface{}{"name": "addon-fluxcd"},
			},
			want: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":        "addon-fluxcd",
					"annotations": map[string]interface{}{oam.AnnotationLastAppliedConfig: "skip"},
				},
			},
		},
		"preserves existing annotations and adds the sentinel": {
			in: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":        "addon-fluxcd",
					"annotations": map[string]interface{}{"custom.io/foo": "bar"},
				},
			},
			want: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "addon-fluxcd",
					"annotations": map[string]interface{}{
						"custom.io/foo":                 "bar",
						oam.AnnotationLastAppliedConfig: "skip",
					},
				},
			},
		},
		"creates metadata when the manifest has none": {
			in: map[string]interface{}{
				"kind": "Application",
			},
			want: map[string]interface{}{
				"kind": "Application",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{oam.AnnotationLastAppliedConfig: "skip"},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			suppressLastAppliedConfig(tc.in)
			assert.Equal(t, tc.want, tc.in)
		})
	}
}

func TestEnsureAddonComponentStateKeepPolicy(t *testing.T) {
	deepCopyMap := func(in map[string]interface{}) map[string]interface{} {
		return runtime.DeepCopyJSONValue(in).(map[string]interface{})
	}
	getPolicies := func(t *testing.T, app map[string]interface{}) []interface{} {
		t.Helper()
		spec, ok := app["spec"].(map[string]interface{})
		require.True(t, ok)
		policies, ok := spec["policies"].([]interface{})
		require.True(t, ok)
		return policies
	}

	t.Run("adds disabled policy", func(t *testing.T) {
		app := map[string]interface{}{}
		ensureAddonComponentStateKeepPolicy(app)
		policies := getPolicies(t, app)
		require.Len(t, policies, 1)
		assert.Equal(t, map[string]interface{}{
			"name":       "addon-component-state-keep",
			"type":       v1alpha1.ApplyOncePolicyType,
			"properties": map[string]interface{}{"enable": false},
		}, policies[0])
	})

	t.Run("preserves unrelated policies", func(t *testing.T) {
		topology := map[string]interface{}{
			"name": "deploy-local", "type": "topology",
			"properties": map[string]interface{}{"clusters": []interface{}{"local"}},
		}
		expectedTopology := deepCopyMap(topology)
		app := map[string]interface{}{
			"spec": map[string]interface{}{"policies": []interface{}{topology}},
		}
		ensureAddonComponentStateKeepPolicy(app)
		policies := getPolicies(t, app)
		require.Len(t, policies, 2)
		assert.Equal(t, expectedTopology, policies[0])
	})

	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("preserves explicit apply-once enable=%t", enabled), func(t *testing.T) {
			explicit := map[string]interface{}{
				"name": "addon-authored", "type": v1alpha1.ApplyOncePolicyType,
				"properties": map[string]interface{}{"enable": enabled},
			}
			app := map[string]interface{}{
				"spec": map[string]interface{}{"policies": []interface{}{explicit}},
			}
			expected := deepCopyMap(app)
			ensureAddonComponentStateKeepPolicy(app)
			assert.Equal(t, expected, app)
		})
	}

	t.Run("preserves package-authored rules-only apply-once", func(t *testing.T) {
		app := map[string]interface{}{
			"spec": map[string]interface{}{
				"policies": []interface{}{map[string]interface{}{
					"name": "not-keep-CRD",
					"type": v1alpha1.ApplyOncePolicyType,
					"properties": map[string]interface{}{
						"rules": []interface{}{map[string]interface{}{
							"selector": map[string]interface{}{
								"resourceTypes": []interface{}{"CustomResourceDefinition"},
							},
							"strategy": map[string]interface{}{
								"path":   []interface{}{"*"},
								"affect": "onStateKeep",
							},
						}},
					},
				}},
			},
		}
		expected := deepCopyMap(app)

		ensureAddonComponentStateKeepPolicy(app)

		assert.Equal(t, expected, app)
	})

	t.Run("preserves malformed explicit apply-once for validation", func(t *testing.T) {
		explicit := map[string]interface{}{
			"name": "invalid-addon-policy", "type": v1alpha1.ApplyOncePolicyType,
		}
		app := map[string]interface{}{
			"spec": map[string]interface{}{"policies": []interface{}{explicit}},
		}
		expected := deepCopyMap(app)
		ensureAddonComponentStateKeepPolicy(app)
		assert.Equal(t, expected, app)
	})

	t.Run("uses deterministic name suffix", func(t *testing.T) {
		app := map[string]interface{}{"spec": map[string]interface{}{
			"policies": []interface{}{map[string]interface{}{
				"name": "addon-component-state-keep", "type": "garbage-collect",
				"properties": map[string]interface{}{},
			}},
		}}
		ensureAddonComponentStateKeepPolicy(app)
		policies := getPolicies(t, app)
		require.Len(t, policies, 2)
		assert.Equal(t, "addon-component-state-keep-2",
			policies[1].(map[string]interface{})["name"])
	})

	t.Run("is idempotent", func(t *testing.T) {
		app := map[string]interface{}{}
		ensureAddonComponentStateKeepPolicy(app)
		ensureAddonComponentStateKeepPolicy(app)
		assert.Len(t, getPolicies(t, app), 1)
	})
}

func TestAppendAuxComponentsGroupsAndOmitsEmpty(t *testing.T) {
	appMap := map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "Application",
		"spec": map[string]interface{}{
			"components": []interface{}{
				map[string]interface{}{"name": "fluxcd-ns", "type": "k8s-objects"},
			},
		},
	}
	def := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "ComponentDefinition",
		"metadata":   map[string]interface{}{"name": "helm", "creationTimestamp": nil},
		"status":     map[string]interface{}{"phase": "x"},
	}}
	groups := []auxComponent{
		{name: "addon-definitions", objects: []*unstructured.Unstructured{def}},
		{name: "addon-schemas", objects: nil}, // empty -> omitted
	}

	appendAuxComponents(appMap, groups)

	comps := appMap["spec"].(map[string]interface{})["components"].([]interface{})
	require.Len(t, comps, 2) // the original component + addon-definitions only

	added := comps[1].(map[string]interface{})
	assert.Equal(t, "addon-definitions", added["name"])
	assert.Equal(t, "k8s-objects", added["type"])

	objs := added["properties"].(map[string]interface{})["objects"].([]interface{})
	require.Len(t, objs, 1)
	obj := objs[0].(map[string]interface{})
	_, hasStatus := obj["status"]
	assert.False(t, hasStatus, "aux object status must be stripped")
	_, hasCT := obj["metadata"].(map[string]interface{})["creationTimestamp"]
	assert.False(t, hasCT, "aux object creationTimestamp must be stripped")
}

func TestResolveAndRenderFinalizesApplication(t *testing.T) {
	r := &rendererImpl{
		cli: fakeClientWithRegistry(t),
		findPackagesFn: func(_ context.Context, _ client.Client, addonNames, registryNames []string) ([]*pkgaddon.WholeAddonPackage, error) {
			assert.Equal(t, []string{"example"}, addonNames)
			assert.Empty(t, registryNames)
			return []*pkgaddon.WholeAddonPackage{{
				InstallPackage: pkgaddon.InstallPackage{
					Meta: pkgaddon.Meta{Name: "example", Version: "1.0.0"},
					AppTemplate: &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{"package.example/preserved": "true"},
					}},
				},
				RegistryName: "fixture",
			}}, nil
		},
	}

	res, err := r.resolveAndRender(context.Background(), api.AddonRequest{
		Name:                "example",
		SkipVersionValidate: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", res.ResolvedVersion)
	assert.Equal(t, "fixture", res.Registry)
	assert.Equal(t, "Application", res.Application["kind"])
	metadata, ok := res.Application["metadata"].(map[string]interface{})
	require.True(t, ok, "Application.metadata must be a map[string]interface{}")
	annotations, ok := metadata["annotations"].(map[string]interface{})
	require.True(t, ok, "metadata.annotations must be a map[string]interface{}")
	assert.Equal(t, "true", annotations["package.example/preserved"])
	assert.Equal(t, "skip", annotations[oam.AnnotationLastAppliedConfig])
	labels, ok := metadata["labels"].(map[string]interface{})
	require.True(t, ok, "metadata.labels must be a map[string]interface{}")
	assert.Equal(t, "fixture", labels[oam.LabelAddonRegistry])

	spec, ok := res.Application["spec"].(map[string]interface{})
	require.True(t, ok, "Application.spec must be a map[string]interface{}")
	comps, ok := spec["components"].([]interface{})
	require.True(t, ok, "spec.components must be a []interface{}")
	assert.NotEmpty(t, comps)

	policies, ok := spec["policies"].([]interface{})
	require.True(t, ok, "spec.policies must be a []interface{}")
	var foundDisabledApplyOnce bool
	for _, item := range policies {
		policy, ok := item.(map[string]interface{})
		if ok && policy["type"] == v1alpha1.ApplyOncePolicyType {
			assert.Equal(t, map[string]interface{}{"enable": false}, policy["properties"])
			foundDisabledApplyOnce = true
		}
	}
	assert.True(t, foundDisabledApplyOnce, "resolveAndRender must disable implicit apply-once")
}
