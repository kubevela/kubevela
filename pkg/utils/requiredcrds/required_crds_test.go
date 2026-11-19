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

package requiredcrds

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// mapperWith builds a RESTMapper serving exactly the given CRDs, so anything
// left out returns the NoKindMatchError a real cluster returns for a CRD that
// was never installed.
func mapperWith(crds []CRD) meta.RESTMapper {
	var versions []schema.GroupVersion
	for _, c := range crds {
		versions = append(versions, c.GroupVersion())
	}
	m := meta.NewDefaultRESTMapper(versions)
	for _, c := range crds {
		m.Add(c.GroupVersionKind, meta.RESTScopeNamespace)
	}
	return m
}

func TestVerifyPassesWhenEverythingIsInstalled(t *testing.T) {
	require.NoError(t, Verify(mapperWith(Required)))
}

// One restart per missing CRD is the difference between one upgrade and four, so
// the first missing one must not short-circuit the rest.
func TestVerifyNamesEveryMissingCRD(t *testing.T) {
	var present []CRD
	for _, c := range Required {
		if c.Plural == "sourcedefinitions" || c.Plural == "resourcetrackers" {
			continue
		}
		present = append(present, c)
	}

	err := Verify(mapperWith(present))
	require.Error(t, err)
	require.Contains(t, err.Error(), "sourcedefinitions.core.oam.dev")
	require.Contains(t, err.Error(), "resourcetrackers.core.oam.dev")
	require.Contains(t, err.Error(), "2 required CustomResourceDefinition(s)")

	// The message has to carry the fix, not just the diagnosis.
	require.Contains(t, err.Error(), "kubectl apply -f vela-core/crds/")
	require.Contains(t, err.Error(), "vela install")
	require.Contains(t, err.Error(), "https://kubevela.io/docs/")
}

// An unreachable API server is not a missing CRD, and saying so sends an operator
// to the wrong page.
func TestVerifyDistinguishesADiscoveryFailure(t *testing.T) {
	err := Verify(brokenMapper{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot determine whether CRD")
	require.NotContains(t, err.Error(), "not installed on this cluster")
}

type brokenMapper struct{ meta.RESTMapper }

func (brokenMapper) RESTMapping(schema.GroupKind, ...string) (*meta.RESTMapping, error) {
	return nil, errors.New("connection refused")
}

// The lists here and the chart's crds/ directory are two statements of the same
// thing, and a CRD added to the chart and forgotten here is exactly the silence
// this package exists to remove. Requiring every chart CRD to be classified means
// adding one forces the question "is vela-core allowed to start without this?"
// rather than defaulting to yes.
func TestEveryChartCRDIsClassified(t *testing.T) {
	const chartDir = "../../../charts/vela-core/crds"

	files, err := filepath.Glob(filepath.Join(chartDir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "no CRDs found in %s", chartDir)

	classified := map[string]CRD{}
	for _, c := range append(append([]CRD{}, Required...), Optional...) {
		_, dup := classified[c.Name()]
		require.False(t, dup, "%s is listed twice", c.Name())
		classified[c.Name()] = c
	}

	inChart := map[string]bool{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)

		crd := &apiextensionsv1.CustomResourceDefinition{}
		require.NoError(t, yaml.Unmarshal(raw, crd), "parsing %s", f)

		name := crd.Spec.Names.Plural + "." + crd.Spec.Group
		inChart[name] = true

		listed, ok := classified[name]
		require.True(t, ok,
			"%s is shipped in the chart but is in neither Required nor Optional; "+
				"decide whether vela-core may start without it", name)

		require.Equal(t, crd.Spec.Names.Kind, listed.Kind, "kind mismatch for %s", name)

		served := map[string]bool{}
		for _, v := range crd.Spec.Versions {
			if v.Served {
				served[v.Name] = true
			}
		}
		require.True(t, served[listed.Version],
			"%s is checked at version %q, which the chart does not serve (served: %v)",
			name, listed.Version, served)
	}

	for name := range classified {
		require.True(t, inChart[name],
			"%s is checked at startup but the chart no longer ships it", name)
	}
}
