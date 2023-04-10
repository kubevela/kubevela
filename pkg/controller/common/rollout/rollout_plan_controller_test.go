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

	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

func Test_TryMovingToNextBatch(t *testing.T) {
	tests := map[string]struct {
		r                     Controller
		rolloutSpec           *v1alpha1.RolloutPlan
		rolloutStatus         *v1alpha1.RolloutStatus
		wantNextBatch         int32
		wantBatchRollingState v1alpha1.BatchRollingState
	}{
		"stay at the same batch": {
			rolloutSpec: &v1alpha1.RolloutPlan{
				BatchPartition: pointer.Int32(3),
			},
			rolloutStatus: &v1alpha1.RolloutStatus{
				CurrentBatch:      2,
				RollingState:      v1alpha1.RollingInBatchesState,
				BatchRollingState: v1alpha1.BatchReadyState,
			},
			wantNextBatch:         3,
			wantBatchRollingState: v1alpha1.BatchInitializingState,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := &Controller{
				rolloutSpec:   tt.rolloutSpec,
				rolloutStatus: tt.rolloutStatus,
			}
			r.tryMovingToNextBatch()
			if r.rolloutStatus.CurrentBatch != tt.wantNextBatch {
				t.Errorf("\n%s\n batch miss match: want batch `%d`, got batch:`%d`\n", name,
					tt.wantNextBatch, r.rolloutStatus.CurrentBatch)
			}
			if r.rolloutStatus.BatchRollingState != tt.wantBatchRollingState {
				t.Errorf("\n%s\nstate miss match: want state `%s`, got state:`%s`\n", name,
					tt.wantBatchRollingState, r.rolloutStatus.BatchRollingState)
			}
		})
	}
}
