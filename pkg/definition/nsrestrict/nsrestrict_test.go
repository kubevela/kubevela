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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func names(patterns ...string) *common.DefinitionRestrictions {
	return &common.DefinitionRestrictions{Namespaces: patterns}
}

func selects(matchLabels map[string]string) *common.DefinitionRestrictions {
	return &common.DefinitionRestrictions{
		NamespaceSelector: &metav1.LabelSelector{MatchLabels: matchLabels},
	}
}

func compDef(spec *common.DefinitionRestrictions, annotations map[string]string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webservice", Namespace: "vela-system", Annotations: annotations,
		},
		Spec: v1beta1.ComponentDefinitionSpec{Restrictions: spec},
	}
}

func TestOf(t *testing.T) {
	testCases := map[string]struct {
		obj  client.Object
		want *common.DefinitionRestrictions
	}{
		"neither spec nor annotation is unrestricted": {
			obj: compDef(nil, nil), want: nil,
		},
		"spec names": {
			obj:  compDef(names("vela-system", "tenant-*"), nil),
			want: names("vela-system", "tenant-*"),
		},
		"spec selector": {
			obj:  compDef(selects(map[string]string{"tenant": "true"}), nil),
			want: selects(map[string]string{"tenant": "true"}),
		},
		"annotation names": {
			obj:  compDef(nil, map[string]string{oam.AnnotationRestrictNamespaces: "vela-system,tenant-*"}),
			want: names("vela-system", "tenant-*"),
		},
		// A selector has no annotation form.
		"a non-empty spec block wins over the annotation": {
			obj: compDef(names("only-spec"),
				map[string]string{oam.AnnotationRestrictNamespaces: "only-annotation"}),
			want: names("only-spec"),
		},
		"an empty spec block falls through to the annotation": {
			obj: compDef(&common.DefinitionRestrictions{},
				map[string]string{oam.AnnotationRestrictNamespaces: "from-annotation"}),
			want: names("from-annotation"),
		},
		"annotation entries are trimmed and empties dropped": {
			obj:  compDef(nil, map[string]string{oam.AnnotationRestrictNamespaces: " vela-system , , tenant-*  "}),
			want: names("vela-system", "tenant-*"),
		},
		"an annotation of only separators is unrestricted": {
			obj: compDef(nil, map[string]string{oam.AnnotationRestrictNamespaces: " , "}), want: nil,
		},
		"trait definition reads its own spec": {
			obj: &v1beta1.TraitDefinition{
				Spec: v1beta1.TraitDefinitionSpec{Restrictions: names("tenant-*")},
			},
			want: names("tenant-*"),
		},
		"policy definition reads its own spec": {
			obj: &v1beta1.PolicyDefinition{
				Spec: v1beta1.PolicyDefinitionSpec{Restrictions: names("tenant-*")},
			},
			want: names("tenant-*"),
		},
		"workflow step definition reads its own spec": {
			obj: &v1beta1.WorkflowStepDefinition{
				Spec: v1beta1.WorkflowStepDefinitionSpec{Restrictions: names("tenant-*")},
			},
			want: names("tenant-*"),
		},
		"source definition reads its own spec": {
			obj: &v1beta1.SourceDefinition{
				Spec: v1beta1.SourceDefinitionSpec{Restrictions: names("tenant-*")},
			},
			want: names("tenant-*"),
		},
		// WorkloadDefinition's CRD is frozen, so it carries a restriction in the
		// annotation only. No caller looks one up; this covers the accessor.
		"workload definition reads the annotation": {
			obj: &v1beta1.WorkloadDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{oam.AnnotationRestrictNamespaces: "tenant-*"},
				},
			},
			want: names("tenant-*"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, Of(tc.obj))
		})
	}
}

func TestAllows(t *testing.T) {
	testCases := map[string]struct {
		r        *common.DefinitionRestrictions
		ns       string
		nsLabels map[string]string
		want     bool
	}{
		"nil allows anything":            {r: nil, ns: "default", want: true},
		"an empty block allows anything": {r: &common.DefinitionRestrictions{}, ns: "default", want: true},

		"exact name match":                {r: names("vela-system"), ns: "vela-system", want: true},
		"exact name mismatch":             {r: names("vela-system"), ns: "default", want: false},
		"glob match":                      {r: names("tenant-*"), ns: "tenant-a", want: true},
		"glob missesns prefix":            {r: names("tenant-*"), ns: "tenant", want: false},
		"any pattern may match":           {r: names("vela-system", "tenant-*"), ns: "tenant-a", want: true},
		"star matches all":                {r: names("*"), ns: "anything", want: true},
		"character class":                 {r: names("tenant-[ab]"), ns: "tenant-b", want: true},
		"name matching is case sensitive": {r: names("Tenant-a"), ns: "tenant-a", want: false},
		// The definition webhooks reject one on write; this is the second line.
		"an unparseable glob denies": {r: names("tenant-["), ns: "tenant-a", want: false},
		"an unparseable glob does not shadow a good one": {
			r: names("tenant-[", "tenant-a"), ns: "tenant-a", want: true,
		},

		"selector match": {
			r: selects(map[string]string{"tenant": "true"}), ns: "acme",
			nsLabels: map[string]string{"tenant": "true"}, want: true,
		},
		"selector mismatch": {
			r: selects(map[string]string{"tenant": "true"}), ns: "acme",
			nsLabels: map[string]string{"tenant": "false"}, want: false,
		},
		"selector needs every label": {
			r: &common.DefinitionRestrictions{NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"tenant": "true", "tier": "gold"},
			}}, ns: "acme",
			nsLabels: map[string]string{"tenant": "true"}, want: false,
		},
		"an empty selector matches every namespace": {
			r:  &common.DefinitionRestrictions{NamespaceSelector: &metav1.LabelSelector{}},
			ns: "anything", nsLabels: map[string]string{}, want: true,
		},
		// nil means the namespace could not be read.
		"unreadable namespace labels deny a selector": {
			r: selects(map[string]string{"tenant": "true"}), ns: "acme", nsLabels: nil, want: false,
		},
		"matchExpressions In": {
			r: &common.DefinitionRestrictions{NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key: "env", Operator: metav1.LabelSelectorOpIn, Values: []string{"prod", "staging"},
				}},
			}}, ns: "acme", nsLabels: map[string]string{"env": "staging"}, want: true,
		},
		"matchExpressions Exists": {
			r: &common.DefinitionRestrictions{NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key: "tenant", Operator: metav1.LabelSelectorOpExists,
				}},
			}}, ns: "acme", nsLabels: map[string]string{"tenant": "anything"}, want: true,
		},

		// The fields are alternatives: either matching is enough.
		"the name matches though the selector does not": {
			r: &common.DefinitionRestrictions{
				Namespaces:        []string{"vela-system"},
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tenant": "true"}},
			}, ns: "vela-system", nsLabels: map[string]string{}, want: true,
		},
		"the selector matches though the name does not": {
			r: &common.DefinitionRestrictions{
				Namespaces:        []string{"vela-system"},
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tenant": "true"}},
			}, ns: "acme", nsLabels: map[string]string{"tenant": "true"}, want: true,
		},
		"neither matches": {
			r: &common.DefinitionRestrictions{
				Namespaces:        []string{"vela-system"},
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tenant": "true"}},
			}, ns: "acme", nsLabels: map[string]string{}, want: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, Allows(tc.r, tc.ns, tc.nsLabels))
		})
	}
}

func TestNeedsNamespaceLabels(t *testing.T) {
	assert.False(t, NeedsNamespaceLabels(compDef(nil, nil), "default"))
	assert.False(t, NeedsNamespaceLabels(compDef(names("tenant-*"), nil), "default"))
	assert.True(t, NeedsNamespaceLabels(compDef(selects(map[string]string{"tenant": "true"}), nil), "default"))
	// The annotation carries names only.
	assert.False(t, NeedsNamespaceLabels(compDef(nil,
		map[string]string{oam.AnnotationRestrictNamespaces: "tenant-*"}), "default"))

	// A name glob that already admits the namespace settles it, so the labels are
	// not needed and a namespace that cannot be read does not deny.
	both := &common.DefinitionRestrictions{
		Namespaces:        []string{"tenant-*"},
		NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "gold"}},
	}
	assert.False(t, NeedsNamespaceLabels(compDef(both, nil), "tenant-a"))
	assert.True(t, NeedsNamespaceLabels(compDef(both, nil), "default"))
}

func TestCheck(t *testing.T) {
	t.Run("an allowed namespace passes", func(t *testing.T) {
		require.NoError(t, Check(compDef(names("tenant-*"), nil), "tenant-a", nil))
	})

	t.Run("an unrestricted definition passes", func(t *testing.T) {
		require.NoError(t, Check(compDef(nil, nil), "anywhere", nil))
	})

	t.Run("a denied namespace names the kind, the definition and the namespace", func(t *testing.T) {
		err := Check(compDef(names("vela-system", "tenant-*"), nil), "default", nil)
		require.Error(t, err)
		for _, want := range []string{"ComponentDefinition", "webservice", "default"} {
			assert.Contains(t, err.Error(), want, "error should mention %q", want)
		}
	})

	// Whoever hits this cannot use the definition, so the error must not tell them
	// how the cluster is carved up.
	t.Run("the error does not disclose what the restriction allows", func(t *testing.T) {
		both := &common.DefinitionRestrictions{
			Namespaces:        []string{"vela-system", "tenant-acme"},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "gold"}},
		}
		err := Check(compDef(both, nil), "default", map[string]string{})
		require.Error(t, err)
		for _, leak := range []string{"vela-system", "tenant-acme", "tier", "gold"} {
			assert.NotContains(t, err.Error(), leak, "the error leaks %q", leak)
		}
	})

	// The operator does get the detail, through the log.
	t.Run("Describe renders the patterns for the operator", func(t *testing.T) {
		d := Describe(compDef(names("vela-system", "tenant-*"), nil))
		assert.Contains(t, d, "vela-system")
		assert.Contains(t, d, "tenant-*")

		d = Describe(compDef(selects(map[string]string{"tenant": "true"}), nil))
		assert.Contains(t, d, "tenant=true")
	})

	t.Run("each kind names itself", func(t *testing.T) {
		kinds := map[client.Object]string{
			&v1beta1.ComponentDefinition{Spec: v1beta1.ComponentDefinitionSpec{Restrictions: names("x")}}:       "ComponentDefinition",
			&v1beta1.TraitDefinition{Spec: v1beta1.TraitDefinitionSpec{Restrictions: names("x")}}:               "TraitDefinition",
			&v1beta1.PolicyDefinition{Spec: v1beta1.PolicyDefinitionSpec{Restrictions: names("x")}}:             "PolicyDefinition",
			&v1beta1.WorkflowStepDefinition{Spec: v1beta1.WorkflowStepDefinitionSpec{Restrictions: names("x")}}: "WorkflowStepDefinition",
			&v1beta1.SourceDefinition{Spec: v1beta1.SourceDefinitionSpec{Restrictions: names("x")}}:             "SourceDefinition",
			&v1beta1.WorkloadDefinition{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{oam.AnnotationRestrictNamespaces: "x"},
			}}: "WorkloadDefinition",
		}
		for obj, kind := range kinds {
			err := Check(obj, "default", nil)
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), kind), "want %q prefix, got %q", kind, err.Error())
		}
	})

	t.Run("a nil object passes", func(t *testing.T) {
		var cd *v1beta1.ComponentDefinition
		require.NoError(t, Check(cd, "default", nil))
	})
}

func TestValidate(t *testing.T) {
	testCases := map[string]struct {
		r       *common.DefinitionRestrictions
		wantErr string
	}{
		"nil is valid":                 {r: nil},
		"names and globs are valid":    {r: names("vela-system", "tenant-*", "a-[bc]")},
		"a selector is valid":          {r: selects(map[string]string{"tenant": "true"})},
		"an empty entry is rejected":   {r: names("vela-system", ""), wantErr: "empty"},
		"a blank entry is rejected":    {r: names("  "), wantErr: "empty"},
		"an unparseable glob rejected": {r: names("tenant-["), wantErr: "tenant-["},
		"an invalid selector operator is rejected": {
			r: &common.DefinitionRestrictions{NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "env", Operator: "Nonsense"}},
			}},
			wantErr: "not a valid label selector",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := Validate(tc.r)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateObject(t *testing.T) {
	t.Run("rejects a bad spec glob", func(t *testing.T) {
		require.Error(t, ValidateObject(compDef(names("tenant-["), nil)))
	})

	t.Run("rejects a bad spec selector", func(t *testing.T) {
		require.Error(t, ValidateObject(compDef(&common.DefinitionRestrictions{
			NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "env", Operator: "Nonsense"}},
			},
		}, nil)))
	})

	t.Run("rejects a bad annotation glob", func(t *testing.T) {
		require.Error(t, ValidateObject(compDef(nil,
			map[string]string{oam.AnnotationRestrictNamespaces: "tenant-["})))
	})

	// A broken annotation shadowed by a spec block becomes live as soon as that
	// block is cleared.
	t.Run("rejects a bad annotation even when the spec overrides it", func(t *testing.T) {
		require.Error(t, ValidateObject(compDef(names("vela-system"),
			map[string]string{oam.AnnotationRestrictNamespaces: "tenant-["})))
	})

	t.Run("accepts a definition with neither", func(t *testing.T) {
		require.NoError(t, ValidateObject(compDef(nil, nil)))
	})
}

// The CLI lists definitions generically, so restrictions have to be readable
// without the typed object.
func TestOfUnstructured(t *testing.T) {
	toUnstructured := func(obj client.Object) unstructured.Unstructured {
		raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		require.NoError(t, err)
		return unstructured.Unstructured{Object: raw}
	}

	t.Run("reads the spec block", func(t *testing.T) {
		got := OfUnstructured(toUnstructured(compDef(names("vela-system", "tenant-*"), nil)))
		require.NotNil(t, got)
		assert.Equal(t, []string{"vela-system", "tenant-*"}, got.Namespaces)
	})

	t.Run("reads a selector out of the spec block", func(t *testing.T) {
		got := OfUnstructured(toUnstructured(compDef(selects(map[string]string{"tenant": "true"}), nil)))
		require.NotNil(t, got)
		require.NotNil(t, got.NamespaceSelector)
		assert.Equal(t, map[string]string{"tenant": "true"}, got.NamespaceSelector.MatchLabels)
	})

	t.Run("falls back to the annotation", func(t *testing.T) {
		got := OfUnstructured(toUnstructured(compDef(nil,
			map[string]string{oam.AnnotationRestrictNamespaces: "tenant-*"})))
		require.NotNil(t, got)
		assert.Equal(t, []string{"tenant-*"}, got.Namespaces)
	})

	t.Run("an unrestricted definition yields nil", func(t *testing.T) {
		assert.Nil(t, OfUnstructured(toUnstructured(compDef(nil, nil))))
	})

	t.Run("a definition with no spec at all yields nil", func(t *testing.T) {
		assert.Nil(t, OfUnstructured(unstructured.Unstructured{Object: map[string]interface{}{}}))
	})

	// Whatever OfUnstructured reads has to decide the same way the typed path does.
	t.Run("agrees with the typed accessor", func(t *testing.T) {
		for _, def := range []*v1beta1.ComponentDefinition{
			compDef(names("tenant-*"), nil),
			compDef(selects(map[string]string{"tenant": "true"}), nil),
			compDef(nil, map[string]string{oam.AnnotationRestrictNamespaces: "vela-system"}),
			compDef(nil, nil),
		} {
			for _, ns := range []string{"tenant-a", "vela-system", "default"} {
				labels := map[string]string{"tenant": "true"}
				assert.Equal(t,
					Allows(Of(def), ns, labels),
					Allows(OfUnstructured(toUnstructured(def)), ns, labels),
					"disagreed for %q", ns)
			}
		}
	})
}

// A restrictions block that cannot be read must not resolve to "unrestricted".
func TestOfUnstructuredFailsClosed(t *testing.T) {
	for name, spec := range map[string]interface{}{
		"restrictions is a string": "tenant-*",
		"restrictions is a list":   []interface{}{"tenant-*"},
		"namespaces is a string":   map[string]interface{}{"namespaces": "tenant-*"},
	} {
		t.Run(name, func(t *testing.T) {
			obj := unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{"restrictions": spec},
			}}
			r := OfUnstructured(obj)
			require.NotNil(t, r, "an unreadable block must not read as unrestricted")
			assert.False(t, Allows(r, "anywhere", map[string]string{"tier": "gold"}))
		})
	}
}

// A padded pattern matches nothing, so the spec channel rejects it rather than
// accept it silently.
func TestValidateRejectsPaddedPatterns(t *testing.T) {
	for _, p := range []string{" tenant-*", "tenant-* ", "\ttenant-*"} {
		err := Validate(names(p))
		require.Error(t, err, "%q should be rejected", p)
		assert.Contains(t, err.Error(), "whitespace")
	}
	require.NoError(t, Validate(names("tenant-*")))
}

// KindOf must not assume a pointer type.
func TestKindOfDoesNotPanicOnUnexpectedTypes(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("ComponentDefinition")
	require.NotPanics(t, func() {
		assert.Equal(t, "ComponentDefinition", KindOf(u))
	})
	require.NotPanics(t, func() {
		assert.Equal(t, "Application", KindOf(&v1beta1.Application{}))
	})
}
