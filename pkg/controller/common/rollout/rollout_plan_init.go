package rollout

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// ReconcileRolloutPlan generates the rollout plan and reconcile it
func ReconcileRolloutPlan(ctx context.Context, client client.Client, rolloutSpec *v1alpha1.RolloutPlan,
	targetWorkload, sourceWorkload *unstructured.Unstructured) error {
	klog.InfoS("generate the rollout plan", "rollout Spec", rolloutSpec,
		"target workload", klog.KObj(targetWorkload))
	return nil
}
