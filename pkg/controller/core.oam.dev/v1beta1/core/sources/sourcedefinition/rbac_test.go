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
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The source cache lives in Secrets that KubeVela owns, so the controller reads
// and writes them as itself rather than as the Application's identity. That makes
// the grant the controller holds the only thing standing between the cache and a
// Forbidden, and it is written in two places that do not check each other: the
// kubebuilder markers on this controller, and the chart's :manager ClusterRole.
//
// Neither failure is loud. The chart binds the ServiceAccount to cluster-admin
// whenever authentication.enabled is false, which is the default, so a missing
// grant is invisible until an operator turns authentication on - at which point
// every cache write fails and every source falls back to re-fetching forever.
func TestControllerMayWriteTheSourceCache(t *testing.T) {
	// What the cache stores actually call. source_cache_store_secret.go creates,
	// updates and deletes; cache_gc.go deletes; Touch updates.
	required := []string{"create", "delete", "get", "list", "update", "watch"}

	t.Run("kubebuilder markers", func(t *testing.T) {
		src, err := os.ReadFile("sourcedefinition_controller.go")
		if err != nil {
			t.Fatal(err)
		}
		re := regexp.MustCompile(`\+kubebuilder:rbac:groups="",resources=secrets,verbs=(\S+)`)
		m := re.FindSubmatch(src)
		if m == nil {
			t.Fatal("no secrets rbac marker on the controller; the generated role would grant nothing")
		}
		assertCovers(t, "marker", strings.Split(string(m[1]), ";"), required)
	})

	t.Run("chart :manager ClusterRole", func(t *testing.T) {
		chart, err := os.ReadFile("../../../../../../../charts/vela-core/templates/kubevela-controller.yaml")
		if err != nil {
			t.Fatal(err)
		}
		verbs, ok := secretVerbsInChart(string(chart))
		if !ok {
			t.Fatal("no rule granting secrets in the chart; the controller cannot reach its own cache")
		}
		assertCovers(t, "chart", verbs, required)
	})
}

// secretVerbsInChart returns the union of verbs every rule mentioning secrets
// grants. A union rather than one rule, because splitting reads and writes across
// two rules is a legitimate way to write it.
func secretVerbsInChart(chart string) ([]string, bool) {
	rule := regexp.MustCompile(`resources: \[([^\]]*)\]\s*\n\s*verbs: \[([^\]]*)\]`)
	seen := map[string]bool{}
	found := false
	for _, m := range rule.FindAllStringSubmatch(chart, -1) {
		if !strings.Contains(m[1], "secrets") {
			continue
		}
		found = true
		for _, v := range strings.Split(m[2], ",") {
			seen[strings.Trim(strings.TrimSpace(v), `"`)] = true
		}
	}
	var out []string
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out, found
}

func assertCovers(t *testing.T, where string, granted, required []string) {
	t.Helper()
	has := map[string]bool{}
	for _, v := range granted {
		v = strings.Trim(strings.TrimSpace(v), `"`)
		if v == "*" {
			return
		}
		has[v] = true
	}
	var missing []string
	for _, r := range required {
		if !has[r] {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%s grants secrets %v, missing %v — the source cache cannot be written",
			where, granted, missing)
	}
}
