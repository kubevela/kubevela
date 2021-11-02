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

package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
)

// DefinitionUsecase definition usecase, Implement the management of ComponentDefinition、TraitDefinition and WorkflowStepDefinition.
type DefinitionUsecase interface {
	// ListDefinitions list definition base info
	ListDefinitions(ctx context.Context, envName, defType string) ([]*apisv1.DefinitionBase, error)
	// DetailDefinition get definition detail
	DetailDefinition(ctx context.Context, name string) (*apisv1.DetailDefinitionResponse, error)
}

type definitionUsecaseImpl struct {
	kubeClient client.Client
	caches     map[string]*utils.MemoryCache
}

// NewDefinitionUsecase new definition usecase
func NewDefinitionUsecase() DefinitionUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &definitionUsecaseImpl{kubeClient: kubecli, caches: make(map[string]*utils.MemoryCache)}
}

func (d *definitionUsecaseImpl) ListDefinitions(ctx context.Context, envName, defType string) ([]*apisv1.DefinitionBase, error) {
	var defs []*apisv1.DefinitionBase
	switch defType {
	case "component":
		if mc := d.caches["componentDefinitions"]; mc != nil && !mc.IsExpired() {
			return mc.GetData().([]*apisv1.DefinitionBase), nil
		}
		var componentDefinitions v1beta1.ComponentDefinitionList
		if err := d.kubeClient.List(ctx, &componentDefinitions, &client.ListOptions{
			Namespace: types.DefaultKubeVelaNS,
		}); err != nil {
			return nil, err
		}
		for _, cd := range componentDefinitions.Items {
			defs = append(defs, &apisv1.DefinitionBase{
				Name:        cd.Name,
				Description: cd.Annotations[types.AnnDescription],
			})
		}
		d.caches["componentDefinitions"] = utils.NewMemoryCache(defs, time.Minute*3)

	case "trait":
		if mc := d.caches["traitDefinitions"]; mc != nil && !mc.IsExpired() {
			return mc.GetData().([]*apisv1.DefinitionBase), nil
		}
		var traitDefinitions v1beta1.TraitDefinitionList
		if err := d.kubeClient.List(ctx, &traitDefinitions, &client.ListOptions{
			Namespace: types.DefaultKubeVelaNS,
		}); err != nil {
			return nil, err
		}
		for _, td := range traitDefinitions.Items {
			defs = append(defs, &apisv1.DefinitionBase{
				Name:        td.Name,
				Description: td.Annotations[types.AnnDescription],
			})
		}
		d.caches["traitDefinitions"] = utils.NewMemoryCache(defs, time.Minute*3)

	case "workflowstep":
		if mc := d.caches["workflowStepDefinitions"]; mc != nil && !mc.IsExpired() {
			return mc.GetData().([]*apisv1.DefinitionBase), nil
		}
		var stepDefinitions v1beta1.WorkflowStepDefinitionList
		if err := d.kubeClient.List(ctx, &stepDefinitions, &client.ListOptions{
			Namespace: types.DefaultKubeVelaNS,
		}); err != nil {
			return nil, err
		}
		for _, sd := range stepDefinitions.Items {
			defs = append(defs, &apisv1.DefinitionBase{
				Name:        sd.Name,
				Description: sd.Annotations[types.AnnDescription],
			})
		}
		d.caches["workflowStepDefinitions"] = utils.NewMemoryCache(defs, time.Minute*3)
	}

	return defs, nil
}

// DetailDefinition get definition detail
func (d *definitionUsecaseImpl) DetailDefinition(ctx context.Context, name string) (*apisv1.DetailDefinitionResponse, error) {
	var cm v1.ConfigMap
	if err := d.kubeClient.Get(ctx, k8stypes.NamespacedName{
		Namespace: types.DefaultKubeVelaNS,
		Name:      fmt.Sprintf("schema-%s", name),
	}, &cm); err != nil {
		return nil, err
	}

	data, ok := cm.Data["openapi-v3-json-schema"]
	if !ok {
		return nil, fmt.Errorf("failed to get definition schema")
	}
	schema := &apisv1.DefinitionSchema{}
	if err := json.Unmarshal([]byte(data), schema); err != nil {
		return nil, err
	}
	return &apisv1.DetailDefinitionResponse{
		Schema: schema,
	}, nil
}
