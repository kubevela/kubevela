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

package cachekey

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

// The generated expression is what lands in storage.key and what the resolver
// evaluates unchanged, so these strings are the contract. Changing one changes
// the cache identity of every definition regenerated afterwards.
func TestKeyExpression(t *testing.T) {
	cases := []struct {
		name    string
		defName string
		dims    []string // as read by Infer: "field" or "field[index]"
		want    string
		wantErr string
	}{
		{
			// A source reading no context is shared everywhere, so the key is a
			// plain literal with nothing to interpolate.
			name:    "no dimensions is a bare literal",
			defName: "backstage-component",
			dims:    nil,
			want:    `"backstage-component"`,
		},
		{
			name:    "one dimension",
			defName: "cluster-lookup",
			dims:    []string{"cluster"},
			want:    `"cluster-lookup-\(context.cluster)"`,
		},
		{
			name:    "several dimensions keep the order they were given",
			defName: "tenant-data",
			dims:    []string{"cluster", "namespace", "name"},
			want:    `"tenant-data-\(context.cluster)-\(context.namespace)-\(context.name)"`,
		},
		{
			// A label value may contain characters that are legal in a label and
			// illegal in an object name, so it contributes to the hash rather than
			// the prefix. A source reading only labels has a bare prefix.
			name:    "an indexed dimension is not inlined",
			defName: "governance-metadata",
			dims:    []string{"appLabels[example.org/service-name]"},
			want:    `"governance-metadata"`,
		},
		{
			name:    "only the segment is inlined",
			defName: "svc",
			dims:    []string{"cluster", "appLabels[team]"},
			want:    `"svc-\(context.cluster)"`,
		},
		{
			name:    "a definition name that is not key-safe is rejected",
			defName: "Cluster.Lookup",
			dims:    []string{"cluster"},
			wantErr: "not allowed",
		},
		{
			name:    "an empty definition name is rejected",
			defName: "",
			dims:    nil,
			wantErr: "empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules, rerr := LoadRules()
			if rerr != nil {
				t.Fatalf("loading rules: %v", rerr)
			}
			got, err := KeyExpression(tc.defName, parseDims(t, tc.dims), rules)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected an error mentioning %q, got %s", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected an error mentioning %q, got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %s\n     got %s", tc.want, got)
			}
		})
	}
}

// A generated expression that does not compile would be discovered at resolve
// time, in a cluster, as an opaque CUE error - so prove it parses here.
func TestKeyExpressionCompiles(t *testing.T) {
	dims := parseDims(t, []string{"name", "cluster", "appLabels[example.org/service-name]"})
	rules, rerr := LoadRules()
	if rerr != nil {
		t.Fatalf("loading rules: %v", rerr)
	}
	expr, err := KeyExpression("governance-metadata", dims, rules)
	if err != nil {
		t.Fatalf("generating: %v", err)
	}

	// Supply the context the expression reads, then check it evaluates to the
	// key we expect rather than merely parsing.
	src := `
context: {
  cluster: "prod-cluster"
  name:    "api"
  appLabels: "example.org/service-name": "checkout"
}
key: ` + expr + "\n"

	v := cuecontext.New().CompileString(src)
	if v.Err() != nil {
		t.Fatalf("generated expression does not compile: %v\n%s", v.Err(), src)
	}
	got, err := v.LookupPath(cuePathKey()).String()
	if err != nil {
		t.Fatalf("evaluating key: %v", err)
	}
	// The label is hashed by the resolver, so it is absent here.
	const want = "governance-metadata-api-prod-cluster"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// parseDims turns the test's shorthand back into Dimensions, preserving order.
func parseDims(t *testing.T, specs []string) []Dimension {
	t.Helper()
	var dims []Dimension
	for i, s := range specs {
		d := Dimension{order: i}
		if open := strings.Index(s, "["); open >= 0 {
			if !strings.HasSuffix(s, "]") {
				t.Fatalf("malformed dimension %q in test data", s)
			}
			d.Field = s[:open]
			d.Index = s[open+1 : len(s)-1]
		} else {
			d.Field = s
		}
		dims = append(dims, d)
	}
	return dims
}

// The key expression is the readable prefix only. Fields that are not renderable
// - a struct, a label value that may contain a dot, anything legitimately empty -
// contribute to the hash instead, so they must not appear here.
func TestKeyExpressionInlinesOnlySegments(t *testing.T) {
	rules, err := LoadRules()
	if err != nil {
		t.Fatalf("loading rules: %v", err)
	}

	dims, err := Infer(`
output: {
  a: context.cluster
  b: context.clusterVersion.major
  c: context.appLabels["team"]
  d: context.publishVersion
}
`, rules)
	if err != nil {
		t.Fatalf("inferring: %v", err)
	}

	got, err := KeyExpression("svc", dims, rules)
	if err != nil {
		t.Fatalf("generating: %v", err)
	}

	if !strings.Contains(got, `\(context.cluster)`) {
		t.Errorf("cluster is a Kubernetes name and should be inlined; got %s", got)
	}
	for _, absent := range []string{"clusterVersion", "appLabels", "publishVersion"} {
		if strings.Contains(got, absent) {
			t.Errorf("%s is not renderable into a name and must not be inlined; got %s", absent, got)
		}
	}
}

// Every dimension read is recorded, whether or not it is inlined - the resolver
// hashes exactly this set, and hashing more (every label on the object, say) would
// change the identity whenever GitOps stamped an unrelated annotation.
func TestKeyInputsRecordEveryDimension(t *testing.T) {
	rules, err := LoadRules()
	if err != nil {
		t.Fatalf("loading rules: %v", err)
	}
	dims, err := Infer(`
output: {
  a: context.cluster
  b: context.appLabels["team"]
}
`, rules)
	if err != nil {
		t.Fatalf("inferring: %v", err)
	}

	got := KeyInputs(dims)
	want := []string{"cluster", "appLabels[team]"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
