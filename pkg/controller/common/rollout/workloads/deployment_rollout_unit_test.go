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

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

func TestCalculateCurrentSource(t *testing.T) {
	cases := map[string]struct {
		rolloutSpec  *v1alpha1.RolloutPlan
		currentBatch int32
		totalSize    int32
		want         int32
	}{
		"PercentBatch0": {
			rolloutSpec:  rolloutPercentSpec,
			currentBatch: 0,
			totalSize:    10,
			want:         8,
		},
		"PercentBatch1": {
			rolloutSpec:  rolloutPercentSpec,
			currentBatch: 1,
			totalSize:    10,
			want:         4,
		},
		"PercentBatch2": {
			rolloutSpec:  rolloutPercentSpec,
			currentBatch: 2,
			totalSize:    100,
			want:         10,
		},
		"PercentBatch3": {
			rolloutSpec:  rolloutPercentSpec,
			currentBatch: 3,
			totalSize:    1000,
			want:         0,
		},
		"MixedBatch0": {
			rolloutSpec:  rolloutMixedSpec,
			currentBatch: 0,
			totalSize:    100,
			want:         99,
		},
		"MixedBatch1": {
			rolloutSpec:  rolloutMixedSpec,
			currentBatch: 1,
			totalSize:    100,
			want:         79,
		},
		"MixedBatch2": {
			rolloutSpec:  rolloutMixedSpec,
			currentBatch: 2,
			totalSize:    15,
			want:         3,
		},
		"RelaxedBatch3": {
			rolloutSpec:  rolloutRelaxSpec,
			currentBatch: 3,
			totalSize:    15,
			want:         0,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			controller := DeploymentRolloutController{
				deploymentController: deploymentController{
					workloadController: workloadController{
						rolloutSpec: tc.rolloutSpec,
						rolloutStatus: &v1alpha1.RolloutStatus{
							CurrentBatch: tc.currentBatch,
						},
					},
				},
			}
			ct := controller.calculateCurrentSource(tc.totalSize)
			if tc.want-ct != 0 {
				t.Errorf("\n%s\ncalculateCurrentTarget(...): -want count, +got count:\n%d, %d", name, tc.want, ct)
			}
		})
	}
}
