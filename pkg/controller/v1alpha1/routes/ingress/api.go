package ingress

import (
	"fmt"

	"k8s.io/api/networking/v1beta1"

	standardv1alpha1 "github.com/oam-dev/kubevela/api/v1alpha1"
)

const TypeNginx = "nginx"

type RouteIngress interface {
	Construct(routeTrait *standardv1alpha1.Route) []*v1beta1.Ingress
}

func GetRouteIngress(provider string) (RouteIngress, error) {
	var routeIngress RouteIngress
	switch provider {
	case TypeNginx, "":
		routeIngress = &Nginx{}
	default:
		return nil, fmt.Errorf("unknow route ingress provider '%v', only '%s' is supported now", provider, TypeNginx)
	}
	return routeIngress, nil
}
