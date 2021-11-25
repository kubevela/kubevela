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

package multicluster

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestGetAppliedCluster(t *testing.T) {
	r := require.New(t)
	app := &v1beta1.Application{}
	app.Status.AppliedResources = []common.ClusterObjectReference{{
		Cluster: "cluster-0",
	}}
	cli := fake.NewClientBuilder().WithScheme(common2.Scheme).Build()
	app.Status.PolicyStatus = []common.PolicyStatus{{
		Type:   v1alpha1.EnvBindingPolicyType,
		Status: &runtime.RawExtension{Raw: []byte(`bad value`)},
	}}
	clusters := getAppliedClusters(context.Background(), cli, app)
	r.Equal(1, len(clusters))
	r.Equal("cluster-0", clusters[0])
	envBindingStatus := &v1alpha1.EnvBindingStatus{ClusterConnections: []v1alpha1.ClusterConnection{{
		ClusterName: "cluster-1",
	}, {
		ClusterName: "cluster-2",
	}}}
	bs, err := json.Marshal(envBindingStatus)
	r.NoError(err)
	app.Status.AppliedResources = []common.ClusterObjectReference{}
	app.Status.PolicyStatus = []common.PolicyStatus{{
		Type:   v1alpha1.EnvBindingPolicyType,
		Status: &runtime.RawExtension{Raw: bs},
	}}
	clusters = getAppliedClusters(context.Background(), cli, app)
	r.Equal(2, len(clusters))
	sort.Strings(clusters)
	r.Equal("cluster-1", clusters[0])
	r.Equal("cluster-2", clusters[1])
}
