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

package nsrestrict

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func quotaDef(entries ...common.NamespaceQuota) *common.DefinitionRestrictions {
	return &common.DefinitionRestrictions{Quota: entries}
}

func TestQuotaFor(t *testing.T) {
	gold := common.NamespaceQuota{
		NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "gold"}},
		Limit:             ptr.To(int32(20)),
	}
	tenants := common.NamespaceQuota{Namespaces: []string{"tenant-*"}, Limit: ptr.To(int32(3))}
	fallback := common.NamespaceQuota{Limit: ptr.To(int32(5))}

	t.Run("a name glob matches", func(t *testing.T) {
		q := QuotaFor(compDef(quotaDef(tenants, fallback), nil), "tenant-a", nil)
		require.NotNil(t, q)
		assert.Equal(t, int32(3), *q.Limit)
	})

	t.Run("a selector matches", func(t *testing.T) {
		q := QuotaFor(compDef(quotaDef(gold, fallback), nil), "acme", map[string]string{"tier": "gold"})
		require.NotNil(t, q)
		assert.Equal(t, int32(20), *q.Limit)
	})

	// Ordering is the precedence rule, so an earlier entry wins even when a later
	// one also matches.
	t.Run("the first match wins", func(t *testing.T) {
		q := QuotaFor(compDef(quotaDef(tenants, gold), nil), "tenant-a", map[string]string{"tier": "gold"})
		require.NotNil(t, q)
		assert.Equal(t, int32(3), *q.Limit, "the name entry comes first")
	})

	t.Run("an entry with no matcher is the default", func(t *testing.T) {
		q := QuotaFor(compDef(quotaDef(tenants, fallback), nil), "unrelated", nil)
		require.NotNil(t, q)
		assert.Equal(t, int32(5), *q.Limit)
	})

	t.Run("no entry matches means unlimited", func(t *testing.T) {
		assert.Nil(t, QuotaFor(compDef(quotaDef(tenants), nil), "unrelated", nil))
	})

	t.Run("no quota at all means unlimited", func(t *testing.T) {
		assert.Nil(t, QuotaFor(compDef(names("tenant-*"), nil), "tenant-a", nil))
	})

	// Addons install their own Applications into the system namespace using these
	// definitions, so quota there would break installation.
	t.Run("the system namespace is exempt, even from a zero ceiling", func(t *testing.T) {
		zero := common.NamespaceQuota{Limit: ptr.To(int32(0))}
		assert.Nil(t, QuotaFor(compDef(quotaDef(zero), nil), types.DefaultKubeVelaNS, nil))
	})

	// types.DefaultKubeVelaNS is where addon Applications run, and can be set apart
	// from oam.SystemDefinitionNamespace, where definitions live.
	t.Run("the exemption follows a reconfigured system namespace", func(t *testing.T) {
		prevDefault, prevDefNS := types.DefaultKubeVelaNS, oam.SystemDefinitionNamespace
		defer func() { types.DefaultKubeVelaNS, oam.SystemDefinitionNamespace = prevDefault, prevDefNS }()
		types.DefaultKubeVelaNS, oam.SystemDefinitionNamespace = "kubevela-system", "kubevela-system"

		zero := common.NamespaceQuota{Limit: ptr.To(int32(0))}
		assert.Nil(t, QuotaFor(compDef(quotaDef(zero), nil), "kubevela-system", nil))
		assert.NotNil(t, QuotaFor(compDef(quotaDef(zero), nil), "vela-system", nil),
			"a name neither variable holds is an ordinary namespace")
	})
}

func TestQuotaNeedsNamespaceLabels(t *testing.T) {
	sel := common.NamespaceQuota{
		NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "gold"}},
		Limit:             ptr.To(int32(20)),
	}
	byName := common.NamespaceQuota{Namespaces: []string{"tenant-*"}, Limit: ptr.To(int32(3))}
	fallback := common.NamespaceQuota{Limit: ptr.To(int32(5))}

	assert.True(t, QuotaNeedsNamespaceLabels(compDef(quotaDef(sel), nil), "acme"))
	assert.False(t, QuotaNeedsNamespaceLabels(compDef(quotaDef(byName), nil), "tenant-a"))
	// An earlier name match settles it before the selector is reached.
	assert.False(t, QuotaNeedsNamespaceLabels(compDef(quotaDef(byName, sel), nil), "tenant-a"))
	assert.True(t, QuotaNeedsNamespaceLabels(compDef(quotaDef(byName, sel), nil), "acme"))
	// A no-matcher default decides without labels.
	assert.False(t, QuotaNeedsNamespaceLabels(compDef(quotaDef(fallback, sel), nil), "acme"))
	assert.False(t, QuotaNeedsNamespaceLabels(compDef(quotaDef(sel), nil), types.DefaultKubeVelaNS))
}

func TestExceedsQuota(t *testing.T) {
	testCases := map[string]struct {
		q            *common.NamespaceQuota
		total        int
		refuse, warn bool
	}{
		"no quota":                   {nil, 99, false, false},
		"below both":                 {&common.NamespaceQuota{Warn: ptr.To(int32(8)), Limit: ptr.To(int32(10))}, 7, false, false},
		"at the warning":             {&common.NamespaceQuota{Warn: ptr.To(int32(8)), Limit: ptr.To(int32(10))}, 8, false, true},
		"at the ceiling still ok":    {&common.NamespaceQuota{Warn: ptr.To(int32(8)), Limit: ptr.To(int32(10))}, 10, false, true},
		"above the ceiling":          {&common.NamespaceQuota{Warn: ptr.To(int32(8)), Limit: ptr.To(int32(10))}, 11, true, false},
		"warn only never refuses":    {&common.NamespaceQuota{Warn: ptr.To(int32(2))}, 1000, false, true},
		"a limit alone never warns":  {&common.NamespaceQuota{Limit: ptr.To(int32(3))}, 3, false, false},
		"a limit alone, above":       {&common.NamespaceQuota{Limit: ptr.To(int32(3))}, 4, true, false},
		"a zero ceiling forbids":     {&common.NamespaceQuota{Limit: ptr.To(int32(0))}, 1, true, false},
		"a zero ceiling admits none": {&common.NamespaceQuota{Limit: ptr.To(int32(0))}, 0, false, false},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			refuse, warn := ExceedsQuota(tc.q, tc.total)
			assert.Equal(t, tc.refuse, refuse, "refuse")
			assert.Equal(t, tc.warn, warn, "warn")
		})
	}
}

func TestValidateQuota(t *testing.T) {
	testCases := map[string]struct {
		q       common.NamespaceQuota
		wantErr string
	}{
		"warn and limit":       {common.NamespaceQuota{Warn: ptr.To(int32(8)), Limit: ptr.To(int32(10))}, ""},
		"warn alone":           {common.NamespaceQuota{Warn: ptr.To(int32(8))}, ""},
		"a limit alone":        {common.NamespaceQuota{Limit: ptr.To(int32(10))}, ""},
		"equal is fine":        {common.NamespaceQuota{Warn: ptr.To(int32(10)), Limit: ptr.To(int32(10))}, ""},
		"neither":              {common.NamespaceQuota{}, "does nothing"},
		"warn above the limit": {common.NamespaceQuota{Warn: ptr.To(int32(11)), Limit: ptr.To(int32(10))}, "never warn"},
		"a bad glob":           {common.NamespaceQuota{Namespaces: []string{"tenant-["}, Limit: ptr.To(int32(1))}, "not a valid pattern"},
		"a padded glob":        {common.NamespaceQuota{Namespaces: []string{"tenant-* "}, Limit: ptr.To(int32(1))}, "whitespace"},
		"an empty entry":       {common.NamespaceQuota{Namespaces: []string{""}, Limit: ptr.To(int32(1))}, "empty"},
		"a bad selector": {common.NamespaceQuota{
			NamespaceSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "e", Operator: "Nonsense"}}},
			Limit:             ptr.To(int32(1)),
		}, "not a valid label selector"},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := Validate(quotaDef(tc.q))
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Contains(t, err.Error(), "quota[0]", "the message should say which entry")
		})
	}
}

// A definition may cap usage without restricting which namespaces may use it. That
// block declares something, so it must not fall through to the annotation, and it
// must not be read as restricting every namespace to nothing.
func TestQuotaOnlyBlockIsNotEmpty(t *testing.T) {
	def := compDef(quotaDef(common.NamespaceQuota{Limit: ptr.To(int32(3))}), nil)

	require.NotNil(t, Of(def), "a quota-only block declares something")
	assert.True(t, Allows(Of(def), "anywhere", nil), "a quota restricts no namespace")
	require.NotNil(t, QuotaFor(def, "anywhere", nil), "the quota still applies")
	require.NoError(t, Check(def, "anywhere", nil), "and nothing is denied by it")
}

// A quota is spec-only, so adding one must not void a namespace restriction the
// annotation declares. Before quota existed a non-empty spec block always named
// namespaces, so "the spec wins" and "the spec replaces the annotation" were the
// same statement; they are not any more, and the security-relevant half is that
// adding a quota cannot quietly open a definition to every namespace.
func TestQuotaDoesNotVoidTheAnnotation(t *testing.T) {
	def := compDef(quotaDef(common.NamespaceQuota{Limit: ptr.To(int32(3))}),
		map[string]string{"definition.oam.dev/restrict-namespaces": "tenant-*"})

	assert.True(t, Allows(Of(def), "tenant-a", nil), "the annotation still admits its namespaces")
	assert.False(t, Allows(Of(def), "default", nil), "and still excludes the rest")
	assert.Error(t, Check(def, "default", nil))
	require.NotNil(t, QuotaFor(def, "tenant-a", nil), "while the spec's quota applies")
}

// The spec still settles the namespaces where it names any, annotation or not.
func TestSpecNamespacesStillBeatTheAnnotation(t *testing.T) {
	r := &common.DefinitionRestrictions{
		Namespaces: []string{"prod-*"},
		Quota:      []common.NamespaceQuota{{Limit: ptr.To(int32(3))}},
	}
	def := compDef(r, map[string]string{"definition.oam.dev/restrict-namespaces": "tenant-*"})

	assert.True(t, Allows(Of(def), "prod-a", nil), "the spec names the namespaces")
	assert.False(t, Allows(Of(def), "tenant-a", nil), "the annotation is ignored, as before")
}

// The common case: a definition with no restrictions at all is unlimited, and
// settles without reading the namespace.
func TestQuotaOnAnUnrestrictedDefinition(t *testing.T) {
	def := compDef(nil, nil)
	assert.Nil(t, QuotaFor(def, "tenant-a", nil))
	assert.False(t, QuotaNeedsNamespaceLabels(def, "tenant-a"))
}

// Entries that all name namespaces, none of them this one: unlimited, and the
// labels are never needed to say so.
func TestQuotaWithNoMatchingEntry(t *testing.T) {
	def := compDef(quotaDef(
		common.NamespaceQuota{Namespaces: []string{"prod-*"}, Limit: ptr.To(int32(1))},
		common.NamespaceQuota{Namespaces: []string{"staging-*"}, Limit: ptr.To(int32(2))},
	), nil)
	assert.Nil(t, QuotaFor(def, "tenant-a", nil))
	assert.False(t, QuotaNeedsNamespaceLabels(def, "tenant-a"))
}

// A namespace accumulates components and the traits on them. A policy, workflow
// step or source is part of how one Application is assembled, so a quota there
// would say nothing, and is refused on write rather than ignored at admission.
func TestQuotaOnlyOnCountableKinds(t *testing.T) {
	quota := []common.NamespaceQuota{{Limit: ptr.To(int32(3))}}
	restrictions := &common.DefinitionRestrictions{Quota: quota}

	testCases := map[string]struct {
		obj     client.Object
		allowed bool
	}{
		"a component type is counted": {&v1beta1.ComponentDefinition{
			Spec: v1beta1.ComponentDefinitionSpec{Restrictions: restrictions}}, true},
		"a trait type is counted": {&v1beta1.TraitDefinition{
			Spec: v1beta1.TraitDefinitionSpec{Restrictions: restrictions}}, true},
		"a policy is not": {&v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{Restrictions: restrictions}}, false},
		"a workflow step is not": {&v1beta1.WorkflowStepDefinition{
			Spec: v1beta1.WorkflowStepDefinitionSpec{Restrictions: restrictions}}, false},
		"a source is not": {&v1beta1.SourceDefinition{
			Spec: v1beta1.SourceDefinitionSpec{Restrictions: restrictions}}, false},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := ValidateObject(tc.obj)
			if tc.allowed {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "quota is not supported on")
			assert.Contains(t, err.Error(), "ComponentDefinition and TraitDefinition")
		})
	}
}

// A namespace restriction stays available on every kind; only the quota is limited.
func TestRestrictionsStillAllowedOnEveryKind(t *testing.T) {
	r := &common.DefinitionRestrictions{Namespaces: []string{"tenant-*"}}
	assert.NoError(t, ValidateObject(&v1beta1.PolicyDefinition{
		Spec: v1beta1.PolicyDefinitionSpec{Restrictions: r}}))
	assert.NoError(t, ValidateObject(&v1beta1.SourceDefinition{
		Spec: v1beta1.SourceDefinitionSpec{Restrictions: r}}))
}

// The first matching entry wins, so an entry with no matcher takes every namespace.
// Anything after it could never apply, which is a mistake rather than a policy.
func TestQuotaOrderingRejectsAnUnreachableEntry(t *testing.T) {
	tenants := common.NamespaceQuota{Namespaces: []string{"tenant-*"}, Limit: ptr.To(int32(3))}
	fallback := common.NamespaceQuota{Limit: ptr.To(int32(5))}

	t.Run("the default belongs last", func(t *testing.T) {
		assert.NoError(t, Validate(quotaDef(tenants, fallback)))
	})

	t.Run("a default first shadows everything after it", func(t *testing.T) {
		err := Validate(quotaDef(fallback, tenants))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quota[0] matches every namespace")
		assert.Contains(t, err.Error(), "quota[1] onwards could never apply")
	})

	t.Run("two defaults are the same mistake", func(t *testing.T) {
		assert.Error(t, Validate(quotaDef(fallback, fallback)))
	})

	t.Run("a single default is fine", func(t *testing.T) {
		assert.NoError(t, Validate(quotaDef(fallback)))
	})
}

// The two system namespaces are configured by different paths and can disagree:
// oam.SystemDefinitionNamespace follows --system-definition-namespace, which the
// chart sets to the release namespace, while only the CLI binds
// types.DefaultKubeVelaNS from the environment. Exempting one would leave the
// other quota'd on any install outside vela-system.
func TestQuotaExemptsBothSystemNamespaces(t *testing.T) {
	def := compDef(quotaDef(common.NamespaceQuota{Limit: ptr.To(int32(0))}), nil)

	prevDefault, prevDefNS := types.DefaultKubeVelaNS, oam.SystemDefinitionNamespace
	defer func() { types.DefaultKubeVelaNS, oam.SystemDefinitionNamespace = prevDefault, prevDefNS }()

	// A release installed into "kubevela", with addon Applications still placed at
	// the compiled default.
	types.DefaultKubeVelaNS = "vela-system"
	oam.SystemDefinitionNamespace = "kubevela"

	assert.Nil(t, QuotaFor(def, "vela-system", nil), "where addon Applications run")
	assert.Nil(t, QuotaFor(def, "kubevela", nil), "where the release is installed")
	assert.NotNil(t, QuotaFor(def, "tenant-a", nil), "a tenant is still capped")

	assert.False(t, QuotaNeedsNamespaceLabels(def, "vela-system"))
	assert.False(t, QuotaNeedsNamespaceLabels(def, "kubevela"))
}
