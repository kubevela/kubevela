/*
 Copyright 2021. The KubeVela Authors.

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
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networkv1beta1 "k8s.io/api/networking/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apis "github.com/oam-dev/kubevela/apis/types"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "query"
	// HelmReleaseKind is the kind of HelmRelease
	HelmReleaseKind = "HelmRelease"

	annoAmbassadorServiceName      = "ambassador.service/name"
	annoAmbassadorServiceNamespace = "ambassador.service/namespace"
)

var fluxcdGroupVersion = schema.GroupVersion{Group: "helm.toolkit.fluxcd.io", Version: "v2beta1"}

type provider struct {
	cli client.Client
	cfg *rest.Config
}

// Resource refer to an object with cluster info
type Resource struct {
	Cluster   string                     `json:"cluster"`
	Component string                     `json:"component"`
	Revision  string                     `json:"revision"`
	Object    *unstructured.Unstructured `json:"object"`
}

// Option is the query option
type Option struct {
	Name      string       `json:"name"`
	Namespace string       `json:"namespace"`
	Filter    FilterOption `json:"filter,omitempty"`
	// WithStatus means query the object from the cluster and get the latest status
	// This field only suitable for ListResourcesInApp
	WithStatus bool `json:"withStatus,omitempty"`
}

// FilterOption filter resource created by component
type FilterOption struct {
	Cluster          string   `json:"cluster,omitempty"`
	ClusterNamespace string   `json:"clusterNamespace,omitempty"`
	Components       []string `json:"components,omitempty"`
	APIVersion       string   `json:"apiVersion,omitempty"`
	Kind             string   `json:"kind,omitempty"`
}

// ListResourcesInApp lists CRs created by Application, this provider queries the object data.
func (h *provider) ListResourcesInApp(ctx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("app")
	if err != nil {
		return err
	}
	opt := Option{}
	if err = val.UnmarshalTo(&opt); err != nil {
		return err
	}
	collector := NewAppCollector(h.cli, opt)
	appResList, err := collector.CollectResourceFromApp()
	if err != nil {
		return v.FillObject(err.Error(), "err")
	}
	if appResList == nil {
		appResList = make([]Resource, 0)
	}
	return fillQueryResult(v, appResList, "list")
}

// ListAppliedResources list applied resource from tracker, this provider only queries the metadata.
func (h *provider) ListAppliedResources(ctx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("app")
	if err != nil {
		return err
	}
	opt := Option{}
	if err = val.UnmarshalTo(&opt); err != nil {
		return err
	}
	collector := NewAppCollector(h.cli, opt)
	app := new(v1beta1.Application)
	appKey := client.ObjectKey{Name: opt.Name, Namespace: opt.Namespace}
	if err := h.cli.Get(context.Background(), appKey, app); err != nil {
		return v.FillObject(err.Error(), "err")
	}
	appResList, err := collector.ListApplicationResources(app)
	if err != nil {
		return v.FillObject(err.Error(), "err")
	}
	if appResList == nil {
		appResList = make([]*querytypes.AppliedResource, 0)
	}
	return fillQueryResult(v, appResList, "list")
}

// GetApplicationResourceTree get resource tree of application
func (h *provider) GetApplicationResourceTree(ctx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("app")
	if err != nil {
		return err
	}
	opt := Option{}
	if err = val.UnmarshalTo(&opt); err != nil {
		return err
	}
	collector := NewAppCollector(h.cli, opt)
	app := new(v1beta1.Application)
	appKey := client.ObjectKey{Name: opt.Name, Namespace: opt.Namespace}
	if err := h.cli.Get(context.Background(), appKey, app); err != nil {
		return v.FillObject(err.Error(), "err")
	}
	appResList, err := collector.ListApplicationResources(app)
	if err != nil {
		return v.FillObject(err.Error(), "err")
	}
	// merge user defined customize rule before every request.
	err = mergeCustomRules(context.Background(), h.cli)
	if err != nil {
		return err
	}
	for _, resource := range appResList {
		root := querytypes.ResourceTreeNode{
			APIVersion: resource.APIVersion,
			Kind:       resource.Kind,
			Cluster:    resource.Cluster,
			Namespace:  resource.Namespace,
			Name:       resource.Name,
			UID:        resource.UID,
		}
		root.LeafNodes, err = iteratorChildResources(context.Background(), resource.Cluster, h.cli, root, 1)
		if err != nil {
			// if the resource has been deleted, continue access next appliedResource don't break the whole request
			if kerrors.IsNotFound(err) {
				continue
			}
			return v.FillObject(err.Error(), "err")
		}
		rootObject, err := fetchObjectWithResourceTreeNode(context.Background(), resource.Cluster, h.cli, root)
		if err != nil {
			return v.FillObject(err.Error(), "err")
		}
		rootStatus, err := checkResourceStatus(*rootObject)
		if err != nil {
			return v.FillObject(err.Error(), "err")
		}
		root.HealthStatus = *rootStatus
		addInfo, err := additionalInfo(*rootObject)
		if err != nil {
			return err
		}
		root.AdditionalInfo = addInfo
		root.CreationTimestamp = rootObject.GetCreationTimestamp().Time
		if !rootObject.GetDeletionTimestamp().IsZero() {
			root.DeletionTimestamp = rootObject.GetDeletionTimestamp().Time
		}
		resource.ResourceTree = &root
	}
	if appResList == nil {
		appResList = []*querytypes.AppliedResource{}
	}
	return fillQueryResult(v, appResList, "list")
}

func (h *provider) CollectPods(ctx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("value")
	if err != nil {
		return err
	}
	cluster, err := v.GetString("cluster")
	if err != nil {
		return err
	}
	obj := new(unstructured.Unstructured)
	if err = val.UnmarshalTo(obj); err != nil {
		return err
	}

	var pods []*unstructured.Unstructured
	var collector PodCollector

	switch obj.GroupVersionKind() {
	case fluxcdGroupVersion.WithKind(HelmReleaseKind):
		collector = helmReleasePodCollector
	default:
		collector = NewPodCollector(obj.GroupVersionKind())
	}

	pods, err = collector(h.cli, obj, cluster)
	if err != nil {
		return v.FillObject(err.Error(), "err")
	}
	return fillQueryResult(v, pods, "list")
}

func (h *provider) SearchEvents(ctx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("value")
	if err != nil {
		return err
	}
	cluster, err := v.GetString("cluster")
	if err != nil {
		return err
	}
	obj := new(unstructured.Unstructured)
	if err = val.UnmarshalTo(obj); err != nil {
		return err
	}

	listCtx := multicluster.ContextWithClusterName(context.Background(), cluster)
	fieldSelector := getEventFieldSelector(obj)
	eventList := corev1.EventList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingFieldsSelector{
			Selector: fieldSelector,
		},
	}
	if err := h.cli.List(listCtx, &eventList, listOpts...); err != nil {
		return v.FillObject(err.Error(), "err")
	}
	return fillQueryResult(v, eventList.Items, "list")
}

// GeneratorServiceEndpoints generator service endpoints is available for common component type,
// such as webservice or helm
// it can not support the cloud service component currently
func (h *provider) GeneratorServiceEndpoints(wfctx wfContext.Context, v *value.Value, act types.Action) error {
	ctx := context.Background()
	findResource := func(obj client.Object, name, namespace, cluster string) error {
		obj.SetNamespace(namespace)
		obj.SetName(name)
		gctx, cancel := context.WithTimeout(ctx, time.Second*10)
		defer cancel()
		if err := h.cli.Get(multicluster.ContextWithClusterName(gctx, cluster),
			client.ObjectKeyFromObject(obj), obj); err != nil {
			if kerrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return nil
	}
	val, err := v.LookupValue("app")
	if err != nil {
		return err
	}
	opt := Option{}
	if err = val.UnmarshalTo(&opt); err != nil {
		return err
	}
	app := new(v1beta1.Application)
	err = findResource(app, opt.Name, opt.Namespace, "")
	if err != nil {
		return fmt.Errorf("query app failure %w", err)
	}
	serviceEndpoints := make([]querytypes.ServiceEndpoint, 0)
	var clusterGatewayNodeIP = make(map[string]string)
	collector := NewAppCollector(h.cli, opt)
	resources, err := collector.ListApplicationResources(app)
	if err != nil {
		return err
	}

	for i, resource := range resources {
		cluster := resources[i].Cluster
		selectorNodeIP := func() string {
			if ip, exist := clusterGatewayNodeIP[cluster]; exist {
				return ip
			}
			ip := selectorNodeIP(ctx, cluster, h.cli)
			if ip != "" {
				clusterGatewayNodeIP[cluster] = ip
			}
			return ip
		}
		switch resource.Kind {
		case "Ingress":
			if resource.GroupVersionKind().Group == networkv1beta1.GroupName && (resource.GroupVersionKind().Version == "v1beta1" || resource.GroupVersionKind().Version == "v1") {
				var ingress networkv1beta1.Ingress
				ingress.SetGroupVersionKind(resource.GroupVersionKind())
				if err := findResource(&ingress, resource.Name, resource.Namespace, resource.Cluster); err != nil {
					klog.Error(err, fmt.Sprintf("find v1 Ingress %s/%s from cluster %s failure", resource.Name, resource.Namespace, resource.Cluster))
					continue
				}
				serviceEndpoints = append(serviceEndpoints, generatorFromIngress(ingress, cluster, resource.Component)...)
			} else {
				klog.Warning("not support ingress version", "version", resource.GroupVersionKind())
			}
		case "Service":
			var service corev1.Service
			service.SetGroupVersionKind(resource.GroupVersionKind())
			if err := findResource(&service, resource.Name, resource.Namespace, resource.Cluster); err != nil {
				klog.Error(err, fmt.Sprintf("find v1 Service %s/%s from cluster %s failure", resource.Name, resource.Namespace, resource.Cluster))
				continue
			}
			serviceEndpoints = append(serviceEndpoints, generatorFromService(service, selectorNodeIP, cluster, resource.Component, "")...)
		case helmapi.HelmReleaseGVK.Kind:
			obj := new(unstructured.Unstructured)
			obj.SetNamespace(resource.Namespace)
			obj.SetName(resource.Name)
			hc := NewHelmReleaseCollector(h.cli, obj)
			services, err := hc.CollectServices(ctx, resource.Cluster)
			if err != nil {
				klog.Error(err, "collect service by helm release failure", "helmRelease", resource.Name, "namespace", resource.Namespace, "cluster", resource.Cluster)
			}
			for _, service := range services {
				serviceEndpoints = append(serviceEndpoints, generatorFromService(service, selectorNodeIP, cluster, resource.Component, "")...)
			}
			ingress, err := hc.CollectIngress(ctx, resource.Cluster)
			if err != nil {
				klog.Error(err, "collect ingres by helm release failure", "helmRelease", resource.Name, "namespace", resource.Namespace, "cluster", resource.Cluster)
			}
			for _, uns := range ingress {
				var ingress networkv1beta1.Ingress
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(uns.UnstructuredContent(), &ingress); err != nil {
					klog.Errorf("fail to convert unstructured to ingress %s", err.Error())
					continue
				}
				serviceEndpoints = append(serviceEndpoints, generatorFromIngress(ingress, cluster, resource.Component)...)
			}
		case "SeldonDeployment":
			obj := new(unstructured.Unstructured)
			obj.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "machinelearning.seldon.io",
				Version: "v1",
				Kind:    "SeldonDeployment",
			})
			if err := findResource(obj, resource.Name, resource.Namespace, resource.Cluster); err != nil {
				klog.Error(err, fmt.Sprintf("find v1 Seldon Deployment %s/%s from cluster %s failure", resource.Name, resource.Namespace, resource.Cluster))
				continue
			}
			anno := obj.GetAnnotations()
			serviceName := "ambassador"
			serviceNS := "vela-system"
			if anno != nil {
				if anno[annoAmbassadorServiceName] != "" {
					serviceName = anno[annoAmbassadorServiceName]
				}
				if anno[annoAmbassadorServiceNamespace] != "" {
					serviceNS = anno[annoAmbassadorServiceNamespace]
				}
			}
			var service corev1.Service
			if err := findResource(&service, serviceName, serviceNS, resource.Cluster); err != nil {
				klog.Error(err, fmt.Sprintf("find v1 Service %s/%s from cluster %s failure", serviceName, serviceNS, resource.Cluster))
				continue
			}
			serviceEndpoints = append(serviceEndpoints, generatorFromService(service, selectorNodeIP, cluster, resource.Component, fmt.Sprintf("/seldon/%s/%s", resource.Namespace, resource.Name))...)
		}
	}
	return fillQueryResult(v, serviceEndpoints, "list")
}

var (
	terminatedContainerNotFoundRegex = regexp.MustCompile("previous terminated container .+ in pod .+ not found")
)

func isTerminatedContainerNotFound(err error) bool {
	return err != nil && terminatedContainerNotFoundRegex.MatchString(err.Error())
}

func (h *provider) CollectLogsInPod(ctx wfContext.Context, v *value.Value, act types.Action) error {
	cluster, err := v.GetString("cluster")
	if err != nil {
		return errors.Wrapf(err, "invalid cluster")
	}
	namespace, err := v.GetString("namespace")
	if err != nil {
		return errors.Wrapf(err, "invalid namespace")
	}
	pod, err := v.GetString("pod")
	if err != nil {
		return errors.Wrapf(err, "invalid pod name")
	}
	val, err := v.LookupValue("options")
	if err != nil {
		return errors.Wrapf(err, "invalid log options")
	}
	opts := &corev1.PodLogOptions{}
	if err = val.UnmarshalTo(opts); err != nil {
		return errors.Wrapf(err, "invalid log options content")
	}
	cliCtx := multicluster.ContextWithClusterName(context.Background(), cluster)
	clientSet, err := kubernetes.NewForConfig(h.cfg)
	if err != nil {
		return errors.Wrapf(err, "failed to create kubernetes clientset")
	}
	podInst, err := clientSet.CoreV1().Pods(namespace).Get(cliCtx, pod, v1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get pod")
	}
	req := clientSet.CoreV1().Pods(namespace).GetLogs(pod, opts)
	readCloser, err := req.Stream(cliCtx)
	if err != nil && !isTerminatedContainerNotFound(err) {
		return errors.Wrapf(err, "failed to get stream logs")
	}
	r := bufio.NewReader(readCloser)
	var b strings.Builder
	var readErr error
	if err == nil {
		defer func() {
			_ = readCloser.Close()
		}()
		for {
			s, err := r.ReadString('\n')
			b.WriteString(s)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					readErr = err
				}
				break
			}
		}
	} else {
		readErr = err
	}
	toDate := v1.Now()
	var fromDate v1.Time
	// nolint
	if opts.SinceTime != nil {
		fromDate = *opts.SinceTime
	} else if opts.SinceSeconds != nil {
		fromDate = v1.NewTime(toDate.Add(time.Duration(-(*opts.SinceSeconds) * int64(time.Second))))
	} else {
		fromDate = podInst.CreationTimestamp
	}
	o := map[string]interface{}{
		"logs": b.String(),
		"info": map[string]interface{}{
			"fromDate": fromDate,
			"toDate":   toDate,
		},
	}
	if readErr != nil {
		o["err"] = readErr.Error()
	}
	return v.FillObject(o, "outputs")
}

// Install register handlers to provider discover.
func Install(p providers.Providers, cli client.Client, cfg *rest.Config) {
	prd := &provider{
		cli: cli,
		cfg: cfg,
	}

	p.Register(ProviderName, map[string]providers.Handler{
		"listResourcesInApp":      prd.ListResourcesInApp,
		"listAppliedResources":    prd.ListAppliedResources,
		"collectPods":             prd.CollectPods,
		"searchEvents":            prd.SearchEvents,
		"collectLogsInPod":        prd.CollectLogsInPod,
		"collectServiceEndpoints": prd.GeneratorServiceEndpoints,
		"getApplicationTree":      prd.GetApplicationResourceTree,
	})
}

func generatorFromService(service corev1.Service, selectorNodeIP func() string, cluster, component, path string) []querytypes.ServiceEndpoint {
	var serviceEndpoints []querytypes.ServiceEndpoint
	switch service.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		for _, port := range service.Spec.Ports {
			judgeAppProtocol := judgeAppProtocol(port.Port)
			for _, ingress := range service.Status.LoadBalancer.Ingress {
				if ingress.Hostname != "" {
					serviceEndpoints = append(serviceEndpoints, querytypes.ServiceEndpoint{
						Endpoint: querytypes.Endpoint{
							Protocol:    port.Protocol,
							AppProtocol: &judgeAppProtocol,
							Host:        ingress.Hostname,
							Port:        int(port.Port),
							Path:        path,
						},
						Ref: corev1.ObjectReference{
							Kind:            "Service",
							Namespace:       service.ObjectMeta.Namespace,
							Name:            service.ObjectMeta.Name,
							UID:             service.UID,
							APIVersion:      service.APIVersion,
							ResourceVersion: service.ResourceVersion,
						},
						Cluster:   cluster,
						Component: component,
					})
				}
				if ingress.IP != "" {
					serviceEndpoints = append(serviceEndpoints, querytypes.ServiceEndpoint{
						Endpoint: querytypes.Endpoint{
							Protocol:    port.Protocol,
							AppProtocol: &judgeAppProtocol,
							Host:        ingress.IP,
							Port:        int(port.Port),
							Path:        path,
						},
						Ref: corev1.ObjectReference{
							Kind:            "Service",
							Namespace:       service.ObjectMeta.Namespace,
							Name:            service.ObjectMeta.Name,
							UID:             service.UID,
							APIVersion:      service.APIVersion,
							ResourceVersion: service.ResourceVersion,
						},
						Cluster:   cluster,
						Component: component,
					})
				}
			}
		}
	case corev1.ServiceTypeNodePort:
		for _, port := range service.Spec.Ports {
			judgeAppProtocol := judgeAppProtocol(port.Port)
			serviceEndpoints = append(serviceEndpoints, querytypes.ServiceEndpoint{
				Endpoint: querytypes.Endpoint{
					Protocol:    port.Protocol,
					Port:        int(port.NodePort),
					AppProtocol: &judgeAppProtocol,
					Host:        selectorNodeIP(),
					Path:        path,
				},
				Ref: corev1.ObjectReference{
					Kind:            "Service",
					Namespace:       service.ObjectMeta.Namespace,
					Name:            service.ObjectMeta.Name,
					UID:             service.UID,
					APIVersion:      service.APIVersion,
					ResourceVersion: service.ResourceVersion,
				},
				Cluster:   cluster,
				Component: component,
			})
		}
	case corev1.ServiceTypeClusterIP, corev1.ServiceTypeExternalName:
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
		return "http"
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
	for _, rule := range ingress.Spec.Rules {
		var appProtocol = getAppProtocol(rule.Host)
		var appPort = getEndpointPort(appProtocol)
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				serviceEndpoints = append(serviceEndpoints, querytypes.ServiceEndpoint{
					Endpoint: querytypes.Endpoint{
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: &appProtocol,
						Host:        rule.Host,
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
