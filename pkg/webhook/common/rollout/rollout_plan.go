package rollout

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// DefaultRolloutPlan set the default values for a rollout plan
func DefaultRolloutPlan(rollout *v1alpha1.RolloutPlan) {

}

// ValidateCreate validate the rollout plan
func ValidateCreate(rollout *v1alpha1.RolloutPlan) field.ErrorList {
	// 1. The total number of replicas in the batches match the current target resource pod size
	// 2. The TargetSize and NumBatches are mutually exclusive to RolloutBatches
	return nil
}

// ValidateUpdate validate if one can change the rollout plan from the previous psec
func ValidateUpdate(new *v1alpha1.RolloutPlan, prev *v1alpha1.RolloutPlan) field.ErrorList {
	// Only a few fields can change after a rollout plan is set
	return nil
}
