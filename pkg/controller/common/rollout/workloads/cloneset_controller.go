package workloads

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// CloneSetController is responsible for handle Cloneset type of workloads
type CloneSetController struct {
	client         client.Client
	rolloutSpec    *v1alpha1.RolloutPlan
	targetWorkload *unstructured.Unstructured
}

// Initialize first verify that the cloneset status is compatible with the rollout spec
// it then set the cloneset partition the same as the replicas (no new pod) and add an annotation
func (c *CloneSetController) Initialize() (int32, error) {
	return 0, nil
}

// RolloutPods calculates the number of pods we can upgrade once according to the rollout spec
// and then set the partition accordingly
func (c *CloneSetController) RolloutPods() (int32, error) {
	return 0, nil
}

// Finalize makes sure the Cloneset is all upgraded and
func (c *CloneSetController) Finalize() error {
	return nil
}
