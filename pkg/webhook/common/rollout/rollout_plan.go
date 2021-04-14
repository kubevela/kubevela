/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rollout

import (
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// DefaultRolloutBatches set the default values for a rollout batches
// This is called by the mutation webhooks and before the validators
func DefaultRolloutBatches(rollout *v1alpha1.RolloutPlan) {
	if rollout.TargetSize != nil && rollout.NumBatches != nil && rollout.RolloutBatches == nil {
		// create the rollout batch based on the total size and num batches if it's not set
		// leave it for the validator to validate more if they are both set
		numBatches := int(*rollout.NumBatches)
		// create the batch array
		rollout.RolloutBatches = make([]v1alpha1.RolloutBatch, numBatches)
		FillRolloutBatches(rollout, int(*rollout.TargetSize), numBatches)
		for i, batch := range rollout.RolloutBatches {
			klog.InfoS("mutation webhook assigns rollout plan", "batch", i, "replica",
				batch.Replicas.IntValue())
		}
	}
}

// DefaultRolloutPlan set the default values for a rollout plan
func DefaultRolloutPlan(rollout *v1alpha1.RolloutPlan) {
	if len(rollout.RolloutStrategy) == 0 {
		rollout.RolloutStrategy = v1alpha1.IncreaseFirstRolloutStrategyType
	}
}

// FillRolloutBatches fills the replicas in each batch depends on the total size and number of batches
func FillRolloutBatches(rollout *v1alpha1.RolloutPlan, totalSize int, numBatches int) {
	total := totalSize
	for total > 0 {
		for i := numBatches - 1; i >= 0 && total > 0; i-- {
			replica := rollout.RolloutBatches[i].Replicas.IntValue() + 1
			rollout.RolloutBatches[i].Replicas = intstr.FromInt(replica)
			total--
		}
	}
}

// ValidateCreate validate the rollout plan
func ValidateCreate(client client.Client, rollout *v1alpha1.RolloutPlan, rootPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if rollout.RolloutBatches == nil {
		allErrs = append(allErrs, field.Required(rootPath.Child("rolloutBatches"), "the rollout has to have batches"))
	}

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

	if rollout.RolloutStrategy != v1alpha1.IncreaseFirstRolloutStrategyType &&
		rollout.RolloutStrategy != v1alpha1.DecreaseFirstRolloutStrategyType {
		allErrs = append(allErrs, field.Invalid(rootPath.Child("rolloutStrategy"),
			rollout.RolloutStrategy, "the rolloutStrategy can only be IncreaseFirst or DecreaseFirst"))
	}

	// validate the webhooks
	allErrs = append(allErrs, validateWebhook(rollout, rootPath)...)

	// validate the rollout batches
	allErrs = append(allErrs, validateRolloutBatches(rollout, rootPath)...)

	// TODO: The total number of num in the batches match the current target resource pod size
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
			if rw.Method != http.MethodPost && rw.Method != http.MethodGet && rw.Method != http.MethodPut {
				allErrs = append(allErrs, field.Invalid(webhookPath.Index(i),
					rw.Method, "the rollout webhook method can only be Get/PUT/POST"))
			}
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

func validateRolloutBatches(rollout *v1alpha1.RolloutPlan, rootPath *field.Path) (allErrs field.ErrorList) {
	if rollout.RolloutBatches != nil {
		batchesPath := rootPath.Child("rolloutBatches")
		for i, rb := range rollout.RolloutBatches {
			rolloutBatchPath := batchesPath.Index(i)
			// validate rb.Replicas with a common total number
			value, err := intstr.GetValueFromIntOrPercent(&rb.Replicas, 100, true)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(rolloutBatchPath.Child("replicas"),
					rb.Replicas, fmt.Sprintf("invalid replica value, err = %s", err)))
			} else if value < 0 {
				allErrs = append(allErrs, field.Invalid(rolloutBatchPath.Child("replicas"),
					value, "negative replica value"))
			}
		}
	}
	return allErrs
}

// ValidateUpdate validate if one can change the rollout plan from the previous psec
func ValidateUpdate(client client.Client, new *v1alpha1.RolloutPlan, prev *v1alpha1.RolloutPlan,
	rootPath *field.Path) field.ErrorList {
	// TODO: Enforce that only a few fields can change after a rollout plan is set
	var allErrs field.ErrorList

	return allErrs
}
