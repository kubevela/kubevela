/*
 Copyright 2022 The KubeVela Authors.

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

package query

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkv1beta1 "k8s.io/api/networking/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apis "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

// GeneratorServiceEndpoints generator service endpoints is available for common component type,
// such as webservice or helm
// it can not support the cloud service component currently
func (h *provider) GeneratorServiceEndpoints(wfctx wfContext.Context, v *value.Value, act types.Action) error {
	ctx := context.Background()

	val, err := v.LookupValue("app")
	if err != nil {
		return err
	}
	opt := Option{}
	if err = val.UnmarshalTo(&opt); err != nil {
		return err
	}
	app := new(v1beta1.Application)
	err = findResource(ctx, h.cli, app, opt.Name, opt.Namespace, "")
	if err != nil {
		return fmt.Errorf("query app failure %w", err)
	}
	serviceEndpoints := make([]querytypes.ServiceEndpoint, 0)
	var clusterGatewayNodeIP = make(map[string]string)
	collector := NewAppCollector(h.cli, opt)
	resources, err := collector.ListApplicationResources(app, true)
	if err != nil {
		return err
	}
	for i, resource := range resources {
		cluster := resources[i].Cluster
		cachedSelectorNodeIP := func() string {
			if ip, exist := clusterGatewayNodeIP[cluster]; exist {
				return ip
			}
			ip := selectorNodeIP(ctx, cluster, h.cli)
			if ip != "" {
				clusterGatewayNodeIP[cluster] = ip
			}
			return ip
		}
		if resource.ResourceTree != nil {
			serviceEndpoints = append(serviceEndpoints, getEndpointFromNode(ctx, h.cli, resource.ResourceTree, resource.Component, cachedSelectorNodeIP)...)
		} else {
			serviceEndpoints = append(serviceEndpoints, getServiceEndpoints(ctx, h.cli, resource.GroupVersionKind(), resource.Name, resource.Namespace, resource.Cluster, resource.Component, cachedSelectorNodeIP)...)
		}
	}
	return fillQueryResult(v, serviceEndpoints, "list")
}

func getEndpointFromNode(ctx context.Context, cli client.Client, node *querytypes.ResourceTreeNode, component string, cachedSelectorNodeIP func() string) []querytypes.ServiceEndpoint {
	if node == nil {
		return nil
	}
	var serviceEndpoints []querytypes.ServiceEndpoint
	serviceEndpoints = append(serviceEndpoints, getServiceEndpoints(ctx, cli, node.GroupVersionKind(), node.Name, node.Namespace, node.Cluster, component, cachedSelectorNodeIP)...)
	for _, child := range node.LeafNodes {
		serviceEndpoints = append(serviceEndpoints, getEndpointFromNode(ctx, cli, child, component, cachedSelectorNodeIP)...)
	}
	return serviceEndpoints
}

func getServiceEndpoints(ctx context.Context, cli client.Client, gvk schema.GroupVersionKind, name, namespace, cluster, component string, cachedSelectorNodeIP func() string) []querytypes.ServiceEndpoint {
	var serviceEndpoints []querytypes.ServiceEndpoint
	switch gvk.Kind {
	case "Ingress":
		if gvk.Group == networkv1beta1.GroupName && (gvk.Version == "v1beta1" || gvk.Version == "v1") {
			var ingress networkv1beta1.Ingress
			ingress.SetGroupVersionKind(gvk)
			if err := findResource(ctx, cli, &ingress, name, namespace, cluster); err != nil {
				klog.Error(err, fmt.Sprintf("find v1 Ingress %s/%s from cluster %s failure", name, namespace, cluster))
				return nil
			}
			serviceEndpoints = append(serviceEndpoints, generatorFromIngress(ingress, cluster, component)...)
		} else {
			klog.Warning("not support ingress version", "version", gvk)
		}
	case "Service":
		var service corev1.Service
		service.SetGroupVersionKind(gvk)
		if err := findResource(ctx, cli, &service, name, namespace, cluster); err != nil {
			klog.Error(err, fmt.Sprintf("find v1 Service %s/%s from cluster %s failure", name, namespace, cluster))
			return nil
		}
		serviceEndpoints = append(serviceEndpoints, generatorFromService(service, cachedSelectorNodeIP, cluster, component, "")...)
	case helmapi.HelmReleaseGVK.Kind:
		obj := new(unstructured.Unstructured)
		obj.SetNamespace(namespace)
		obj.SetName(name)
		hc := NewHelmReleaseCollector(cli, obj)
		services, err := hc.CollectServices(ctx, cluster)
		if err != nil {
			klog.Error(err, "collect service by helm release failure", "helmRelease", name, "namespace", namespace, "cluster", cluster)
			return nil
		}
		for _, service := range services {
			serviceEndpoints = append(serviceEndpoints, generatorFromService(service, cachedSelectorNodeIP, cluster, component, "")...)
		}
		ingresses, err := hc.CollectIngress(ctx, cluster)
		if err != nil {
			klog.Error(err, "collect ingres by helm release failure", "helmRelease", name, "namespace", namespace, "cluster", cluster)
			return nil
		}
		for _, uns := range ingresses {
			var ingress networkv1beta1.Ingress
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(uns.UnstructuredContent(), &ingress); err != nil {
				klog.Errorf("fail to convert unstructured to ingress %s", err.Error())
				continue
			}
			serviceEndpoints = append(serviceEndpoints, generatorFromIngress(ingress, cluster, component)...)
		}
	case "SeldonDeployment":
		obj := new(unstructured.Unstructured)
		obj.SetGroupVersionKind(gvk)
		if err := findResource(ctx, cli, obj, name, namespace, cluster); err != nil {
			klog.Error(err, fmt.Sprintf("find v1 Seldon Deployment %s/%s from cluster %s failure", name, namespace, cluster))
			return nil
		}
		anno := obj.GetAnnotations()
		serviceName := "ambassador"
		serviceNS := apis.DefaultKubeVelaNS
		if anno != nil {
			if anno[annoAmbassadorServiceName] != "" {
				serviceName = anno[annoAmbassadorServiceName]
			}
			if anno[annoAmbassadorServiceNamespace] != "" {
				serviceNS = anno[annoAmbassadorServiceNamespace]
			}
		}
		var service corev1.Service
		if err := findResource(ctx, cli, &service, serviceName, serviceNS, cluster); err != nil {
			klog.Error(err, fmt.Sprintf("find v1 Service %s/%s from cluster %s failure", serviceName, serviceNS, cluster))
			return nil
		}
		serviceEndpoints = append(serviceEndpoints, generatorFromService(service, cachedSelectorNodeIP, cluster, component, fmt.Sprintf("/seldon/%s/%s", namespace, name))...)
	case "HTTPRoute":
		var route gatewayv1alpha2.HTTPRoute
		route.SetGroupVersionKind(gvk)
		if err := findResource(ctx, cli, &route, name, namespace, cluster); err != nil {
			klog.Error(err, fmt.Sprintf("find HTTPRoute %s/%s from cluster %s failure", name, namespace, cluster))
			return nil
		}
		serviceEndpoints = append(serviceEndpoints, generatorFromHTTPRoute(ctx, cli, route, cluster, component)...)
	}
	return serviceEndpoints
}

func findResource(ctx context.Context, cli client.Client, obj client.Object, name, namespace, cluster string) error {
	obj.SetNamespace(namespace)
	obj.SetName(name)
	gctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	if err := cli.Get(multicluster.ContextWithClusterName(gctx, cluster),
		client.ObjectKeyFromObject(obj), obj); err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func generatorFromService(service corev1.Service, selectorNodeIP func() string, cluster, component, path string) []querytypes.ServiceEndpoint {
	var serviceEndpoints []querytypes.ServiceEndpoint

	var objRef = corev1.ObjectReference{
		Kind:            "Service",
		Namespace:       service.ObjectMeta.Namespace,
		Name:            service.ObjectMeta.Name,
		UID:             service.UID,
		APIVersion:      service.APIVersion,
		ResourceVersion: service.ResourceVersion,
	}

	formatEndpoint := func(host, appProtocol string, portProtocol corev1.Protocol, portNum int32, inner bool) querytypes.ServiceEndpoint {
		return querytypes.ServiceEndpoint{
			Endpoint: querytypes.Endpoint{
				Protocol:    portProtocol,
				AppProtocol: &appProtocol,
				Host:        host,
				Port:        int(portNum),
				Path:        path,
				Inner:       inner,
			},
			Ref:       objRef,
			Cluster:   cluster,
			Component: component,
		}
	}
	switch service.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		for _, port := range service.Spec.Ports {
			appp := judgeAppProtocol(port.Port)
			for _, ingress := range service.Status.LoadBalancer.Ingress {
				if ingress.Hostname != "" {
					serviceEndpoints = append(serviceEndpoints, formatEndpoint(ingress.Hostname, appp, port.Protocol, port.Port, false))
				}
				if ingress.IP != "" {
					serviceEndpoints = append(serviceEndpoints, formatEndpoint(ingress.IP, appp, port.Protocol, port.Port, false))
				}
			}
		}
	case corev1.ServiceTypeNodePort:
		for _, port := range service.Spec.Ports {
			appp := judgeAppProtocol(port.Port)
			serviceEndpoints = append(serviceEndpoints, formatEndpoint(selectorNodeIP(), appp, port.Protocol, port.NodePort, false))
		}
	case corev1.ServiceTypeClusterIP, corev1.ServiceTypeExternalName:
		for _, port := range service.Spec.Ports {
			appp := judgeAppProtocol(port.Port)
			serviceEndpoints = append(serviceEndpoints, formatEndpoint(fmt.Sprintf("%s.%s", service.Name, service.Namespace), appp, port.Protocol, port.Port, true))
		}
	}
	return serviceEndpoints
}

func generatorFromIngress(ingress networkv1beta1.Ingress, cluster, component string) (serviceEndpoints []querytypes.ServiceEndpoint) {
	getAppProtocol := func(host string) string {
		if len(ingress.Spec.TLS) > 0 {
			for _, tls := range ingress.Spec.TLS {
				if len(tls.Hosts) > 0 && utils.StringsContain(tls.Hosts, host) {
					return querytypes.HTTPS
				}
				if len(tls.Hosts) == 0 {
					return querytypes.HTTPS
				}
			}
		}
		return querytypes.HTTP
	}
	// It depends on the Ingress Controller
	getEndpointPort := func(appProtocol string) int {
		if appProtocol == querytypes.HTTPS {
			if port, err := strconv.Atoi(ingress.Annotations[apis.AnnoIngressControllerHTTPSPort]); port > 0 && err == nil {
				return port
			}
			return 443
		}
		if port, err := strconv.Atoi(ingress.Annotations[apis.AnnoIngressControllerHTTPPort]); port > 0 && err == nil {
			return port
		}
		return 80
	}

	// The host in rule maybe empty, means access the application by the Gateway Host(IP)
	getHost := func(host string) string {
		if host != "" {
			return host
		}
		return ingress.Annotations[apis.AnnoIngressControllerHost]
	}

	for _, rule := range ingress.Spec.Rules {
		var appProtocol = getAppProtocol(rule.Host)
		var appPort = getEndpointPort(appProtocol)
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				serviceEndpoints = append(serviceEndpoints, querytypes.ServiceEndpoint{
					Endpoint: querytypes.Endpoint{
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: &appProtocol,
						Host:        getHost(rule.Host),
						Path:        path.Path,
						Port:        appPort,
					},
					Ref: corev1.ObjectReference{
						Kind:            "Ingress",
						Namespace:       ingress.ObjectMeta.Namespace,
						Name:            ingress.ObjectMeta.Name,
						UID:             ingress.UID,
						APIVersion:      ingress.APIVersion,
						ResourceVersion: ingress.ResourceVersion,
					},
					Cluster:   cluster,
					Component: component,
				})
			}
		}
	}
	return serviceEndpoints
}

func getGatewayPortAndProtocol(ctx context.Context, cli client.Client, defaultNamespace, cluster string, parents []gatewayv1alpha2.ParentRef) (string, int) {
	for _, parent := range parents {
		if parent.Kind != nil && *parent.Kind == "Gateway" {
			var gateway gatewayv1alpha2.Gateway
			namespace := defaultNamespace
			if parent.Namespace != nil {
				namespace = string(*parent.Namespace)
			}
			if err := findResource(ctx, cli, &gateway, string(parent.Name), namespace, cluster); err != nil {
				log.Logger.Errorf("query the Gateway %s/%s/%s failure %s", cluster, namespace, string(parent.Name), err.Error())
			}
			for _, listener := range gateway.Spec.Listeners {
				if listener.Name == *parent.SectionName {
					var protocol = querytypes.HTTP
					if listener.Protocol == gatewayv1alpha2.HTTPSProtocolType {
						protocol = querytypes.HTTPS
					}
					var port = int(listener.Port)
					// The gateway listener port may not be the externally exposed port.
					// For example, the traefik addon has a default port mapping configuration of 8443->443 8000->80
					// So users could set the `ports-mapping` annotation.
					if mapping := gateway.Annotations["ports-mapping"]; mapping != "" {
						fmt.Println(mapping)
						for _, portItem := range strings.Split(mapping, ",") {
							if portMap := strings.Split(portItem, ":"); len(portMap) == 2 {
								if portMap[0] == fmt.Sprintf("%d", listener.Port) {
									newPort, err := strconv.Atoi(portMap[1])
									if err == nil {
										port = newPort
									}
								}
							}
						}
					}
					return protocol, port
				}
			}
		}
	}
	return querytypes.HTTP, 80
}

func generatorFromHTTPRoute(ctx context.Context, cli client.Client, route gatewayv1alpha2.HTTPRoute, cluster, component string) []querytypes.ServiceEndpoint {
	existPath := make(map[string]bool)
	var serviceEndpoints []querytypes.ServiceEndpoint
	for _, rule := range route.Spec.Rules {
		for _, host := range route.Spec.Hostnames {
			appProtocol, appPort := getGatewayPortAndProtocol(ctx, cli, route.Namespace, cluster, route.Spec.ParentRefs)
			for _, match := range rule.Matches {
				path := ""
				if match.Path != nil && (match.Path.Type == nil || string(*match.Path.Type) == string(gatewayv1alpha2.PathMatchPathPrefix)) {
					path = *match.Path.Value
				}
				if !existPath[path] {
					existPath[path] = true
					serviceEndpoints = append(serviceEndpoints, querytypes.ServiceEndpoint{
						Endpoint: querytypes.Endpoint{
							Protocol:    corev1.ProtocolTCP,
							AppProtocol: &appProtocol,
							Host:        string(host),
							Path:        path,
							Port:        appPort,
						},
						Ref: corev1.ObjectReference{
							Kind:            route.Kind,
							Namespace:       route.ObjectMeta.Namespace,
							Name:            route.ObjectMeta.Name,
							UID:             route.UID,
							APIVersion:      route.APIVersion,
							ResourceVersion: route.ResourceVersion,
						},
						Cluster:   cluster,
						Component: component,
					})
				}
			}
		}
	}
	return serviceEndpoints
}

func selectorNodeIP(ctx context.Context, clusterName string, client client.Client) string {
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	var nodes corev1.NodeList
	if err := client.List(multicluster.ContextWithClusterName(ctx, clusterName), &nodes); err != nil {
		return ""
	}
	if len(nodes.Items) == 0 {
		return ""
	}
	var gatewayNode *corev1.Node
	var workerNodes []corev1.Node
	for i, node := range nodes.Items {
		if _, exist := node.Labels[apis.LabelNodeRoleGateway]; exist {
			gatewayNode = &nodes.Items[i]
			break
		} else if _, exist := node.Labels[apis.LabelNodeRoleWorker]; exist {
			workerNodes = append(workerNodes, nodes.Items[i])
		}
	}
	if gatewayNode == nil && len(workerNodes) > 0 {
		gatewayNode = &workerNodes[0]
	}
	if gatewayNode == nil {
		gatewayNode = &nodes.Items[0]
	}
	if gatewayNode != nil {
		var addressMap = make(map[corev1.NodeAddressType]string)
		for _, address := range gatewayNode.Status.Addresses {
			addressMap[address.Type] = address.Address
		}
		// first get external ip
		if ip, exist := addressMap[corev1.NodeExternalIP]; exist {
			return ip
		}
		if ip, exist := addressMap[corev1.NodeInternalIP]; exist {
			return ip
		}
	}
	return ""
}

// judgeAppProtocol  RFC-6335 and http://www.iana.org/assignments/service-names).
func judgeAppProtocol(port int32) string {
	switch port {
	case 80, 8080:
		return querytypes.HTTP
	case 443:
		return querytypes.HTTPS
	case 3306:
		return querytypes.Mysql
	case 6379:
		return querytypes.Redis
	default:
		return ""
	}
}
