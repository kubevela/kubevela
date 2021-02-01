package workloads

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// WorkloadControllerFactory is the factory that creates controllers for different types of workload
type WorkloadControllerFactory struct {
	client         client.Client
	rolloutSpec    *v1alpha1.RolloutPlan
	targetWorkload *unstructured.Unstructured
	sourceWorkload *unstructured.Unstructured
}

// NewWorkloadControllerFactory creates a WorkloadControllerFactory
func NewWorkloadControllerFactory(ctx context.Context, client client.Client, rolloutSpec *v1alpha1.RolloutPlan,
	targetWorkload, sourceWorkload *unstructured.Unstructured) *WorkloadControllerFactory {
	return &WorkloadControllerFactory{
		client:         client,
		rolloutSpec:    rolloutSpec,
		targetWorkload: targetWorkload,
		sourceWorkload: sourceWorkload,
	}
}

// GetController generates the controller depends on the workload type
func (f *WorkloadControllerFactory) GetController(kind schema.GroupVersionKind) WorkloadController {
	cloneSetCtrl := &CloneSetController{
		client:         f.client,
		rolloutSpec:    f.rolloutSpec,
		targetWorkload: f.targetWorkload,
	}

	switch kind.Kind {
	case "CloneSet":
		return cloneSetCtrl

	default:
		return cloneSetCtrl
	}
}
