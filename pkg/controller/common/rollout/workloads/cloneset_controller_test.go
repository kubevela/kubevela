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
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

func TestVerifyRolloutBatchReplicaValue4Cloneset(t *testing.T) {
	// Compared to `deployment_controller_test.go`, there is one case less as common is already 100% covered, so only an
	// error and nil error for `err := VerifySumOfBatchSizes(c.rolloutSpec, totalReplicas)` is enough.
	var int2 int32 = 2
	cases := map[string]struct {
		c             *CloneSetController
		totalReplicas int32
		want          error
	}{
		"ClonsetTargetSizeIsNotAvaialbe": {
			c: &CloneSetController{
				rolloutSpec: &v1alpha1.RolloutPlan{
					TargetSize: &int2,
					RolloutBatches: []v1alpha1.RolloutBatch{{
						Replicas: intstr.FromInt(1),
					},
					},
				},
			},
			totalReplicas: 3,
			want:          fmt.Errorf("the rollout plan is attempting to scale the cloneset, target = 2, cloneset size = 3"),
		},
		"BatchSizeMismatchesClonesetSize": {
			c: &CloneSetController{
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{{
						Replicas: intstr.FromInt(1),
					},
					},
				},
			},
			totalReplicas: 3,
			want:          fmt.Errorf("the rollout plan batch size mismatch, total batch size = 1, totalReplicas size = 3"),
		},
		"BatchSizeMatchesClonesetSize": {
			c: &CloneSetController{
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{
						{
							Replicas: intstr.FromInt(1),
						},
						{
							Replicas: intstr.FromInt(2),
						},
					},
				},
			},
			totalReplicas: 3,
			want:          nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.c.verifyRolloutBatchReplicaValue(tc.totalReplicas)
			if diff := cmp.Diff(tc.want, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nverifyRolloutBatchReplicaValue(...): -want error, +got error:\n%s", name, diff)
			}
		})
	}
}

//func TestVerifySpec(t *testing.T) {
//	useExistCluster := false
//	testEnv = &envtest.Environment{
//		UseExistingCluster: &useExistCluster,
//	}
//	cfg, err := testEnv.Start()
//	assert.NoError(t, err)
//
//	err = oamCore.AddToScheme(scheme.Scheme)
//	assert.NoError(t, err)
//
//	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
//	assert.NoError(t, err)
//
//	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
//		Scheme:             scheme.Scheme,
//		MetricsBindAddress: "0",
//		Port:               48081,
//	})
//	assert.NoError(t, err)
//
//
//	var (
//		namespace  = "rollout-ns"
//		name       = "rollout1"
//		appRollout = v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
//		//namespacedName = client.ObjectKey{Name: name, Namespace: namespace}
//		ctx = context.TODO()
//
//		ns = corev1.Namespace{
//			ObjectMeta: metav1.ObjectMeta{
//				Name: namespace,
//			},
//		}
//	)
//	assert.NoError(t, k8sClient.Create(ctx, &ns))
//
//	type want struct {
//		consistent bool
//		err        error
//	}
//	cases := map[string]struct {
//		c    *CloneSetController
//		want want
//	}{
//		"CouldNotFetchClonesetWorkload": {
//			c: &CloneSetController{
//				client: k8sClient,
//				rolloutSpec: &v1alpha1.RolloutPlan{
//					RolloutBatches: []v1alpha1.RolloutBatch{{
//						Replicas: intstr.FromInt(1),
//					},
//					},
//				},
//				rolloutStatus:    &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState},
//				parentController: &appRollout,
//				recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
//					WithAnnotations("controller", "AppRollout"),
//			},
//			want: want{
//				consistent: false,
//				err:        nil,
//			},
//		},
//		"the source deployment is still being reconciled, need to be paused": {
//
//		},
//	}
//
//	for name, tc := range cases {
//		t.Run(name, func(t *testing.T) {
//			consistent, err := tc.c.VerifySpec(ctx)
//			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
//				t.Errorf("\n%s\nVerifySpec(...): -want error, +got error:\n%s", name, diff)
//			}
//			if diff := cmp.Diff(tc.want.consistent, consistent); diff != "" {
//				t.Errorf("\n%s\nVerifySpec(...): -want, +got:\n%s", name, diff)
//			}
//		})
//	}
//}
