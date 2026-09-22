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
)

func TestValidateCacheKey(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		wantErr string // substring; "" means it must be accepted
	}{
		{name: "simple", key: "cluster-config-reader"},
		{name: "with digits", key: "cluster-config-reader-us-east-1"},
		{name: "empty", key: "", wantErr: "must not be empty"},
		{name: "blank", key: "   ", wantErr: "must not be empty"},
		{name: "uppercase", key: "Cluster-Config", wantErr: "not allowed"},
		{name: "dots", key: "cluster.config", wantErr: "not allowed"},
		{
			// The KEP's own Backstage example: an entityRef interpolated into a key.
			name:    "backstage entity ref",
			key:     "backstage-component-component:default/api",
			wantErr: "not allowed",
		},
		{name: "too long", key: strings.Repeat("a", MaxCacheKeyLen+1), wantErr: "exceeding"},
		{name: "at the limit", key: strings.Repeat("a", MaxCacheKeyLen)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCacheKey(tc.key)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("expected key %q to be accepted, got: %v", tc.key, err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("expected error containing %q for key %q, got nil", tc.wantErr, tc.key)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestInvalidCacheKeyCharsReportsEachDistinctChar(t *testing.T) {
	got := InvalidCacheKeyChars("a:b/c:d")
	// Each offending character is reported once, in first-seen order.
	if !strings.Contains(got, `':'`) || !strings.Contains(got, `'/'`) {
		t.Fatalf("expected both ':' and '/' to be reported, got %q", got)
	}
	if strings.Count(got, `':'`) != 1 {
		t.Fatalf("expected ':' reported once, got %q", got)
	}
}

// TestResolveCachePolicyDoesNotRunProviders proves the cache key is computed
// without executing the definition's provider functions.
//
// The template below reads a ConfigMap that cannot be fetched (the test has no
// reachable cluster). If policy resolution resolved providers - as it did before -
// this would error. Computing the key must not perform the I/O the cache exists
// to avoid.
