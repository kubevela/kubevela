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

package common

import (
	"testing"

	"github.com/kubevela/pkg/util/compression"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// Each library switch is backed by a KubeVela gate. The library's default for an unset
// switch (false) must equal the gate's default, and toggling the gate must toggle the
// switch, so behaviour is unchanged from when the library read the gates itself.
func TestApplySwitchesFollowTheirGates(t *testing.T) {
	cfg := apply.CurrentConfig()
	for name, tc := range map[string]struct {
		gate   featuregate.Feature
		enable func() bool
	}{
		"ReplaceOnUpdate":       {features.ApplyResourceByReplace, cfg.ReplaceOnUpdate},
		"LegacyOwnerValidation": {features.LegacyResourceOwnerValidation, cfg.LegacyOwnerValidation},
	} {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, tc.enable, "switch not wired")
			require.False(t, utilfeature.DefaultFeatureGate.Enabled(tc.gate), "gate default differs from the library default (false)")
			require.False(t, tc.enable())
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, tc.gate, true)
			require.True(t, tc.enable(), "switch does not follow its gate")
		})
	}
}

func TestApplyUsesKubeVelasScheme(t *testing.T) {
	require.Same(t, Scheme, apply.CurrentConfig().Scheme)
}

func TestResourceTrackerCompressionFollowsItsGates(t *testing.T) {
	compress := resourcetracker.CurrentConfig().Compression
	require.NotNil(t, compress, "compression not wired")
	require.False(t, utilfeature.DefaultFeatureGate.Enabled(features.GzipResourceTracker))
	require.False(t, utilfeature.DefaultFeatureGate.Enabled(features.ZstdResourceTracker))
	require.Equal(t, compression.Type(""), compress(), "default must leave trackers uncompressed")

	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.GzipResourceTracker, true)
	require.Equal(t, compression.Gzip, compress())
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ZstdResourceTracker, true)
	require.Equal(t, compression.Zstd, compress(), "zstd wins when both gates are on")
}

func TestResourceTrackerListsAreCounted(t *testing.T) {
	require.NotNil(t, resourcetracker.CurrentConfig().OnList, "list metric not wired")
}

// Handing a shared resource to the next sharer labels it as that sharer's. Keys without a
// kind are Applications', parsed as the promotion code always parsed them.
func TestSharerKeysResolveToOwnerLabels(t *testing.T) {
	appLabels := func(ns, name string) map[string]string {
		return map[string]string{"app.oam.dev/name": name, "app.oam.dev/namespace": ns,
			"owner.oam.dev/kind": "Application", "owner.oam.dev/name": name, "owner.oam.dev/namespace": ns}
	}
	require.Equal(t, appLabels("team-a", "shop"), resourcetracker.LabelsForKey("team-a/shop"))
	require.Equal(t, appLabels("default", "shop"), resourcetracker.LabelsForKey("shop"))
	require.Equal(t, appLabels("team-a", "shop"), resourcetracker.LabelsForKey("Application/team-a/shop"), "the canonical form of the same owner")
	require.Equal(t, map[string]string{"owner.oam.dev/kind": "Component", "owner.oam.dev/namespace": "team-a", "owner.oam.dev/name": "backend"},
		resourcetracker.LabelsForKey("Component/team-a/backend"))
}

// Keys written without a kind are Applications': the sharer lists on clusters have always
// held <namespace>/<name>.
func TestOwnerKeysWithoutAKindAreApplications(t *testing.T) {
	require.Equal(t, "Application/team-a/shop", apply.CanonicalOwnerKey("team-a/shop"))
	require.Equal(t, "Application", apply.CurrentConfig().DefaultOwnerKind)
}

// Resources written before owner labels carry app.oam.dev/* only; other kinds must still see
// them as the Application's.
func TestAppLabelsNameTheLegacyOwner(t *testing.T) {
	legacy := apply.CurrentConfig().LegacyControlledBy
	require.NotNil(t, legacy)
	cm := &corev1.ConfigMap{}
	cm.SetLabels(map[string]string{"app.oam.dev/name": "shop", "app.oam.dev/namespace": "team-a"})
	require.Equal(t, "Application/team-a/shop", legacy(cm))
	require.Empty(t, legacy(&corev1.ConfigMap{}), "no marks, no owner")
}

// The tracker-listing metric has always carried "application"; a dashboard selecting on it
// must keep working whatever an owner kind calls itself.
// A gate can go off as well as on: the switches read it per call, so both directions matter.
func TestSwitchesFollowTheGateBackDown(t *testing.T) {
	require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{string(features.ApplyResourceByReplace): true}))
	require.True(t, apply.CurrentConfig().ReplaceOnUpdate())
	require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{string(features.ApplyResourceByReplace): false}))
	require.False(t, apply.CurrentConfig().ReplaceOnUpdate(), "the switch follows the gate back down")
}

func TestTrackerListingsCountUnderTheKindsLowercaseName(t *testing.T) {
	onList := resourcetracker.CurrentConfig().OnList
	require.NotNil(t, onList)
	count := func(kind string) float64 {
		return testutil.ToFloat64(metrics.ListResourceTrackerCounter.WithLabelValues(kind))
	}
	before := count("application")
	onList(v1beta1.ApplicationKind)
	require.Equal(t, before+1, count("application"), "an Application counts as it always has")

	beforeComponent := count("component")
	onList("Component")
	require.Equal(t, beforeComponent+1, count("component"))
}
