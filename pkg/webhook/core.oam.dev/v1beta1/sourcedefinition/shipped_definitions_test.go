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

package sourcedefinition

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/yaml"

	"github.com/stretchr/testify/require"
)

// The definitions the chart ships have to survive their own admission webhook.
//
// Every check this handler applies has been tightened at some point after the
// definitions were written - schema: became mandatory, the cache key started
// being re-derived, consumableFrom gained a surface check - and each time the
// shipped set was found broken by hand rather than by a test. This runs the
// real handler over the real generated chart files, so it fails on the change
// that breaks them.
func TestShippedSourceDefinitionsPassAdmission(t *testing.T) {
	dir := "../../../../../charts/vela-core/templates/defwithtemplate"
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	// The chart templates the namespace; nothing else in these files is Helm.
	helmNamespace := regexp.MustCompile(`\{\{[^}]*\}\}`)

	checked := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)
		if !strings.Contains(string(raw), "kind: SourceDefinition") {
			continue
		}
		cleaned := helmNamespace.ReplaceAll(raw, []byte("vela-system"))

		t.Run(entry.Name(), func(t *testing.T) {
			var obj map[string]interface{}
			require.NoError(t, yaml.Unmarshal(cleaned, &obj))
			asJSON, err := yaml.YAMLToJSON(cleaned)
			require.NoError(t, err)

			resp := handler(t).Handle(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID:       "shipped",
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group: "core.oam.dev", Version: "v1beta1", Resource: "sourcedefinitions",
					},
					Object: runtime.RawExtension{Raw: asJSON},
				},
			})
			require.True(t, resp.Allowed, "%v", resp.Result)
		})
		checked++
	}
	require.NotZero(t, checked, "no shipped SourceDefinitions were found to check")
}
