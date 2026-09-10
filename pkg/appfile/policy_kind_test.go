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

package appfile

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// IsBuiltinPolicyType must agree with the switch that decides which policies are
// rendered, or a policy would be given expression rules meant for the other kind.
//
// Read from the source rather than restated, because a type added to the switch
// and not here is exactly the drift that would go unnoticed - the policy would be
// treated as rendered and handed a resolver it never uses.
func TestBuiltinPolicyTypesMatchParser(t *testing.T) {
	src, err := os.ReadFile("parser.go")
	if err != nil {
		t.Fatalf("reading parser.go: %v", err)
	}
	fn := regexp.MustCompile(`(?s)func \(p \*Parser\) parsePolicies\(.*?\n}`).Find(src)
	if fn == nil {
		t.Fatal("could not find parsePolicies")
	}

	var inSwitch []string
	for _, m := range regexp.MustCompile(`case v1alpha1\.(\w+):`).FindAllSubmatch(fn, -1) {
		inSwitch = append(inSwitch, string(m[1]))
	}
	if len(inSwitch) == 0 {
		t.Fatal("found no cases; the test has lost track of the switch")
	}

	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`v1alpha1\.(\w+):\s+true`).FindAllSubmatch(
		mustRead(t, "policy_kind.go"), -1) {
		declared[string(m[1])] = true
	}

	var missing, extra []string
	seen := map[string]bool{}
	for _, name := range inSwitch {
		seen[name] = true
		if !declared[name] {
			missing = append(missing, name)
		}
	}
	for name := range declared {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("parsePolicies treats %v as built-in but builtinPolicyTypes does not; "+
			"they would be given the rendered policy's expression rules", missing)
	}
	if len(extra) > 0 {
		t.Errorf("builtinPolicyTypes lists %v but parsePolicies renders them; "+
			"their expressions would be substituted twice", extra)
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return b
}
