/*
Copyright 2026 The KubeVela Authors.

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
	"bytes"
	"context"
	"fmt"

	errors2 "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/servingcert"
)

// EnsureClusterGatewayTLS makes sure that the cluster-gateway serving cert in
// the Secret <secretNamespace>/<secretName> is valid. It also makes sure that
// the cluster-gateway APIService trusts its CA. It returns the PEM CA bundle
// that clients must trust. You can call it periodically. It renews the cert
// when necessary and repairs a caBundle that something changed or removed.
//
// Older charts rendered the APIService with a placeholder caBundle, and a
// post-install Job patched it later. Since Kubernetes 1.36, the aggregator
// uses the caBundle to verify TLS when it checks availability
// (kubernetes/kubernetes#137157). This causes a deadlock in
// `helm install --wait`:
//  1. vela-core waits for the APIService to become available.
//  2. Helm waits for vela-core before it runs the Job.
//
// Call this function before vela-core waits for the APIService. This removes
// the deadlock.
func EnsureClusterGatewayTLS(ctx context.Context, c client.Client, secretNamespace, secretName string) ([]byte, error) {
	apiService := &apiregistrationv1.APIService{}
	if err := c.Get(ctx, types.NamespacedName{Name: ClusterGatewayAPIServiceName}, apiService); err != nil {
		return nil, errors2.Wrapf(err, "failed to get ClusterGateway APIService %s", ClusterGatewayAPIServiceName)
	}
	if apiService.Spec.Service == nil {
		return nil, fmt.Errorf("ClusterGateway APIService %s has no service, cannot set up its TLS", ClusterGatewayAPIServiceName)
	}
	svc := servingcert.Service{Namespace: apiService.Spec.Service.Namespace, Name: apiService.Spec.Service.Name}

	res, err := servingcert.EnsureSecret(ctx, c, secretNamespace, secretName, svc)
	if err != nil {
		return nil, err
	}
	if err = ensureClusterGatewayCABundle(ctx, c, res.CABundle); err != nil {
		return nil, err
	}
	return res.CABundle, nil
}

func ensureClusterGatewayCABundle(ctx context.Context, c client.Client, ca []byte) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		apiService := &apiregistrationv1.APIService{}
		if err := c.Get(ctx, types.NamespacedName{Name: ClusterGatewayAPIServiceName}, apiService); err != nil {
			return err
		}
		if bytes.Equal(apiService.Spec.CABundle, ca) && !apiService.Spec.InsecureSkipTLSVerify {
			return nil
		}
		apiService.Spec.CABundle = ca
		apiService.Spec.InsecureSkipTLSVerify = false
		if err := c.Update(ctx, apiService); err != nil {
			return err
		}
		klog.InfoS("Set the caBundle of the ClusterGateway APIService", "apiService", ClusterGatewayAPIServiceName)
		return nil
	})
	return errors2.Wrapf(err, "failed to set the caBundle of ClusterGateway APIService %s", ClusterGatewayAPIServiceName)
}
