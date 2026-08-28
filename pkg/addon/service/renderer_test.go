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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/util/singleton"

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

func TestNewRenderer(t *testing.T) {
	r := NewRenderer()
	require.NotNil(t, r)
	_, ok := r.(*rendererImpl)
	assert.True(t, ok, "NewRenderer must return a *rendererImpl")
}

func TestRegisterInstallsTheDefaultRenderer(t *testing.T) {
	Register()
	assert.NotNil(t, api.DefaultRenderer())
	_, ok := api.DefaultRenderer().(*rendererImpl)
	assert.True(t, ok, "Register must install a *rendererImpl as the default renderer")
}

func TestFetchExactVersionDefaultsToRegistryLookup(t *testing.T) {
	r := &rendererImpl{cli: fakeClientWithRegistry(t)}
	_, err := r.fetchExactVersion(context.Background(), "missing-registry", "example", "1.0.0")
	require.Error(t, err, "with no fetchExactFn override, the default path must hit the real registry lookup")
}

func TestValidateSystemRequirements(t *testing.T) {
	t.Run("skips when rest config is unavailable", func(t *testing.T) {
		r := &rendererImpl{cli: fakeClientWithRegistry(t)}
		err := r.validateSystemRequirements(context.Background(), "example", &pkgaddon.InstallPackage{})
		assert.NoError(t, err)
	})

	t.Run("passes when the addon declares no system requirements", func(t *testing.T) {
		r := &rendererImpl{
			cli:    fakeClientWithRegistry(t),
			config: &rest.Config{Host: "https://example.invalid"},
		}
		err := r.validateSystemRequirements(context.Background(), "example", &pkgaddon.InstallPackage{})
		assert.NoError(t, err)
	})

	t.Run("wraps discovery client construction errors", func(t *testing.T) {
		r := &rendererImpl{
			cli: fakeClientWithRegistry(t),
			config: &rest.Config{
				Host:            "https://example.invalid",
				TLSClientConfig: rest.TLSClientConfig{Insecure: true, CAData: []byte("bogus-ca")},
			},
		}
		err := r.validateSystemRequirements(context.Background(), "example", &pkgaddon.InstallPackage{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "build discovery client")
	})

	t.Run("wraps requirement check failures", func(t *testing.T) {
		r := &rendererImpl{
			cli:    fakeClientWithRegistry(t),
			config: &rest.Config{Host: "https://example.invalid"},
		}
		installPkg := &pkgaddon.InstallPackage{
			Meta: pkgaddon.Meta{SystemRequirements: &pkgaddon.SystemRequirements{VelaVersion: ">=1.0.0"}},
		}
		err := r.validateSystemRequirements(context.Background(), "example", installPkg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not meet system requirements")
	})
}

func TestAuxComponentsPropagatesRenderErrors(t *testing.T) {
	r := &rendererImpl{cli: fakeClientWithRegistry(t)}
	installPkg := &pkgaddon.InstallPackage{
		Definitions: []pkgaddon.ElementFile{{Name: "broken.yaml", Data: "{"}},
	}
	_, err := r.auxComponents(context.Background(), installPkg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken.yaml")
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

// TestRenderAddonConcurrentMissesResolveOnce pins the singleflight collapse:
// concurrent requests for the same not-yet-cached key must not each pay the
// full registry/render cost. Without it, N concurrent misses race resolveFn
// N times before any of them observes another's cache write.
func TestRenderAddonConcurrentMissesResolveOnce(t *testing.T) {
	r := &rendererImpl{cli: fakeClientWithRegistry(t)}
	var calls int32
	release := make(chan struct{})
	r.resolveFn = func(_ context.Context, req api.AddonRequest) (*api.AddonResult, error) {
		atomic.AddInt32(&calls, 1)
		<-release
		return &api.AddonResult{ResolvedVersion: req.Version}, nil
	}

	req := api.AddonRequest{Name: "example", Version: "1.0.0", Properties: map[string]interface{}{"replicas": 1}}
	const concurrency = 10
	type outcome struct {
		res *api.AddonResult
		err error
	}
	outcomes := make(chan outcome, concurrency)
	for range concurrency {
		go func() {
			res, err := r.RenderAddon(context.Background(), req)
			outcomes <- outcome{res, err}
		}()
	}

	// Give every goroutine a chance to reach resolveFn and block on release
	// before letting the single in-flight call complete.
	time.Sleep(50 * time.Millisecond)
	close(release)

	first := <-outcomes
	require.NoError(t, first.err)
	for i := 1; i < concurrency; i++ {
		o := <-outcomes
		require.NoError(t, o.err)
		assert.Same(t, first.res, o.res, "every concurrent caller must observe the same resolved result")
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "concurrent misses on the same key must resolve exactly once")
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
	ha, okA := hashProperties(a)
	hb, okB := hashProperties(b)
	require.True(t, okA)
	require.True(t, okB)
	assert.Equal(t, ha, hb, "same map (any order) must hash equally")

	c := map[string]interface{}{"a": 1, "b": 3}
	hc, okC := hashProperties(c)
	require.True(t, okC)
	assert.NotEqual(t, ha, hc, "different maps must hash differently")

	hn1, ok1 := hashProperties(nil)
	hn2, ok2 := hashProperties(nil)
	require.True(t, ok1)
	require.True(t, ok2)
	assert.Equal(t, hn1, hn2, "nil must hash stably")
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
	// Name the components explicitly rather than asserting NotEmpty. This fixture's
	// AppTemplate carries no components and no YAML/CUE templates, so the only
	// component is the args Secret that RenderArgsSecret always emits; a bare
	// NotEmpty would pass on that alone and assert nothing about the grouping.
	compNames := make([]string, 0, len(comps))
	for _, item := range comps {
		comp, isMap := item.(map[string]interface{})
		require.True(t, isMap, "each component must be a map[string]interface{}")
		name, _ := comp["name"].(string)
		compNames = append(compNames, name)
		assert.Equal(t, "k8s-objects", comp["type"], "auxiliaries are wrapped as k8s-objects")
	}
	assert.Equal(t, []string{"addon-secret"}, compNames)

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

// TestAppendAuxComponentsAvoidsNameCollisions covers an addon whose own template
// authors a component named like one of the fixed auxiliary categories. Emitting
// the auxiliary under the same name would produce a duplicate component and fail
// Application validation.
func TestAppendAuxComponentsAvoidsNameCollisions(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]interface{}{"name": "cm"},
	}}

	appMap := map[string]interface{}{
		"spec": map[string]interface{}{
			"components": []interface{}{
				map[string]interface{}{"name": "addon-definitions", "type": "k8s-objects"},
				map[string]interface{}{"name": "addon-definitions-2", "type": "k8s-objects"},
			},
		},
	}

	appendAuxComponents(appMap, []auxComponent{{name: "addon-definitions", objects: []*unstructured.Unstructured{obj}}})

	comps := appMap["spec"].(map[string]interface{})["components"].([]interface{})
	names := make([]string, 0, len(comps))
	seen := map[string]bool{}
	for _, c := range comps {
		name := c.(map[string]interface{})["name"].(string)
		assert.False(t, seen[name], "duplicate component name %q", name)
		seen[name] = true
		names = append(names, name)
	}
	assert.Equal(t, []string{"addon-definitions", "addon-definitions-2", "addon-definitions-3"}, names,
		"the auxiliary must skip past every name the addon already used")
}

// TestUniqueComponentName pins the suffixing behavior directly.
func TestUniqueComponentName(t *testing.T) {
	assert.Equal(t, "addon-views", uniqueComponentName("addon-views", map[string]bool{}))
	assert.Equal(t, "addon-views-2", uniqueComponentName("addon-views", map[string]bool{"addon-views": true}))
	assert.Equal(t, "addon-views-3", uniqueComponentName("addon-views",
		map[string]bool{"addon-views": true, "addon-views-2": true}))
}

// TestUnhashablePropertiesBypassTheCache covers the collision the placeholder key
// used to allow: two requests whose properties cannot be marshaled would share one
// key and be served each other's render. Such requests must skip the cache instead.
func TestUnhashablePropertiesBypassTheCache(t *testing.T) {
	unhashable := map[string]interface{}{"fn": func() {}}

	_, ok := hashProperties(unhashable)
	assert.False(t, ok, "a non-marshalable map must not yield a usable hash")

	_, cacheable := cacheKey(api.AddonRequest{Name: "a", Version: "1.0.0", Properties: unhashable})
	assert.False(t, cacheable, "an unhashable request must not be cacheable")

	// Two distinct unhashable requests must each get their own render rather than
	// sharing one cache entry.
	calls := 0
	r := &rendererImpl{resolveFn: func(_ context.Context, req api.AddonRequest) (*api.AddonResult, error) {
		calls++
		return &api.AddonResult{ResolvedVersion: req.Version, Registry: fmt.Sprint(calls)}, nil
	}}

	first, err := r.RenderAddon(context.Background(), api.AddonRequest{
		Name: "a", Version: "1.0.0", Properties: map[string]interface{}{"fn": func() {}, "x": 1},
	})
	require.NoError(t, err)
	second, err := r.RenderAddon(context.Background(), api.AddonRequest{
		Name: "a", Version: "1.0.0", Properties: map[string]interface{}{"fn": func() {}, "x": 2},
	})
	require.NoError(t, err)

	assert.Equal(t, 2, calls, "each unhashable request must be resolved on its own")
	assert.NotEqual(t, first.Registry, second.Registry, "the second request must not be served the first result")
}

// TestResolvePinnedVersionTriesEveryCandidateRegistry covers the two symptoms of
// the old latest-first resolution: an addon present in several registries where
// only a later one publishes the pin, and a pin that must not depend on the
// latest release loading at all.
func TestResolvePinnedVersionTriesEveryCandidateRegistry(t *testing.T) {
	t.Run("falls through to the registry that has the version", func(t *testing.T) {
		var tried []string
		r := &rendererImpl{
			fetchExactFn: func(_ context.Context, registryName, addonName, version string) (*pkgaddon.InstallPackage, error) {
				tried = append(tried, registryName)
				if registryName != "second" {
					return nil, pkgaddon.ErrNotExist
				}
				return &pkgaddon.InstallPackage{Meta: pkgaddon.Meta{Name: addonName, Version: version}}, nil
			},
		}

		pkg, reg, err := r.resolvePinnedVersion(context.Background(), "example", "2.0.0", []string{"first", "second"})
		require.NoError(t, err)
		assert.Equal(t, "second", reg)
		assert.Equal(t, "2.0.0", pkg.Version)
		assert.Equal(t, []string{"first", "second"}, tried, "every candidate must be tried in order")
	})

	t.Run("stops at the first registry that serves the pin", func(t *testing.T) {
		var tried []string
		r := &rendererImpl{
			fetchExactFn: func(_ context.Context, registryName, addonName, version string) (*pkgaddon.InstallPackage, error) {
				tried = append(tried, registryName)
				return &pkgaddon.InstallPackage{Meta: pkgaddon.Meta{Name: addonName, Version: version}}, nil
			},
		}

		_, reg, err := r.resolvePinnedVersion(context.Background(), "example", "1.0.0", []string{"first", "second"})
		require.NoError(t, err)
		assert.Equal(t, "first", reg)
		assert.Equal(t, []string{"first"}, tried)
	})

	t.Run("reports every registry's reason when none has the pin", func(t *testing.T) {
		r := &rendererImpl{
			fetchExactFn: func(_ context.Context, registryName, _, _ string) (*pkgaddon.InstallPackage, error) {
				return nil, fmt.Errorf("boom in %s", registryName)
			},
		}

		_, _, err := r.resolvePinnedVersion(context.Background(), "example", "9.9.9", []string{"first", "second"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom in first")
		assert.Contains(t, err.Error(), "boom in second", "the error must not hide later registries' reasons")
	})
}

// TestResolvePinnedVersionEmptyCandidatesListsRegistries covers the path taken
// when the caller does not narrow the search to one registry: candidates must
// be filled in from every configured registry before any lookup is tried.
func TestResolvePinnedVersionEmptyCandidatesListsRegistries(t *testing.T) {
	t.Run("fails when no registry is configured", func(t *testing.T) {
		r := &rendererImpl{cli: fakeClientWithRegistry(t)}
		_, _, err := r.resolvePinnedVersion(context.Background(), "example", "1.0.0", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no addon registries are configured")
	})

	t.Run("tries every registry discovered from the store", func(t *testing.T) {
		cli := fakeClientWithRegistry(t)
		require.NoError(t, pkgaddon.NewRegistryDataStore(cli).AddRegistry(context.Background(), pkgaddon.Registry{Name: "discovered"}))

		var tried []string
		r := &rendererImpl{
			cli: cli,
			fetchExactFn: func(_ context.Context, registryName, addonName, version string) (*pkgaddon.InstallPackage, error) {
				tried = append(tried, registryName)
				return &pkgaddon.InstallPackage{Meta: pkgaddon.Meta{Name: addonName, Version: version}}, nil
			},
		}

		pkg, reg, err := r.resolvePinnedVersion(context.Background(), "example", "1.0.0", nil)
		require.NoError(t, err)
		assert.Equal(t, "discovered", reg)
		assert.Equal(t, "1.0.0", pkg.Version)
		assert.Equal(t, []string{"discovered"}, tried)
	})
}

// TestResolveAndRenderPinnedVersionDoesNotNeedLatest is the regression guard for
// #25: a broken latest release must not block a valid pin, so the pinned path must
// not consult the latest-package lookup at all.
func TestResolveAndRenderPinnedVersionDoesNotNeedLatest(t *testing.T) {
	latestCalls := 0
	r := &rendererImpl{
		cli: fakeClientWithRegistry(t),
		findPackagesFn: func(context.Context, client.Client, []string, []string) ([]*pkgaddon.WholeAddonPackage, error) {
			latestCalls++
			return nil, fmt.Errorf("latest release is broken")
		},
		fetchExactFn: func(_ context.Context, registryName, addonName, version string) (*pkgaddon.InstallPackage, error) {
			return &pkgaddon.InstallPackage{
				Meta:        pkgaddon.Meta{Name: addonName, Version: version},
				AppTemplate: &v1beta1.Application{},
			}, nil
		},
	}

	res, err := r.resolveAndRender(context.Background(), api.AddonRequest{
		Name: "example", Version: "1.0.0", Registry: "fixture", SkipVersionValidate: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", res.ResolvedVersion)
	assert.Equal(t, "fixture", res.Registry)
	assert.Zero(t, latestCalls, "a pinned request must not depend on resolving latest")
}

// TestClientAndRestConfigFallBackToSingleton covers the production path where
// no client/config was injected, so client() and restConfig() must read the
// kubevela-pkg singletons. It sets those process-wide singletons via Set (not
// the real loader, which would dial a live cluster), so it must stay the last
// test in this file: every earlier test relies on a zero-value rendererImpl
// falling through this branch to a still-nil singleton.
func TestClientAndRestConfigFallBackToSingleton(t *testing.T) {
	fakeCli := fakeClientWithRegistry(t)
	singleton.KubeClient.Set(fakeCli)
	cfg := &rest.Config{Host: "https://example.invalid"}
	singleton.KubeConfig.Set(cfg)

	r := &rendererImpl{}
	assert.Same(t, fakeCli, r.client())
	assert.Same(t, cfg, r.restConfig())
}
