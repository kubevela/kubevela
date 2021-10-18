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
	"encoding/json"

	cs20151215 "github.com/alibabacloud-go/cs-20151215/v2/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	"github.com/alibabacloud-go/tea/tea"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
)

const (
	aliyunAPIEndpoint = "cs.cn-hangzhou.aliyuncs.com"
)

// AliyunCloudProvider describes the cloud provider in aliyun
type AliyunCloudProvider struct {
	*cs20151215.Client
}

// NewAliyunCloudProvider create aliyun cloud provider
func NewAliyunCloudProvider(accessKeyID string, accessKeySecret string) (*AliyunCloudProvider, error) {
	config := &openapi.Config{
		AccessKeyId:     pointer.String(accessKeyID),
		AccessKeySecret: pointer.String(accessKeySecret),
	}
	config.Endpoint = tea.String(aliyunAPIEndpoint)
	c, err := cs20151215.NewClient(config)
	if err != nil {
		return nil, err
	}
	return &AliyunCloudProvider{Client: c}, nil
}

// ListCloudClusters list clusters with page info, return clusters, total count and error
func (provider *AliyunCloudProvider) ListCloudClusters(pageNumber int, pageSize int) ([]*CloudCluster, int, error) {
	describeClustersV1Request := &cs20151215.DescribeClustersV1Request{
		PageSize:   pointer.Int64(int64(pageSize)),
		PageNumber: pointer.Int64(int64(pageNumber)),
	}
	resp, err := provider.DescribeClustersV1(describeClustersV1Request)
	if err != nil {
		return nil, 0, err
	}
	var clusters []*CloudCluster
	for _, cluster := range resp.Body.Clusters {
		labels := map[string]string{}
		for _, tag := range cluster.Tags {
			labels[*tag.Key] = *tag.Value
		}
		url := &struct {
			APIServerEndpoint string `json:"api_server_endpoint"`
		}{}
		if err = json.Unmarshal([]byte(*cluster.MasterUrl), url); err != nil {
			klog.Info("failed to unmarshal masterUrl %s", *cluster.MasterUrl)
		}
		clusters = append(clusters, &CloudCluster{
			ID:       *cluster.ClusterId,
			Name:     *cluster.Name,
			Type:     *cluster.ClusterType,
			Labels:   labels,
			Status:   *cluster.State,
			Endpoint: url.APIServerEndpoint,
		})
	}
	return clusters, int(*resp.Body.PageInfo.TotalCount), nil
}

// GetClusterKubeConfig get cluster kubeconfig by clusterID
func (provider *AliyunCloudProvider) GetClusterKubeConfig(clusterID string) (string, error) {
	req := &cs20151215.DescribeClusterUserKubeconfigRequest{}
	resp, err := provider.DescribeClusterUserKubeconfig(pointer.String(clusterID), req)
	if err != nil {
		return "", err
	}
	return *resp.Body.Config, nil
}
