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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamstandard "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
)

func TestDisableControllerOwner(t *testing.T) {
	w := &unstructured.Unstructured{}
	owners := []metav1.OwnerReference{
		{Name: "test-1", Controller: pointer.BoolPtr(false)},
		{Name: "test-2", Controller: pointer.BoolPtr(true)},
	}
	w.SetOwnerReferences(owners)
	disableControllerOwner(w)
	assert.Equal(t, 2, len(w.GetOwnerReferences()))
	for _, reference := range w.GetOwnerReferences() {
		assert.Equal(t, false, *reference.Controller)
	}
}

func TestEnableControllerOwner(t *testing.T) {
	w := &unstructured.Unstructured{}
	owners := []metav1.OwnerReference{
		{Name: "test-1", Controller: pointer.BoolPtr(false), Kind: v1beta1.ResourceTrackerKind},
		{Name: "test-2", Controller: pointer.BoolPtr(false), Kind: v1alpha2.ApplicationKind},
	}
	w.SetOwnerReferences(owners)
	enableControllerOwner(w)
	assert.Equal(t, 2, len(w.GetOwnerReferences()))
	for _, reference := range w.GetOwnerReferences() {
		if reference.Kind == v1beta1.ResourceTrackerKind {
			assert.Equal(t, true, *reference.Controller)
		} else {
			assert.Equal(t, false, *reference.Controller)
		}
	}
}

func TestHandleTerminated(t *testing.T) {
	testcases := map[string]struct {
		rollout v1beta1.AppRollout
		want    bool
	}{
		"succeed": {
			rollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					SourceAppRevisionName: "v1",
					TargetAppRevisionName: "v2",
				},
				Status: common.AppRolloutStatus{
					LastSourceAppRevision:         "v1",
					LastUpgradedTargetAppRevision: "v2",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RolloutSucceedState,
					},
				},
			},
			want: true,
		},
		"failed": {
			rollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					SourceAppRevisionName: "v1",
					TargetAppRevisionName: "v2",
				},
				Status: common.AppRolloutStatus{
					LastSourceAppRevision:         "v1",
					LastUpgradedTargetAppRevision: "v2",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RolloutFailedState,
					},
				},
			},
			want: true,
		},
		"restart after succeed": {
			rollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					SourceAppRevisionName: "v2",
					TargetAppRevisionName: "v3",
				},
				Status: common.AppRolloutStatus{
					LastSourceAppRevision:         "v2",
					LastUpgradedTargetAppRevision: "v1",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RolloutSucceedState,
					},
				},
			},
			want: false,
		},
		"restart after failed": {
			rollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					SourceAppRevisionName: "v2",
					TargetAppRevisionName: "v3",
				},
				Status: common.AppRolloutStatus{
					LastSourceAppRevision:         "v2",
					LastUpgradedTargetAppRevision: "v1",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RolloutFailedState,
					},
				},
			},
			want: false,
		},
		"still in middle of rollout": {
			rollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					SourceAppRevisionName: "v1",
					TargetAppRevisionName: "v2",
				},
				Status: common.AppRolloutStatus{
					LastSourceAppRevision:         "v1",
					LastUpgradedTargetAppRevision: "v2",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState: oamstandard.RollingInBatchesState,
					},
				},
			},
			want: false,
		},
		"last scale have finished": {
			rollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "v1",
					RolloutPlan: oamstandard.RolloutPlan{
						TargetSize: pointer.Int32Ptr(2),
					},
				},
				Status: common.AppRolloutStatus{
					LastUpgradedTargetAppRevision: "v1",
					RolloutStatus: oamstandard.RolloutStatus{
						RollingState:      oamstandard.RolloutSucceedState,
						RolloutTargetSize: 2,
					},
				},
			},
			want: true,
		},
		"modify targetSize trigger scale operation again": {
			rollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "v1",
					RolloutPlan: oamstandard.RolloutPlan{
						TargetSize: pointer.Int32Ptr(4),
					},
				},
				Status: common.AppRolloutStatus{
					LastUpgradedTargetAppRevision: "v1",
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
		got := handleRollingTerminated(c.rollout)
		if got != c.want {
			t.Errorf("%s result missmatch want:%v got: %v", casename, c.want, got)
		}
	}
}

func TestGetWorkloadReplicasPath(t *testing.T) {
	deploy := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appsv1",
			Kind:       "Deployment",
		},
	}
	u, err := util.Object2Unstructured(deploy)
	if err != nil {
		t.Errorf("deployment shounld't meet an error %w", err)
	}
	pathStr, err := GetWorkloadReplicasPath(*u)
	if err != nil {
		t.Errorf("deployment should handle deployment")
	}
	if pathStr != "spec.replicas" {
		t.Errorf("deployPath error got %s", pathStr)
	}
	ds := appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appsv1",
			Kind:       "DaemonSet",
		},
	}
	u, err = util.Object2Unstructured(ds)
	if err != nil {
		t.Errorf("ds shounld't meet an error %w", err)
	}
	_, err = GetWorkloadReplicasPath(*u)
	if err == nil {
		t.Errorf("daemonset shouldn't support")
	}
}
