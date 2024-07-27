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
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/workflow/pkg/providers/legacy/kube"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "query"
	// HelmReleaseKind is the kind of HelmRelease
	HelmReleaseKind = "HelmRelease"

	annoAmbassadorServiceName      = "ambassador.service/name"
	annoAmbassadorServiceNamespace = "ambassador.service/namespace"
)

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

	// WithTree means recursively query the resource tree.
	WithTree bool `json:"withTree,omitempty"`
}

// FilterOption filter resource created by component
type FilterOption struct {
	Cluster          string   `json:"cluster,omitempty"`
	ClusterNamespace string   `json:"clusterNamespace,omitempty"`
	Components       []string `json:"components,omitempty"`
	APIVersion       string   `json:"apiVersion,omitempty"`
	Kind             string   `json:"kind,omitempty"`
	QueryNewest      bool     `json:"queryNewest,omitempty"`
}

// ListVars is the vars for list
type ListVars struct {
	App Option `json:"app"`
}

// ListParams is the params for list
type ListParams = oamprovidertypes.OAMParams[ListVars]

// ListResult is the result for list
type ListResult[T any] struct {
	List  []T    `json:"list"`
	Error string `json:"err,omitempty"`
}

// ListResourcesInApp lists CRs created by Application, this provider queries the object data.
func ListResourcesInApp(ctx context.Context, params *ListParams) (*ListResult[Resource], error) {
	collector := NewAppCollector(params.KubeClient, params.Params.App)
	appResList, err := collector.CollectResourceFromApp(ctx)
	if err != nil {
		// nolint:nilerr
		return &ListResult[Resource]{Error: err.Error()}, nil
	}
	if appResList == nil {
		appResList = make([]Resource, 0)
	}
	return &ListResult[Resource]{List: appResList}, nil
}

// ListAppliedResources list applied resource from tracker, this provider only queries the metadata.
func ListAppliedResources(ctx context.Context, params *ListParams) (*ListResult[querytypes.AppliedResource], error) {
	opt := params.Params.App
	cli := params.KubeClient
	collector := NewAppCollector(cli, opt)
	app := new(v1beta1.Application)
	appKey := client.ObjectKey{Name: opt.Name, Namespace: opt.Namespace}
	if err := cli.Get(ctx, appKey, app); err != nil {
		// nolint:nilerr
		return &ListResult[querytypes.AppliedResource]{Error: err.Error()}, nil
	}
	appResList, err := collector.ListApplicationResources(ctx, app)
	if err != nil {
		// nolint:nilerr
		return &ListResult[querytypes.AppliedResource]{Error: err.Error()}, nil
	}
	if appResList == nil {
		appResList = make([]querytypes.AppliedResource, 0)
	}
	return &ListResult[querytypes.AppliedResource]{List: appResList}, nil
}

// CollectResources collects resources from the cluster
func CollectResources(ctx context.Context, params *ListParams) (*ListResult[querytypes.ResourceItem], error) {
	opt := params.Params.App
	cli := params.KubeClient
	collector := NewAppCollector(cli, opt)
	app := new(v1beta1.Application)
	appKey := client.ObjectKey{Name: opt.Name, Namespace: opt.Namespace}
	if err := cli.Get(ctx, appKey, app); err != nil {
		// nolint:nilerr
		return &ListResult[querytypes.ResourceItem]{Error: err.Error()}, nil
	}
	appResList, err := collector.ListApplicationResources(ctx, app)
	if err != nil {
		// nolint:nilerr
		return &ListResult[querytypes.ResourceItem]{Error: err.Error()}, nil
	}
	var resources = make([]querytypes.ResourceItem, 0)
	for _, res := range appResList {
		if res.ResourceTree != nil {
			resources = append(resources, buildResourceArray(res, res.ResourceTree, res.ResourceTree, opt.Filter.Kind, opt.Filter.APIVersion)...)
		} else if res.Kind == opt.Filter.Kind && res.APIVersion == opt.Filter.APIVersion {
			object := &unstructured.Unstructured{}
			object.SetAPIVersion(opt.Filter.APIVersion)
			object.SetKind(opt.Filter.Kind)
			if err := cli.Get(ctx, apimachinerytypes.NamespacedName{Namespace: res.Namespace, Name: res.Name}, object); err == nil {
				resources = append(resources, buildResourceItem(res, querytypes.Workload{
					APIVersion: app.APIVersion,
					Kind:       app.Kind,
					Name:       app.Name,
					Namespace:  app.Namespace,
				}, object))
			} else {
				klog.Errorf("failed to get the service:%s", err.Error())
			}
		}
	}
	return &ListResult[querytypes.ResourceItem]{List: resources}, nil
}

// SearchVars is the vars for search
type SearchVars struct {
	Value   *unstructured.Unstructured `json:"value"`
	Cluster string                     `json:"cluster"`
}

// SearchParams is the params for search
type SearchParams = oamprovidertypes.OAMParams[SearchVars]

// SearchEvents searches events
func SearchEvents(ctx context.Context, params *SearchParams) (*ListResult[corev1.Event], error) {
	obj := params.Params.Value
	if obj == nil {
		return nil, fmt.Errorf("please provide a object value to search events")
	}
	cluster := params.Params.Cluster
	cli := params.KubeClient

	listCtx := multicluster.ContextWithClusterName(ctx, cluster)
	fieldSelector := getEventFieldSelector(obj)
	eventList := corev1.EventList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingFieldsSelector{
			Selector: fieldSelector,
		},
	}
	if err := cli.List(listCtx, &eventList, listOpts...); err != nil {
		// nolint:nilerr
		return &ListResult[corev1.Event]{Error: err.Error()}, nil
	}
	return &ListResult[corev1.Event]{List: eventList.Items}, nil
}

// LogVars is the vars for log
type LogVars struct {
	Cluster   string                `json:"cluster"`
	Namespace string                `json:"namespace"`
	Pod       string                `json:"pod"`
	Options   *corev1.PodLogOptions `json:"options,omitempty"`
}

// LogParams is the params for log
type LogParams = oamprovidertypes.OAMParams[LogVars]

// LogResult is the result for log
type LogResult struct {
	Outputs map[string]interface{} `json:"outputs"`
}

// CollectLogsInPod collects logs in pod
func CollectLogsInPod(ctx context.Context, params *LogParams) (*LogResult, error) {
	cluster := params.Params.Cluster
	namespace := params.Params.Namespace
	pod := params.Params.Pod
	if pod == "" {
		return nil, fmt.Errorf("please provide a pod name to collect logs")
	}
	opts := params.Params.Options
	if opts == nil || opts.Container == "" {
		return nil, fmt.Errorf("please provide the container name to collect logs")
	}
	cliCtx := multicluster.ContextWithClusterName(ctx, cluster)
	cfg := params.KubeConfig
	cfg.Wrap(pkgmulticluster.NewTransportWrapper())
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create kubernetes client")
	}
	var defaultOutputs = make(map[string]interface{})
	var errMsg string
	podInst, err := clientSet.CoreV1().Pods(namespace).Get(cliCtx, pod, v1.GetOptions{})
	if err != nil {
		errMsg += fmt.Sprintf("failed to get pod: %s; ", err.Error())
	}
	req := clientSet.CoreV1().Pods(namespace).GetLogs(pod, opts)
	readCloser, err := req.Stream(cliCtx)
	if err != nil {
		errMsg += fmt.Sprintf("failed to get stream logs %s; ", err.Error())
	}
	if readCloser != nil && podInst != nil {
		r := bufio.NewReader(readCloser)
		buffer := bytes.NewBuffer(nil)
		var readErr error
		defer func() {
			_ = readCloser.Close()
		}()
		for {
			s, err := r.ReadString('\n')
			buffer.WriteString(s)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					readErr = err
				}
				break
			}
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
		// the cue string can not support the special characters
		logs := base64.StdEncoding.EncodeToString(buffer.Bytes())
		defaultOutputs = map[string]interface{}{
			"logs": logs,
			"info": map[string]interface{}{
				"fromDate": fromDate,
				"toDate":   toDate,
			},
		}
		if readErr != nil {
			errMsg += readErr.Error()
		}
	}
	if errMsg != "" {
		klog.Warningf(errMsg)
		defaultOutputs["err"] = errMsg
	}
	return &LogResult{Outputs: defaultOutputs}, nil
}

//go:embed ql.cue
var qlTemplate string

// GetTemplate returns the cue template.
func GetTemplate() string {
	return strings.Join([]string{qlTemplate, kube.GetTemplate()}, "\n")
}

// GetProviders returns the cue providers.
func GetProviders() map[string]cuexruntime.ProviderFn {
	qlProvider := map[string]cuexruntime.ProviderFn{
		"listResourcesInApp":      oamprovidertypes.OAMGenericProviderFn[ListVars, ListResult[Resource]](ListResourcesInApp),
		"listAppliedResources":    oamprovidertypes.OAMGenericProviderFn[ListVars, ListResult[querytypes.AppliedResource]](ListAppliedResources),
		"collectResources":        oamprovidertypes.OAMGenericProviderFn[ListVars, ListResult[querytypes.ResourceItem]](CollectResources),
		"searchEvents":            oamprovidertypes.OAMGenericProviderFn[SearchVars, ListResult[corev1.Event]](SearchEvents),
		"collectLogsInPod":        oamprovidertypes.OAMGenericProviderFn[LogVars, LogResult](CollectLogsInPod),
		"collectServiceEndpoints": oamprovidertypes.OAMGenericProviderFn[ListVars, ListResult[querytypes.ServiceEndpoint]](CollectServiceEndpoints),
	}
	kubeProviders := kube.GetProviders()
	for k, v := range kubeProviders {
		qlProvider[k] = v
	}
	return qlProvider
}
