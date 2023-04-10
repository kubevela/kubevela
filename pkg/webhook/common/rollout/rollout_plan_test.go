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
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

func TestDefaultRolloutPlan_EvenlyDivide(t *testing.T) {
	var numBatch int32 = 5
	rollout := &v1alpha1.RolloutPlan{
		TargetSize: &numBatch,
		NumBatches: &numBatch,
	}
	DefaultRolloutBatches(rollout)

	if len(rollout.RolloutBatches) != int(numBatch) {
		t.Errorf("number of batch %d does not equal to %d ", len(rollout.RolloutBatches), numBatch)
	}
	for i, batch := range rollout.RolloutBatches {
		if batch.Replicas.IntVal != int32(1) {
			t.Errorf("batch %d replica does not equal to 1", i)
		}
	}
}

func TestDefaultRolloutPlan_HasRemanence(t *testing.T) {
	var numBatch int32 = 5
	rollout := &v1alpha1.RolloutPlan{
		TargetSize: pointer.Int32(8),
		NumBatches: &numBatch,
	}
	DefaultRolloutBatches(rollout)

	if len(rollout.RolloutBatches) != int(numBatch) {
		t.Errorf("number of batch %d does not equal to %d ", len(rollout.RolloutBatches), numBatch)
	}
	if rollout.RolloutBatches[0].Replicas.IntValue() != 1 {
		t.Errorf("batch 0's replica %d does not equal to 1", rollout.RolloutBatches[0].Replicas.IntValue())
	}
	if rollout.RolloutBatches[1].Replicas.IntValue() != 1 {
		t.Errorf("batch 1's replica %d does not equal to 1", rollout.RolloutBatches[1].Replicas.IntValue())
	}
	if rollout.RolloutBatches[2].Replicas.IntValue() != 2 {
		t.Errorf("batch 2's replica %d does not equal to 2", rollout.RolloutBatches[2].Replicas.IntValue())
	}
	if rollout.RolloutBatches[3].Replicas.IntValue() != 2 {
		t.Errorf("batch 3's replica %d does not equal to 2", rollout.RolloutBatches[3].Replicas.IntValue())
	}
	if rollout.RolloutBatches[4].Replicas.IntValue() != 2 {
		t.Errorf("batch 4's replica %d does not equal to 2", rollout.RolloutBatches[4].Replicas.IntValue())
	}
}

func TestDefaultRolloutPlan_NotEnough(t *testing.T) {
	var numBatch int32 = 5
	rollout := &v1alpha1.RolloutPlan{
		TargetSize: pointer.Int32(4),
		NumBatches: &numBatch,
	}
	DefaultRolloutBatches(rollout)

	if len(rollout.RolloutBatches) != int(numBatch) {
		t.Errorf("number of batch %d does not equal to %d ", len(rollout.RolloutBatches), numBatch)
	}
	if rollout.RolloutBatches[0].Replicas.IntValue() != 0 {
		t.Errorf("batch 0's replica %d does not equal to 0", rollout.RolloutBatches[0].Replicas.IntValue())
	}
	if rollout.RolloutBatches[1].Replicas.IntValue() != 1 {
		t.Errorf("batch 1's replica %d does not equal to 1", rollout.RolloutBatches[1].Replicas.IntValue())
	}
	if rollout.RolloutBatches[2].Replicas.IntValue() != 1 {
		t.Errorf("batch 2's replica %d does not equal to 1", rollout.RolloutBatches[2].Replicas.IntValue())
	}
	if rollout.RolloutBatches[3].Replicas.IntValue() != 1 {
		t.Errorf("batch 3's replica %d does not equal to 1", rollout.RolloutBatches[3].Replicas.IntValue())
	}
	if rollout.RolloutBatches[4].Replicas.IntValue() != 1 {
		t.Errorf("batch 4's replica %d does not equal to 1", rollout.RolloutBatches[4].Replicas.IntValue())
	}
}

func TestFillRolloutBatches_WithPositiveOriginalSize(t *testing.T) {
	var numBatch int32 = 4
	rollout := &v1alpha1.RolloutPlan{
		RolloutBatches: make([]v1alpha1.RolloutBatch, numBatch),
	}
	FillRolloutBatches(rollout, 9, 4)

	if rollout.RolloutBatches[0].Replicas.IntValue() != 2 {
		t.Errorf("batch 0's replica %d does not equal to 2", rollout.RolloutBatches[0].Replicas.IntValue())
	}
	if rollout.RolloutBatches[1].Replicas.IntValue() != 2 {
		t.Errorf("batch 1's replica %d does not equal to 2", rollout.RolloutBatches[1].Replicas.IntValue())
	}
	if rollout.RolloutBatches[2].Replicas.IntValue() != 2 {
		t.Errorf("batch 2's replica %d does not equal to 2", rollout.RolloutBatches[2].Replicas.IntValue())
	}
	if rollout.RolloutBatches[3].Replicas.IntValue() != 3 {
		t.Errorf("batch 3's replica %d does not equal to 3", rollout.RolloutBatches[3].Replicas.IntValue())
	}
}

func TestValidateIllegalReplicas(t *testing.T) {
	illegalReplica := &v1alpha1.RolloutPlan{
		RolloutBatches: []v1alpha1.RolloutBatch{
			{
				Replicas: intstr.FromString("0.2"),
			},
		},
	}
	if errList := validateRolloutBatches(illegalReplica, field.NewPath("spec")); len(errList) != 1 {
		t.Error("should invalidate illegal replica value")
	}

	illegalReplica = &v1alpha1.RolloutPlan{
		RolloutBatches: []v1alpha1.RolloutBatch{
			{
				Replicas: intstr.FromString("ab"),
			},
		},
	}
	if errList := validateRolloutBatches(illegalReplica, field.NewPath("spec")); len(errList) != 1 {
		t.Error("should invalidate illegal replica value")
	}
	// negative replica case
	negativeReplica := &v1alpha1.RolloutPlan{
		RolloutBatches: []v1alpha1.RolloutBatch{
			{
				Replicas: intstr.FromInt(-1),
			},
		},
	}
	if errList := validateRolloutBatches(negativeReplica, field.NewPath("spec")); len(errList) == 0 {
		t.Error("should invalidate negative replica value")
	}
}
