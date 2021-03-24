package workloads

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"

	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
)

var csScaleHandler cloneSetHandler = &cloneSetScaleHandler{}

// cloneSetScaleHandler is responsible for CloneSet scale
type cloneSetScaleHandler struct {
	*CloneSetController
}

// NewCloneSetScaleController creates CloneSet scale controller
func NewCloneSetScaleController(client client.Client, recorder event.Recorder, parentController oam.Object, rolloutSpec *v1alpha1.RolloutPlan, rolloutStatus *v1alpha1.RolloutStatus, workloadName types.NamespacedName) WorkloadController {
	return &CloneSetController{
		client:                 client,
		recorder:               recorder,
		parentController:       parentController,
		rolloutSpec:            rolloutSpec,
		rolloutStatus:          rolloutStatus,
		workloadNamespacedName: workloadName,
		handler:                csScaleHandler,
	}
}

func (c *cloneSetScaleHandler) verifySpec(rolloutSpec *v1alpha1.RolloutPlan, cloneSet *kruise.CloneSet) error {
	return nil
}

func (c *cloneSetScaleHandler) initialize(ctx context.Context, cloneSet *kruise.CloneSet) error {
	return nil
}

// RolloutOneBatch update CloneSet spec replicas directly
func (c *cloneSetScaleHandler) rolloutOneBatchPods(ctx context.Context, cloneSet *kruise.CloneSet, newPodTargets int) error {
	targetReplicas := int32(newPodTargets)
	cloneSet.Spec.Replicas = &targetReplicas
	return nil
}
