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
	"fmt"

	v1alpha12 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	v14 "k8s.io/api/storage/v1"
	v13 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	errors3 "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// ensureResourceTrackerCRDInstalled ensures resourcetracker to be installed in child cluster
func ensureResourceTrackerCRDInstalled(ctx context.Context, c client.Client, clusterName string) error {
	remoteCtx := ContextWithClusterName(ctx, clusterName)
	crdName := types2.NamespacedName{Name: "resourcetrackers." + v1beta1.Group}
	if err := c.Get(remoteCtx, crdName, &v13.CustomResourceDefinition{}); err != nil {
		if !errors2.IsNotFound(err) {
			return errors.Wrapf(err, "failed to check resourcetracker crd in cluster %s", clusterName)
		}
		crd := &v13.CustomResourceDefinition{}
		if err = c.Get(ctx, crdName, crd); err != nil {
			return errors.Wrapf(err, "failed to get resourcetracker crd in hub cluster")
		}
		crd.ObjectMeta = v12.ObjectMeta{
			Name:        crdName.Name,
			Annotations: crd.Annotations,
			Labels:      crd.Labels,
		}
		if err = c.Create(remoteCtx, crd); err != nil {
			return errors.Wrapf(err, "failed to create resourcetracker crd in cluster %s", clusterName)
		}
	}
	return nil
}

// ensureClusterNotExists checks if child cluster has already been joined, if joined, error is returned
func ensureClusterNotExists(ctx context.Context, c client.Client, clusterName string) error {
	secret := &v1.Secret{}
	err := c.Get(ctx, types2.NamespacedName{Name: clusterName, Namespace: ClusterGatewaySecretNamespace}, secret)
	if err == nil {
		return fmt.Errorf("cluster %s already exists", clusterName)
	}
	if !errors2.IsNotFound(err) {
		return errors.Wrapf(err, "failed to check duplicate cluster secret")
	}
	return nil
}

// getMutableClusterSecret retrieves the cluster secret and check if any application is using the cluster
func getMutableClusterSecret(ctx context.Context, c client.Client, clusterName string) (*v1.Secret, error) {
	clusterSecret := &v1.Secret{}
	if err := c.Get(ctx, types2.NamespacedName{Namespace: ClusterGatewaySecretNamespace, Name: clusterName}, clusterSecret); err != nil {
		return nil, errors.Wrapf(err, "failed to find target cluster secret %s", clusterName)
	}
	labels := clusterSecret.GetLabels()
	if labels == nil || labels[v1alpha12.LabelKeyClusterCredentialType] == "" {
		return nil, fmt.Errorf("invalid cluster secret %s: cluster credential type label %s is not set", clusterName, v1alpha12.LabelKeyClusterCredentialType)
	}
	ebs := &v1alpha1.EnvBindingList{}
	if err := c.List(ctx, ebs); err != nil {
		return nil, errors.Wrap(err, "failed to find EnvBindings to check clusters")
	}
	errs := errors3.ErrorList{}
	for _, eb := range ebs.Items {
		for _, decision := range eb.Status.ClusterDecisions {
			if decision.Cluster == clusterName {
				errs.Append(fmt.Errorf("application %s/%s (env: %s, envBinding: %s) is currently using cluster %s", eb.Namespace, eb.Labels[oam.LabelAppName], decision.Env, eb.Name, clusterName))
			}
		}
	}
	if errs.HasError() {
		return nil, errors.Wrapf(errs, "cluster %s is in use now", clusterName)
	}
	return clusterSecret, nil
}

// JoinClusterByKubeConfig add child cluster by kubeconfig path, return cluster info and error
func JoinClusterByKubeConfig(_ctx context.Context, k8sClient client.Client, kubeconfigPath string, clusterName string) (*api.Cluster, error) {
	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get kubeconfig")
	}
	if len(config.CurrentContext) == 0 {
		return nil, fmt.Errorf("current-context is not set")
	}
	ctx, ok := config.Contexts[config.CurrentContext]
	if !ok {
		return nil, fmt.Errorf("current-context %s not found", config.CurrentContext)
	}
	cluster, ok := config.Clusters[ctx.Cluster]
	if !ok {
		return nil, fmt.Errorf("cluster %s not found", ctx.Cluster)
	}
	authInfo, ok := config.AuthInfos[ctx.AuthInfo]
	if !ok {
		return nil, fmt.Errorf("authInfo %s not found", ctx.AuthInfo)
	}

	if clusterName == "" {
		clusterName = ctx.Cluster
	}
	if clusterName == ClusterLocalName {
		return cluster, fmt.Errorf("cannot use `%s` as cluster name, it is reserved as the local cluster", ClusterLocalName)
	}

	if err := ensureClusterNotExists(_ctx, k8sClient, clusterName); err != nil {
		return cluster, errors.Wrapf(err, "cannot use cluster name %s", clusterName)
	}

	var credentialType v1alpha12.CredentialType
	data := map[string][]byte{
		"endpoint": []byte(cluster.Server),
		"ca.crt":   cluster.CertificateAuthorityData,
	}
	if len(authInfo.Token) > 0 {
		credentialType = v1alpha12.CredentialTypeServiceAccountToken
		data["token"] = []byte(authInfo.Token)
	} else {
		credentialType = v1alpha12.CredentialTypeX509Certificate
		data["tls.crt"] = authInfo.ClientCertificateData
		data["tls.key"] = authInfo.ClientKeyData
	}
	secret := &v1.Secret{
		ObjectMeta: v12.ObjectMeta{
			Name:      clusterName,
			Namespace: ClusterGatewaySecretNamespace,
			Labels: map[string]string{
				v1alpha12.LabelKeyClusterCredentialType: string(credentialType),
			},
		},
		Type: v1.SecretTypeOpaque,
		Data: data,
	}

	if err := k8sClient.Create(_ctx, secret); err != nil {
		return cluster, errors.Wrapf(err, "failed to add cluster to kubernetes")
	}
	if err := ensureResourceTrackerCRDInstalled(_ctx, k8sClient, clusterName); err != nil {
		_ = k8sClient.Delete(_ctx, secret)
		return cluster, errors.Wrapf(err, "failed to ensure resourcetracker crd installed in cluster %s", clusterName)
	}
	return cluster, nil
}

// DetachCluster detach cluster by name, if cluster is using by application, it will return error
func DetachCluster(ctx context.Context, k8sClient client.Client, clusterName string) error {
	if clusterName == ClusterLocalName {
		return fmt.Errorf("cannot delete `%s` cluster, it is reserved as the local cluster", ClusterLocalName)
	}
	clusterSecret, err := getMutableClusterSecret(ctx, k8sClient, clusterName)
	if err != nil {
		return errors.Wrapf(err, "cluster %s is not mutable now", clusterName)
	}
	return k8sClient.Delete(ctx, clusterSecret)
}

// RenameCluster rename cluster
func RenameCluster(ctx context.Context, k8sClient client.Client, oldClusterName string, newClusterName string) error {
	if newClusterName == ClusterLocalName {
		return fmt.Errorf("cannot use `%s` as cluster name, it is reserved as the local cluster", ClusterLocalName)
	}
	clusterSecret, err := getMutableClusterSecret(ctx, k8sClient, oldClusterName)
	if err != nil {
		return errors.Wrapf(err, "cluster %s is not mutable now", oldClusterName)
	}
	if err := ensureClusterNotExists(ctx, k8sClient, newClusterName); err != nil {
		return errors.Wrapf(err, "cannot set cluster name to %s", newClusterName)
	}
	if err := k8sClient.Delete(ctx, clusterSecret); err != nil {
		return errors.Wrapf(err, "failed to rename cluster from %s to %s", oldClusterName, newClusterName)
	}
	clusterSecret.ObjectMeta = v12.ObjectMeta{
		Name:        newClusterName,
		Namespace:   ClusterGatewaySecretNamespace,
		Labels:      clusterSecret.Labels,
		Annotations: clusterSecret.Annotations,
	}
	if err := k8sClient.Create(ctx, clusterSecret); err != nil {
		return errors.Wrapf(err, "failed to rename cluster from %s to %s", oldClusterName, newClusterName)
	}
	return nil
}

// ClusterInfo describes the basic information of a cluster
type ClusterInfo struct {
	Nodes          *v1.NodeList
	WorkerNumber   int
	MasterNumber   int
	MemoryCapacity resource.Quantity
	CPUCapacity    resource.Quantity
	StorageClasses *v14.StorageClassList
}

// GetClusterInfo retrieves current cluster info from cluster
func GetClusterInfo(_ctx context.Context, k8sClient client.Client, clusterName string) (*ClusterInfo, error) {
	ctx := ContextWithClusterName(_ctx, clusterName)
	nodes := &v1.NodeList{}
	if err := k8sClient.List(ctx, nodes); err != nil {
		return nil, errors.Wrapf(err, "failed to list cluster nodes")
	}
	var workerNumber, masterNumber int
	var memoryCapacity, cpuCapacity resource.Quantity
	for _, node := range nodes.Items {
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			masterNumber++
		} else {
			workerNumber++
		}
		capacity := node.Status.Capacity
		memoryCapacity.Add(*capacity.Memory())
		cpuCapacity.Add(*capacity.Cpu())
	}
	storageClasses := &v14.StorageClassList{}
	if err := k8sClient.List(ctx, storageClasses); err != nil {
		return nil, errors.Wrapf(err, "failed to list storage classes")
	}
	return &ClusterInfo{
		Nodes:          nodes,
		WorkerNumber:   workerNumber,
		MasterNumber:   masterNumber,
		MemoryCapacity: memoryCapacity,
		CPUCapacity:    cpuCapacity,
		StorageClasses: storageClasses,
	}, nil
}
