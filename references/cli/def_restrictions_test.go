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

package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
)

// The builtin definitions are regenerated on every chart upgrade, so a restriction
// on one survives only if the chart stamps it on at render time.
func TestRestrictionsPlaceholder(t *testing.T) {
	newDef := func(spec map[string]interface{}) *pkgdef.Definition {
		def := &pkgdef.Definition{Unstructured: unstructured.Unstructured{Object: map[string]interface{}{}}}
		def.SetName("webservice")
		if spec != nil {
			def.Object["spec"] = spec
		}
		return def
	}

	curated := func() *pkgdef.Definition {
		return newDef(map[string]interface{}{
			"restrictions": map[string]interface{}{"namespaces": []interface{}{"from-cue"}},
			"schematic":    map[string]interface{}{},
		})
	}

	t.Run("opted in, the marker replaces whatever the CUE source declared", func(t *testing.T) {
		// Chart values are the only source of a builtin's restrictions, so one in the
		// .cue file must not reach the chart.
		t.Setenv(RestrictionGenEnvName, "true")
		def := curated()
		markRestrictionsPlaceholder(def)
		spec := def.Object["spec"].(map[string]interface{})
		assert.Equal(t, HelmChartRestrictionsPlaceholder, spec[restrictionsField])
	})

	// Only WITH_RESTRICTION_GEN hands restrictions to the chart.
	t.Run("not opted in, curated restrictions survive untouched", func(t *testing.T) {
		def := curated()
		markRestrictionsPlaceholder(def)
		spec := def.Object["spec"].(map[string]interface{})
		assert.Equal(t,
			map[string]interface{}{"namespaces": []interface{}{"from-cue"}},
			spec[restrictionsField])
	})

	t.Run("the opt-in only accepts true", func(t *testing.T) {
		for _, v := range []string{"", "1", "yes", "false"} {
			t.Setenv(RestrictionGenEnvName, v)
			def := curated()
			markRestrictionsPlaceholder(def)
			spec := def.Object["spec"].(map[string]interface{})
			assert.NotEqual(t, HelmChartRestrictionsPlaceholder, spec[restrictionsField],
				"%q should not opt in", v)
		}
	})

	t.Run("a definition with no spec is left alone", func(t *testing.T) {
		t.Setenv(RestrictionGenEnvName, "true")
		def := newDef(nil)
		require.NotPanics(t, func() { markRestrictionsPlaceholder(def) })
	})

	// The include sits behind a "#" so the file is still valid YAML unrendered.
	t.Run("the marker line becomes a commented, keyed include", func(t *testing.T) {
		in := "spec:\n  restrictions: '" + HelmChartRestrictionsPlaceholder + "'\n  schematic:\n    cue: {}\n"
		got := replaceRestrictionsPlaceholder(in, "webservice")
		assert.Equal(t,
			"spec:\n  #{{ include \"definitionRestrictions\" (dict \"name\" \"webservice\" \"root\" $) }}\n  schematic:\n    cue: {}\n",
			got)
		for _, line := range strings.Split(got, "\n") {
			assert.False(t, strings.HasPrefix(strings.TrimSpace(line), "{{"),
				"no line may start with a directive, or the file stops being YAML: %q", line)
		}
	})

	// Definitions rendered without a marker must pass through untouched.
	t.Run("yaml without a marker is untouched", func(t *testing.T) {
		in := "spec:\n  schematic:\n    cue: {}\n"
		assert.Equal(t, in, replaceRestrictionsPlaceholder(in, "webservice"))
	})

	t.Run("the definition name is carried into the include", func(t *testing.T) {
		in := "  restrictions: '" + HelmChartRestrictionsPlaceholder + "'\n"
		assert.Contains(t, replaceRestrictionsPlaceholder(in, "k8s-objects"), `"name" "k8s-objects"`)
	})
}
