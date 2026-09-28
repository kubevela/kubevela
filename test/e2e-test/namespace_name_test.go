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

package controllers_test

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
)

func TestCoreRandomNamespaceNameBoundsAndUniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		name := randomNamespaceName("tenant-nsrestrict")
		if seen[name] {
			t.Fatalf("duplicate namespace %q", name)
		}
		seen[name] = true
		if !strings.HasPrefix(name, "tenant-nsrestrict-") {
			t.Fatalf("namespace %q lost its restriction prefix", name)
		}
		if errs := validation.IsDNS1123Label(name); len(errs) != 0 {
			t.Fatalf("invalid namespace %q: %v", name, errs)
		}
	}
	for _, name := range []string{
		randomNamespaceName(strings.Repeat("long-prefix-", 10)),
		"app-postdispatch-multi-component-unhealthy-" + randomNamespaceName(""),
	} {
		if errs := validation.IsDNS1123Label(name); len(errs) != 0 {
			t.Errorf("invalid or oversized namespace %q: %v", name, errs)
		}
	}
}
