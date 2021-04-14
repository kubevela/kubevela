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

func TestVerifyRolloutBatchReplicaValue4Deployment(t *testing.T) {
	cases := map[string]struct {
		c             *DeploymentController
		totalReplicas int32
		want          error
	}{
		"BatchSizeMismatchesDeploymentSize": {
			c: &DeploymentController{
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
		"TheSizeOfAllBatchesExceptTheLastOneExceedTotalReplicas": {
			c: &DeploymentController{
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{
						{
							Replicas: intstr.FromInt(1),
						},
						{
							Replicas: intstr.FromInt(3),
						},
						{
							Replicas: intstr.FromInt(2),
						},
					},
				},
			},
			totalReplicas: 3,
			want:          fmt.Errorf("the rollout plan batch size mismatch, total batch size = 4, totalReplicas size = 3"),
		},
		"BatchSizeMatchesDeploymentSize": {
			c: &DeploymentController{
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
