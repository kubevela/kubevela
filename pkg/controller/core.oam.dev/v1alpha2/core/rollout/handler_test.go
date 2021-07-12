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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamstandard "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

func TestHandleTerminated(t *testing.T) {
	testcases := map[string]struct {
		rollout v1beta1.Rollout
		want    bool
	}{
		"succeed": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					SourceRevisionName: "v1",
					TargetRevisionName: "v2",
				},
				Status: v1beta1.RolloutStatus{
					LastSourceRevision:         "v1",
					LastUpgradedTargetRevision: "v2",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RolloutSucceedState,
					},
				},
			},
			want: true,
		},
		"failed": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					SourceRevisionName: "v1",
					TargetRevisionName: "v2",
				},
				Status: v1beta1.RolloutStatus{
					LastSourceRevision:         "v1",
					LastUpgradedTargetRevision: "v2",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RolloutFailedState,
					},
				},
			},
			want: true,
		},
		"restart after succeed": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					SourceRevisionName: "v2",
					TargetRevisionName: "v3",
				},
				Status: v1beta1.RolloutStatus{
					LastSourceRevision:         "v2",
					LastUpgradedTargetRevision: "v1",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RolloutSucceedState,
					},
				},
			},
			want: false,
		},
		"restart after failed": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					SourceRevisionName: "v2",
					TargetRevisionName: "v3",
				},
				Status: v1beta1.RolloutStatus{
					LastSourceRevision:         "v2",
					LastUpgradedTargetRevision: "v1",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RolloutFailedState,
					},
				},
			},
			want: false,
		},
		"still in middle of rollout": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					SourceRevisionName: "v1",
					TargetRevisionName: "v2",
				},
				Status: v1beta1.RolloutStatus{
					LastSourceRevision:         "v1",
					LastUpgradedTargetRevision: "v2",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RollingInBatchesState,
					},
				},
			},
			want: false,
		},
		"last scale have finished": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					TargetRevisionName: "v1",
					RolloutPlan: oamstandard.RolloutPlan{
						TargetSize: pointer.Int32Ptr(2),
					},
				},
				Status: v1beta1.RolloutStatus{
					LastUpgradedTargetRevision: "v1",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState:      oamstandard.RolloutSucceedState,
						RolloutTargetSize: 2,
					},
				},
			},
			want: true,
		},
		"modify targetSize trigger scale operation again": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					TargetRevisionName: "v1",
					RolloutPlan: oamstandard.RolloutPlan{
						TargetSize: pointer.Int32Ptr(4),
					},
				},
				Status: v1beta1.RolloutStatus{
					LastUpgradedTargetRevision: "v1",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState:      oamstandard.RollingInBatchesState,
						RolloutTargetSize: 2,
					},
				},
			},
			want: false,
		},
	}
	for casename, c := range testcases {
		got := checkRollingTerminated(c.rollout)
		if got != c.want {
			t.Errorf("%s result missmatch want:%v got: %v", casename, c.want, got)
		}
	}
}

func Test_isRolloutModified(t *testing.T) {
	tests := map[string]struct {
		rollout v1beta1.Rollout
		want    bool
	}{
		"initial case when no source or target set": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					TargetRevisionName: "target1",
					SourceRevisionName: "source1",
				},
				Status: v1beta1.RolloutStatus{
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RollingInBatchesState,
					},
				},
			},
			want: false,
		},
		"scale no change case": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					TargetRevisionName: "target1",
				},
				Status: v1beta1.RolloutStatus{
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RollingInBatchesState,
					},
					LastUpgradedTargetRevision: "target1",
				},
			},
			want: false,
		},
		"rollout no change case": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					TargetRevisionName: "target1",
					SourceRevisionName: "source1",
				},
				Status: v1beta1.RolloutStatus{
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RollingInBatchesState,
					},
					LastUpgradedTargetRevision: "target1",
					LastSourceRevision:         "source1",
				},
			},
			want: false,
		},
		"scale change case": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					TargetRevisionName: "target2",
				},
				Status: v1beta1.RolloutStatus{
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RollingInBatchesState,
					},
					LastUpgradedTargetRevision: "target1",
				},
			},
			want: true,
		},
		"rollout one change case": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					TargetRevisionName: "target2",
					SourceRevisionName: "source1",
				},
				Status: v1beta1.RolloutStatus{
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RollingInBatchesState,
					},
					LastUpgradedTargetRevision: "target1",
					LastSourceRevision:         "source1",
				},
			},
			want: true,
		},
		"rollout both change case": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					TargetRevisionName: "target2",
					SourceRevisionName: "source2",
				},
				Status: v1beta1.RolloutStatus{
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RollingInBatchesState,
					},
					LastUpgradedTargetRevision: "target1",
					LastSourceRevision:         "source1",
				},
			},
			want: true,
		},
		"deleting both change case": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					TargetRevisionName: "target2",
					SourceRevisionName: "source2",
				},
				Status: v1beta1.RolloutStatus{
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RolloutDeletingState,
					},
					LastUpgradedTargetRevision: "target1",
					LastSourceRevision:         "source1",
				},
			},
			want: false,
		},
		"restart a scale operation": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					TargetRevisionName: "target1",
					RolloutPlan: oamstandard.RolloutPlan{
						TargetSize: pointer.Int32Ptr(1),
					},
				},
				Status: v1beta1.RolloutStatus{
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState:      oamstandard.RolloutSucceedState,
						RolloutTargetSize: 2,
					},
					LastUpgradedTargetRevision: "target1",
				},
			},
			want: true,
		},
		"scale have finished": {
			rollout: v1beta1.Rollout{
				Spec: v1beta1.RolloutSpec{
					TargetRevisionName: "target1",
					RolloutPlan: oamstandard.RolloutPlan{
						TargetSize: pointer.Int32Ptr(2),
					},
				},
				Status: v1beta1.RolloutStatus{
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState:      oamstandard.RolloutSucceedState,
						RolloutTargetSize: 2,
					},
					LastUpgradedTargetRevision: "target1",
				},
			},
			want: false,
		},
	}
	for name, tt := range tests {
		h := handler{}
		t.Run(name, func(t *testing.T) {
			if got := h.isRolloutModified(tt.rollout); got != tt.want {
				t.Errorf("isRolloutModified() = %v, want %v", got, tt.want)
			}
		})
	}
}
