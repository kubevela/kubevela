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

package core_oam_dev

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A webhook path is written in three places: the Go registration, the Helm chart
// that installs the webhook configuration, and the local-debug script that points
// the configuration at a developer's machine. A handler missing from either of the
// latter two fails silently in its own way - a validating webhook admits the
// resource unvalidated, a mutating one is simply never applied - while the tests
// covering the handler keep passing.
//
// Neither is hypothetical. The SourceDefinition validating webhook was registered
// in Go and present in the chart while absent from the debug script, so local runs
// exercised none of its checks; and the debug script created no mutating
// configuration at all, leaving every Application and ComponentDefinition apply to
// fail against a scaled-down in-cluster service.
func TestWebhookPathsAreRegisteredEverywhere(t *testing.T) {
	const (
		goSources  = "v1beta1"
		chartDir   = "../../../charts/vela-core/templates/admission-webhooks"
		debugFile  = "../../../hack/debug-webhook-setup.sh"
		pathRegexp = `/(validating|mutating)-core-oam-dev-v1beta1-[a-z]+`
	)

	re := regexp.MustCompile(pathRegexp)

	pathsIn := func(paths []string) []string {
		seen := map[string]bool{}
		var out []string
		for _, p := range paths {
			if seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
		sort.Strings(out)
		return out
	}

	collectFile := func(path string) []string {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		return pathsIn(re.FindAllString(string(raw), -1))
	}

	// The chart splits validating and mutating configurations across files, so
	// scan the directory rather than naming one of them.
	collectDir := func(dir string) []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		var found []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("reading %s: %v", e.Name(), err)
			}
			found = append(found, re.FindAllString(string(raw), -1)...)
		}
		return pathsIn(found)
	}

	// The Go registrations are spread across the per-resource handler packages.
	var registered []string
	err := filepath.Walk(goSources, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		registered = append(registered, re.FindAllString(string(raw), -1)...)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", goSources, err)
	}
	registered = pathsIn(registered)

	if len(registered) == 0 {
		t.Fatal("found no registered webhook paths — the scan is broken, not the code")
	}

	for _, target := range []struct {
		name  string
		paths []string
	}{
		{"helm chart (" + chartDir + ")", collectDir(chartDir)},
		{"debug script (" + debugFile + ")", collectFile(debugFile)},
	} {
		missing := difference(registered, target.paths)
		if len(missing) > 0 {
			t.Errorf("%s is missing webhook paths that are registered in Go: %v\n"+
				"A handler absent here is never invoked, so the resource is admitted unvalidated.", target.name, missing)
		}
		if extra := difference(target.paths, registered); len(extra) > 0 {
			t.Errorf("%s declares webhook paths with no Go handler: %v\n"+
				"Requests to these paths will fail rather than be validated.", target.name, extra)
		}
	}
}

// difference returns the elements of a that are absent from b.
func difference(a, b []string) []string {
	inB := map[string]bool{}
	for _, s := range b {
		inB[s] = true
	}
	var out []string
	for _, s := range a {
		if !inB[s] {
			out = append(out, s)
		}
	}
	return out
}
