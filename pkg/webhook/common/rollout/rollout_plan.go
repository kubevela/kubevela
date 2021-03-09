package rollout

import (
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// DefaultRolloutPlan set the default values for a rollout plan
// This is called by the mutation webhooks and before the validators
func DefaultRolloutPlan(rollout *v1alpha1.RolloutPlan) {
	if rollout.TargetSize != nil && rollout.NumBatches != nil && rollout.RolloutBatches == nil {
		// create the rollout batch based on the total size and num batches if it's not set
		// leave it for the validator to validate more if they are both set
		numBatches := int(*rollout.NumBatches)
		totalSize := int(*rollout.TargetSize)
		// create the batch array
		rollout.RolloutBatches = make([]v1alpha1.RolloutBatch, int(*rollout.NumBatches))
		avg := intstr.FromInt(totalSize / numBatches)
		total := 0
		for i := 0; i < numBatches-1; i++ {
			rollout.RolloutBatches[i].Replicas = avg
			total += avg.IntValue()
		}
		// fill out the last batch
		rollout.RolloutBatches[numBatches-1].Replicas = intstr.FromInt(totalSize - total)
	}
}

// ValidateCreate validate the rollout plan
func ValidateCreate(rollout *v1alpha1.RolloutPlan, rootPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	// TODO: The total number of num in the batches match the current target resource pod size

	// the rollout batch partition is either automatic or positive
	if rollout.BatchPartition != nil && *rollout.BatchPartition < 0 {
		allErrs = append(allErrs, field.Invalid(rootPath.Child("batchPartition"), rollout.BatchPartition,
			"the rollout plan has to be positive"))
	}

	// NumBatches has to be the size of RolloutBatches
	if rollout.NumBatches != nil && len(rollout.RolloutBatches) != int(*rollout.NumBatches) {
		allErrs = append(allErrs, field.Invalid(rootPath.Child("numBatches"), rollout.NumBatches,
			"the num batches does not match the rollout batch size"))
	}

	// validate the webhooks
	allErrs = append(allErrs, validateWebhook(rollout, rootPath)...)

	return allErrs
}

func validateWebhook(rollout *v1alpha1.RolloutPlan, rootPath *field.Path) (allErrs field.ErrorList) {
	// The webhooks in the rollout plan can only be initialize or finalize webhooks
	if rollout.RolloutWebhooks != nil {
		webhookPath := rootPath.Child("rolloutWebhooks")
		for i, rw := range rollout.RolloutWebhooks {
			if rw.Type != v1alpha1.InitializeRolloutHook && rw.Type != v1alpha1.FinalizeRolloutHook {
				allErrs = append(allErrs, field.Invalid(webhookPath.Index(i),
					rw.Type, "the rollout webhook type can only be initialize or finalize webhook"))
			}
			// TODO: check the URL/name uniqueness?
		}
	}

	// The webhooks in the rollout batch can only be pre or post batch types
	if rollout.RolloutBatches != nil {
		batchesPath := rootPath.Child("rolloutBatches")
		for i, rb := range rollout.RolloutBatches {
			rolloutBatchPath := batchesPath.Index(i)
			for j, brw := range rb.BatchRolloutWebhooks {
				if brw.Type != v1alpha1.PostBatchRolloutHook && brw.Type != v1alpha1.PreBatchRolloutHook {
					allErrs = append(allErrs, field.Invalid(rolloutBatchPath.Child("batchRolloutWebhooks").Index(j),
						brw.Type, "the batch webhook type can only be pre or post batch webhook"))
				}
				// TODO: check the URL/name uniqueness?
			}
		}
	}
	return allErrs
}

// ValidateUpdate validate if one can change the rollout plan from the previous psec
func ValidateUpdate(new *v1alpha1.RolloutPlan, prev *v1alpha1.RolloutPlan, rootPath *field.Path) field.ErrorList {
	// TODO: Enforce that only a few fields can change after a rollout plan is set
	var allErrs field.ErrorList

	return allErrs
}
