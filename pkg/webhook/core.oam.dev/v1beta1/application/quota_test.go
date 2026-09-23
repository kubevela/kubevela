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
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kubevela/pkg/controller/sharding"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	velacache "github.com/oam-dev/kubevela/pkg/cache"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// appWith builds an Application whose components are the given types, in order.
func appWith(namespace, name string, types ...string) *v1beta1.Application {
	comps := make([]common.ApplicationComponent, 0, len(types))
	for i, t := range types {
		comps = append(comps, common.ApplicationComponent{Name: fmt.Sprintf("c%d", i), Type: t})
	}
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       v1beta1.ApplicationSpec{Components: comps},
	}
}

// quotaClient registers ComponentTypeIndex, so the indexed path is exercised.
func quotaClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1beta1.Application{}, velacache.DefinitionUsageIndex, func(obj client.Object) []string {
			return velacache.DefinitionUsageOf(obj.(*v1beta1.Application))
		}).
		WithObjects(objs...).Build()
}

// withIndex runs fn as the controller has it when the feature is on.
func withIndex(t *testing.T, fn func()) {
	t.Helper()
	prev := velacache.DefinitionUsageIndexed
	velacache.DefinitionUsageIndexed = true
	defer func() { velacache.DefinitionUsageIndexed = prev }()
	fn()
}

func TestCountComponentsOfType(t *testing.T) {
	// tenant-a holds 3 webservice components: 2 in app-1, 1 in app-2.
	objs := []client.Object{
		appWith("tenant-a", "app-1", "webservice", "webservice"),
		appWith("tenant-a", "app-2", "webservice", "worker"),
		appWith("tenant-a", "app-other", "worker"),
		appWith("tenant-b", "app-elsewhere", "webservice", "webservice"),
	}

	testCases := map[string]struct {
		componentType string
		excluding     string
		expected      int
	}{
		"counts every occurrence, not every Application": {componentType: "webservice", expected: 3},
		"a type nobody uses counts zero":                 {componentType: "gateway", expected: 0},
		"another namespace does not contribute":          {componentType: "worker", expected: 2},
		"the excluded Application is left out":           {componentType: "webservice", excluding: "app-1", expected: 1},
		"excluding a name that is not there changes nothing": {
			componentType: "webservice", excluding: "app-absent", expected: 3},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Both paths must agree: the index only narrows the list, it must not
			// change the count.
			for _, indexed := range []bool{false, true} {
				run := func() {
					got, err := countUsage(context.Background(), quotaClient(t, objs...),
						indexed, "tenant-a", velacache.UsageComponent, tc.componentType, tc.excluding)
					require.NoError(t, err)
					assert.Equal(t, tc.expected, got, "indexed=%v", indexed)
				}
				if indexed {
					withIndex(t, run)
				} else {
					run()
				}
			}
		})
	}
}

// A pinned type counts against the definition it pins, from either side.
func TestCountComponentsOfTypePinnedTypes(t *testing.T) {
	objs := []client.Object{
		appWith("tenant-a", "app-1", "webservice@v1"),
		appWith("tenant-a", "app-2", "webservice"),
	}
	withIndex(t, func() {
		got, err := countUsage(context.Background(), quotaClient(t, objs...),
			true, "tenant-a", velacache.UsageComponent, "webservice", "")
		require.NoError(t, err)
		assert.Equal(t, 2, got)
	})
}

// Each way an Application at the limit can be edited. Without the exclusion, an
// edit that changes nothing fails its own quota.
func TestQuotaSelfExclusionOnUpdate(t *testing.T) {
	const limit = 3
	// tenant-a is at the limit: app-1 has 2 webservice, app-2 has 1.
	objs := []client.Object{
		appWith("tenant-a", "app-1", "webservice", "webservice"),
		appWith("tenant-a", "app-2", "webservice"),
	}

	testCases := map[string]struct {
		incoming *v1beta1.Application
		total    int
		refused  bool
	}{
		"creating another is refused":      {incoming: appWith("tenant-a", "app-3", "webservice"), total: 4, refused: true},
		"a no-op edit at the limit passes": {incoming: appWith("tenant-a", "app-1", "webservice", "webservice"), total: 3},
		"an edit that grows past it is refused": {
			incoming: appWith("tenant-a", "app-1", "webservice", "webservice", "webservice"), total: 4, refused: true},
		"an edit that shrinks passes": {incoming: appWith("tenant-a", "app-1", "webservice"), total: 2},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			existing, err := countUsage(context.Background(), quotaClient(t, objs...),
				false, "tenant-a", velacache.UsageComponent, "webservice", tc.incoming.Name)
			require.NoError(t, err)
			total := existing + usageInApp(tc.incoming, velacache.UsageComponent, "webservice")
			assert.Equal(t, tc.total, total)
			assert.Equal(t, tc.refused, total > limit)
		})
	}
}

func TestComponentsOfType(t *testing.T) {
	app := appWith("tenant-a", "app", "webservice", "webservice@v1", "worker")
	assert.Equal(t, 2, usageInApp(app, velacache.UsageComponent, "webservice"))
	assert.Equal(t, 1, usageInApp(app, velacache.UsageComponent, "worker"))
	assert.Equal(t, 0, usageInApp(app, velacache.UsageComponent, "gateway"))
	assert.Equal(t, 0, usageInApp(appWith("tenant-a", "empty"), velacache.UsageComponent, "webservice"))
}

// quotaCompDef carries a quota and nothing else, which is legal: a definition may
// cap usage without restricting who may use it.
func quotaCompDef(name string, quota ...common.NamespaceQuota) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: oam.SystemDefinitionNamespace},
		Spec: v1beta1.ComponentDefinitionSpec{
			Restrictions: &common.DefinitionRestrictions{Quota: quota},
		},
	}
}

func i32(n int32) *int32 { return &n }

func TestValidateQuotaRefusal(t *testing.T) {
	def := quotaCompDef("webservice", common.NamespaceQuota{Limit: i32(3)})
	// tenant-a already holds 3.
	existing := []client.Object{
		appWith("tenant-a", "app-1", "webservice", "webservice"),
		appWith("tenant-a", "app-2", "webservice"),
	}

	t.Run("a create over the limit is refused, at the offending field", func(t *testing.T) {
		h := nsRestrictHandler(t, append([]client.Object{def}, existing...)...)
		app := appWith("tenant-a", "app-3", "worker", "worker", "webservice")
		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(), app, nil)
		require.Len(t, errs, 1)
		assert.Equal(t, "spec.components[2].type", errs[0].Field)
		assert.Contains(t, errs[0].Detail, `quota for component type "webservice"`)
		assert.Contains(t, errs[0].Detail, `namespace "tenant-a"`)
		// The counts are the operator's, through the log, not the author's.
		assert.NotContains(t, errs[0].Detail, "3")
		assert.NotContains(t, errs[0].Detail, "4")
		assert.Empty(t, warnings)
	})

	t.Run("an edit that changes nothing is admitted at the limit", func(t *testing.T) {
		h := nsRestrictHandler(t, append([]client.Object{def}, existing...)...)
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice", "webservice"), nil)
		assert.Empty(t, errs)
	})

	t.Run("an edit that grows past the limit is refused", func(t *testing.T) {
		h := nsRestrictHandler(t, append([]client.Object{def}, existing...)...)
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice", "webservice", "webservice"), nil)
		assert.Len(t, errs, 3)
	})

	t.Run("another namespace has its own budget", func(t *testing.T) {
		h := nsRestrictHandler(t, append([]client.Object{def}, existing...)...)
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-b", "app-1", "webservice", "webservice", "webservice"), nil)
		assert.Empty(t, errs)
	})

	t.Run("a type with no quota is not counted", func(t *testing.T) {
		h := nsRestrictHandler(t, append([]client.Object{def, quotaCompDef("worker")}, existing...)...)
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-3", "worker", "worker", "worker", "worker"), nil)
		assert.Empty(t, errs)
	})

	t.Run("a limit of 0 forbids the type outright", func(t *testing.T) {
		h := nsRestrictHandler(t, quotaCompDef("webservice", common.NamespaceQuota{Limit: i32(0)}))
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice"), nil)
		assert.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `quota for component type "webservice"`)
	})

	t.Run("the system namespace is exempt", func(t *testing.T) {
		h := nsRestrictHandler(t, quotaCompDef("webservice", common.NamespaceQuota{Limit: i32(0)}))
		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
			appWith(types.DefaultKubeVelaNS, "addon-app", "webservice", "webservice"), nil)
		assert.Empty(t, errs)
		assert.Empty(t, warnings)
	})

	t.Run("the feature gate turns it off", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
			features.RestrictDefinitionNamespaces, false)
		h := nsRestrictHandler(t, append([]client.Object{def}, existing...)...)
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-3", "webservice"), nil)
		assert.Empty(t, errs)
	})
}

func TestValidateQuotaWarning(t *testing.T) {
	existing := []client.Object{appWith("tenant-a", "app-1", "webservice", "webservice")}

	t.Run("below the threshold nothing is said", func(t *testing.T) {
		h := nsRestrictHandler(t, quotaCompDef("webservice", common.NamespaceQuota{Warn: i32(4), Limit: i32(5)}))
		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-2", "webservice"), nil)
		assert.Empty(t, errs)
		assert.Empty(t, warnings)
	})

	t.Run("at the threshold it is admitted with a warning", func(t *testing.T) {
		h := nsRestrictHandler(t, append([]client.Object{
			quotaCompDef("webservice", common.NamespaceQuota{Warn: i32(3), Limit: i32(5)})}, existing...)...)
		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-2", "webservice"), nil)
		assert.Empty(t, errs)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], `ComponentDefinition "webservice"`)
		assert.Contains(t, warnings[0], "using 3 of the 5 allowed")
	})

	t.Run("a warn-only quota warns and never refuses", func(t *testing.T) {
		h := nsRestrictHandler(t, append([]client.Object{
			quotaCompDef("webservice", common.NamespaceQuota{Warn: i32(3)})}, existing...)...)
		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-2", "webservice", "webservice", "webservice"), nil)
		assert.Empty(t, errs)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "using 5")
		assert.NotContains(t, warnings[0], "allowed")
	})

	t.Run("a refusal warns about nothing, it refuses", func(t *testing.T) {
		h := nsRestrictHandler(t, append([]client.Object{
			quotaCompDef("webservice", common.NamespaceQuota{Warn: i32(3), Limit: i32(3)})}, existing...)...)
		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-2", "webservice", "webservice"), nil)
		assert.Len(t, errs, 2)
		assert.Empty(t, warnings)
	})
}

// A quota varying by namespace label, with an unmatched namespace falling through
// to the default entry.
func TestValidateQuotaByNamespaceClass(t *testing.T) {
	def := quotaCompDef("webservice",
		common.NamespaceQuota{
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "gold"}},
			Limit:             i32(20),
		},
		common.NamespaceQuota{Namespaces: []string{"tenant-*"}, Limit: i32(1)},
		common.NamespaceQuota{Limit: i32(2)},
	)
	objs := []client.Object{
		def,
		labelledNamespace("gold-ns", map[string]string{"tier": "gold"}),
		labelledNamespace("tenant-a", nil),
		labelledNamespace("other", nil),
	}

	testCases := map[string]struct {
		namespace  string
		components int
		refused    bool
	}{
		"a gold namespace gets the selector's ceiling":  {"gold-ns", 5, false},
		"a tenant namespace gets the name glob's":       {"tenant-a", 2, true},
		"a tenant namespace within its ceiling passes":  {"tenant-a", 1, false},
		"anything else falls through to the last entry": {"other", 3, true},
		"and passes within it":                          {"other", 2, false},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			h := nsRestrictHandler(t, objs...)
			comps := make([]string, tc.components)
			for i := range comps {
				comps[i] = "webservice"
			}
			errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
				appWith(tc.namespace, "app", comps...), nil)
			assert.Equal(t, tc.refused, len(errs) > 0, "errors: %v", errs)
		})
	}
}

// One definition named two ways: one budget, and one warning.
func TestQuotaCountsAPinnedTypeOnce(t *testing.T) {
	h := nsRestrictHandler(t, []client.Object{
		quotaCompDef("webservice", common.NamespaceQuota{Warn: i32(2), Limit: i32(10)}),
		appWith("tenant-a", "app-1", "webservice"),
	}...)
	errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
		appWith("tenant-a", "app-2", "webservice", "webservice@v1"), nil)
	assert.Empty(t, errs)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "using 3 of the 10 allowed")
}

// The count runs once, but a refusal still names every component behind it.
func TestQuotaRefusalNamesEverySpelling(t *testing.T) {
	h := nsRestrictHandler(t, quotaCompDef("webservice", common.NamespaceQuota{Limit: i32(1)}))
	errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
		appWith("tenant-a", "app-1", "webservice", "webservice@v1"), nil)
	require.Len(t, errs, 2)
	fields := []string{errs[0].Field, errs[1].Field}
	assert.ElementsMatch(t, []string{"spec.components[0].type", "spec.components[1].type"}, fields)
}

// A sharded cache holds one shard's Applications while the webhook runs on the
// master, so the count must go to the API server, without the cache's index.
func TestQuotaReaderUnderSharding(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	cached := fake.NewClientBuilder().WithScheme(scheme).Build()
	live := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := &ValidatingHandler{Client: cached, APIReader: live}

	prevIndexed := velacache.DefinitionUsageIndexed
	velacache.DefinitionUsageIndexed = true
	defer func() { velacache.DefinitionUsageIndexed = prevIndexed }()

	prevSharding := sharding.EnableSharding
	defer func() { sharding.EnableSharding = prevSharding }()

	sharding.EnableSharding = false
	reader, indexed := h.quotaReader()
	assert.Same(t, cached, reader, "unsharded, the cache holds the whole namespace")
	assert.True(t, indexed)

	sharding.EnableSharding = true
	reader, indexed = h.quotaReader()
	assert.Same(t, live, reader, "sharded, the cache holds only this shard")
	assert.False(t, indexed, "the index lives on the cache, not the API server")
}

// With no APIReader to fall back to, the cache is used. Undercounting beats
// failing every admission.
func TestQuotaReaderWithoutAPIReader(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	cached := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := &ValidatingHandler{Client: cached}

	prevSharding := sharding.EnableSharding
	sharding.EnableSharding = true
	defer func() { sharding.EnableSharding = prevSharding }()

	reader, _ := h.quotaReader()
	assert.Same(t, cached, reader)
}

// A quota that cannot be evaluated must refuse, not admit. Admitting on a failed
// read would make a quota bypassable by making the count fail.
func TestQuotaFailsClosed(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	def := quotaCompDef("webservice", common.NamespaceQuota{Limit: i32(3)})

	t.Run("the count cannot be read", func(t *testing.T) {
		h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(def).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(_ context.Context, _ client.WithWatch, list client.ObjectList, _ ...client.ListOption) error {
					if _, ok := list.(*v1beta1.ApplicationList); ok {
						return errors.New("boom")
					}
					return nil
				},
			}).Build()}

		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice"), nil)
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInternal, errs[0].Type, "retryable, not a policy violation")
		assert.Contains(t, errs[0].Detail, "counting its use")
		assert.Empty(t, warnings)
	})

	t.Run("the namespace cannot be read", func(t *testing.T) {
		// Selecting the entry needs the namespace's labels.
		selecting := quotaCompDef("webservice", common.NamespaceQuota{
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "gold"}},
			Limit:             i32(3),
		})
		h := &ValidatingHandler{
			Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(selecting).Build(),
			APIReader: fake.NewClientBuilder().WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
						if _, ok := obj.(*corev1.Namespace); ok {
							return errors.New("boom")
						}
						return nil
					},
				}).Build(),
		}

		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice"), nil)
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInternal, errs[0].Type)
		assert.Contains(t, errs[0].Detail, "reading namespace")
	})
}

// The namespace is read once however many definitions select on its labels.
func TestQuotaReadsTheNamespaceOnce(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "gold"}}

	reads := 0
	h := &ValidatingHandler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			quotaCompDef("webservice", common.NamespaceQuota{NamespaceSelector: selector, Limit: i32(9)}),
			quotaCompDef("worker", common.NamespaceQuota{NamespaceSelector: selector, Limit: i32(9)}),
		).Build(),
		APIReader: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "tenant-a", Labels: map[string]string{"tier": "gold"}}}).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*corev1.Namespace); ok {
						reads++
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).Build(),
	}

	errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
		appWith("tenant-a", "app-1", "webservice", "worker"), nil)
	assert.Empty(t, errs)
	assert.Equal(t, 1, reads)
}

// The check runs on every Application admission in the cluster, and almost no
// definition declares a quota, so the path without one must not list anything.
func TestNoQuotaCostsNoList(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	lists, namespaceGets := 0, 0
	count := interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*v1beta1.ApplicationList); ok {
				lists++
			}
			return c.List(ctx, list, opts...)
		},
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Namespace); ok {
				namespaceGets++
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}

	unrestricted := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: oam.SystemDefinitionNamespace},
	}
	restrictedOnly := restrictedCompDef("worker", oam.SystemDefinitionNamespace, "tenant-*")

	h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(unrestricted, restrictedOnly,
			appWith("tenant-a", "neighbour", "webservice", "worker")).
		WithInterceptorFuncs(count).Build()}

	errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
		appWith("tenant-a", "app-1", "webservice", "worker"), nil)
	assert.Empty(t, errs)
	assert.Empty(t, warnings)
	assert.Zero(t, lists, "no quota, so the namespace is never counted")
	assert.Zero(t, namespaceGets, "and a name glob settles without reading the namespace")
}

// And one quota costs exactly one list, however many components draw on it.
func TestOneQuotaCostsOneList(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))

	lists := 0
	h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(quotaCompDef("webservice", common.NamespaceQuota{Limit: i32(99)})).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*v1beta1.ApplicationList); ok {
					lists++
				}
				return c.List(ctx, list, opts...)
			},
		}).Build()}

	errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
		appWith("tenant-a", "app-1", "webservice", "webservice@v1", "webservice"), nil)
	assert.Empty(t, errs)
	assert.Equal(t, 1, lists)
}

// quotaTraitDef is a TraitDefinition carrying a quota and nothing else.
func quotaTraitDef(name string, quota ...common.NamespaceQuota) *v1beta1.TraitDefinition {
	return &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: oam.SystemDefinitionNamespace},
		Spec: v1beta1.TraitDefinitionSpec{
			Restrictions: &common.DefinitionRestrictions{Quota: quota},
		},
	}
}

// appWithTraits builds an Application of one component carrying the given traits.
func appWithTraits(namespace, name, compType string, traits ...string) *v1beta1.Application {
	comp := common.ApplicationComponent{Name: "web", Type: compType}
	for _, tr := range traits {
		comp.Traits = append(comp.Traits, common.ApplicationTrait{Type: tr})
	}
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{comp}},
	}
}

// A tenant can burn resources through traits as readily as through components, so
// traits carry a quota of their own.
func TestValidateTraitQuota(t *testing.T) {
	def := quotaTraitDef("gateway", common.NamespaceQuota{Warn: i32(2), Limit: i32(2)})

	t.Run("within the limit", func(t *testing.T) {
		h := nsRestrictHandler(t, def)
		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
			appWithTraits("tenant-a", "app-1", "webservice", "gateway"), nil)
		assert.Empty(t, errs)
		assert.Empty(t, warnings)
	})

	t.Run("a trait on two components counts twice", func(t *testing.T) {
		h := nsRestrictHandler(t, def, appWithTraits("tenant-a", "other", "webservice", "gateway"))
		app := appWithTraits("tenant-a", "app-1", "webservice", "gateway", "gateway")
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(), app, nil)
		require.Len(t, errs, 2, "one per offending trait")
		assert.Equal(t, "spec.components[0].traits[0].type", errs[0].Field)
		assert.Contains(t, errs[0].Detail, `quota for trait type "gateway"`)
	})

	t.Run("at the threshold it warns", func(t *testing.T) {
		h := nsRestrictHandler(t, quotaTraitDef("gateway", common.NamespaceQuota{Warn: i32(2), Limit: i32(9)}),
			appWithTraits("tenant-a", "other", "webservice", "gateway"))
		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
			appWithTraits("tenant-a", "app-1", "webservice", "gateway"), nil)
		assert.Empty(t, errs)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], `TraitDefinition "gateway"`)
		assert.Contains(t, warnings[0], "using 2 of the 9 allowed")
	})
}

// A component type and a trait type may share a name. They are separate
// definitions, so they get separate budgets.
func TestQuotaKeepsComponentAndTraitApart(t *testing.T) {
	h := nsRestrictHandler(t,
		quotaCompDef("gateway", common.NamespaceQuota{Limit: i32(9)}),
		quotaTraitDef("gateway", common.NamespaceQuota{Limit: i32(1)}),
		appWithTraits("tenant-a", "other", "gateway", "gateway"))

	// The namespace holds one gateway component and one gateway trait. The
	// component budget has room; the trait budget does not.
	errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
		appWithTraits("tenant-a", "app-1", "gateway", "gateway"), nil)
	require.Len(t, errs, 1)
	assert.Equal(t, "spec.components[0].traits[0].type", errs[0].Field)
	assert.Contains(t, errs[0].Detail, `quota for trait type "gateway"`)
}

// An unread restriction is not an absent one. A definition that cannot be read for
// any reason but NotFound must refuse the Application, or a restriction is
// bypassable by making the read fail.
func TestUnreadableDefinitionFailsClosed(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	testCases := map[string]struct {
		getErr  error
		wantErr bool
	}{
		"an API failure refuses":          {errors.New("etcd is unhappy"), true},
		"a forbidden read refuses":        {apierrors.NewForbidden(schema.GroupResource{Resource: "componentdefinitions"}, "webservice", errors.New("nope")), true},
		"a missing definition is skipped": {apierrors.NewNotFound(schema.GroupResource{Resource: "componentdefinitions"}, "webservice"), false},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*v1beta1.ComponentDefinition); ok {
							return tc.getErr
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).Build()}

			errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
				appWith("tenant-a", "app-1", "webservice"), nil)
			if !tc.wantErr {
				// ValidateComponents reports a missing definition, and better.
				assert.Empty(t, errs)
				return
			}
			require.Len(t, errs, 1)
			assert.Equal(t, field.ErrorTypeInternal, errs[0].Type, "retryable, not a policy violation")
			assert.Contains(t, errs[0].Detail, "cannot read ComponentDefinition")
		})
	}
}

// Lowering a quota under a namespace already above it must leave that namespace
// drainable. An edit that does not add to this Application's own use is admitted
// and flagged, or the only way back under the new limit would be deleting whole
// Applications, which the docs do not ask anyone to do.
func TestQuotaLoweredUnderANamespaceStaysDrainable(t *testing.T) {
	// tenant-a holds 6 webservice components against a limit that is now 2.
	def := quotaCompDef("webservice", common.NamespaceQuota{Limit: i32(2)})
	others := appWith("tenant-a", "app-2", "webservice", "webservice", "webservice")
	old := appWith("tenant-a", "app-1", "webservice", "webservice", "webservice")

	t.Run("shrinking is admitted, with a warning", func(t *testing.T) {
		h := nsRestrictHandler(t, def, others, old)
		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice"), old)
		assert.Empty(t, errs, "an edit that reduces its own use must get through")
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "over its quota at 4 of the 2 allowed")
		assert.Contains(t, warnings[0], "does not add to it")
	})

	t.Run("holding steady is admitted too", func(t *testing.T) {
		h := nsRestrictHandler(t, def, others, old)
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice", "webservice", "webservice"), old)
		assert.Empty(t, errs)
	})

	t.Run("growing is still refused", func(t *testing.T) {
		h := nsRestrictHandler(t, def, others, old)
		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice", "webservice", "webservice", "webservice"), old)
		assert.Len(t, errs, 4, "one per component that contributed")
		assert.Empty(t, warnings)
	})

	t.Run("a create over the limit is still refused", func(t *testing.T) {
		h := nsRestrictHandler(t, def, others, old)
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-3", "webservice"), nil)
		assert.Len(t, errs, 1, "no oldApp means nothing to be no worse than")
	})
}

// A namespace can be lifted out of every quota by annotating it, for getting out
// of the way of an incident. Namespaces are cluster scoped, so this needs cluster
// level RBAC to set.
func TestQuotaExemptNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	exemptNS := func(annotations map[string]string) *corev1.Namespace {
		return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant-a", Annotations: annotations}}
	}
	// tenant-a is already well past a limit of 1.
	objs := func(ns *corev1.Namespace) []client.Object {
		return []client.Object{ns,
			quotaCompDef("webservice", common.NamespaceQuota{Limit: i32(1)}),
			appWith("tenant-a", "neighbour", "webservice", "webservice", "webservice")}
	}
	handler := func(ns *corev1.Namespace) *ValidatingHandler {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs(ns)...).Build()
		return &ValidatingHandler{Client: c, APIReader: c}
	}

	t.Run("annotated true lifts the quota", func(t *testing.T) {
		h := handler(exemptNS(map[string]string{oam.AnnotationQuotaExempt: "true"}))
		errs, warnings := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice"), nil)
		assert.Empty(t, errs)
		assert.Empty(t, warnings, "an exemption is silent to the author, and logged for the operator")
	})

	t.Run("only true counts", func(t *testing.T) {
		for _, v := range []string{"false", "TRUE", "yes", "1", ""} {
			h := handler(exemptNS(map[string]string{oam.AnnotationQuotaExempt: v}))
			errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
				appWith("tenant-a", "app-1", "webservice"), nil)
			assert.Len(t, errs, 1, "value %q must not exempt", v)
		}
	})

	t.Run("no annotation, no exemption", func(t *testing.T) {
		h := handler(exemptNS(nil))
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice"), nil)
		assert.Len(t, errs, 1)
	})

	t.Run("an unreadable namespace leaves the quota in force", func(t *testing.T) {
		// Losing the opt-out must not refuse the Application, and must not admit
		// past the limit either.
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(quotaCompDef("webservice", common.NamespaceQuota{Limit: i32(1)}),
				appWith("tenant-a", "neighbour", "webservice", "webservice")).Build()
		h := &ValidatingHandler{Client: c, APIReader: c}
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice"), nil)
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeForbidden, errs[0].Type, "the limit holds, it is not an internal error")
	})

	t.Run("an exempt namespace is never counted", func(t *testing.T) {
		lists := 0
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(objs(exemptNS(map[string]string{oam.AnnotationQuotaExempt: "true"}))...).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*v1beta1.ApplicationList); ok {
						lists++
					}
					return cl.List(ctx, list, opts...)
				},
			}).Build()
		h := &ValidatingHandler{Client: c, APIReader: c}
		errs, _ := h.ValidateDefinitionRestrictions(context.Background(),
			appWith("tenant-a", "app-1", "webservice"), nil)
		assert.Empty(t, errs)
		assert.Zero(t, lists, "the exemption is checked before the count, so it saves the list")
	})
}
