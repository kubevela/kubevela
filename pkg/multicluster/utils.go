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

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	errors2 "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

type contextKey string

const (
	// ClusterContextKey is the name of cluster using in client http context
	ClusterContextKey = contextKey("ClusterName")
	// ClusterLabelKey specifies which cluster the target k8s object should locate
	ClusterLabelKey = "cluster.oam.dev/clusterName"
	// ClusterLocalName specifies the local cluster
	ClusterLocalName = "local"
)

var (
	// ClusterGatewaySecretNamespace the namespace where cluster-gateway secret locates
	ClusterGatewaySecretNamespace string
)

// Context create context with multi-cluster
func Context(ctx context.Context, obj *unstructured.Unstructured) context.Context {
	return ContextWithClusterName(ctx, obj.GetLabels()[ClusterLabelKey])
}

// ContextWithClusterName create context with multi-cluster by cluster name
func ContextWithClusterName(ctx context.Context, clusterName string) context.Context {
	return context.WithValue(ctx, ClusterContextKey, clusterName)
}

// SetClusterName set cluster name for object
func SetClusterName(obj *unstructured.Unstructured, clusterName string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[ClusterLabelKey] = clusterName
	obj.SetLabels(labels)
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
func Initialize(restConfig *rest.Config) error {
	c, err := client.New(restConfig, client.Options{Scheme: common.Scheme})
	if err != nil {
		return errors2.Wrapf(err, "unable to get client to find cluster gateway service")
	}
	svc, err := WaitUntilClusterGatewayReady(context.Background(), c, 60, 5*time.Second)
	if err != nil {
		return errors2.Wrapf(err, "failed to wait for cluster gateway, unable to use multi-cluster")
	}
	ClusterGatewaySecretNamespace = svc.Namespace
	klog.Infof("find cluster gateway service %s/%s:%d", svc.Namespace, svc.Name, *svc.Port)
	restConfig.Wrap(NewSecretModeMultiClusterRoundTripper)
	return nil
}
