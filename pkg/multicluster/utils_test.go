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

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestUpgradeExistingClusterSecret(t *testing.T) {
	oldClusterGatewaySecretNamespace := ClusterGatewaySecretNamespace
	ClusterGatewaySecretNamespace = "default"
	defer func() {
		ClusterGatewaySecretNamespace = oldClusterGatewaySecretNamespace
	}()
	ctx := context.Background()
	c := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	secret := &v1.Secret{
		ObjectMeta: v12.ObjectMeta{
			Name:      "example-outdated-cluster-secret",
			Namespace: "default",
			Labels: map[string]string{
				"cluster.core.oam.dev/cluster-credential": "tls",
			},
		},
		Type: v1.SecretTypeTLS,
	}
	if err := c.Create(ctx, secret); err != nil {
		t.Fatalf("failed to create fake outdated cluster secret, err: %v", err)
	}
	if err := UpgradeExistingClusterSecret(ctx, c); err != nil {
		t.Fatalf("expect no error while upgrading outdated cluster secret but encounter error: %v", err)
	}
	newSecret := &v1.Secret{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(secret), newSecret); err != nil {
		t.Fatalf("found error while getting updated cluster secret: %v", err)
	}
	if newSecret.Labels[clustercommon.LabelKeyClusterCredentialType] != string(v1alpha1.CredentialTypeX509Certificate) {
		t.Fatalf("updated secret label should has credential type x509")
	}
}
