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

package appkeeper

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func appWith(policies ...v1beta1.AppPolicy) *v1beta1.Application {
	app := &v1beta1.Application{ObjectMeta: v1.ObjectMeta{Name: "app", Namespace: "default", Generation: 1}}
	app.Spec.Policies = policies
	return app
}

func appPolicy(typ, raw string) v1beta1.AppPolicy {
	return v1beta1.AppPolicy{Name: typ, Type: typ, Properties: &runtime.RawExtension{Raw: []byte(raw)}}
}

func TestPoliciesReportsWhichPolicyFailedToParse(t *testing.T) {
	for _, typ := range []string{
		v1alpha1.ApplyOncePolicyType, v1alpha1.GarbageCollectPolicyType, v1alpha1.SharedResourcePolicyType,
		v1alpha1.TakeOverPolicyType, v1alpha1.ReadOnlyPolicyType, v1alpha1.ResourceUpdatePolicyType,
	} {
		t.Run(typ, func(t *testing.T) {
			_, err := Policies(appWith(appPolicy(typ, `bad value`)))
			require.ErrorContains(t, err, "failed to parse "+typ+" policy")
		})
	}
}

func TestPoliciesParsesEachResourcePolicy(t *testing.T) {
	got, err := Policies(appWith(
		appPolicy(v1alpha1.ApplyOncePolicyType, `{"enable":true}`),
		appPolicy(v1alpha1.GarbageCollectPolicyType, `{"keepLegacyResource":true}`),
		appPolicy(v1alpha1.SharedResourcePolicyType, `{"rules":[{"selector":{"resourceTypes":["ConfigMap"]}}]}`),
		appPolicy(v1alpha1.TakeOverPolicyType, `{"rules":[{"selector":{"resourceTypes":["Deployment"]}}]}`),
		appPolicy(v1alpha1.ReadOnlyPolicyType, `{"rules":[{"selector":{"resourceTypes":["Secret"]}}]}`),
		appPolicy(v1alpha1.ResourceUpdatePolicyType, `{"rules":[{"selector":{"resourceTypes":["Job"]},"strategy":{"recreateFields":["spec.template"]}}]}`),
	))
	require.NoError(t, err)
	require.True(t, got.ApplyOnce.Enable)
	require.True(t, got.GarbageCollect.KeepLegacyResource)
	require.Equal(t, []string{"ConfigMap"}, got.SharedResource.Rules[0].Selector.ResourceTypes)
	require.Equal(t, []string{"Deployment"}, got.TakeOver.Rules[0].Selector.ResourceTypes)
	require.Equal(t, []string{"Secret"}, got.ReadOnly.Rules[0].Selector.ResourceTypes)
	require.Equal(t, []string{"Job"}, got.ResourceUpdate.Rules[0].Selector.ResourceTypes)
}

func TestPoliciesMergesPoliciesOfTheSameType(t *testing.T) {
	got, err := Policies(appWith(
		appPolicy(v1alpha1.TakeOverPolicyType, `{"rules":[{"selector":{"resourceTypes":["Deployment"]}}]}`),
		appPolicy(v1alpha1.TakeOverPolicyType, `{"rules":[{"selector":{"resourceTypes":["Service"]}}]}`),
	))
	require.NoError(t, err)
	require.Len(t, got.TakeOver.Rules, 2, "rules from both take-over policies are kept")
}

func TestPoliciesAbsentAreNil(t *testing.T) {
	got, err := Policies(appWith())
	require.NoError(t, err)
	require.Nil(t, got.ApplyOnce)
	require.Nil(t, got.GarbageCollect)
	require.Nil(t, got.TakeOver)
}

func TestNewBuildsAKeeperFromTheApplicationsPolicies(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	_, err := New(context.Background(), cli, appWith(appPolicy(v1alpha1.GarbageCollectPolicyType, `bad value`)))
	require.ErrorContains(t, err, "failed to parse garbage-collect policy")

	rk, err := New(context.Background(), cli, appWith(appPolicy(v1alpha1.GarbageCollectPolicyType, `{"keepLegacyResource":true}`)))
	require.NoError(t, err)
	require.NotNil(t, rk)
}

// Each keeper switch is backed by a KubeVela gate: the keeper's default for an unset switch
// must equal the gate's default, and toggling the gate must toggle the switch, so an
// Application keeper behaves as it did when the library read the gates itself.
func TestKeeperOptionsFollowTheirGates(t *testing.T) {
	opts := KeeperOptions(nil, appWith())

	require.False(t, utilfeature.DefaultFeatureGate.Enabled(features.ApplyOnce), "keeper default for ApplyOnce is false")
	require.False(t, opts.ApplyOnce())
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ApplyOnce, true)
	require.True(t, opts.ApplyOnce())

	require.True(t, utilfeature.DefaultFeatureGate.Enabled(features.PreDispatchDryRun), "keeper default for PreDispatchDryRun is true")
	require.True(t, opts.PreDispatchDryRun())
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.PreDispatchDryRun, false)
	require.False(t, opts.PreDispatchDryRun())
}

func TestKeeperOptionsActAsTheApplicationsUser(t *testing.T) {
	app := appWith()
	app.SetAnnotations(map[string]string{oam.AnnotationApplicationUsername: "alice"})
	u, ok := request.UserFrom(KeeperOptions(nil, app).Requester(context.Background()))
	require.True(t, ok)
	require.Equal(t, "alice", u.GetName())
}

func TestKeeperOptionsRecordStageTimings(t *testing.T) {
	observe := KeeperOptions(nil, appWith()).ObserveStage
	require.NotNil(t, observe)
	observe("gc-rt.test", time.Millisecond) // must not panic on an unseen stage label
}

// Addons are applied once unless their Application says otherwise.
func TestPoliciesDefaultApplyOnceForAddons(t *testing.T) {
	addon := appWith()
	addon.SetLabels(map[string]string{oam.LabelAddonName: "fluxcd"})
	got, err := Policies(addon)
	require.NoError(t, err)
	require.NotNil(t, got.ApplyOnce)
	require.True(t, got.ApplyOnce.Enable)

	explicit := appWith(appPolicy(v1alpha1.ApplyOncePolicyType, `{"enable":false}`))
	explicit.SetLabels(map[string]string{oam.LabelAddonName: "fluxcd"})
	got, err = Policies(explicit)
	require.NoError(t, err)
	require.False(t, got.ApplyOnce.Enable, "an explicit apply-once policy wins over the addon default")

	got, err = Policies(appWith())
	require.NoError(t, err)
	require.Nil(t, got.ApplyOnce, "no default outside addons")
}
