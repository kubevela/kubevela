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

package usecase

import (
	"context"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

// ClusterUsecase cluster manage
type ClusterUsecase interface {
	CreateKubeCluster(context.Context, apis.CreateClusterRequest) (*apis.ClusterBase, error)
}

type clusterUsecaseImpl struct {
	ds datastore.DataStore
}

// NewClusterUsecase new cluster usecase
func NewClusterUsecase(ds datastore.DataStore) ClusterUsecase {
	return &clusterUsecaseImpl{ds: ds}
}

func (c *clusterUsecaseImpl) CreateKubeCluster(ctx context.Context, req apis.CreateClusterRequest) (*apis.ClusterBase, error) {
	return nil, nil
}
