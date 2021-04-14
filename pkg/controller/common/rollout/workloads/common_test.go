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
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

var (
	rolloutPercentSpec = &v1alpha1.RolloutPlan{
		RolloutBatches: []v1alpha1.RolloutBatch{
			{
				Replicas: intstr.FromString("20%"),
			},
			{
				Replicas: intstr.FromString("40%"),
			},
			{
				Replicas: intstr.FromString("30%"),
			},
			{
				Replicas: intstr.FromString("10%"),
			},
		},
	}

	rolloutNumericSpec = &v1alpha1.RolloutPlan{
		RolloutBatches: []v1alpha1.RolloutBatch{
			{
				Replicas: intstr.FromInt(1),
			},
			{
				Replicas: intstr.FromInt(2),
			},
			{
				Replicas: intstr.FromInt(4),
			},
			{
				Replicas: intstr.FromInt(3),
			},
		},
	}

	rolloutMixedSpec = &v1alpha1.RolloutPlan{
		RolloutBatches: []v1alpha1.RolloutBatch{
			{
				Replicas: intstr.FromInt(1),
			},
			{
				Replicas: intstr.FromString("20%"),
			},
			{
				Replicas: intstr.FromString("50%"),
			},
			{
				Replicas: intstr.FromInt(2),
			},
		},
	}

	rolloutRelaxSpec = &v1alpha1.RolloutPlan{
		RolloutBatches: []v1alpha1.RolloutBatch{
			{
				Replicas: intstr.FromString("20%"),
			},
			{
				Replicas: intstr.FromInt(2),
			},
			{
				Replicas: intstr.FromInt(2),
			},
			{
				Replicas: intstr.FromString("50%"),
			},
		},
	}
)

func TestCalculateNewBatchTarget(t *testing.T) {
	// test common rollout
	if got := calculateNewBatchTarget(rolloutMixedSpec, 0, 10, 0); got != 1 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 1)
	}
	if got := calculateNewBatchTarget(rolloutMixedSpec, 0, 10, 1); got != 3 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 3)
	}
	if got := calculateNewBatchTarget(rolloutMixedSpec, 0, 10, 2); got != 8 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 8)
	}
	if got := calculateNewBatchTarget(rolloutMixedSpec, 0, 10, 3); got != 10 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 10)
	}

	// test scale up
	if got := calculateNewBatchTarget(rolloutMixedSpec, 2, 12, 0); got != 3 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 3)
	}
	if got := calculateNewBatchTarget(rolloutMixedSpec, 3, 13, 1); got != 6 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 6)
	}
	if got := calculateNewBatchTarget(rolloutMixedSpec, 4, 14, 2); got != 12 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 12)
	}
	if got := calculateNewBatchTarget(rolloutMixedSpec, 5, 15, 3); got != 15 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 15)
	}

	// test scale down
	if got := calculateNewBatchTarget(rolloutMixedSpec, 10, 0, 0); got != 9 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 9)
	}
	if got := calculateNewBatchTarget(rolloutMixedSpec, 20, 5, 1); got != 16 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 16)
	}
	if got := calculateNewBatchTarget(rolloutMixedSpec, 30, 15, 2); got != 18 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 18)
	}
	if got := calculateNewBatchTarget(rolloutMixedSpec, 40, 10, 3); got != 10 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 10)
	}
}

func TestCalculateNewBatchTargetCornerCases(t *testing.T) {
	// test batch size overflow
	if got := calculateNewBatchTarget(rolloutMixedSpec, 2, 12, 4); got != 12 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 12)
	}
	if got := calculateNewBatchTarget(rolloutMixedSpec, 13, 3, 5); got != 3 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 3)
	}
	// numeric value doesn't match the range
	if got := calculateNewBatchTarget(rolloutNumericSpec, 16, 10, 0); got != 15 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 15)
	}
	if got := calculateNewBatchTarget(rolloutPercentSpec, 10, 10, 2); got != 10 {
		t.Errorf("calculateNewBatchTarget() = %v, want %v", got, 10)
	}
}

func TestVerifyBatchesWithRollout(t *testing.T) {
	if err := verifyBatchesWithRollout(rolloutMixedSpec, 10); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
	if err := verifyBatchesWithRollout(rolloutMixedSpec, 12); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
	if err := verifyBatchesWithRollout(rolloutMixedSpec, 13); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
	if err := verifyBatchesWithRollout(rolloutMixedSpec, 20); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
	if err := verifyBatchesWithRollout(rolloutMixedSpec, 6); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
	// last batch as a percentage always succeeds
	if err := verifyBatchesWithRollout(rolloutRelaxSpec, 10); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
	if err := verifyBatchesWithRollout(rolloutRelaxSpec, 100); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
	if err := verifyBatchesWithRollout(rolloutRelaxSpec, 31); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
	// last can't be zero
	if err := verifyBatchesWithRollout(rolloutRelaxSpec, 6); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
	// test hard number
	if err := verifyBatchesWithRollout(rolloutNumericSpec, 6); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
	if err := verifyBatchesWithRollout(rolloutNumericSpec, 11); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
	if err := verifyBatchesWithRollout(rolloutNumericSpec, 10); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
}

func Test_verifyBatchesWithScale(t *testing.T) {
	if err := verifyBatchesWithScale(rolloutMixedSpec, 10, 0); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
	if err := verifyBatchesWithScale(rolloutMixedSpec, 30, 42); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
	if err := verifyBatchesWithScale(rolloutMixedSpec, 13, 26); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
	if err := verifyBatchesWithScale(rolloutMixedSpec, 13, 20); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
	if err := verifyBatchesWithScale(rolloutMixedSpec, 42, 10); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
	// test hard batch numbers
	if err := verifyBatchesWithScale(rolloutNumericSpec, 22, 32); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
	if err := verifyBatchesWithScale(rolloutNumericSpec, 22, 12); err != nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want nil", err)
	}
	if err := verifyBatchesWithScale(rolloutNumericSpec, 42, 30); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
}
