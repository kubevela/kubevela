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

package cuetest

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/utils"
)

// shippedDefinitions are KubeVela's built-in definitions, which ship in the
// vela-core chart.
const shippedDefinitions = "../../../vela-templates/definitions/internal"

// untested are the shipped definitions with no test file, and why.
var untested = map[string]string{
	"component/ref-objects":        "the controller fetches the objects it names; its template only passes them through",
	"workflowstep/apply-component": "the controller runs its builtin apply-component task; its template only declares parameters",
	"policy/apply-once":            "read by the controller, not rendered",
	"policy/garbage-collect":       "read by the controller, not rendered",
	"policy/override":              "read by the controller, not rendered",
	"policy/read-only":             "read by the controller, not rendered",
	"policy/replication":           "read by the controller, not rendered",
	"policy/resource-update":       "read by the controller, not rendered",
	"policy/shared-resource":       "read by the controller, not rendered",
	"policy/take-over":             "read by the controller, not rendered",
	"policy/topology":              "read by the controller, not rendered",
}

// Every shipped definition has a test file beside it, unless it is listed as
// untested with why; a listed one that gains tests must come off the list.
func TestShippedDefinitionsAreTested(t *testing.T) {
	var missing, stale []string
	seen := map[string]bool{}
	err := filepath.WalkDir(shippedDefinitions, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".cue" || utils.IsCUETestFile(p) {
			return err
		}
		rel, _ := filepath.Rel(shippedDefinitions, strings.TrimSuffix(p, ".cue"))
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		_, statErr := os.Stat(strings.TrimSuffix(p, ".cue") + utils.CUETestFileSuffix)
		_, excused := untested[rel]
		switch {
		case statErr != nil && !excused:
			missing = append(missing, rel)
		case statErr == nil && excused:
			stale = append(stale, rel)
		}
		return nil
	})
	require.NoError(t, err)
	for rel := range untested {
		if !seen[rel] {
			stale = append(stale, rel+" (no such definition)")
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	require.Empty(t, missing, "shipped definitions without a %s beside them", utils.CUETestFileSuffix)
	require.Empty(t, stale, "listed as untested, but tested or gone")
}

// The shipped definitions' own tests pass, pending ones aside.
func TestShippedDefinitions(t *testing.T) {
	suites, err := Load(shippedDefinitions)
	if err != nil && strings.Contains(err.Error(), "no *"+utils.CUETestFileSuffix) {
		t.Skip("no shipped definition has tests yet")
	}
	require.NoError(t, err)
	for _, s := range suites {
		rel, _ := filepath.Rel(shippedDefinitions, s.File)
		t.Run(rel, func(t *testing.T) {
			require.NoError(t, s.Err)
			var run []*Case
			for _, c := range s.Cases {
				if !c.Pending {
					run = append(run, c)
				}
			}
			for i, o := range s.Evaluate(run, RunOptions{}) {
				require.Emptyf(t, o.Failures, "%s", run[i].Name)
			}
		})
	}
}
