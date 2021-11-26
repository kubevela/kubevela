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
	stdctx "context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "query"
	// HelmReleaseKind is the kind of HelmRelease
	HelmReleaseKind = "HelmRelease"
)

var fluxcdGroupVersion = schema.GroupVersion{Group: "helm.toolkit.fluxcd.io", Version: "v2beta1"}

type provider struct {
	cli client.Client
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
}

// FilterOption filter resource created by component
type FilterOption struct {
	Cluster          string   `json:"cluster,omitempty"`
	ClusterNamespace string   `json:"clusterNamespace,omitempty"`
	Components       []string `json:"components,omitempty"`
}

// ListResourcesInApp lists CRs created by Application
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
	return v.FillObject(appResList, "list")
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
	return v.FillObject(pods, "list")
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

	listCtx := multicluster.ContextWithClusterName(stdctx.Background(), cluster)
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
	return v.FillObject(eventList.Items, "list")
}

// Install register handlers to provider discover.
func Install(p providers.Providers, cli client.Client) {
	prd := &provider{
		cli: cli,
	}

	p.Register(ProviderName, map[string]providers.Handler{
		"listResourcesInApp": prd.ListResourcesInApp,
		"collectPods":        prd.CollectPods,
		"searchEvents":       prd.SearchEvents,
	})
}
