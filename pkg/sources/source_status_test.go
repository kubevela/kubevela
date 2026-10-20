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
	"time"

	"github.com/stretchr/testify/require"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

// A value served past its TTL is resolved, because a value is in hand, and
// flagged so the Application controller can report it as Stale. The flag and
// not the expiry timestamp: a Layer 1 hit reports the persistent store's expiry
// while being served as fresh, so the timestamp does not say what happened.
func TestServeStaleFlagsTheStatus(t *testing.T) {
	r := &sourceResolver{
		resolved: map[string]map[string]interface{}{},
		statuses: map[string]SourceResolutionStatus{},
	}
	f := staleFallback{
		name:       "cfg",
		sourceType: "demo",
		policy:     sourceCachePolicy{Key: "demo-abc", OnStaleFailure: sourceCachePolicyUseStale},
		stored: velaprocess.SourceCacheRead{
			Data:      map[string]interface{}{"host": "db"},
			Found:     true,
			Stale:     true,
			ExpiresAt: time.Now().Add(-time.Minute),
		},
	}

	got, ok := r.serveStale(f, "refresh failed; serving stale cached value")
	require.True(t, ok)
	require.Equal(t, map[string]interface{}{"host": "db"}, got)

	st := r.statuses["cfg"]
	require.Equal(t, PhaseResolved, st.Phase, "a value is in hand, so the resolver reports resolved")
	require.True(t, st.Stale, "and says it is past its TTL")
	require.Contains(t, st.Message, "serving stale")
}

// A fresh hit is not flagged, or every binding would read as stale.
func TestAFreshStatusIsNotFlaggedStale(t *testing.T) {
	r := &sourceResolver{statuses: map[string]SourceResolutionStatus{}}
	r.setSourceStatus("cfg", "demo", PhaseResolved, "", "demo-abc", "2026-10-18T00:00:00Z")
	require.False(t, r.statuses["cfg"].Stale)
}
