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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	DetailDefinition(ctx context.Context, name, defType string) (*apisv1.DetailDefinitionResponse, error)
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
	defs := &unstructured.UnstructuredList{}
	switch defType {
	case "component":
		defs.SetAPIVersion("core.oam.dev/v1beta1")
		defs.SetKind("ComponentDefinition")
		return d.listDefinitions(ctx, defs, "componentDefinitions")

	case "trait":
		defs.SetAPIVersion("core.oam.dev/v1beta1")
		defs.SetKind("TraitDefinition")
		return d.listDefinitions(ctx, defs, "traitDefinitions")

	case "workflowstep":
		defs.SetAPIVersion("core.oam.dev/v1beta1")
		defs.SetKind("WorkflowStepDefinition")
		return d.listDefinitions(ctx, defs, "workflowStepDefinitions")

	default:
		return nil, fmt.Errorf("invalid definition type")
	}
}

func (d *definitionUsecaseImpl) listDefinitions(ctx context.Context, list *unstructured.UnstructuredList, cache string) ([]*apisv1.DefinitionBase, error) {
	if mc := d.caches[cache]; mc != nil && !mc.IsExpired() {
		return mc.GetData().([]*apisv1.DefinitionBase), nil
	}
	if err := d.kubeClient.List(ctx, list, &client.ListOptions{
		Namespace: types.DefaultKubeVelaNS,
	}); err != nil {
		return nil, err
	}
	var defs []*apisv1.DefinitionBase
	for _, def := range list.Items {
		defs = append(defs, &apisv1.DefinitionBase{
			Name:        def.GetName(),
			Description: def.GetAnnotations()[types.AnnDescription],
		})
	}
	d.caches[cache] = utils.NewMemoryCache(defs, time.Minute*3)
	return defs, nil
}

// DetailDefinition get definition detail
func (d *definitionUsecaseImpl) DetailDefinition(ctx context.Context, name, defType string) (*apisv1.DetailDefinitionResponse, error) {
	var cm v1.ConfigMap
	if err := d.kubeClient.Get(ctx, k8stypes.NamespacedName{
		Namespace: types.DefaultKubeVelaNS,
		Name:      fmt.Sprintf("%s-schema-%s", defType, name),
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
