package ingress

import (
	"fmt"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"k8s.io/api/networking/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	standardv1alpha1 "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// TypeNginx is a type of route implementation using [Nginx-Ingress](https://github.com/kubernetes/ingress-nginx)
const TypeNginx = "nginx"

// TypeContour is a type of route implementation using [contour ingress](https://github.com/projectcontour/contour)
const TypeContour = "contour"

const (
	// StatusReady represents status is ready
	StatusReady = "Ready"
	// StatusSynced represents status is synced, this mean the controller has reconciled but not ready
	StatusSynced = "Synced"
)

// RouteIngress is an interface of route ingress implementation
type RouteIngress interface {
	Construct(routeTrait *standardv1alpha1.Route) []*v1beta1.Ingress
	CheckStatus(routeTrait *standardv1alpha1.Route) (string, []runtimev1alpha1.Condition)
}

// GetRouteIngress will get real implementation from type, we could support more in the future.
func GetRouteIngress(provider string, client client.Client) (RouteIngress, error) {
	var routeIngress RouteIngress
	switch provider {
	case TypeNginx, "":
		routeIngress = &Nginx{Client: client}
	case TypeContour:
		routeIngress = &Contour{Client: client}
	default:
		return nil, fmt.Errorf("unknow route ingress provider '%v', only '%s' is supported now", provider, TypeNginx)
	}
	return routeIngress, nil
}
