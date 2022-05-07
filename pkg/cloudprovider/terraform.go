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

package cloudprovider

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	types "github.com/oam-dev/terraform-controller/api/types/crossplane-runtime"
	v1beta12 "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/pkg/errors"
	v12 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func computeProviderHashKey(provider string, accessKeyID string, accessKeySecret string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join([]string{provider, accessKeyID, accessKeySecret}, "::"))))[:8] // #nosec
}

// GetCloudClusterFullName construct the full name of cloud cluster which will be used as the name of terraform configuration
func GetCloudClusterFullName(provider string, clusterName string) string {
	return fmt.Sprintf("cloud-cluster-%s-%s", provider, clusterName)
}

func bootstrapTerraformProvider(ctx context.Context, k8sClient client.Client, ns string, provider string, tfProvider string, accessKeyID string, accessKeySecret string, region string) (string, error) {
	hashKey := computeProviderHashKey(provider, accessKeyID, accessKeySecret)
	secretName := fmt.Sprintf("tf-provider-cred-%s-%s", provider, hashKey)
	terraformProviderName := fmt.Sprintf("tf-provider-%s-%s", provider, hashKey)
	secret := v12.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      secretName,
			Namespace: ns,
		},
		StringData: map[string]string{"credentials": fmt.Sprintf("accessKeyID: %s\naccessKeySecret: %s\nsecurityToken:\n", accessKeyID, accessKeySecret)},
		Type:       v12.SecretTypeOpaque,
	}
	var err error
	if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&secret), &v12.Secret{}); err != nil {
		if kerrors.IsNotFound(err) {
			err = k8sClient.Create(ctx, &secret)
		}
		if err != nil {
			return "", errors.Wrapf(err, "failed to upsert terraform provider secret")
		}
	}

	terraformProvider := v1beta12.Provider{
		ObjectMeta: v1.ObjectMeta{
			Name:      terraformProviderName,
			Namespace: ns,
		},
		Spec: v1beta12.ProviderSpec{
			Credentials: v1beta12.ProviderCredentials{
				SecretRef: &types.SecretKeySelector{
					Key: "credentials",
					SecretReference: types.SecretReference{
						Name:      secretName,
						Namespace: ns,
					},
				},
				Source: types.CredentialsSourceSecret,
			},
			Provider: tfProvider,
			Region:   region,
		},
	}
	if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&terraformProvider), &v1beta12.Provider{}); err != nil {
		if kerrors.IsNotFound(err) {
			err = k8sClient.Create(ctx, &terraformProvider)
		}
		if err != nil {
			return "", errors.Wrapf(err, "failed to upsert terraform provider")
		}
	}
	return terraformProviderName, nil
}
