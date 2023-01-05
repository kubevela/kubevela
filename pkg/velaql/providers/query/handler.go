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
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	wfContext "github.com/kubevela/workflow/pkg/context"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/types"

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
func (h *provider) ListResourcesInApp(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("app")
	if err != nil {
		return err
	}
	opt := Option{}
	if err = val.UnmarshalTo(&opt); err != nil {
		return err
	}
	collector := NewAppCollector(h.cli, opt)
	appResList, err := collector.CollectResourceFromApp(ctx)
	if err != nil {
		return v.FillObject(err.Error(), "err")
	}
	if appResList == nil {
		appResList = make([]Resource, 0)
	}
	return fillQueryResult(v, appResList, "list")
}

// ListAppliedResources list applied resource from tracker, this provider only queries the metadata.
func (h *provider) ListAppliedResources(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("app")
	if err != nil {
		return err
	}
	opt := Option{}
	if err = val.UnmarshalTo(&opt); err != nil {
		return v.FillObject(err.Error(), "err")
	}
	collector := NewAppCollector(h.cli, opt)
	app := new(v1beta1.Application)
	appKey := client.ObjectKey{Name: opt.Name, Namespace: opt.Namespace}
	if err = h.cli.Get(ctx, appKey, app); err != nil {
		return v.FillObject(err.Error(), "err")
	}
	appResList, err := collector.ListApplicationResources(ctx, app)
	if err != nil {
		return v.FillObject(err.Error(), "err")
	}
	if appResList == nil {
		appResList = make([]*querytypes.AppliedResource, 0)
	}
	return fillQueryResult(v, appResList, "list")
}

func (h *provider) CollectResources(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("app")
	if err != nil {
		return err
	}
	opt := Option{}
	if err = val.UnmarshalTo(&opt); err != nil {
		return v.FillObject(err.Error(), "err")
	}
	collector := NewAppCollector(h.cli, opt)
	app := new(v1beta1.Application)
	appKey := client.ObjectKey{Name: opt.Name, Namespace: opt.Namespace}
	if err = h.cli.Get(ctx, appKey, app); err != nil {
		return v.FillObject(err.Error(), "err")
	}
	appResList, err := collector.ListApplicationResources(ctx, app)
	if err != nil {
		return v.FillObject(err.Error(), "err")
	}
	var resources = make([]querytypes.ResourceItem, 0)
	for _, res := range appResList {
		if res.ResourceTree != nil {
			resources = append(resources, buildResourceArray(*res, res.ResourceTree, res.ResourceTree, opt.Filter.Kind, opt.Filter.APIVersion)...)
		} else if res.Kind == opt.Filter.Kind && res.APIVersion == opt.Filter.APIVersion {
			var object unstructured.Unstructured
			object.SetAPIVersion(opt.Filter.APIVersion)
			object.SetKind(opt.Filter.Kind)
			if err := h.cli.Get(ctx, apimachinerytypes.NamespacedName{Namespace: res.Namespace, Name: res.Name}, &object); err == nil {
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
	return fillQueryResult(v, resources, "list")
}

func (h *provider) SearchEvents(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act types.Action) error {
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

	listCtx := multicluster.ContextWithClusterName(ctx, cluster)
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

func (h *provider) CollectLogsInPod(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act types.Action) error {
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
	cliCtx := multicluster.ContextWithClusterName(ctx, cluster)
	h.cfg.Wrap(pkgmulticluster.NewTransportWrapper())
	clientSet, err := kubernetes.NewForConfig(h.cfg)
	if err != nil {
		return errors.Wrapf(err, "failed to create kubernetes client")
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
	return v.FillObject(defaultOutputs, "outputs")
}

// Install register handlers to provider discover.
func Install(p types.Providers, cli client.Client, cfg *rest.Config) {
	prd := &provider{
		cli: cli,
		cfg: cfg,
	}

	p.Register(ProviderName, map[string]types.Handler{
		"listResourcesInApp":      prd.ListResourcesInApp,
		"listAppliedResources":    prd.ListAppliedResources,
		"collectResources":        prd.CollectResources,
		"searchEvents":            prd.SearchEvents,
		"collectLogsInPod":        prd.CollectLogsInPod,
		"collectServiceEndpoints": prd.CollectServiceEndpoints,
	})
}
