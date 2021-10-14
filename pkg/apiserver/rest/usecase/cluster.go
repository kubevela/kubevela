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
	"fmt"
	"io/ioutil"
	"time"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// ClusterUsecase cluster manage
type ClusterUsecase interface {
	ListKubeClusters(context.Context, string, int, int) (*apis.ListClusterResponse, error)
	CreateKubeCluster(context.Context, apis.CreateClusterRequest) (*apis.ClusterBase, error)
	GetKubeCluster(context.Context, string) (*apis.DetailClusterResponse, error)
	ModifyKubeCluster(context.Context, apis.CreateClusterRequest, string) (*apis.ClusterBase, error)
	DeleteKubeCluster(context.Context, string) (*apis.ClusterBase, error)
}

type clusterUsecaseImpl struct {
	ds        datastore.DataStore
	k8sClient client.Client
}

// NewClusterUsecase new cluster usecase
func NewClusterUsecase(ds datastore.DataStore, k8sClient client.Client) ClusterUsecase {
	return &clusterUsecaseImpl{ds: ds, k8sClient: k8sClient}
}

func (c *clusterUsecaseImpl) getClusterFromDataStore(ctx context.Context, clusterName string) (*model.Cluster, error) {
	cluster := &model.Cluster{
		Name: clusterName,
	}
	if err := c.ds.Get(ctx, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

func (c *clusterUsecaseImpl) ListKubeClusters(ctx context.Context, query string, page int, pageSize int) (*apis.ListClusterResponse, error) {
	// TODO: Fuzzy query
	clusters, err := c.ds.List(ctx, &model.Cluster{}, &datastore.ListOptions{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list cluster with query %s in data store", query)
	}
	resp := &apis.ListClusterResponse{
		Clusters: []apis.ClusterBase{},
	}
	for _, raw := range clusters {
		cluster, ok := raw.(*model.Cluster)
		if ok {
			resp.Clusters = append(resp.Clusters, *cluster.ToClusterBase())
		}
	}
	return resp, nil
}

func joinClusterByKubeConfigString(ctx context.Context, k8sClient client.Client, clusterName string, kubeConfig string) error {
	tmpFileName := fmt.Sprintf("/tmp/cluster-secret-%s-%s-%d.kubeconfig", clusterName, utils.RandomString(8), time.Now().UnixNano())
	if err := ioutil.WriteFile(tmpFileName, []byte(kubeConfig), 0600); err != nil {
		return errors.Wrapf(err, "failed to write kubeconfig to temp file %s", tmpFileName)
	}
	_, err := multicluster.JoinClusterByKubeConfig(ctx, k8sClient, tmpFileName, clusterName)
	if err != nil {
		return errors.Wrapf(err, "failed to join cluster")
	}
	return nil
}

func createClusterModelFromRequest(req apis.CreateClusterRequest) *model.Cluster {
	return &model.Cluster{
		Name:             req.Name,
		Description:      req.Description,
		Icon:             req.Icon,
		Labels:           req.Labels,
		KubeConfig:       req.KubeConfig,
		KubeConfigSecret: req.KubeConfigSecret,
	}
}

func (c *clusterUsecaseImpl) CreateKubeCluster(ctx context.Context, req apis.CreateClusterRequest) (*apis.ClusterBase, error) {
	cluster := createClusterModelFromRequest(req)
	if req.KubeConfig != "" {
		if err := joinClusterByKubeConfigString(ctx, c.k8sClient, req.Name, req.KubeConfig); err != nil {
			return nil, err
		}
		c.setClusterStatusAndResourceInfo(ctx, cluster)
		return cluster.ToClusterBase(), c.ds.Add(ctx, cluster)
	}
	if req.KubeConfigSecret != "" {
		return nil, errors.Errorf("kubeconfig secret is not supported now")
	}
	return nil, errors.Errorf("kubeconfig or kubeconfig secret must be set")
}

func (c *clusterUsecaseImpl) GetKubeCluster(ctx context.Context, clusterName string) (*apis.DetailClusterResponse, error) {
	cluster, err := c.getClusterFromDataStore(ctx, clusterName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to found cluster %s in data store", clusterName)
	}
	c.setClusterStatusAndResourceInfo(ctx, cluster)
	if err = c.ds.Put(ctx, cluster); err != nil {
		return nil, errors.Wrapf(err, "failed to update cluster %s status info", clusterName)
	}
	return &apis.DetailClusterResponse{
		ClusterBase:     *cluster.ToClusterBase(),
		ResourceInfo:    cluster.ResourceInfo,
		RemoteManageURL: "NA",
		DashboardURL:    "NA",
	}, nil
}

func (c *clusterUsecaseImpl) ModifyKubeCluster(ctx context.Context, req apis.CreateClusterRequest, clusterName string) (*apis.ClusterBase, error) {
	oldCluster, err := c.getClusterFromDataStore(ctx, clusterName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to found cluster %s in data store", clusterName)
	}

	newCluster := createClusterModelFromRequest(req)
	if oldCluster.Name != newCluster.Name || oldCluster.KubeConfig != newCluster.KubeConfig || oldCluster.KubeConfigSecret != newCluster.KubeConfigSecret {
		if newCluster.KubeConfig == "" && newCluster.KubeConfigSecret != "" {
			return nil, errors.Errorf("kubeconfig secret is not supported now")
		}
		if err = multicluster.DetachCluster(ctx, c.k8sClient, oldCluster.Name); err != nil {
			return nil, errors.Wrapf(err, "failed to detach old cluster %s", oldCluster.Name)
		}
		if err = c.ds.Delete(ctx, oldCluster); err != nil {
			return nil, errors.Wrapf(err, "failed to delete old cluster %s from datastore", oldCluster.Name)
		}
		if err = joinClusterByKubeConfigString(ctx, c.k8sClient, newCluster.Name, newCluster.KubeConfig); err != nil {
			return nil, errors.Wrapf(err, "failed to join new cluster %s", newCluster.Name)
		}
		c.setClusterStatusAndResourceInfo(ctx, newCluster)
		if err = c.ds.Add(ctx, newCluster); err != nil {
			return nil, errors.Wrapf(err, "failed to add new cluster %s to datastore", newCluster.Name)
		}
	} else {
		newCluster.ResourceInfo = oldCluster.ResourceInfo
		newCluster.Status = oldCluster.Status
		newCluster.Reason = oldCluster.Reason
		if err = c.ds.Put(ctx, newCluster); err != nil {
			return nil, errors.Wrapf(err, "failed to update cluster %s", newCluster.Name)
		}
	}
	return newCluster.ToClusterBase(), nil
}

func (c *clusterUsecaseImpl) DeleteKubeCluster(ctx context.Context, clusterName string) (*apis.ClusterBase, error) {
	cluster, err := c.getClusterFromDataStore(ctx, clusterName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to found cluster %s in data store", clusterName)
	}
	if err = c.ds.Delete(ctx, cluster); err != nil {
		return nil, errors.Wrapf(err, "failed to delete cluster %s in data store", clusterName)
	}
	if err = multicluster.DetachCluster(ctx, c.k8sClient, clusterName); err != nil {
		return nil, errors.Wrapf(err, "failed to delete cluster %s in kubernetes", clusterName)
	}
	return cluster.ToClusterBase(), nil
}

func (c *clusterUsecaseImpl) setClusterStatusAndResourceInfo(ctx context.Context, cluster *model.Cluster) {
	// TODO add cache
	resourceInfo, err := c.getClusterResourceInfoFromK8s(ctx, cluster.Name)
	if err != nil {
		cluster.Status = "Unhealthy"
		cluster.Reason = fmt.Sprintf("Failed to get cluster resource info: %v", err)
	} else {
		cluster.Status = "Healthy"
		cluster.Reason = ""
		cluster.ResourceInfo = resourceInfo
	}
}

func (c *clusterUsecaseImpl) getClusterResourceInfoFromK8s(ctx context.Context, clusterName string) (apis.ClusterResourceInfo, error) {
	clusterInfo, err := multicluster.GetClusterInfo(ctx, c.k8sClient, clusterName)
	if err != nil {
		return apis.ClusterResourceInfo{}, err
	}
	var storageClassList []string
	for _, cls := range clusterInfo.StorageClasses.Items {
		storageClassList = append(storageClassList, cls.Name)
	}
	// TODO add support for gpu capacity
	return apis.ClusterResourceInfo{
		WorkerNumber:     clusterInfo.WorkerNumber,
		MasterNumber:     clusterInfo.MasterNumber,
		MemoryCapacity:   clusterInfo.MemoryCapacity.Value(),
		CPUCapacity:      clusterInfo.CPUCapacity.Value(),
		GPUCapacity:      0,
		StorageClassList: storageClassList,
	}, nil
}
