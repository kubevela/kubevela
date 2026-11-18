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

	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/stretchr/testify/require"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
)

// policyHashKeeper records what the policy data refresh asks of a resource keeper.
type policyHashKeeper struct {
	resourcekeeper.ResourceKeeper
	hash     string
	recorded bool
	reads    int
	writes   []string
	refresh  int
}

func (k *policyHashKeeper) PolicyContextHash() (string, bool) {
	k.reads++
	return k.hash, k.recorded
}

func (k *policyHashKeeper) SetPolicyContextHash(_ context.Context, hash string) error {
	k.writes = append(k.writes, hash)
	k.hash, k.recorded = hash, true
	return nil
}

func (k *policyHashKeeper) RefreshRecordedMetadata(context.Context, map[string]string, map[string]string, resourcekeeper.MetadataRemoval) (bool, error) {
	k.refresh++
	return false, nil
}

func TestPolicyDataRefreshNeedsPolicyGate(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableApplicationScopedPolicies, false)
	keeper := &policyHashKeeper{}
	handler := &AppHandler{resourceKeeper: keeper}
	r := &Reconciler{}
	r.refreshPolicyData(monitorContext.NewTraceContext(context.Background(), ""), handler, nil, nil, appWithAnnotations(nil))
	require.Zero(t, keeper.reads, "no policy context is tracked while Application-scoped policies are off")
	require.Empty(t, keeper.writes, "no tracker is written while Application-scoped policies are off")
	require.Zero(t, keeper.refresh)

	r.recordPolicyContextHash(monitorContext.NewTraceContext(context.Background(), ""), handler)
	require.Empty(t, keeper.writes, "a finished workflow records no hash while Application-scoped policies are off")
}

func TestPolicyDataRefreshRecordsHashWithPolicyGate(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableApplicationScopedPolicies, true)
	keeper := &policyHashKeeper{}
	handler := &AppHandler{resourceKeeper: keeper, contextHashes: map[string]string{"global:p": "h1"}}
	r := &Reconciler{}
	r.refreshPolicyData(monitorContext.NewTraceContext(context.Background(), ""), handler, nil, nil, appWithAnnotations(nil))
	require.Equal(t, []string{`{"global:p":"h1"}`}, keeper.writes, "a tracker without a hash records the running one")

	keeper.writes = nil
	r.recordPolicyContextHash(monitorContext.NewTraceContext(context.Background(), ""), handler)
	require.Equal(t, []string{`{"global:p":"h1"}`}, keeper.writes)
}

func TestHoldPolicyData(t *testing.T) {
	setRolloutDefaults(t, false, false, false)
	app := appWithAnnotations(map[string]string{oam.AnnotationApplyDataChanges: "on"})
	rendered := func() []RenderedPolicyResult {
		return []RenderedPolicyResult{
			{PolicyName: "on", PolicyType: "on", VersionKey: "global:on", Enabled: true,
				AdditionalContext: map[string]interface{}{"a": "v2"},
				Transforms:        &PolicyOutput{Labels: map[string]string{"a": "v2"}}},
			{PolicyName: "off", PolicyType: "off", VersionKey: "global:off", Enabled: true,
				AdditionalContext: map[string]interface{}{"b": "v2"},
				Transforms:        &PolicyOutput{Labels: map[string]string{"b": "v2"}, Annotations: map[string]string{"note": "v2"}}},
			{PolicyName: "new", PolicyType: "new", VersionKey: "global:new", Enabled: true,
				AdditionalContext: map[string]interface{}{"c": "v2"}},
		}
	}
	applied := map[string]appliedPolicyOutput{
		"global:on":  {Context: map[string]interface{}{"a": "v1"}, Labels: map[string]string{"a": "v1"}},
		"global:off": {Context: map[string]interface{}{"b": "v1"}, Labels: map[string]string{"b": "v1"}},
	}

	input := rendered()
	held := holdPolicyData(app, input, applied, false)
	require.Equal(t, map[string]interface{}{"a": "v2"}, held[0].AdditionalContext, "apply-data-changes on takes the new data")
	require.False(t, held[0].DataPending)
	require.Equal(t, map[string]interface{}{"b": "v1"}, held[1].AdditionalContext, "apply-data-changes off keeps the applied context")
	require.Equal(t, map[string]string{"b": "v1"}, held[1].Transforms.(*PolicyOutput).Labels, "and the applied labels")
	require.Empty(t, held[1].Transforms.(*PolicyOutput).Annotations, "and the applied annotations")
	require.True(t, held[1].DataPending)
	require.Equal(t, map[string]interface{}{"c": "v2"}, held[2].AdditionalContext, "a policy with nothing applied yet takes its data")
	require.False(t, held[2].DataPending)
	require.Equal(t, rendered(), input, "the rendered results, which the policy cache shares, are not written to")

	taken := holdPolicyData(app, rendered(), applied, true)
	require.Equal(t, rendered(), taken, "a new revision takes every policy's data")

	unchanged := rendered()
	unchanged[1].AdditionalContext = map[string]interface{}{"b": "v1"}
	unchanged[1].Transforms = &PolicyOutput{Labels: map[string]string{"b": "v1"}}
	require.False(t, holdPolicyData(app, unchanged, applied, false)[1].DataPending, "data equal to what was applied is not pending")

	recorded := appliedOutputs(held)
	require.Equal(t, applied["global:off"], recorded["global:off"], "what was applied is recorded as applied again")
	require.Equal(t, appliedPolicyOutput{Context: map[string]interface{}{"a": "v2"}, Labels: map[string]string{"a": "v2"}}, recorded["global:on"])
	require.Equal(t, recorded, decodeAppliedOutputs(encodeAppliedOutputs(recorded)), "applied outputs survive the ConfigMap round trip")
}
