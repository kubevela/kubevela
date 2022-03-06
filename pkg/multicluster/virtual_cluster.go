/*
Copyright 2020-2022 The KubeVela Authors.

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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apilabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	apitypes "k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"

	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

const (
	// CredentialTypeOCMManagedCluster identifies the virtual cluster from ocm
	CredentialTypeOCMManagedCluster v1alpha1.CredentialType = "ManagedCluster"
)

// VirtualCluster contains base info of cluster, it unifies the difference between different cluster implementations
// like cluster secret or ocm managed cluster
type VirtualCluster struct {
	Name     string
	Type     v1alpha1.CredentialType
	EndPoint string
	Accepted bool
	Labels   map[string]string
	Metrics  *ClusterMetrics
}

// NewVirtualClusterFromSecret extract virtual cluster from cluster secret
func NewVirtualClusterFromSecret(secret *corev1.Secret) (*VirtualCluster, error) {
	endpoint := string(secret.Data["endpoint"])
	labels := secret.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	if _endpoint, ok := labels[v1alpha1.LabelKeyClusterEndpointType]; ok {
		endpoint = _endpoint
	}
	credType, ok := labels[v1alpha1.LabelKeyClusterCredentialType]
	if !ok {
		return nil, errors.Errorf("secret is not a valid cluster secret, no credential type found")
	}
	return &VirtualCluster{
		Name:     secret.Name,
		Type:     v1alpha1.CredentialType(credType),
		EndPoint: endpoint,
		Accepted: true,
		Labels:   labels,
		Metrics:  metricsMap[secret.Name],
	}, nil
}

// NewVirtualClusterFromManagedCluster extract virtual cluster from ocm managed cluster
func NewVirtualClusterFromManagedCluster(managedCluster *clusterv1.ManagedCluster) (*VirtualCluster, error) {
	if len(managedCluster.Spec.ManagedClusterClientConfigs) == 0 {
		return nil, errors.Errorf("managed cluster has no client config")
	}
	return &VirtualCluster{
		Name:     managedCluster.Name,
		Type:     CredentialTypeOCMManagedCluster,
		EndPoint: "-",
		Accepted: managedCluster.Spec.HubAcceptsClient,
		Labels:   managedCluster.GetLabels(),
		Metrics:  metricsMap[managedCluster.Name],
	}, nil
}

// GetVirtualCluster returns virtual cluster with given clusterName
func GetVirtualCluster(ctx context.Context, c client.Client, clusterName string) (vc *VirtualCluster, err error) {
	secret := &corev1.Secret{}
	err = c.Get(ctx, apitypes.NamespacedName{
		Name:      clusterName,
		Namespace: ClusterGatewaySecretNamespace,
	}, secret)
	var secretErr error
	if err == nil {
		vc, secretErr = NewVirtualClusterFromSecret(secret)
		if secretErr == nil {
			return vc, nil
		}
	}
	if err != nil && !apierrors.IsNotFound(err) {
		secretErr = err
	}

	managedCluster := &clusterv1.ManagedCluster{}
	err = c.Get(ctx, apitypes.NamespacedName{
		Name:      clusterName,
		Namespace: ClusterGatewaySecretNamespace,
	}, managedCluster)
	var managedClusterErr error
	if err == nil {
		vc, managedClusterErr = NewVirtualClusterFromManagedCluster(managedCluster)
		if managedClusterErr == nil {
			return vc, nil
		}
	}

	if err != nil && !apierrors.IsNotFound(err) && !velaerrors.IsCRDNotExists(err) {
		managedClusterErr = err
	}

	if secretErr == nil && managedClusterErr == nil {
		return nil, ErrClusterNotExists
	}

	var errs velaerrors.ErrorList
	if secretErr != nil {
		errs = append(errs, secretErr)
	}
	if managedClusterErr != nil {
		errs = append(errs, managedClusterErr)
	}
	return nil, errs
}

// MatchVirtualClusterLabels filters the list/delete operation of cluster list
type MatchVirtualClusterLabels map[string]string

// ApplyToList applies this configuration to the given list options.
func (m MatchVirtualClusterLabels) ApplyToList(opts *client.ListOptions) {
	sel := apilabels.SelectorFromValidatedSet(map[string]string(m))
	r, err := apilabels.NewRequirement(v1alpha1.LabelKeyClusterCredentialType, selection.Exists, nil)
	if err == nil {
		sel = sel.Add(*r)
	}
	opts.LabelSelector = sel
	opts.Namespace = ClusterGatewaySecretNamespace
}

// ApplyToDeleteAllOf applies this configuration to the given a List options.
func (m MatchVirtualClusterLabels) ApplyToDeleteAllOf(opts *client.DeleteAllOfOptions) {
	m.ApplyToList(&opts.ListOptions)
}

// ListVirtualClusters will get all registered clusters in control plane
func ListVirtualClusters(ctx context.Context, c client.Client) ([]VirtualCluster, error) {
	return FindVirtualClustersByLabels(ctx, c, map[string]string{})
}

// FindVirtualClustersByLabels will get all virtual clusters with matched labels in control plane
func FindVirtualClustersByLabels(ctx context.Context, c client.Client, labels map[string]string) ([]VirtualCluster, error) {
	var clusters []VirtualCluster
	secrets := corev1.SecretList{}
	if err := c.List(ctx, &secrets, MatchVirtualClusterLabels(labels)); err != nil {
		return nil, errors.Wrapf(err, "failed to get clusterSecret secrets")
	}
	for _, secret := range secrets.Items {
		vc, err := NewVirtualClusterFromSecret(secret.DeepCopy())
		if err == nil {
			vc.Metrics = metricsMap[vc.Name]
			clusters = append(clusters, *vc)
		}
	}

	managedClusters := clusterv1.ManagedClusterList{}
	if err := c.List(context.Background(), &managedClusters, client.MatchingLabels(labels)); err != nil && !velaerrors.IsCRDNotExists(err) {
		return nil, errors.Wrapf(err, "failed to get managed clusters")
	}
	for _, managedCluster := range managedClusters.Items {
		vc, err := NewVirtualClusterFromManagedCluster(managedCluster.DeepCopy())
		if err == nil {
			vc.Metrics = metricsMap[vc.Name]
			clusters = append(clusters, *vc)
		}
	}
	return clusters, nil
}
