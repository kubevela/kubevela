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

package workloads

import (
	"fmt"

	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// verifyBatchesWithRollout verifies that the the sum of all the batch replicas is valid given the total replica
// each batch replica can be absolute or a percentage
func verifyBatchesWithRollout(rolloutSpec *v1alpha1.RolloutPlan, totalReplicas int32) error {
	// if not set, the sum of all the batch sizes minus the last batch cannot be more than the totalReplicas
	totalRollout := 0
	for i := 0; i < len(rolloutSpec.RolloutBatches)-1; i++ {
		rb := rolloutSpec.RolloutBatches[i]
		batchSize, _ := intstr.GetValueFromIntOrPercent(&rb.Replicas, int(totalReplicas), true)
		totalRollout += batchSize
	}
	if totalRollout >= int(totalReplicas) {
		return fmt.Errorf("the rollout plan batch size mismatch, total batch size = %d, totalReplicas size = %d",
			totalRollout, totalReplicas)
	}

	// include the last batch if it has an int value
	// we ignore the last batch percentage since it is very likely to cause rounding errors
	lastBatch := rolloutSpec.RolloutBatches[len(rolloutSpec.RolloutBatches)-1]
	if lastBatch.Replicas.Type == intstr.Int {
		totalRollout += int(lastBatch.Replicas.IntVal)
		// now that they should be the same
		if totalRollout != int(totalReplicas) {
			return fmt.Errorf("the rollout plan batch size mismatch, total batch size = %d, totalReplicas size = %d",
				totalRollout, totalReplicas)
		}
	}
	return nil
}

// verifyBatchesWithScale verifies that executing batches finally reach the target size starting from original size
func verifyBatchesWithScale(rolloutSpec *v1alpha1.RolloutPlan, originalSize, targetSize int) error {
	totalRollout := originalSize
	for i := 0; i < len(rolloutSpec.RolloutBatches)-1; i++ {
		rb := rolloutSpec.RolloutBatches[i]
		if targetSize > originalSize {
			batchSize, _ := intstr.GetValueFromIntOrPercent(&rb.Replicas, targetSize-originalSize, true)
			totalRollout += batchSize
		} else {
			batchSize, _ := intstr.GetValueFromIntOrPercent(&rb.Replicas, originalSize-targetSize, true)
			totalRollout -= batchSize
		}
	}
	//nolint ifElseChain
	if targetSize > originalSize {
		if totalRollout >= targetSize {
			return fmt.Errorf("the rollout plan increased too much, total batch size = %d, targetSize size = %d",
				totalRollout, targetSize)
		}
	} else if targetSize < originalSize {
		if totalRollout <= targetSize {
			return fmt.Errorf("the rollout plan reduced too much, total batch size = %d, targetSize size = %d",
				totalRollout, targetSize)
		}
	} else if totalRollout != targetSize {
		return fmt.Errorf("the rollout plan changed on no-op scale, total batch size = %d, targetSize size = %d",
			totalRollout, targetSize)
	}
	// include the last batch if it has an int value
	// we ignore the last batch percentage since it is very likely to cause rounding errors
	lastBatch := rolloutSpec.RolloutBatches[len(rolloutSpec.RolloutBatches)-1]
	if lastBatch.Replicas.Type == intstr.Int {
		if targetSize > originalSize {
			totalRollout += int(lastBatch.Replicas.IntVal)
		} else {
			totalRollout -= int(lastBatch.Replicas.IntVal)
		}
		// now that they should be the same
		if totalRollout != targetSize {
			return fmt.Errorf("the rollout plan batch size mismatch, total batch size = %d, targetSize size = %d",
				totalRollout, targetSize)
		}
	}
	return nil
}

func calculateNewBatchTarget(rolloutSpec *v1alpha1.RolloutPlan, originalSize, targetSize, currentBatch int) int {
	newPodTarget := originalSize
	if currentBatch == len(rolloutSpec.RolloutBatches)-1 {
		newPodTarget = targetSize
		// special handle the last batch, we ignore the rest of the batch in case there are rounding errors
		klog.InfoS("use the target size as the total pod target for the last rolling batch",
			"current batch", currentBatch, "new  pod target", newPodTarget)
		return newPodTarget
	}
	for i := 0; i <= currentBatch && i < len(rolloutSpec.RolloutBatches); i++ {
		if targetSize > originalSize {
			batchSize, _ := intstr.GetValueFromIntOrPercent(&rolloutSpec.RolloutBatches[i].Replicas, targetSize-originalSize,
				true)
			newPodTarget += batchSize
		} else {
			batchSize, _ := intstr.GetValueFromIntOrPercent(&rolloutSpec.RolloutBatches[i].Replicas, originalSize-targetSize,
				true)
			newPodTarget -= batchSize
		}
	}
	klog.InfoS("calculated the number of new pod size", "current batch", currentBatch,
		"new pod target", newPodTarget)
	return newPodTarget
}

func getDeployReplicaSize(deploy *apps.Deployment) int32 {
	// replicas default is 1
	if deploy.Spec.Replicas != nil {
		return *deploy.Spec.Replicas
	}
	return 1
}
