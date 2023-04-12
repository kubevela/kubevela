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
	"time"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"
	errors2 "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	errors3 "github.com/oam-dev/kubevela/pkg/utils/errors"
)

const (
	// ClusterLocalName specifies the local cluster
	ClusterLocalName = pkgmulticluster.Local
)

var (
	// ClusterGatewaySecretNamespace the namespace where cluster-gateway secret locates
	ClusterGatewaySecretNamespace = "vela-system"
)

// ClusterNameInContext extract cluster name from context
func ClusterNameInContext(ctx context.Context) string {
	cluster, _ := pkgmulticluster.ClusterFrom(ctx)
	return cluster
}

// ContextWithClusterName create context with multi-cluster by cluster name
func ContextWithClusterName(ctx context.Context, clusterName string) context.Context {
	return pkgmulticluster.WithCluster(ctx, clusterName)
}

// ContextInLocalCluster create context in local cluster
func ContextInLocalCluster(ctx context.Context) context.Context {
	return pkgmulticluster.WithCluster(ctx, ClusterLocalName)
}

// ResourcesWithClusterName set cluster name for resources
func ResourcesWithClusterName(clusterName string, objs ...*unstructured.Unstructured) []*unstructured.Unstructured {
	var _objs []*unstructured.Unstructured
	for _, obj := range objs {
		if obj != nil {
			if oam.GetCluster(obj) == "" {
				oam.SetCluster(obj, clusterName)
			}
			_objs = append(_objs, obj)
		}
	}
	return _objs
}

// GetClusterGatewayService get cluster gateway backend service reference
// if service is ready, service is returned and no error is returned
// if service exists but is not ready, both service and error are returned
// if service does not exist, only error is returned
func GetClusterGatewayService(ctx context.Context, c client.Client) (*apiregistrationv1.ServiceReference, error) {
	gv := v1alpha1.SchemeGroupVersion
	apiService := &apiregistrationv1.APIService{}
	apiServiceName := gv.Version + "." + gv.Group
	if err := c.Get(ctx, types.NamespacedName{Name: apiServiceName}, apiService); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("ClusterGateway APIService %s is not found", apiServiceName)
		}
		return nil, errors2.Wrapf(err, "failed to get ClusterGateway APIService %s", apiServiceName)
	}
	if apiService.Spec.Service == nil {
		return nil, fmt.Errorf("ClusterGateway APIService should use the service exposed by dedicated apiserver instead of being handled locally")
	}
	svc := apiService.Spec.Service
	status := apiregistrationv1.ConditionUnknown
	for _, condition := range apiService.Status.Conditions {
		if condition.Type == apiregistrationv1.Available {
			status = condition.Status
		}
	}
	if status == apiregistrationv1.ConditionTrue {
		return svc, nil
	}
	return svc, fmt.Errorf("ClusterGateway APIService (%s/%s:%d) is not ready, current status: %s", svc.Namespace, svc.Name, svc.Port, status)
}

// WaitUntilClusterGatewayReady wait cluster gateway service to be ready to serve
func WaitUntilClusterGatewayReady(ctx context.Context, c client.Client, maxRetry int, interval time.Duration) (svc *apiregistrationv1.ServiceReference, err error) {
	for i := 0; i < maxRetry; i++ {
		if svc, err = GetClusterGatewayService(ctx, c); err != nil {
			klog.Infof("waiting for cluster gateway service: %v", err)
			time.Sleep(interval)
		} else {
			return
		}
	}
	return nil, errors2.Wrapf(err, "failed to wait cluster gateway service (retry=%d)", maxRetry)
}

// Initialize prepare multicluster environment by checking cluster gateway service in clusters and hack rest config to use cluster gateway
// if cluster gateway service is not ready, it will wait up to 5 minutes
func Initialize(restConfig *rest.Config, autoUpgrade bool) (client.Client, error) {
	c, err := client.New(restConfig, client.Options{Scheme: common.Scheme})
	if err != nil {
		return nil, errors2.Wrapf(err, "unable to get client to find cluster gateway service")
	}
	svc, err := WaitUntilClusterGatewayReady(context.Background(), c, 60, 5*time.Second)
	if err != nil {
		return nil, ErrDetectClusterGateway
	}
	ClusterGatewaySecretNamespace = svc.Namespace
	if autoUpgrade {
		if err = UpgradeExistingClusterSecret(context.Background(), c); err != nil {
			// this error do not affect the running of current version
			klog.ErrorS(err, "error encountered while grading existing cluster secret to the latest version")
		}
	}
	return c, nil
}

// UpgradeExistingClusterSecret upgrade outdated cluster secrets in v1.1.1 to latest
func UpgradeExistingClusterSecret(ctx context.Context, c client.Client) error {
	const outdatedClusterCredentialLabelKey = "cluster.core.oam.dev/cluster-credential"
	secrets := &v1.SecretList{}
	if err := c.List(ctx, secrets, client.InNamespace(ClusterGatewaySecretNamespace), client.HasLabels{outdatedClusterCredentialLabelKey}); err != nil {
		if err != nil {
			return errors2.Wrapf(err, "failed to find outdated cluster secrets to do upgrade")
		}
	}
	errs := errors3.ErrorList{}
	for _, item := range secrets.Items {
		credType := item.Labels[clustercommon.LabelKeyClusterCredentialType]
		if credType == "" && item.Type == v1.SecretTypeTLS {
			item.Labels[clustercommon.LabelKeyClusterCredentialType] = string(v1alpha1.CredentialTypeX509Certificate)
			if err := c.Update(ctx, item.DeepCopy()); err != nil {
				errs = append(errs, errors2.Wrapf(err, "failed to update outdated secret %s", item.Name))
			}
		}
	}
	if errs.HasError() {
		return errs
	}
	return nil
}

// ListExistingClusterSecrets list existing cluster secrets
func ListExistingClusterSecrets(ctx context.Context, c client.Client) ([]v1.Secret, error) {
	secrets := &v1.SecretList{}
	if err := c.List(ctx, secrets, client.InNamespace(ClusterGatewaySecretNamespace), client.HasLabels{clustercommon.LabelKeyClusterCredentialType}); err != nil {
		return nil, errors2.Wrapf(err, "failed to list cluster secrets")
	}
	return secrets.Items, nil
}
