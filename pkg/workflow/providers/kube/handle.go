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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "kube"
)

// Dispatcher is a client for apply resources.
type Dispatcher func(ctx context.Context, manifests ...*unstructured.Unstructured) error

type provider struct {
	apply func(ctx context.Context, manifests ...*unstructured.Unstructured) error
	cli   client.Client
}

// Apply create or update CR in cluster.
func (h *provider) Apply(ctx wfContext.Context, v *value.Value, act types.Action) error {
	var workload = new(unstructured.Unstructured)
	if err := v.UnmarshalTo(workload); err != nil {
		return err
	}

	deployCtx := context.Background()
	if workload.GetNamespace() == "" {
		workload.SetNamespace("default")
	}
	if err := h.apply(deployCtx, workload); err != nil {
		return err
	}
	return v.FillObject(workload.Object)
}

// Read get CR from cluster.
func (h *provider) Read(ctx wfContext.Context, v *value.Value, act types.Action) error {
	obj := new(unstructured.Unstructured)
	if err := v.UnmarshalTo(obj); err != nil {
		return err
	}
	key, err := client.ObjectKeyFromObject(obj)
	if err != nil {
		return err
	}
	if key.Namespace == "" {
		key.Namespace = "default"
	}
	if err := h.cli.Get(context.Background(), key, obj); err != nil {
		return err
	}
	return v.FillObject(obj.Object, "result")
}

// Install register handlers to provider discover.
func Install(p providers.Providers, cli client.Client, apply Dispatcher) {
	prd := &provider{
		apply: apply,
		cli:   cli,
	}
	p.Register(ProviderName, map[string]providers.Handler{
		"apply": prd.Apply,
		"read":  prd.Read,
	})
}
