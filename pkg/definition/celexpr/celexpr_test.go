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

package celexpr

import (
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// A binding is read as a field selection, so its name has to be an identifier.
//
// This is the one constraint CEL imposes that CUE did not: `source.cluster-info`
// parses as subtraction, and there is no bracket form to fall back on because
// `source` is an object type rather than a map. Catching it when the binding is
// declared gives a better error than a compile failure inside every expression
// that reads it.
func TestBindingNames(t *testing.T) {
	for _, ok := range []string{"cfg", "clusterInfo", "app_config", "_x", "s3"} {
		if err := ValidBindingName(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"cluster-info", "app.config", "9lives", "has space", ""} {
		if err := ValidBindingName(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}

	// The message has to name the fix, not just the rule.
	err := ValidBindingName("cluster-info")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"cluster-info", "source.cluster-info", "clusterinfo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message should mention %q; got %v", want, err)
		}
	}
	t.Logf("%v", err)
}

// And a bad name is refused when the environment is built, not left to surface as
// a confusing compile error inside each expression.
func TestEnvRejectsBadBinding(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString("s: {host: string}").LookupPath(cue.ParsePath("s"))
	if _, err := env(map[string]cue.Value{"cluster-info": v}, nil); err == nil {
		t.Fatal("Env should refuse a hyphenated binding name")
	} else {
		t.Logf("%v", err)
	}
}
