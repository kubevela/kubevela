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

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// CloudClusterCreatorLabelKey labels the creator of cloud cluster
	CloudClusterCreatorLabelKey = "api.core.oam.dev/cloud-cluster-creator"
)

// CloudClusterProvider abstracts the cloud provider to provide cluster access
type CloudClusterProvider interface {
	IsInvalidKey(err error) bool
	ListCloudClusters(pageNumber int, pageSize int) ([]*CloudCluster, int, error)
	GetClusterKubeConfig(clusterID string) (string, error)
	GetClusterInfo(clusterID string) (*CloudCluster, error)
	CreateCloudCluster(ctx context.Context, clusterName string, zone string, worker int, cpu int64, mem int64) (string, error)
}

// GetClusterProvider creates interface for getting cloud cluster provider
func GetClusterProvider(provider string, accessKeyID string, accessKeySecret string, k8sClient client.Client) (CloudClusterProvider, error) {
	switch provider {
	case ProviderAliyun:
		return NewAliyunCloudProvider(accessKeyID, accessKeySecret, k8sClient)
	default:
		return nil, errors.Errorf("cluster provider %s is not implemented", provider)
	}
}
