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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kubevela/pkg/util/singleton"
	"github.com/oam-dev/cluster-gateway/pkg/generated/clientset/versioned"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	apilabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	apitypes "k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// InitClusterInfo will initialize control plane cluster info
func InitClusterInfo(cfg *rest.Config) error {
	ctx := context.Background()
	var err error
	types.ControlPlaneClusterVersion, err = GetVersionInfoFromCluster(ctx, ClusterLocalName, cfg)
	if err != nil {
		return err
	}
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.DisableBootstrapClusterInfo) {
		clusters, err := NewClusterClient(singleton.KubeClient.Get()).List(ctx)
		if err != nil {
			return errors.Wrap(err, "fail to get registered clusters")
		}
		for _, cluster := range clusters.Items {
			if err = SetClusterVersionInfo(ctx, cfg, cluster.Name); err != nil {
				klog.Warningf("set cluster version for %s: %v, skip it...", cluster.Name, err)
				continue
			}
		}
	}
	return nil
}

// VirtualCluster contains base info of cluster, it unifies the difference between different cluster implementations
// like cluster secret or ocm managed cluster
type VirtualCluster struct {
	Name     string
	Alias    string
	Type     v1alpha1.CredentialType
	EndPoint string
	Accepted bool
	Labels   map[string]string
	Metrics  *ClusterMetrics
	Object   client.Object
}

// FullName the name with alias if available
func (vc *VirtualCluster) FullName() string {
	if vc.Alias != "" {
		return fmt.Sprintf("%s (%s)", vc.Name, vc.Alias)
	}
	return vc.Name
}

func getClusterAlias(o client.Object) string {
	if annots := o.GetAnnotations(); annots != nil {
		return annots[v1alpha1.AnnotationClusterAlias]
	}
	return ""
}

func setClusterAlias(o client.Object, alias string) {
	annots := o.GetAnnotations()
	if annots == nil {
		annots = map[string]string{}
	}
	annots[v1alpha1.AnnotationClusterAlias] = alias
	o.SetAnnotations(annots)
}

// NewVirtualClusterFromLocal return virtual cluster corresponding to local cluster
func NewVirtualClusterFromLocal() *VirtualCluster {
	return &VirtualCluster{
		Name:     ClusterLocalName,
		Type:     types.CredentialTypeInternal,
		EndPoint: types.ClusterBlankEndpoint,
		Accepted: true,
		Labels:   map[string]string{},
		Metrics:  metricsMap[ClusterLocalName],
	}
}

// NewVirtualClusterFromSecret extract virtual cluster from cluster secret
func NewVirtualClusterFromSecret(secret *corev1.Secret) (*VirtualCluster, error) {
	endpoint := string(secret.Data["endpoint"])
	labels := secret.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	if _endpoint, ok := labels[clustercommon.LabelKeyClusterEndpointType]; ok {
		endpoint = _endpoint
	}
	credType, ok := labels[clustercommon.LabelKeyClusterCredentialType]
	if !ok {
		return nil, errors.Errorf("secret is not a valid cluster secret, no credential type found")
	}
	return &VirtualCluster{
		Name:     secret.Name,
		Alias:    getClusterAlias(secret),
		Type:     v1alpha1.CredentialType(credType),
		EndPoint: endpoint,
		Accepted: true,
		Labels:   labels,
		Metrics:  metricsMap[secret.Name],
		Object:   secret,
	}, nil
}

// NewVirtualClusterFromManagedCluster extract virtual cluster from ocm managed cluster
func NewVirtualClusterFromManagedCluster(managedCluster *clusterv1.ManagedCluster) (*VirtualCluster, error) {
	if len(managedCluster.Spec.ManagedClusterClientConfigs) == 0 {
		return nil, errors.Errorf("managed cluster has no client config")
	}
	return &VirtualCluster{
		Name:     managedCluster.Name,
		Alias:    getClusterAlias(managedCluster),
		Type:     types.CredentialTypeOCMManagedCluster,
		EndPoint: types.ClusterBlankEndpoint,
		Accepted: managedCluster.Spec.HubAcceptsClient,
		Labels:   managedCluster.GetLabels(),
		Metrics:  metricsMap[managedCluster.Name],
		Object:   managedCluster,
	}, nil
}

// GetVirtualCluster returns virtual cluster with given clusterName
func GetVirtualCluster(ctx context.Context, c client.Client, clusterName string) (vc *VirtualCluster, err error) {
	if clusterName == ClusterLocalName {
		return NewVirtualClusterFromLocal(), nil
	}
	if ClusterGatewaySecretNamespace == "" {
		ClusterGatewaySecretNamespace = types.DefaultKubeVelaNS
	}
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
	r, err := apilabels.NewRequirement(clustercommon.LabelKeyClusterCredentialType, selection.Exists, nil)
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
	clusters, err := FindVirtualClustersByLabels(ctx, c, map[string]string{})
	if err != nil {
		return nil, err
	}
	return append([]VirtualCluster{*NewVirtualClusterFromLocal()}, clusters...), nil
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
			clusters = append(clusters, *vc)
		}
	}
	return clusters, nil
}

// ClusterNameMapper mapper for cluster names
type ClusterNameMapper interface {
	GetClusterName(string) string
}

type clusterAliasMapper map[string]string

// GetClusterName .
func (cm clusterAliasMapper) GetClusterName(cluster string) string {
	if alias := strings.TrimSpace(cm[cluster]); alias != "" {
		return fmt.Sprintf("%s (%s)", cluster, alias)
	}
	return cluster
}

// NewClusterNameMapper load all clusters and return the mapper of their names
func NewClusterNameMapper(ctx context.Context, c client.Client) (ClusterNameMapper, error) {
	cm := clusterAliasMapper(make(map[string]string))
	clusters := &v1alpha1.VirtualClusterList{}
	if err := c.List(ctx, clusters); err == nil {
		for _, cluster := range clusters.Items {
			cm[cluster.Name] = cluster.Spec.Alias
		}
		return cm, nil
	} else if err != nil && !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
		return nil, err
	}
	vcs, err := ListVirtualClusters(ctx, c)
	if err != nil {
		return nil, err
	}
	for _, cluster := range vcs {
		cm[cluster.Name] = cluster.Alias
	}
	return cm, nil
}

// SetClusterVersionInfo update cluster version info into virtual cluster object
func SetClusterVersionInfo(ctx context.Context, cfg *rest.Config, clusterName string) error {
	cli, err := client.New(cfg, client.Options{Scheme: common.Scheme})
	if err != nil {
		return err
	}
	if clusterName == ClusterLocalName {
		return nil
	}
	vc, err := GetVirtualCluster(ctx, cli, clusterName)
	if err != nil {
		return errors.Wrap(err, "get virtual cluster")
	}
	_, err = getClusterVersionFromObject(vc.Object)
	if err == nil {
		// info already exist
		return nil
	}
	cv, err := GetVersionInfoFromCluster(ctx, clusterName, cfg)
	if err != nil {
		return err
	}
	setClusterVersion(vc.Object, cv)
	klog.Infof("joining cluster %s with version: %s", clusterName, cv.GitVersion)
	return cli.Update(ctx, vc.Object)
}

// GetVersionInfoFromObject will get cluster version info from virtual cluster, it will fall back to control plane cluster version if any error occur
func GetVersionInfoFromObject(ctx context.Context, cli client.Client, clusterName string) types.ClusterVersion {
	vc, err := GetVirtualCluster(ctx, cli, clusterName)
	if err != nil {
		klog.Warningf("get virtual cluster for %s err %v, using control plane cluster version", clusterName, err)
		return types.ControlPlaneClusterVersion
	}
	cv, err := getClusterVersionFromObject(vc.Object)
	if err != nil {
		klog.Warningf("get version info for %s err %v, using control plane cluster version", clusterName, err)
		return types.ControlPlaneClusterVersion
	}
	return cv
}

func setClusterVersion(o client.Object, info types.ClusterVersion) {
	if o == nil {
		return
	}
	ann := o.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	content, _ := json.Marshal(info)
	ann[types.AnnotationClusterVersion] = string(content)
	o.SetAnnotations(ann)
}

func getClusterVersionFromObject(o client.Object) (types.ClusterVersion, error) {
	if o == nil {
		return types.ControlPlaneClusterVersion, nil
	}
	var cv types.ClusterVersion
	ann := o.GetAnnotations()
	if ann == nil {
		return cv, errors.New("no cluster version info")
	}
	versionRaw := ann[types.AnnotationClusterVersion]
	err := json.Unmarshal([]byte(versionRaw), &cv)
	return cv, err
}

// GetVersionInfoFromCluster will add remote cluster version info into secret annotation
func GetVersionInfoFromCluster(ctx context.Context, clusterName string, cfg *rest.Config) (types.ClusterVersion, error) {
	var cv types.ClusterVersion
	content, err := RequestRawK8sAPIForCluster(ctx, "version", clusterName, cfg)
	if err != nil {
		return cv, err
	}
	if err = json.Unmarshal(content, &cv); err != nil {
		return cv, err
	}
	return cv, nil
}

// RequestRawK8sAPIForCluster will request multi-cluster K8s API with raw client, such as /healthz, /version, etc
func RequestRawK8sAPIForCluster(ctx context.Context, path, clusterName string, cfg *rest.Config) ([]byte, error) {
	cfg.GroupVersion = &schema.GroupVersion{Group: "", Version: "v1"}
	cfg.NegotiatedSerializer = scheme.Codecs
	defer func() {
		cfg.GroupVersion = nil
		cfg.NegotiatedSerializer = nil
	}()
	if clusterName == ClusterLocalName {
		restClient, err := rest.RESTClientFor(cfg)
		if err != nil {
			return nil, errors.Wrap(err, "fail to get local cluster")
		}
		return restClient.Get().AbsPath(path).DoRaw(ctx)
	}
	return versioned.NewForConfigOrDie(cfg).ClusterV1alpha1().ClusterGateways().RESTClient(clusterName).Get().AbsPath(path).DoRaw(ctx)
}

// NewClusterClient create virtual cluster client
func NewClusterClient(cli client.Client) v1alpha1.VirtualClusterClient {
	return v1alpha1.NewVirtualClusterClient(cli, ClusterGatewaySecretNamespace, true)
}
