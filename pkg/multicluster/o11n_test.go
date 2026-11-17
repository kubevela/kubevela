/*
Copyright 2020-2022 The KubeVela Authors.

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

package multicluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func labelledNode(name string, labels map[string]string) *corev1.Node {
	node := FakeNode(name, "2", "1024")
	node.Labels = labels
	return node
}

func TestGetClusterInfo(t *testing.T) {
	testCases := map[string]struct {
		nodes                []client.Object
		expectedMasterNumber int
		expectedWorkerNumber int
	}{
		"modern cluster, only control-plane label (k8s >= 1.24)": {
			nodes: []client.Object{
				labelledNode("cp-1", map[string]string{labelNodeRoleControlPlane: ""}),
				labelledNode("cp-2", map[string]string{labelNodeRoleControlPlane: ""}),
				labelledNode("worker-1", nil),
			},
			expectedMasterNumber: 2,
			expectedWorkerNumber: 1,
		},
		"legacy cluster, only master label (k8s < 1.20)": {
			nodes: []client.Object{
				labelledNode("cp-1", map[string]string{labelNodeRoleMaster: ""}),
				labelledNode("worker-1", nil),
				labelledNode("worker-2", nil),
			},
			expectedMasterNumber: 1,
			expectedWorkerNumber: 2,
		},
		"transitional cluster, both labels on the same node (k8s 1.20-1.23)": {
			nodes: []client.Object{
				labelledNode("cp-1", map[string]string{
					labelNodeRoleMaster:       "",
					labelNodeRoleControlPlane: "",
				}),
				labelledNode("cp-2", map[string]string{
					labelNodeRoleMaster:       "",
					labelNodeRoleControlPlane: "",
				}),
				labelledNode("worker-1", nil),
			},
			expectedMasterNumber: 2,
			expectedWorkerNumber: 1,
		},
		"mixed cluster, control plane and labelled workers": {
			nodes: []client.Object{
				labelledNode("cp-1", map[string]string{labelNodeRoleControlPlane: ""}),
				labelledNode("cp-2", map[string]string{labelNodeRoleMaster: ""}),
				labelledNode("worker-1", map[string]string{types.LabelNodeRoleWorker: ""}),
				labelledNode("worker-2", map[string]string{types.LabelNodeRoleWorker: ""}),
				labelledNode("worker-3", nil),
			},
			expectedMasterNumber: 2,
			expectedWorkerNumber: 3,
		},
		"all workers, no control plane label at all": {
			nodes: []client.Object{
				labelledNode("worker-1", nil),
				labelledNode("worker-2", map[string]string{types.LabelNodeRoleWorker: ""}),
			},
			expectedMasterNumber: 0,
			expectedWorkerNumber: 2,
		},
		"empty cluster": {
			nodes:                nil,
			expectedMasterNumber: 0,
			expectedWorkerNumber: 0,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(tc.nodes...).Build()
			info, err := GetClusterInfo(context.Background(), cli, "test-cluster")
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedMasterNumber, info.MasterNumber)
			assert.Equal(t, tc.expectedWorkerNumber, info.WorkerNumber)
			// every node must be counted exactly once
			assert.Equal(t, len(tc.nodes), info.MasterNumber+info.WorkerNumber)
			assert.Equal(t, len(tc.nodes), len(info.Nodes.Items))
		})
	}
}
