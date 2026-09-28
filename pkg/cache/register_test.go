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

package cache

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// recordingIndexer stands in for the cache, which cannot be built without an API
// server.
type recordingIndexer struct {
	registered map[string]client.IndexerFunc
	failOn     string
}

func (r *recordingIndexer) IndexField(_ context.Context, obj client.Object, field string, fn client.IndexerFunc) error {
	key := fmt.Sprintf("%T/%s", obj, field)
	if r.failOn == key {
		return errors.New("boom")
	}
	if r.registered == nil {
		r.registered = map[string]client.IndexerFunc{}
	}
	r.registered[key] = fn
	return nil
}

func (r *recordingIndexer) names() []string {
	out := make([]string, 0, len(r.registered))
	for k := range r.registered {
		out = append(out, k)
	}
	return out
}

const (
	rtIndex    = "*v1beta1.ResourceTracker/app"
	revIndex   = "*v1beta1.ApplicationRevision/app"
	usageIndex = "*v1beta1.Application/definitionUsage"
)

func withFlags(t *testing.T, optimize, gate bool) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.RestrictDefinitionNamespaces, gate)
	prevOptimize, prevIndexed := OptimizeListOp, DefinitionUsageIndexed
	OptimizeListOp, DefinitionUsageIndexed = optimize, false
	t.Cleanup(func() { OptimizeListOp, DefinitionUsageIndexed = prevOptimize, prevIndexed })
}

func TestRegisterIndexes(t *testing.T) {
	testCases := map[string]struct {
		optimize bool
		gate     bool
		expected []string
		indexed  bool
	}{
		"no informer indexes at all": {false, true, nil, false},
		"the quota feature is off":   {true, false, []string{rtIndex, revIndex}, false},
		"everything on":              {true, true, []string{rtIndex, revIndex, usageIndex}, true},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			withFlags(t, tc.optimize, tc.gate)
			idx := &recordingIndexer{}
			require.NoError(t, registerIndexes(context.Background(), idx))
			assert.ElementsMatch(t, tc.expected, idx.names())
			assert.Equal(t, tc.indexed, DefinitionUsageIndexed,
				"DefinitionUsageIndexed must say what was registered")
		})
	}
}

// A failed registration is returned, and does not leave DefinitionUsageIndexed
// claiming an index that is not there.
func TestRegisterIndexesReportsFailure(t *testing.T) {
	for _, failing := range []string{rtIndex, revIndex, usageIndex} {
		t.Run(failing, func(t *testing.T) {
			withFlags(t, true, true)
			err := registerIndexes(context.Background(), &recordingIndexer{failOn: failing})
			require.Error(t, err)
			assert.False(t, DefinitionUsageIndexed)
		})
	}
}

// The registered index function is the one the quota count reads back, so it must
// key on the same names.
func TestRegisteredDefinitionUsageIndexFunc(t *testing.T) {
	withFlags(t, true, true)
	idx := &recordingIndexer{}
	require.NoError(t, registerIndexes(context.Background(), idx))

	fn := idx.registered[usageIndex]
	require.NotNil(t, fn)
	assert.ElementsMatch(t, []string{"component/webservice", "component/worker", "trait/gateway"},
		fn(&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant-a"},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
				{Type: "webservice", Traits: []common.ApplicationTrait{{Type: "gateway"}}},
				{Type: "webservice@v1"}, {Type: "worker"},
			}},
		}))
	assert.Nil(t, fn(&v1beta1.ResourceTracker{}), "anything else indexes under nothing")
}

// The app indexes key on "<namespace>/<name>", which is what GetAppRevisions and
// the ResourceTracker lookups query by.
func TestRegisteredAppIndexFuncs(t *testing.T) {
	withFlags(t, true, true)
	idx := &recordingIndexer{}
	require.NoError(t, registerIndexes(context.Background(), idx))

	rt := &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: "rt", Labels: map[string]string{
		oam.LabelAppNamespace: "tenant-a", oam.LabelAppName: "web"}}}
	assert.Equal(t, []string{"tenant-a/web"}, idx.registered[rtIndex](rt))

	rev := &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{
		Name: "web-v1", Namespace: "tenant-a", Labels: map[string]string{oam.LabelAppName: "web"}}}
	assert.Equal(t, []string{"tenant-a/web"}, idx.registered[revIndex](rev))
}
