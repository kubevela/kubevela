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
	"encoding/json"
	"fmt"
	"strings"

	cs20151215 "github.com/alibabacloud-go/cs-20151215/v3/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	types "github.com/oam-dev/terraform-controller/api/types/crossplane-runtime"
	v1beta12 "github.com/oam-dev/terraform-controller/api/v1beta2"
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	aliyunAPIEndpoint = "cs.cn-hangzhou.aliyuncs.com"
)

// AliyunCloudProvider describes the cloud provider in aliyun
type AliyunCloudProvider struct {
	*cs20151215.Client
	k8sClient       client.Client
	accessKeyID     string
	accessKeySecret string
}

// NewAliyunCloudProvider create aliyun cloud provider
func NewAliyunCloudProvider(accessKeyID string, accessKeySecret string, k8sClient client.Client) (*AliyunCloudProvider, error) {
	config := &openapi.Config{
		AccessKeyId:     pointer.String(accessKeyID),
		AccessKeySecret: pointer.String(accessKeySecret),
	}
	config.Endpoint = tea.String(aliyunAPIEndpoint)
	c, err := cs20151215.NewClient(config)
	if err != nil {
		return nil, err
	}
	return &AliyunCloudProvider{Client: c, k8sClient: k8sClient, accessKeyID: accessKeyID, accessKeySecret: accessKeySecret}, nil
}

// IsInvalidKey check if error is InvalidAccessKey or InvalidSecretKey
func (provider *AliyunCloudProvider) IsInvalidKey(err error) bool {
	return strings.Contains(err.Error(), "InvalidAccessKeyId") || strings.Contains(err.Error(), "Code: SignatureDoesNotMatch")
}

func (provider *AliyunCloudProvider) decodeClusterLabels(tags []*cs20151215.Tag) map[string]string {
	labels := map[string]string{}
	for _, tag := range tags {
		if tag != nil {
			labels[getString(tag.Key)] = getString(tag.Value)
		}
	}
	return labels
}

func (provider *AliyunCloudProvider) decodeClusterURL(masterURL string) (url struct {
	APIServerEndpoint         string `json:"api_server_endpoint"`
	IntranetAPIServerEndpoint string `json:"intranet_api_server_endpoint"`
}) {
	if err := json.Unmarshal([]byte(masterURL), &url); err != nil {
		klog.Info("failed to unmarshal masterUrl %s", masterURL)
	}
	return
}

func (provider *AliyunCloudProvider) getDashboardURL(clusterID string) string {
	return fmt.Sprintf("https://cs.console.aliyun.com/#/k8s/cluster/%s/v2/info/overview", clusterID)
}

func getString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
		if cluster == nil {
			continue
		}
		labels := provider.decodeClusterLabels(cluster.Tags)
		url := provider.decodeClusterURL(getString(cluster.MasterUrl))
		clusters = append(clusters, &CloudCluster{
			ID:           getString(cluster.ClusterId),
			Name:         getString(cluster.Name),
			Type:         getString(cluster.ClusterType),
			Zone:         getString(cluster.ZoneId),
			ZoneID:       getString(cluster.ZoneId),
			RegionID:     getString(cluster.RegionId),
			VpcID:        getString(cluster.VpcId),
			Labels:       labels,
			Status:       getString(cluster.State),
			APIServerURL: url.APIServerEndpoint,
			DashBoardURL: provider.getDashboardURL(getString(cluster.ClusterId)),
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

// GetClusterInfo retrieves cluster info by clusterID
func (provider *AliyunCloudProvider) GetClusterInfo(clusterID string) (*CloudCluster, error) {
	resp, err := provider.DescribeClusterDetail(pointer.String(clusterID))
	if err != nil {
		return nil, err
	}
	cluster := resp.Body
	labels := provider.decodeClusterLabels(cluster.Tags)
	url := provider.decodeClusterURL(*cluster.MasterUrl)
	return &CloudCluster{
		Provider:     ProviderAliyun,
		ID:           *cluster.ClusterId,
		Name:         *cluster.Name,
		Type:         *cluster.ClusterType,
		Zone:         *cluster.ZoneId,
		ZoneID:       *cluster.ZoneId,
		RegionID:     *cluster.RegionId,
		VpcID:        *cluster.VpcId,
		Labels:       labels,
		Status:       *cluster.State,
		APIServerURL: url.APIServerEndpoint,
		DashBoardURL: provider.getDashboardURL(*cluster.ClusterId),
	}, nil
}

// CreateCloudCluster create cloud cluster
func (provider *AliyunCloudProvider) CreateCloudCluster(ctx context.Context, clusterName string, zone string, worker int, cpu int64, mem int64) (string, error) {
	name := GetCloudClusterFullName(ProviderAliyun, clusterName)
	ns := util.GetRuntimeNamespace()
	terraformProviderName, err := bootstrapTerraformProvider(ctx, provider.k8sClient, ns, ProviderAliyun, "alibaba", provider.accessKeyID, provider.accessKeySecret, "cn-hongkong")
	if err != nil {
		return "", errors.Wrapf(err, "failed to bootstrap terraform provider")
	}
	properties := map[string]interface{}{
		"k8s_name_prefix": clusterName,
	}
	if zone != "" {
		properties["zone_id"] = zone
	}
	if cpu != 0 {
		properties["cpu_core_count"] = cpu
	}
	if mem != 0 {
		properties["memory_size"] = mem
	}
	if worker != 0 {
		properties["k8s_worker_number"] = worker
	}
	bs, err := json.Marshal(properties)
	if err != nil {
		return name, errors.Wrapf(err, "failed to marshal cloud cluster app properties")
	}

	cfg := v1beta12.Configuration{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				CloudClusterCreatorLabelKey: ProviderAliyun,
			},
		},
		Spec: v1beta12.ConfigurationSpec{
			ProviderReference: &types.Reference{
				Name:      terraformProviderName,
				Namespace: ns,
			},
			Remote:   "https://github.com/kubevela-contrib/terraform-modules.git",
			Variable: &runtime.RawExtension{Raw: bs},
		},
	}
	cfg.Spec.Path = "alibaba/cs/dedicated-kubernetes"
	cfg.Spec.WriteConnectionSecretToReference = &types.SecretReference{
		Name:      name,
		Namespace: ns,
	}

	if err = provider.k8sClient.Create(ctx, &cfg); err != nil {
		return name, errors.Wrapf(err, "failed to create cloud cluster terraform configuration")
	}

	return name, nil
}
