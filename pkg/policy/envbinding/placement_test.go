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

package envbinding

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestUpdateClusterConnections(t *testing.T) {
	app := &v1beta1.Application{}
	app.Status.LatestRevision = &common.Revision{Name: "v1"}
	status := &v1alpha1.EnvBindingStatus{
		ClusterConnections: []v1alpha1.ClusterConnection{{
			ClusterName:        "cluster-1",
			LastActiveRevision: "v0",
		}, {
			ClusterName:        "cluster-2",
			LastActiveRevision: "v0",
		}},
	}
	decisions := []v1alpha1.PlacementDecision{{
		Cluster: "cluster-1",
	}, {
		Cluster: "cluster-3",
	}}
	updateClusterConnections(status, decisions, app)

	r := require.New(t)
	expectedConnections := []v1alpha1.ClusterConnection{{
		ClusterName:        "cluster-1",
		LastActiveRevision: "v1",
	}, {
		ClusterName:        "cluster-2",
		LastActiveRevision: "v0",
	}, {
		ClusterName:        "cluster-3",
		LastActiveRevision: "v1",
	}}
	r.Equal(len(expectedConnections), len(status.ClusterConnections))
	for idx, conn := range expectedConnections {
		_conn := status.ClusterConnections[idx]
		r.Equal(conn.ClusterName, _conn.ClusterName)
		r.Equal(conn.LastActiveRevision, _conn.LastActiveRevision)
	}
}
