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

package clustermanager

import (
	"context"
	"fmt"

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// MatchClusterLabels filters the list/delete operation of cluster list
type MatchClusterLabels map[string]string

// ApplyToList applies this configuration to the given list options.
func (m MatchClusterLabels) ApplyToList(opts *client.ListOptions) {
	sel := labels.SelectorFromValidatedSet(map[string]string(m))
	r, err := labels.NewRequirement(v1alpha1.LabelKeyClusterCredentialType, selection.Exists, nil)
	if err == nil {
		sel = sel.Add(*r)
	}
	opts.LabelSelector = sel
	opts.Namespace = multicluster.ClusterGatewaySecretNamespace
}

// ApplyToDeleteAllOf applies this configuration to the given a List options.
func (m MatchClusterLabels) ApplyToDeleteAllOf(opts *client.DeleteAllOfOptions) {
	m.ApplyToList(&opts.ListOptions)
}

// GetClient returns a kube client for given kubeConfigData
func GetClient(kubeConfigData []byte) (client.Client, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigData)
	if err != nil {
		return nil, err
	}
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	return client.New(restConfig, client.Options{Scheme: common.Scheme})
}

// GetRegisteredClusters will get all registered clusters in control plane
func GetRegisteredClusters(c client.Client) ([]types.Cluster, error) {
	return GetRegisteredClustersWithLabels(c, map[string]string{})
}

// GetRegisteredClustersWithLabels will get registered clusters with labels in control plane
func GetRegisteredClustersWithLabels(c client.Client, labels map[string]string) ([]types.Cluster, error) {
	var clusters []types.Cluster
	secrets := corev1.SecretList{}
	if err := c.List(context.Background(), &secrets, MatchClusterLabels(labels)); err != nil {
		return nil, errors.Wrapf(err, "failed to get clusterSecret secrets")
	}
	for _, clusterSecret := range secrets.Items {
		endpoint := string(clusterSecret.Data["endpoint"])
		if endp, ok := clusterSecret.GetLabels()[v1alpha1.LabelKeyClusterEndpointType]; ok {
			endpoint = endp
		}
		clusters = append(clusters, types.Cluster{
			Name:     clusterSecret.Name,
			Type:     clusterSecret.GetLabels()[v1alpha1.LabelKeyClusterCredentialType],
			EndPoint: endpoint,
			Accepted: true,
		})
	}

	crdName := k8stypes.NamespacedName{Name: "managedclusters." + clusterv1.GroupName}
	if err := c.Get(context.Background(), crdName, &crdv1.CustomResourceDefinition{}); err != nil {
		if apierror.IsNotFound(err) {
			return clusters, nil
		}
		return nil, err
	}

	managedClusters := clusterv1.ManagedClusterList{}
	if err := c.List(context.Background(), &managedClusters, MatchClusterLabels(labels)); err != nil {
		return nil, errors.Wrapf(err, "failed to get managed clusters")
	}
	for _, cluster := range managedClusters.Items {
		if len(cluster.Spec.ManagedClusterClientConfigs) != 0 {
			clusters = append(clusters, types.Cluster{
				Name:     cluster.Name,
				Type:     "OCM ManagedServiceAccount",
				EndPoint: "-",
				Accepted: cluster.Spec.HubAcceptsClient,
			})
		}
	}
	return clusters, nil
}

// EnsureClusterNotExists will check the cluster is not existed in control plane
func EnsureClusterNotExists(c client.Client, clusterName string) error {
	exist, err := clusterExists(c, clusterName)
	if err != nil {
		return err
	}
	if exist {
		return fmt.Errorf("cluster %s already exists", clusterName)
	}
	return nil
}

// EnsureClusterExists will check the cluster is existed in control plane
func EnsureClusterExists(c client.Client, clusterName string) error {
	exist, err := clusterExists(c, clusterName)
	if err != nil {
		return err
	}
	if !exist {
		return fmt.Errorf("cluster %s not exists", clusterName)
	}
	return nil
}

// clusterExists will check whether the cluster exist or not
func clusterExists(c client.Client, clusterName string) (bool, error) {
	err := c.Get(context.Background(),
		k8stypes.NamespacedName{
			Name:      clusterName,
			Namespace: multicluster.ClusterGatewaySecretNamespace,
		},
		&corev1.Secret{})
	if err == nil {
		return true, nil
	}
	if !apierror.IsNotFound(err) {
		return false, errors.Wrapf(err, "failed to check duplicate cluster")
	}

	crdName := k8stypes.NamespacedName{Name: "managedclusters." + clusterv1.GroupName}
	if err = c.Get(context.Background(), crdName, &crdv1.CustomResourceDefinition{}); err != nil {
		if apierror.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to get managedcluster CRD to check duplicate cluster")
	}
	err = c.Get(context.Background(), k8stypes.NamespacedName{
		Name:      clusterName,
		Namespace: multicluster.ClusterGatewaySecretNamespace,
	}, &clusterv1.ManagedCluster{})
	if err == nil {
		return true, nil
	}
	if !apierror.IsNotFound(err) {
		return false, errors.Wrapf(err, "failed to check duplicate cluster")
	}
	return false, nil
}
