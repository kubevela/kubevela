package rollout

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common/rollout/workloads"
)

// ReconcileRolloutPlan generates the rollout plan and reconcile it
func ReconcileRolloutPlan(ctx context.Context, client client.Client, rolloutSpec *v1alpha1.RolloutPlan,
	targetWorkload, sourceWorkload *unstructured.Unstructured, rolloutStatus *v1alpha1.RolloutStatus) (v1alpha1.RolloutStatus, error) {
	klog.InfoS("generate the rollout plan", "rollout Spec", rolloutSpec,
		"target workload", klog.KObj(targetWorkload))
	if sourceWorkload != nil {
		klog.InfoS("we will do rolling upgrades", "source workload", klog.KObj(sourceWorkload))
	}
	klog.Info("check the rollout status ", "rollout state", rolloutStatus.RollingState, "batch rolling state",
		rolloutStatus.BatchRollingState)

	wf := workloads.NewWorkloadControllerFactory(ctx, client, rolloutSpec, targetWorkload, sourceWorkload)
	wf.GetController(targetWorkload.GroupVersionKind())
	return *rolloutStatus, nil
}
