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

package application

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/pkg/oam"
)

// The guards decide whether a re-resolved source causes a re-dispatch, and each
// one refuses for a different reason. A test per reason, because collapsing them
// into one condition is how the wrong one gets dropped.
func TestAutoUpdatingSourceChangedGuards(t *testing.T) {
	base := sourceRefreshInputs{
		component: "web",
		cluster:   "local",
		hashes:    map[string]string{"cfg": "aaa"},
		updating:  map[string]struct{}{"cfg": {}},
		consumes:  true,
		trackable: true,
		settled:   true,
	}
	h := &AppHandler{Client: fake.NewClientBuilder().Build()}

	for _, tc := range []struct {
		name string
		with func(*sourceRefreshInputs)
		why  string
	}{
		{"reads no source", func(in *sourceRefreshInputs) { in.consumes = false },
			"nothing to compare"},
		{"no binding opted in", func(in *sourceRefreshInputs) { in.updating = nil },
			"a change nobody asked to follow is not a reason to dispatch"},
		{"workload managed by a trait", func(in *sourceRefreshInputs) { in.trackable = false },
			"there is nowhere to stamp a baseline, so every source would read as changed"},
		{"still unhealthy", func(in *sourceRefreshInputs) { in.settled = false },
			"it is being dispatched for another reason anyway"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.with(&in)
			require.False(t, h.autoUpdatingSourceChanged(context.Background(), in), tc.why)
		})
	}
}

// The live workload carries the baseline. A binding whose hash has moved since
// it was stamped is a change; one that has not is not.
func TestAutoUpdatingSourceChangedComparesAgainstTheLiveWorkload(t *testing.T) {
	live := func(hashes string) *unstructured.Unstructured {
		u := &unstructured.Unstructured{}
		u.SetAPIVersion("apps/v1")
		u.SetKind("Deployment")
		u.SetNamespace("default")
		u.SetName("web")
		u.SetAnnotations(map[string]string{oam.AnnotationSourceResolvedHash: hashes})
		return u
	}
	in := func(current map[string]string, watched ...string) sourceRefreshInputs {
		updating := map[string]struct{}{}
		for _, w := range watched {
			updating[w] = struct{}{}
		}
		return sourceRefreshInputs{
			component: "web", cluster: "local",
			baseline: func() map[string]string {
				return map[string]string{"cfg": "aaa", "other": "bbb"}
			},
			hashes: current, updating: updating,
			consumes: true, trackable: true, settled: true,
		}
	}

	h := &AppHandler{Client: fake.NewClientBuilder().
		WithObjects(live(`{"cfg":"aaa","other":"bbb"}`)).Build()}
	ctx := context.Background()

	require.False(t, h.autoUpdatingSourceChanged(ctx,
		in(map[string]string{"cfg": "aaa", "other": "bbb"}, "cfg")),
		"nothing moved, so nothing to do")

	require.True(t, h.autoUpdatingSourceChanged(ctx,
		in(map[string]string{"cfg": "zzz", "other": "bbb"}, "cfg")),
		"the binding that opted in re-resolved")

	require.False(t, h.autoUpdatingSourceChanged(ctx,
		in(map[string]string{"cfg": "aaa", "other": "zzz"}, "cfg")),
		"a binding that did not opt in changed, which is not this function's business")
}

// Only the default stage carries the workload, so a trait dispatched before or
// after it had nothing to compare against and never followed a source it reads.
// Every stage of one component shares a single read of the live baseline, taken
// before the default stage stamps the new one over it.
func TestAutoUpdatingSourceChangedSharesOneBaselineAcrossStages(t *testing.T) {
	reads := 0
	shared := func() map[string]string {
		reads++
		return map[string]string{"cfg": "aaa"}
	}
	in := sourceRefreshInputs{
		component: "web", cluster: "local",
		baseline: shared,
		hashes:   map[string]string{"cfg": "zzz"},
		updating: map[string]struct{}{"cfg": {}},
		consumes: true, trackable: true, settled: true,
	}

	h := &AppHandler{Client: fake.NewClientBuilder().Build()}
	ctx := context.Background()

	// Pre, default and post: each stage asks, each gets the same answer.
	for stage := 0; stage < 3; stage++ {
		require.True(t, h.autoUpdatingSourceChanged(ctx, in),
			"every stage must see the change, not only the one holding the workload")
	}
	require.Equal(t, 3, reads, "the caller memoises; this asserts each stage does ask")
}
