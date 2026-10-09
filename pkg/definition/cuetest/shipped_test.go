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
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/utils"
)

// shippedDefinitions are KubeVela's built-in definitions, which ship in the
// vela-core chart.
const shippedDefinitions = "../../../vela-templates/definitions/internal"

// shippedDefinitionTestsEnv opts into the tests of the shipped definitions,
// which CI runs in their own job (make test-definitions), not with the unit tests.
const shippedDefinitionTestsEnv = "VELA_SHIPPED_DEFINITION_TESTS"

func requireShippedDefinitionTests(t *testing.T) {
	t.Helper()
	if os.Getenv(shippedDefinitionTestsEnv) == "" {
		t.Skipf("set %s=1 to run, or use make test-definitions", shippedDefinitionTestsEnv)
	}
}

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
	requireShippedDefinitionTests(t)
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
	requireShippedDefinitionTests(t)
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

// uncovered are declared parameters no case sets, and why: a whole
// definition as "<kind>/<name>", or one parameter as "<kind>/<name>:<path>".
// A listed one that gains a case must come off the list.
var uncovered = map[string]string{
	"workflowstep/suspend:message":               "only its @pending case sets it, until kubevela/workflow's builtin.#Suspend reads it",
	"trait/k8s-update-strategy:targetAPIVersion": "declared but never read by the template, so a case has nothing to assert",
}

// coverageDepth is how deep into a parameter's fields the coverage check looks.
const coverageDepth = 2

// Every parameter a shipped definition declares, to coverageDepth, is set by
// a case that runs.
func TestShippedParametersAreCovered(t *testing.T) {
	requireShippedDefinitionTests(t)
	suites, err := Load(shippedDefinitions)
	require.NoError(t, err)
	compiler, err := testCompiler()
	require.NoError(t, err)
	var missing, stale []string
	listed := map[string]bool{}
	for _, s := range suites {
		require.NoError(t, s.Err)
		rel, _ := filepath.Rel(shippedDefinitions, s.File)
		def := strings.TrimSuffix(rel, utils.CUETestFileSuffix)
		if len(s.Cases) == 0 {
			continue
		}
		if _, skip := uncovered[def]; skip {
			listed[def] = true
			continue
		}
		// the controller supplies context when it renders; here it only needs to resolve
		v, err := compiler.CompileStringWithOptions(context.Background(), s.Cases[0].Subject.Template+"\ncontext: {...}\n", cuex.DisableResolveProviderFunctions{})
		require.NoError(t, err, def)
		param := v.LookupPath(cue.ParsePath("parameter"))
		if !param.Exists() {
			require.NoErrorf(t, v.Err(), "%s: its template does not evaluate, so its parameters cannot be read", def)
			continue
		}
		declared := map[string]bool{}
		declaredPaths(param, "", 1, declared)
		set := map[string]bool{}
		for _, c := range s.Cases {
			if !c.Pending {
				setPaths(c.Input.Parameter, "", 1, set)
			}
		}
		for p := range declared {
			key := def + ":" + p
			_, excused := uncovered[key]
			switch {
			case excused:
				listed[key] = true
			case !set[p]:
				missing = append(missing, key)
			}
		}
	}
	for key := range uncovered {
		if !listed[key] {
			stale = append(stale, key)
		}
	}
	for key := range listed {
		if def, p, ok := strings.Cut(key, ":"); ok && coveredElsewhere(suites, def, p) {
			stale = append(stale, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	require.Empty(t, missing, "declared parameters no running case sets; add a case, or list them in uncovered with why")
	require.Empty(t, stale, "listed as uncovered, but covered or gone")
}

// declaredPaths adds the dotted path of each field v declares, optional ones
// included, descending into structs and list elements. A disjunction declares
// the fields of all its branches.
func declaredPaths(v cue.Value, prefix string, depth int, into map[string]bool) {
	if op, args := v.Expr(); op == cue.OrOp {
		for _, a := range args {
			declaredPaths(a, prefix, depth, into)
		}
		return
	}
	it, err := v.Fields(cue.Optional(true))
	if err != nil {
		return
	}
	for it.Next() {
		p := prefix + it.Selector().Unquoted()
		into[p] = true
		if depth >= coverageDepth {
			continue
		}
		child := it.Value()
		if elem := child.LookupPath(cue.MakePath(cue.AnyIndex)); child.IncompleteKind()&cue.ListKind != 0 && elem.Exists() {
			child = elem
		}
		declaredPaths(child, p+".", depth+1, into)
	}
}

// setPaths adds the dotted path of each field a case's parameter sets, the
// way declaredPaths names them.
func setPaths(v any, prefix string, depth int, into map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			p := prefix + k
			into[p] = true
			if depth < coverageDepth {
				setPaths(child, p+".", depth+1, into)
			}
		}
	case []any:
		for _, e := range v {
			setPaths(e, prefix, depth, into)
		}
	}
}

// coveredElsewhere reports whether a running case of def sets path p.
func coveredElsewhere(suites []*Suite, def, p string) bool {
	for _, s := range suites {
		rel, _ := filepath.Rel(shippedDefinitions, s.File)
		if strings.TrimSuffix(rel, utils.CUETestFileSuffix) != def {
			continue
		}
		set := map[string]bool{}
		for _, c := range s.Cases {
			if !c.Pending {
				setPaths(c.Input.Parameter, "", 1, set)
			}
		}
		return set[p]
	}
	return false
}
