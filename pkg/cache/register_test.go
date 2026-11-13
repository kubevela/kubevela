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

	workflowv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

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
	pinIndex   = "*v1beta1.Application/definitionPin"
)

func withFlags(t *testing.T, optimize, gate bool) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.RestrictDefinitionNamespaces, gate)
	prevOptimize, prevIndexed, prevPins := OptimizeListOp, DefinitionUsageIndexed, DefinitionPinIndexed
	OptimizeListOp, DefinitionUsageIndexed, DefinitionPinIndexed = optimize, false, false
	t.Cleanup(func() {
		OptimizeListOp, DefinitionUsageIndexed, DefinitionPinIndexed = prevOptimize, prevIndexed, prevPins
	})
}

func TestRegisterIndexes(t *testing.T) {
	testCases := map[string]struct {
		optimize bool
		gate     bool
		expected []string
		indexed  bool
	}{
		"no informer indexes at all": {false, true, nil, false},
		"the quota feature is off":   {true, false, []string{rtIndex, revIndex, pinIndex}, false},
		"everything on":              {true, true, []string{rtIndex, revIndex, pinIndex, usageIndex}, true},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			withFlags(t, tc.optimize, tc.gate)
			idx := &recordingIndexer{}
			require.NoError(t, registerIndexes(context.Background(), idx))
			assert.ElementsMatch(t, tc.expected, idx.names())
			assert.Equal(t, tc.indexed, DefinitionUsageIndexed,
				"DefinitionUsageIndexed must say what was registered")
			assert.Equal(t, tc.optimize, DefinitionPinIndexed,
				"DefinitionPinIndexed must say what was registered")
		})
	}
}

// A failed registration is returned, and does not leave DefinitionUsageIndexed
// claiming an index that is not there.
func TestRegisterIndexesReportsFailure(t *testing.T) {
	for _, failing := range []string{rtIndex, revIndex, pinIndex, usageIndex} {
		t.Run(failing, func(t *testing.T) {
			withFlags(t, true, true)
			err := registerIndexes(context.Background(), &recordingIndexer{failOn: failing})
			require.Error(t, err)
			assert.False(t, DefinitionUsageIndexed)
			if failing != usageIndex {
				assert.False(t, DefinitionPinIndexed)
			}
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

// Only pinned types are indexed, by kind and pin exactly as written, so the revision
// garbage collector can ask for the Applications pinning one revision.
func TestRegisteredDefinitionPinIndexFunc(t *testing.T) {
	withFlags(t, true, false)
	idx := &recordingIndexer{}
	require.NoError(t, registerIndexes(context.Background(), idx))

	fn := idx.registered[pinIndex]
	require.NotNil(t, fn)
	assert.ElementsMatch(t, []string{
		"Component/webservice@v1", "Trait/gateway@v2", "Policy/topology@v1",
		"WorkflowStep/deploy@v3", "WorkflowStep/notification@v1.2.0", "Source/config@v1",
	}, fn(&v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant-a"},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{Type: "webservice@v1", Traits: []common.ApplicationTrait{{Type: "gateway@v2"}, {Type: "scaler"}}},
				{Type: "webservice@v1"}, {Type: "worker"},
			},
			Policies: []v1beta1.AppPolicy{{Type: "topology@v1"}, {Type: "override"}},
			Workflow: &v1beta1.Workflow{Steps: []workflowv1alpha1.WorkflowStep{
				{WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{Type: "deploy@v3"}},
				{
					WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{Type: "step-group"},
					SubSteps:         []workflowv1alpha1.WorkflowStepBase{{Type: "notification@v1.2.0"}},
				},
			}},
			Sources: []v1beta1.ApplicationSource{{Name: "cfg", Type: "config@v1"}},
		},
	}))
	assert.Empty(t, fn(&v1beta1.Application{Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{{Type: "webservice"}},
	}}), "an Application with no pins is indexed under nothing")
}
