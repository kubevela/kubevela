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
	"sort"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
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

const (
	definitionAPIVersion       = "core.oam.dev/v1beta1"
	kindComponentDefinition    = "ComponentDefinition"
	kindTraitDefinition        = "TraitDefinition"
	kindWorkflowStepDefinition = "WorkflowStepDefinition"
)

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
		defs.SetAPIVersion(definitionAPIVersion)
		defs.SetKind(kindComponentDefinition)
		return d.listDefinitions(ctx, defs, kindComponentDefinition)

	case "trait":
		defs.SetAPIVersion(definitionAPIVersion)
		defs.SetKind(kindTraitDefinition)
		return d.listDefinitions(ctx, defs, kindTraitDefinition)

	case "workflowstep":
		defs.SetAPIVersion(definitionAPIVersion)
		defs.SetKind(kindWorkflowStepDefinition)
		return d.listDefinitions(ctx, defs, kindWorkflowStepDefinition)

	default:
		return nil, bcode.ErrDefinitionTypeNotSupport
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
	if !utils.StringsContain([]string{"component", "trait", "workflowstep"}, defType) {
		return nil, bcode.ErrDefinitionTypeNotSupport
	}
	var cm v1.ConfigMap
	if err := d.kubeClient.Get(ctx, k8stypes.NamespacedName{
		Namespace: types.DefaultKubeVelaNS,
		Name:      fmt.Sprintf("%s-schema-%s", defType, name),
	}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, bcode.ErrDefinitionNoSchema
		}
		return nil, err
	}

	data, ok := cm.Data[types.OpenapiV3JSONSchema]
	if !ok {
		return nil, bcode.ErrDefinitionNoSchema
	}
	schema := &openapi3.Schema{}
	if err := schema.UnmarshalJSON([]byte(data)); err != nil {
		return nil, err
	}
	// render default ui schema
	defaultUISchema := renderDefaultUISchema("", schema)
	// patch from custom ui schema
	customUISchema := d.renderCustomUISchema(ctx, name, defType, defaultUISchema)
	return &apisv1.DetailDefinitionResponse{
		APISchema: schema,
		UISchema:  customUISchema,
	}, nil
}

func (d *definitionUsecaseImpl) renderCustomUISchema(ctx context.Context, name, defType string, defaultSchema []*utils.UIParameter) []*utils.UIParameter {
	var cm v1.ConfigMap
	if err := d.kubeClient.Get(ctx, k8stypes.NamespacedName{
		Namespace: types.DefaultKubeVelaNS,
		Name:      fmt.Sprintf("%s-uischema-%s", defType, name),
	}, &cm); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Logger.Errorf("find uischema configmap from cluster failure %s", err.Error())
		}
		return defaultSchema
	}
	data, ok := cm.Data[types.UISchema]
	if !ok {
		return defaultSchema
	}
	schema := []*utils.UIParameter{}
	if err := json.Unmarshal([]byte(data), &schema); err != nil {
		log.Logger.Errorf("unmarshal ui schema failure %s", err.Error())
		return defaultSchema
	}
	return patchSchema(defaultSchema, schema)
}

func patchSchema(defaultSchema, customSchema []*utils.UIParameter) []*utils.UIParameter {
	var customSchemaMap = make(map[string]*utils.UIParameter, len(customSchema))
	for i, custom := range customSchema {
		customSchemaMap[custom.JSONKey] = customSchema[i]
	}
	for i := range defaultSchema {
		dSchema := defaultSchema[i]
		if cusSchema, exist := customSchemaMap[dSchema.JSONKey]; exist {
			if cusSchema.Description != "" {
				dSchema.Description = cusSchema.Description
			}
			if cusSchema.Label != "" {
				dSchema.Label = cusSchema.Label
			}
			if cusSchema.SubParameterGroupOption != nil {
				dSchema.SubParameterGroupOption = cusSchema.SubParameterGroupOption
			}
			if cusSchema.Validate != nil {
				dSchema.Validate = cusSchema.Validate
			}
			if cusSchema.UIType != "" {
				dSchema.UIType = cusSchema.UIType
			}
			if cusSchema.Disable != nil {
				dSchema.Disable = cusSchema.Disable
			}
			if cusSchema.SubParameters != nil {
				dSchema.SubParameters = patchSchema(dSchema.SubParameters, cusSchema.SubParameters)
			}
			if cusSchema.Sort != 0 {
				dSchema.Sort = cusSchema.Sort
			}
		}
	}
	sort.Slice(defaultSchema, func(i, j int) bool {
		return defaultSchema[i].Sort < defaultSchema[j].Sort
	})
	return defaultSchema
}

func renderDefaultUISchema(lastKey string, apiSchema *openapi3.Schema) []*utils.UIParameter {
	if apiSchema == nil {
		return nil
	}
	var params []*utils.UIParameter
	for key, property := range apiSchema.Properties {
		nextKey := key
		if lastKey != "" {
			nextKey = fmt.Sprintf("%s.%s", lastKey, key)
		}
		if property.Value != nil {
			param := renderUIParameter(nextKey, utils.FirstUpper(key), property, apiSchema.Required)
			params = append(params, param)
		}
	}
	return params
}

func renderUIParameter(key, label string, property *openapi3.SchemaRef, required []string) *utils.UIParameter {
	var parameter utils.UIParameter
	subType := ""
	if property.Value.Items != nil {
		if property.Value.Items.Value != nil {
			subType = property.Value.Items.Value.Type
		}
		parameter.SubParameters = renderDefaultUISchema(key+".[]", property.Value.Items.Value)
	}
	if property.Value.Properties != nil {
		parameter.SubParameters = renderDefaultUISchema(key, property.Value)
	}
	parameter.Validate = &utils.Validate{}
	parameter.Validate.DefaultValue = property.Value.Default
	for _, enum := range property.Value.Enum {
		parameter.Validate.Options = append(parameter.Validate.Options, utils.Option{Label: utils.RenderLabel(enum), Value: enum})
	}
	parameter.JSONKey = key
	parameter.Description = property.Value.Description
	parameter.Label = label
	parameter.UIType = utils.GetDefaultUIType(property.Value.Type, len(parameter.Validate.Options) != 0, subType)
	parameter.Validate.Max = property.Value.Max
	parameter.Validate.MaxLength = property.Value.MaxLength
	parameter.Validate.Min = property.Value.Min
	parameter.Validate.MinLength = property.Value.MinLength
	parameter.Validate.Pattern = property.Value.Pattern
	parameter.Validate.Required = utils.StringsContain(required, property.Value.Title)
	parameter.Sort = 100
	return &parameter
}
