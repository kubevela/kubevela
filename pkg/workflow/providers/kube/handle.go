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

package kube

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "kube"
)

// Dispatcher is a client for apply resources.
type Dispatcher func(ctx context.Context, cluster string, owner common.ResourceCreatorRole, manifests ...*unstructured.Unstructured) error

// Deleter is a client for delete resources.
type Deleter func(ctx context.Context, cluster string, owner common.ResourceCreatorRole, manifest *unstructured.Unstructured) error

type provider struct {
	app    *v1beta1.Application
	apply  Dispatcher
	delete Deleter
	cli    client.Client
}

// Apply create or update CR in cluster.
func (h *provider) Apply(ctx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("value")
	if err != nil {
		return err
	}
	var workload = new(unstructured.Unstructured)
	pv, err := v.Field("patch")
	if pv.Exists() && err == nil {
		base, err := model.NewBase(val.CueValue())
		if err != nil {
			return err
		}

		patcher, err := model.NewOther(pv)
		if err != nil {
			return err
		}
		if err := base.Unify(patcher); err != nil {
			return err
		}
		workload, err = base.Unstructured()
		if err != nil {
			return err
		}
	} else if err := val.UnmarshalTo(workload); err != nil {
		return err
	}
	if workload.GetNamespace() == "" {
		workload.SetNamespace("default")
	}
	cluster, err := v.GetString("cluster")
	if err != nil {
		return err
	}
	deployCtx := multicluster.ContextWithClusterName(context.Background(), cluster)
	deployCtx = auth.ContextWithUserInfo(deployCtx, h.app)
	if err := h.apply(deployCtx, cluster, common.WorkflowResourceCreator, workload); err != nil {
		return err
	}
	return cue.FillUnstructuredObject(v, workload, "value")
}

// ApplyInParallel create or update CRs in parallel.
func (h *provider) ApplyInParallel(ctx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("value")
	if err != nil {
		return err
	}
	iter, err := val.CueValue().List()
	if err != nil {
		return err
	}
	workloadNum := 0
	for iter.Next() {
		workloadNum++
	}
	var workloads = make([]*unstructured.Unstructured, workloadNum)
	if err = val.UnmarshalTo(&workloads); err != nil {
		return err
	}
	for i := range workloads {
		if workloads[i].GetNamespace() == "" {
			workloads[i].SetNamespace("default")
		}
	}
	cluster, err := v.GetString("cluster")
	if err != nil {
		return err
	}
	deployCtx := multicluster.ContextWithClusterName(context.Background(), cluster)
	deployCtx = auth.ContextWithUserInfo(deployCtx, h.app)
	if err = h.apply(deployCtx, cluster, common.WorkflowResourceCreator, workloads...); err != nil {
		return v.FillObject(err, "err")
	}
	return nil
}

// Read get CR from cluster.
func (h *provider) Read(ctx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("value")
	if err != nil {
		return err
	}
	obj := new(unstructured.Unstructured)
	if err := val.UnmarshalTo(obj); err != nil {
		return err
	}
	key := client.ObjectKeyFromObject(obj)
	if key.Namespace == "" {
		key.Namespace = "default"
	}
	cluster, err := v.GetString("cluster")
	if err != nil {
		return err
	}
	readCtx := multicluster.ContextWithClusterName(context.Background(), cluster)
	readCtx = auth.ContextWithUserInfo(readCtx, h.app)
	if err := h.cli.Get(readCtx, key, obj); err != nil {
		return v.FillObject(err.Error(), "err")
	}
	return cue.FillUnstructuredObject(v, obj, "value")
}

// List lists CRs from cluster.
func (h *provider) List(ctx wfContext.Context, v *value.Value, act types.Action) error {
	r, err := v.LookupValue("resource")
	if err != nil {
		return err
	}
	resource := &metav1.TypeMeta{}
	if err := r.UnmarshalTo(resource); err != nil {
		return err
	}
	list := &unstructured.UnstructuredList{Object: map[string]interface{}{
		"kind":       resource.Kind,
		"apiVersion": resource.APIVersion,
	}}

	type filters struct {
		Namespace      string            `json:"namespace"`
		MatchingLabels map[string]string `json:"matchingLabels"`
	}
	filterValue, err := v.LookupValue("filter")
	if err != nil {
		return err
	}
	filter := &filters{}
	if err := filterValue.UnmarshalTo(filter); err != nil {
		return err
	}
	cluster, err := v.GetString("cluster")
	if err != nil {
		return err
	}
	listOpts := []client.ListOption{
		client.InNamespace(filter.Namespace),
		client.MatchingLabels(filter.MatchingLabels),
	}
	readCtx := multicluster.ContextWithClusterName(context.Background(), cluster)
	readCtx = auth.ContextWithUserInfo(readCtx, h.app)
	if err := h.cli.List(readCtx, list, listOpts...); err != nil {
		return v.FillObject(err.Error(), "err")
	}
	return cue.FillUnstructuredObject(v, list, "list")
}

// Delete deletes CR from cluster.
func (h *provider) Delete(ctx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("value")
	if err != nil {
		return err
	}
	obj := new(unstructured.Unstructured)
	if err := val.UnmarshalTo(obj); err != nil {
		return err
	}
	cluster, err := v.GetString("cluster")
	if err != nil {
		return err
	}
	deleteCtx := multicluster.ContextWithClusterName(context.Background(), cluster)
	deleteCtx = auth.ContextWithUserInfo(deleteCtx, h.app)
	if err := h.delete(deleteCtx, cluster, common.WorkflowResourceCreator, obj); err != nil {
		return v.FillObject(err.Error(), "err")
	}
	return nil
}

// Install register handlers to provider discover.
func Install(p providers.Providers, app *v1beta1.Application, cli client.Client, apply Dispatcher, deleter Deleter) {
	if app != nil {
		app = app.DeepCopy()
	}
	prd := &provider{
		app:    app,
		apply:  apply,
		delete: deleter,
		cli:    cli,
	}
	p.Register(ProviderName, map[string]providers.Handler{
		"apply":             prd.Apply,
		"apply-in-parallel": prd.ApplyInParallel,
		"read":              prd.Read,
		"list":              prd.List,
		"delete":            prd.Delete,
	})
}
