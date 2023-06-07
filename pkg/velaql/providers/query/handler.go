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
	"encoding/base64"
	"fmt"
	"io"
	"time"

	cuelang "cuelang.org/go/cue"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
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
}

// ListResourcesInApp lists CRs created by Application, this provider queries the object data.
func ListResourcesInApp(ctx context.Context, v cuelang.Value) (cuelang.Value, error) {
	val := v.LookupPath(cuelang.ParsePath("app"))
	if err := val.Err(); err != nil {
		return v, err
	}
	opt := Option{}
	if err := val.Decode(&opt); err != nil {
		return v, err
	}
	collector := NewAppCollector(singleton.KubeClient.Get(), opt)
	appResList, err := collector.CollectResourceFromApp(ctx)
	if err != nil {
		v = v.FillPath(cuelang.ParsePath("err"), err)
		return v, v.Err()
	}
	if appResList == nil {
		appResList = make([]Resource, 0)
	}
	v = fillQueryResult(v, appResList, "list")
	return v, v.Err()
}

// ListAppliedResources list applied resource from tracker, this provider only queries the metadata.
func ListAppliedResources(ctx context.Context, v cuelang.Value) (cuelang.Value, error) {
	val := v.LookupPath(cuelang.ParsePath("app"))
	if err := val.Err(); err != nil {
		return val, err
	}
	opt := Option{}
	if err := val.Decode(&opt); err != nil {
		val = val.FillPath(cuelang.ParsePath("err"), err)
		return val, val.Err()
	}
	cli := singleton.KubeClient.Get()
	collector := NewAppCollector(cli, opt)
	app := new(v1beta1.Application)
	appKey := client.ObjectKey{Name: opt.Name, Namespace: opt.Namespace}
	if err := cli.Get(ctx, appKey, app); err != nil {
		val = val.FillPath(cuelang.ParsePath("err"), err)
		return val, val.Err()
	}
	appResList, err := collector.ListApplicationResources(ctx, app)
	if err != nil {
		val = val.FillPath(cuelang.ParsePath("err"), err)
		return val, val.Err()
	}
	if appResList == nil {
		appResList = make([]*querytypes.AppliedResource, 0)
	}
	v = fillQueryResult(v, appResList, "list")
	return v, v.Err()
}

// CollectResources .
func CollectResources(ctx context.Context, v cuelang.Value) (cuelang.Value, error) {
	val := v.LookupPath(cuelang.ParsePath("app"))
	if err := val.Err(); err != nil {
		return val, err
	}
	opt := Option{}
	if err := val.Decode(&opt); err != nil {
		val = val.FillPath(cuelang.ParsePath("err"), err)
		return val, val.Err()
	}
	cli := singleton.KubeClient.Get()
	collector := NewAppCollector(cli, opt)
	app := new(v1beta1.Application)
	appKey := client.ObjectKey{Name: opt.Name, Namespace: opt.Namespace}
	if err := cli.Get(ctx, appKey, app); err != nil {
		val = val.FillPath(cuelang.ParsePath("err"), err)
		return val, val.Err()
	}
	appResList, err := collector.ListApplicationResources(ctx, app)
	if err != nil {
		val = val.FillPath(cuelang.ParsePath("err"), err)
		return val, val.Err()
	}
	var resources = make([]querytypes.ResourceItem, 0)
	for _, res := range appResList {
		if res.ResourceTree != nil {
			resources = append(resources, buildResourceArray(*res, res.ResourceTree, res.ResourceTree, opt.Filter.Kind, opt.Filter.APIVersion)...)
		} else if res.Kind == opt.Filter.Kind && res.APIVersion == opt.Filter.APIVersion {
			var object unstructured.Unstructured
			object.SetAPIVersion(opt.Filter.APIVersion)
			object.SetKind(opt.Filter.Kind)
			if err := cli.Get(ctx, apimachinerytypes.NamespacedName{Namespace: res.Namespace, Name: res.Name}, &object); err == nil {
				resources = append(resources, buildResourceItem(*res, querytypes.Workload{
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
	v = fillQueryResult(v, resources, "list")
	return v, v.Err()
}

// SearchEvents .
func SearchEvents(ctx context.Context, v cuelang.Value) (cuelang.Value, error) {
	val := v.LookupPath(cuelang.ParsePath("value"))
	if err := val.Err(); err != nil {
		return val, err
	}
	cluster, err := v.LookupPath(cuelang.ParsePath("cluster")).String()
	if err != nil {
		return val, err
	}
	obj := new(unstructured.Unstructured)
	if err = val.Decode(obj); err != nil {
		return val, err
	}

	listCtx := multicluster.ContextWithClusterName(ctx, cluster)
	fieldSelector := getEventFieldSelector(obj)
	eventList := corev1.EventList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingFieldsSelector{
			Selector: fieldSelector,
		},
	}
	if err := singleton.KubeClient.Get().List(listCtx, &eventList, listOpts...); err != nil {
		val = val.FillPath(cuelang.ParsePath("err"), err)
		return val, val.Err()
	}
	v = fillQueryResult(v, eventList.Items, "list")
	return v, v.Err()
}

// CollectLogsInPod .
func CollectLogsInPod(ctx context.Context, v cuelang.Value) (cuelang.Value, error) {
	cluster, err := v.LookupPath(cuelang.ParsePath("cluster")).String()
	if err != nil {
		return v, errors.Wrapf(err, "invalid cluster")
	}
	namespace, err := v.LookupPath(cuelang.ParsePath("namespace")).String()
	if err != nil {
		return v, errors.Wrapf(err, "invalid namespace")
	}
	pod, err := v.LookupPath(cuelang.ParsePath("pod")).String()
	if err != nil {
		return v, errors.Wrapf(err, "invalid pod name")
	}
	opts := &corev1.PodLogOptions{}
	if err := v.LookupPath(cuelang.ParsePath("options")).Decode(opts); err != nil {
		return v, errors.Wrapf(err, "invalid log options content")
	}
	cliCtx := multicluster.ContextWithClusterName(ctx, cluster)
	cfg := rest.CopyConfig(singleton.KubeConfig.Get())
	cfg.Wrap(pkgmulticluster.NewTransportWrapper())
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return v, errors.Wrapf(err, "failed to create kubernetes client")
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
	v = v.FillPath(cuelang.ParsePath("outputs"), defaultOutputs)
	return v, v.Err()
}

// GetProviders returns the cue providers.
func GetProviders() map[string]cuexruntime.ProviderFn {
	return map[string]cuexruntime.ProviderFn{
		"listResourcesInApp":      cuexruntime.NativeProviderFn(ListResourcesInApp),
		"listAppliedResources":    cuexruntime.NativeProviderFn(ListAppliedResources),
		"collectResources":        cuexruntime.NativeProviderFn(CollectResources),
		"searchEvents":            cuexruntime.NativeProviderFn(SearchEvents),
		"collectLogsInPod":        cuexruntime.NativeProviderFn(CollectLogsInPod),
		"collectServiceEndpoints": cuexruntime.NativeProviderFn(CollectServiceEndpoints),
	}
}
