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

package sources

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A read was looked up by splitting its rendered dotted path, so a key
// containing a dot - a ConfigMap entry called app.properties, a domain-prefixed
// label - was read as two segments and found nothing. The value still
// substituted, so nothing looked wrong; what was lost is the consumed-value
// record, and with it the hash that drives auto-update for that binding.
func TestLookupMapSegmentsKeepsDottedKeysWhole(t *testing.T) {
	data := map[string]interface{}{
		"data": map[string]interface{}{
			"app.properties": "k=v",
		},
		"labels": map[string]interface{}{
			"app.kubernetes.io/name": "web",
		},
		"items": []interface{}{
			map[string]interface{}{"name": "first"},
		},
		"host": "example.com",
	}

	for _, tc := range []struct {
		segments []string
		want     interface{}
	}{
		{[]string{"data", "app.properties"}, "k=v"},
		{[]string{"labels", "app.kubernetes.io/name"}, "web"},
		{[]string{"items", "0", "name"}, "first"},
		{[]string{"host"}, "example.com"},
	} {
		got, ok := lookupMapSegments(data, tc.segments)
		require.True(t, ok, "segments %v", tc.segments)
		require.Equal(t, tc.want, got)
	}

	// No segments is a read of the binding entire.
	got, ok := lookupMapSegments(data, nil)
	require.True(t, ok)
	require.Equal(t, data, got)

	_, ok = lookupMapSegments(data, []string{"data", "app", "properties"})
	require.False(t, ok, "a dotted key must not be reachable as two segments")
}
