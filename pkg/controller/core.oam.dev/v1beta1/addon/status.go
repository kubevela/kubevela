package addon

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// setPhase sets status.phase and stamps status.lastReconciledAt, but only when
// the phase value changes, so steady-state reconciles do not churn status.
func setPhase(ad *v1beta1.Addon, phase v1beta1.AddonPhase) {
	if ad.Status.Phase == phase {
		return
	}
	now := metav1.Now()
	ad.Status.Phase = phase
	ad.Status.LastReconciledAt = &now
}

// setCondition upserts a metav1.Condition. ObservedGeneration is always refreshed
// to the addon's generation; LastTransitionTime moves only when Status, Reason,
// or Message change relative to the existing condition of the same type.
func setCondition(ad *v1beta1.Addon, condType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	conds := ad.Status.Conditions
	for i := range conds {
		if conds[i].Type == condType {
			changed := conds[i].Status != status || conds[i].Reason != reason || conds[i].Message != message
			conds[i].Status = status
			conds[i].Reason = reason
			conds[i].Message = message
			conds[i].ObservedGeneration = ad.GetGeneration()
			if changed {
				conds[i].LastTransitionTime = now
			}
			return
		}
	}
	ad.Status.Conditions = append(conds, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: ad.GetGeneration(),
		LastTransitionTime: now,
	})
}

// patchStatus writes the status subresource with the controller's field manager.
// It updates observedGeneration and is a no-op when status is unchanged.
func (r *Reconciler) patchStatus(ctx context.Context, base, ad *v1beta1.Addon) error {
	ad.Status.ObservedGeneration = ad.GetGeneration()
	if equality.Semantic.DeepEqual(base.Status, ad.Status) {
		return nil
	}
	return r.Status().Patch(ctx, ad, client.MergeFrom(base), client.FieldOwner(FieldManagerAddonController))
}
