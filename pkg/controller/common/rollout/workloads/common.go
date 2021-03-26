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

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// VerifySumOfBatchSizes verifies that the the sum of all the batch replicas is valid given the total replica
// each batch replica can be absolute or a percentage
func VerifySumOfBatchSizes(rolloutSpec *v1alpha1.RolloutPlan, totalReplicas int32) error {
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
