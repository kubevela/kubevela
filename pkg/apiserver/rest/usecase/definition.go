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
	"time"

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
	// ListComponentDefinitions list component definition base info
	ListComponentDefinitions(ctx context.Context, envName string) ([]*apisv1.ComponentDefinitionBase, error)
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

func (d *definitionUsecaseImpl) ListComponentDefinitions(ctx context.Context, envName string) ([]*apisv1.ComponentDefinitionBase, error) {
	// check cache
	if mc := d.caches["componentDefinitions"]; mc != nil && !mc.IsExpired() {
		return mc.GetData().([]*apisv1.ComponentDefinitionBase), nil
	}
	var componentDefinitions v1beta1.ComponentDefinitionList
	if err := d.kubeClient.List(ctx, &componentDefinitions, &client.ListOptions{}); err != nil {
		return nil, err
	}
	var cdb []*apisv1.ComponentDefinitionBase
	for _, cd := range componentDefinitions.Items {
		cdb = append(cdb, &apisv1.ComponentDefinitionBase{
			Name:        cd.Name,
			Description: cd.Annotations[types.AnnDescription],
		})
	}
	// set cache
	d.caches["componentDefinitions"] = utils.NewMemoryCache(cdb, time.Minute*3)
	return cdb, nil
}
