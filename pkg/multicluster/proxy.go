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
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/klog/v2"

	clusterapi "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

type ContextKey string

const (
	// ClusterContextKey is the name of cluster using in client http context
	ClusterContextKey ContextKey = "ClusterName"
	// ClusterLabelKey specifies which cluster the target k8s object should locate
	ClusterLabelKey = "cluster.oam.dev/clusterName"
	// ApplicationClusterLabelKey specifies which cluster the target application should place its resources
	ApplicationClusterLabelKey = "app.cluster.oam.dev/clusterName"
)

type secretMultiClusterRoundTripper struct {
	rt http.RoundTripper
}

// NewSecretModeMultiClusterRoundTripper will re-write the API path to one of the multi-cluster resource for a request if context has the value
func NewSecretModeMultiClusterRoundTripper(rt http.RoundTripper) http.RoundTripper {
	return &secretMultiClusterRoundTripper{
		rt: rt,
	}
}

// FormatProxyURL will format the request API path by the cluster gateway resources rule
func FormatProxyURL(clusterName, originalPath string) string {
	originalPath = strings.TrimPrefix(originalPath, "/")
	return strings.Join([]string{"/apis", clusterapi.SchemeGroupVersion.Group, clusterapi.SchemeGroupVersion.Version, "clustergateways", clusterName, "proxy", originalPath}, "/")
}

// RoundTrip is the main function for the re-write API path logic
func (rt *secretMultiClusterRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	clusterName, ok := ctx.Value(ClusterContextKey).(string)
	if !ok || clusterName == "" {
		return rt.rt.RoundTrip(req)
	}

	req.URL.Path = FormatProxyURL(clusterName, req.URL.Path)
	return rt.rt.RoundTrip(req)
}

func tryCancelRequest(rt http.RoundTripper, req *http.Request) {
	type canceler interface {
		CancelRequest(*http.Request)
	}
	switch rt := rt.(type) {
	case canceler:
		rt.CancelRequest(req)
	case utilnet.RoundTripperWrapper:
		tryCancelRequest(rt.WrappedRoundTripper(), req)
	default:
		klog.Warningf("Unable to cancel request for %T", rt)
	}
}

// CancelRequest will try cancel request with the inner round tripper
func (rt *secretMultiClusterRoundTripper) CancelRequest(req *http.Request) {
	tryCancelRequest(rt.WrappedRoundTripper(), req)
}

// WrappedRoundTripper can get the wrapped RoundTripper
func (rt *secretMultiClusterRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.rt }

// Context create context with multi-cluster
func Context(ctx context.Context, obj *unstructured.Unstructured) context.Context {
	return ContextWithClusterName(ctx, obj.GetLabels()[ClusterLabelKey])
}

// ContextWithClusterName create context with multi-cluster by cluster name
func ContextWithClusterName(ctx context.Context, clusterName string) context.Context {
	return context.WithValue(ctx, ClusterContextKey, clusterName)
}

// ContextForApplicationResource create context with multi-cluster for application resource
func ContextForApplicationResource(ctx context.Context, application *v1beta1.Application) context.Context {
	return context.WithValue(ctx, ClusterLabelKey, application.GetLabels()[ApplicationClusterLabelKey])
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
