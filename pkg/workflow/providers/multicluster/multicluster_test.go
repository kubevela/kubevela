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
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

func TestListClusters(t *testing.T) {
	multicluster.ClusterGatewaySecretNamespace = types.DefaultKubeVelaNS
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	clusterNames := []string{"cluster-a", "cluster-b"}
	for _, secretName := range clusterNames {
		secret := &corev1.Secret{}
		secret.Name = secretName
		secret.Namespace = multicluster.ClusterGatewaySecretNamespace
		secret.Labels = map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)}
		r.NoError(cli.Create(context.Background(), secret))
	}
	res, err := ListClusters(ctx, &oamprovidertypes.Params[any]{
		RuntimeParams: oamprovidertypes.RuntimeParams{
			KubeClient: cli,
		},
	})
	r.NoError(err)
	r.Equal(clusterNames, res.Returns.Outputs.Clusters)
}
