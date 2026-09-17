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

package health

import (
	"os"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The addon and module components render an Application of their own, so their
// health is the health of that Application. These policies are evaluated only at
// runtime, against a live resource, so a mistake in them does not show up in any
// build: the addon or module just silently reports healthy again.
//
// The policies read their fields inside a nested _app struct on purpose. Reading
// them as plain hidden fields of the file, and then naming one of those fields in
// a comprehension condition, makes CUE resolve the field before the comprehension
// that fills it has run, and every service silently reads back as an empty list.
func TestOwnedApplicationPolicies(t *testing.T) {
	definitions := map[string]string{
		"addon":  "../../../../vela-templates/definitions/internal/component/addon.cue",
		"module": "../../../../vela-templates/definitions/internal/component/module.cue",
	}

	testCases := map[string]struct {
		status      interface{}
		wantHealthy bool
		wantMessage string
	}{
		"a running Application with every component healthy is healthy": {
			status: map[string]interface{}{
				"status":   "running",
				"services": []interface{}{service("api", true, "")},
			},
			wantHealthy: true,
			wantMessage: "Ready:1/1",
		},
		"one unhealthy component makes it unhealthy and is named in the count": {
			status: map[string]interface{}{
				"status": "running",
				"services": []interface{}{
					service("api", true, ""),
					service("widget", false, `addon "nest-module-poc" not found in registries [my-addons]`),
				},
			},
			wantMessage: "Ready:1/2 widget unhealthy",
		},
		"an unhealthy component is named, its own message stays on its own Application": {
			status: map[string]interface{}{
				"status":   "unhealthy",
				"services": []interface{}{service("widget", false, "")},
			},
			wantMessage: "Ready:0/1 widget unhealthy",
		},
		"a phase other than running is unhealthy even with no components": {
			status:      map[string]interface{}{"status": "rendering"},
			wantMessage: "rendering",
		},
		"an Application that has not reported a status yet is unhealthy": {
			status:      map[string]interface{}{},
			wantMessage: "pending",
		},
		"an Application with no status at all is unhealthy": {
			wantMessage: "pending",
		},
	}

	for kind, path := range definitions {
		healthPolicy, customStatus := ownedApplicationPolicies(t, path, kind)
		for name, tc := range testCases {
			t.Run(kind+": "+name, func(t *testing.T) {
				output := map[string]interface{}{"metadata": map[string]interface{}{"name": kind + "-app"}}
				if tc.status != nil {
					output["status"] = tc.status
				}

				result, err := GetStatus(
					map[string]interface{}{"output": output},
					&StatusRequest{Health: healthPolicy, Custom: customStatus},
				)

				require.NoError(t, err)
				assert.Equal(t, tc.wantHealthy, result.Healthy)
				assert.Equal(t, tc.wantMessage, result.Message)
			})
		}
	}
}

func service(name string, healthy bool, message string) map[string]interface{} {
	svc := map[string]interface{}{"name": name, "healthy": healthy}
	if message != "" {
		svc["message"] = message
	}
	return svc
}

// ownedApplicationPolicies reads the policies out of the definition the chart
// ships, so the test cannot drift from what runs in a cluster. The template
// section is cut away first: it imports vela/addon and vela/module, which only
// resolve inside the CueX compiler.
func ownedApplicationPolicies(t *testing.T, path, kind string) (string, string) {
	t.Helper()
	source, err := os.ReadFile(path)
	require.NoError(t, err)
	definition := string(source)
	if i := strings.Index(definition, "\ntemplate: {"); i >= 0 {
		definition = definition[:i]
	}
	definition = strings.Replace(definition, "\"vela/"+kind+"\"", "", 1)

	compiled := cuecontext.New().CompileString(definition)
	require.NoError(t, compiled.Err())

	healthPolicy, err := compiled.LookupPath(value.FieldPath(kind, "attributes", "status", "healthPolicy")).String()
	require.NoError(t, err)
	customStatus, err := compiled.LookupPath(value.FieldPath(kind, "attributes", "status", "customStatus")).String()
	require.NoError(t, err)
	return healthPolicy, customStatus
}
