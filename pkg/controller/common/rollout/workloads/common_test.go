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

	"k8s.io/utils/pointer"

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

	rolloutOverFlowSpec = &v1alpha1.RolloutPlan{
		RolloutBatches: []v1alpha1.RolloutBatch{
			{
				Replicas: intstr.FromString("140%"),
			},
			{
				Replicas: intstr.FromInt(1),
			},
			{
				Replicas: intstr.FromString("40%"),
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
	// test current batch overflow
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

func TestVerifyBatchesWithRolloutNormal(t *testing.T) {
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
}

func TestVerifyBatchesWithRolloutRelaxed(t *testing.T) {
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
	// overflow always fail
	if err := verifyBatchesWithRollout(rolloutOverFlowSpec, 10); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
	if err := verifyBatchesWithRollout(rolloutOverFlowSpec, 100); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
	if err := verifyBatchesWithRollout(rolloutOverFlowSpec, 1); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
	if err := verifyBatchesWithRollout(rolloutOverFlowSpec, 0); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
}

func TestVerifyEmptyRolloutBatches(t *testing.T) {
	plan := &v1alpha1.RolloutPlan{
		TargetSize: pointer.Int32(2),
	}
	if err := verifyBatchesWithRollout(plan, 3); err == nil {
		t.Errorf("verifyBatchesWithRollout() = %v, want error", nil)
	}
}

func TestVerifyBatchesWithRolloutNumeric(t *testing.T) {
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

func Test_VerifyBatchesWithScalePassCases(t *testing.T) {
	tests := map[string]struct {
		rolloutSpec  *v1alpha1.RolloutPlan
		originalSize int
		targetSize   int
	}{
		"percent equal case 1": {
			rolloutSpec:  rolloutPercentSpec,
			originalSize: 12,
			targetSize:   12,
		},
		"percent equal case 2": {
			rolloutSpec:  rolloutPercentSpec,
			originalSize: 0,
			targetSize:   0,
		},
		"percent increase case": {
			rolloutSpec:  rolloutPercentSpec,
			originalSize: 12,
			targetSize:   22,
		},
		"percent decrease case 1": {
			rolloutSpec:  rolloutPercentSpec,
			originalSize: 27,
			targetSize:   7,
		},
		"percent decrease case 2": {
			rolloutSpec:  rolloutPercentSpec,
			originalSize: 27,
			targetSize:   0,
		},
		"relax increase 1": {
			rolloutSpec:  rolloutRelaxSpec,
			originalSize: 12,
			targetSize:   32,
		},
		"relax increase 2": {
			rolloutSpec:  rolloutRelaxSpec,
			originalSize: 12,
			targetSize:   22,
		},
		"mix increase 1": {
			rolloutSpec:  rolloutMixedSpec,
			originalSize: 13,
			targetSize:   26,
		},
		"mix increase 2": {
			rolloutSpec:  rolloutMixedSpec,
			originalSize: 30,
			targetSize:   42,
		},
		"mix decrease 1": {
			rolloutSpec:  rolloutMixedSpec,
			originalSize: 32,
			targetSize:   20,
		},
		"mix decrease 2": {
			rolloutSpec:  rolloutMixedSpec,
			originalSize: 12,
			targetSize:   0,
		},
		"numeric increase": {
			rolloutSpec:  rolloutNumericSpec,
			originalSize: 16,
			targetSize:   26,
		},
		"numeric decrease": {
			rolloutSpec:  rolloutNumericSpec,
			originalSize: 13,
			targetSize:   3,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if err := verifyBatchesWithScale(tt.rolloutSpec, tt.originalSize, tt.targetSize); err != nil {
				t.Errorf("verifyBatchesWithScale() error = %v, want pass", err)
			}
		})
	}
}

func Test_VerifyBatchesWithScaleFailCases(t *testing.T) {
	tests := map[string]struct {
		rolloutSpec  *v1alpha1.RolloutPlan
		originalSize int
		targetSize   int
	}{
		"total percent more than 100 increase": {
			rolloutSpec:  rolloutOverFlowSpec,
			originalSize: 12,
			targetSize:   115,
		},
		"total percent more than 100 decrease": {
			rolloutSpec:  rolloutOverFlowSpec,
			originalSize: 312,
			targetSize:   15,
		},
		"total percent more than 100 equal": {
			rolloutSpec:  rolloutOverFlowSpec,
			originalSize: 12,
			targetSize:   12,
		},
		"percent increase case less than batch number": {
			rolloutSpec:  rolloutPercentSpec,
			originalSize: 12,
			targetSize:   15,
		},
		"percent decrease case": {
			rolloutSpec:  rolloutPercentSpec,
			originalSize: 10,
			targetSize:   7,
		},
		"relax increase too little": {
			rolloutSpec:  rolloutRelaxSpec,
			originalSize: 12,
			targetSize:   17,
		},
		"mix increase": {
			rolloutSpec:  rolloutMixedSpec,
			originalSize: 13,
			targetSize:   20,
		},
		"mix decrease": {
			rolloutSpec:  rolloutMixedSpec,
			originalSize: 42,
			targetSize:   33,
		},
		"numeric increase": {
			rolloutSpec:  rolloutNumericSpec,
			originalSize: 16,
			targetSize:   32,
		},
		"numeric decrease 1": {
			rolloutSpec:  rolloutNumericSpec,
			originalSize: 13,
			targetSize:   10,
		},
		"numeric decrease 2": {
			rolloutSpec:  rolloutNumericSpec,
			originalSize: 16,
			targetSize:   10,
		},
		"empty rollingBatches": {
			rolloutSpec:  &v1alpha1.RolloutPlan{},
			targetSize:   3,
			originalSize: 2,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if err := verifyBatchesWithScale(tt.rolloutSpec, tt.originalSize, tt.targetSize); err == nil {
				t.Errorf("verifyBatchesWithScale passed, want fail")
			}
		})
	}
}
