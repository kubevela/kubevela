package addon

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestBuildArgs(t *testing.T) {
	none, err := buildArgs(&v1beta1.Addon{})
	assert.NoError(t, err)
	assert.Nil(t, none)

	ad := &v1beta1.Addon{Spec: v1beta1.AddonSpec{
		Parameters: &runtime.RawExtension{Raw: []byte(`{"replicas":2,"ns":"flux"}`)},
	}}
	args, err := buildArgs(ad)
	assert.NoError(t, err)
	assert.Equal(t, float64(2), args["replicas"])
	assert.Equal(t, "flux", args["ns"])

	bad := &v1beta1.Addon{Spec: v1beta1.AddonSpec{
		Parameters: &runtime.RawExtension{Raw: []byte(`not-json`)},
	}}
	_, err = buildArgs(bad)
	assert.Error(t, err)
}

func TestIsRegistryUnreachable(t *testing.T) {
	assert.True(t, isRegistryUnreachable(errSourceUnresolved))
	wrapped := errors.Join(errSourceUnresolved, errors.New("dial tcp timeout"))
	assert.True(t, isRegistryUnreachable(wrapped))
	assert.False(t, isRegistryUnreachable(errors.New("render failed")))
}

func TestSourceResolvedStaleFor(t *testing.T) {
	ad := &v1beta1.Addon{}
	assert.False(t, sourceResolvedStaleFor(ad, failedThreshold))

	old := metav1.NewTime(time.Now().Add(-11 * time.Minute))
	ad.Status.Conditions = []metav1.Condition{{
		Type: v1beta1.AddonConditionSourceResolved, Status: metav1.ConditionFalse, LastTransitionTime: old,
	}}
	assert.True(t, sourceResolvedStaleFor(ad, failedThreshold))

	ad.Status.Conditions[0].LastTransitionTime = metav1.Now()
	assert.False(t, sourceResolvedStaleFor(ad, failedThreshold))
}

func TestInstallOptions(t *testing.T) {
	assert.Empty(t, installOptions(&v1beta1.Addon{}))
	assert.Len(t, installOptions(&v1beta1.Addon{Spec: v1beta1.AddonSpec{SkipVersionCheck: true}}), 1)
	assert.Len(t, installOptions(&v1beta1.Addon{Spec: v1beta1.AddonSpec{OverrideDefinitions: true}}), 1)
	assert.Len(t, installOptions(&v1beta1.Addon{Spec: v1beta1.AddonSpec{SkipVersionCheck: true, OverrideDefinitions: true}}), 2)
}
