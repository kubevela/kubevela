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

	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/oam-dev/cluster-gateway/pkg/generated/clientset/versioned"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// InitClusterInfo will initialize control plane cluster info
func InitClusterInfo(ctx context.Context, cfg *rest.Config) error {
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
	vcs, err := NewClusterClient(c).List(ctx)
	if err != nil {
		return nil, err
	}
	for _, cluster := range vcs.Items {
		cm[cluster.Name] = cluster.Spec.Alias
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
	vc, err := NewClusterClient(cli).Get(ctx, clusterName)
	if err != nil {
		return errors.Wrap(err, "get virtual cluster")
	}
	_, err = getClusterVersionFromObject(vc.Raw)
	if err == nil {
		// info already exist
		return nil
	}
	cv, err := GetVersionInfoFromCluster(ctx, clusterName, cfg)
	if err != nil {
		return err
	}
	bs, _ := json.Marshal(cv)
	_ = k8s.AddAnnotation(vc.Raw, v1alpha1.AnnotationClusterVersion, string(bs))
	klog.Infof("joining cluster %s with version: %s", clusterName, cv.GitVersion)
	return cli.Update(ctx, vc.Raw)
}

// GetVersionInfoFromObject will get cluster version info from virtual cluster, it will fall back to control plane cluster version if any error occur
func GetVersionInfoFromObject(ctx context.Context, cli client.Client, clusterName string) version.Info {
	vc, err := NewClusterClient(cli).Get(ctx, clusterName)
	if err != nil {
		klog.Warningf("get virtual cluster for %s err %v, using control plane cluster version", clusterName, err)
		return types.ControlPlaneClusterVersion
	}
	cv, err := getClusterVersionFromObject(vc.Raw)
	if err != nil {
		klog.Warningf("get version info for %s err %v, using control plane cluster version", clusterName, err)
		return types.ControlPlaneClusterVersion
	}
	return cv
}

func getClusterVersionFromObject(o client.Object) (version.Info, error) {
	if o == nil {
		return types.ControlPlaneClusterVersion, nil
	}
	var cv version.Info
	err := json.Unmarshal([]byte(k8s.GetAnnotation(o, v1alpha1.AnnotationClusterVersion)), &cv)
	return cv, err
}

// GetVersionInfoFromCluster will add remote cluster version info into secret annotation
func GetVersionInfoFromCluster(ctx context.Context, clusterName string, cfg *rest.Config) (version.Info, error) {
	var cv version.Info
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
