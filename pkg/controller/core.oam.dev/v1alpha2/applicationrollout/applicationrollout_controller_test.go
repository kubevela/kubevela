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

package applicationrollout

import (
	"testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

func Test_isRolloutModified(t *testing.T) {
	tests := map[string]struct {
		appRollout v1beta1.AppRollout
		want       bool
	}{
		"initial case when no source or target set": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target1",
					SourceAppRevisionName: "source1",
				},
				Status: v1beta1.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
				},
			},
			want: false,
		},
		"scale no change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target1",
				},
				Status: v1beta1.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
					LastUpgradedTargetAppRevision: "target1",
				},
			},
			want: false,
		},
		"rollout no change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target1",
					SourceAppRevisionName: "source1",
				},
				Status: v1beta1.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
					LastUpgradedTargetAppRevision: "target1",
					LastSourceAppRevision:         "source1",
				},
			},
			want: false,
		},
		"scale change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target2",
				},
				Status: v1beta1.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
					LastUpgradedTargetAppRevision: "target1",
				},
			},
			want: true,
		},
		"rollout one change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target2",
					SourceAppRevisionName: "source1",
				},
				Status: v1beta1.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
					LastUpgradedTargetAppRevision: "target1",
					LastSourceAppRevision:         "source1",
				},
			},
			want: true,
		},
		"rollout both change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target2",
					SourceAppRevisionName: "source2",
				},
				Status: v1beta1.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
					LastUpgradedTargetAppRevision: "target1",
					LastSourceAppRevision:         "source1",
				},
			},
			want: true,
		},
		"deleting both change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target2",
					SourceAppRevisionName: "source2",
				},
				Status: v1beta1.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RolloutDeletingState,
					},
					LastUpgradedTargetAppRevision: "target1",
					LastSourceAppRevision:         "source1",
				},
			},
			want: false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isRolloutModified(tt.appRollout); got != tt.want {
				t.Errorf("isRolloutModified() = %v, want %v", got, tt.want)
			}
		})
	}
}
