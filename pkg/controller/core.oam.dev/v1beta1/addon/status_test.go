package addon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestSetPhaseStampsTimeOnChangeOnly(t *testing.T) {
	ad := &v1beta1.Addon{}
	setPhase(ad, v1beta1.AddonPhaseInstalling)
	assert.Equal(t, v1beta1.AddonPhaseInstalling, ad.Status.Phase)
	assert.NotNil(t, ad.Status.LastReconciledAt)

	first := ad.Status.LastReconciledAt.DeepCopy()
	time.Sleep(2 * time.Millisecond)
	setPhase(ad, v1beta1.AddonPhaseInstalling) // same phase -> no-op
	assert.Equal(t, first, ad.Status.LastReconciledAt, "same phase must not re-stamp time")

	setPhase(ad, v1beta1.AddonPhaseRunning) // change -> stamp moves
	assert.NotEqual(t, first, ad.Status.LastReconciledAt)
}

func TestSetConditionMovesTimeOnSemanticChangeOnly(t *testing.T) {
	ad := &v1beta1.Addon{}
	ad.Generation = 7
	setCondition(ad, v1beta1.AddonConditionReady, metav1.ConditionFalse, "Reconciling", "starting")
	c := findCond(ad, v1beta1.AddonConditionReady)
	assert.Equal(t, int64(7), c.ObservedGeneration)
	t0 := c.LastTransitionTime

	time.Sleep(2 * time.Millisecond)
	setCondition(ad, v1beta1.AddonConditionReady, metav1.ConditionFalse, "Reconciling", "starting") // identical
	assert.Equal(t, t0, findCond(ad, v1beta1.AddonConditionReady).LastTransitionTime, "identical call must not move time")

	setCondition(ad, v1beta1.AddonConditionReady, metav1.ConditionFalse, "Reconciling", "new message") // message change
	assert.NotEqual(t, t0, findCond(ad, v1beta1.AddonConditionReady).LastTransitionTime, "message change must move time")
}

func findCond(ad *v1beta1.Addon, t string) *metav1.Condition {
	for i := range ad.Status.Conditions {
		if ad.Status.Conditions[i].Type == t {
			return &ad.Status.Conditions[i]
		}
	}
	return nil
}
