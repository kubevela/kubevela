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
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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
	ListDefinitions(ctx context.Context, envName, defType, appliedWorkloads string) ([]*apisv1.DefinitionBase, error)
	// DetailDefinition get definition detail
	DetailDefinition(ctx context.Context, name, defType string) (*apisv1.DetailDefinitionResponse, error)
	// AddDefinitionUISchema add or update custom definition ui schema
	AddDefinitionUISchema(ctx context.Context, name, defType, configRaw string) ([]*utils.UIParameter, error)
}

type definitionUsecaseImpl struct {
	kubeClient client.Client
	caches     *utils.MemoryCacheStore
}

const (
	definitionAPIVersion       = "core.oam.dev/v1beta1"
	kindComponentDefinition    = "ComponentDefinition"
	kindTraitDefinition        = "TraitDefinition"
	kindWorkflowStepDefinition = "WorkflowStepDefinition"
	kindPolicyDefinition       = "PolicyDefinition"
)

// NewDefinitionUsecase new definition usecase
func NewDefinitionUsecase() DefinitionUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &definitionUsecaseImpl{kubeClient: kubecli, caches: utils.NewMemoryCacheStore(context.Background())}
}

func (d *definitionUsecaseImpl) ListDefinitions(ctx context.Context, envName, defType, appliedWorkload string) ([]*apisv1.DefinitionBase, error) {
	defs := &unstructured.UnstructuredList{}
	switch defType {
	case "component":
		defs.SetAPIVersion(definitionAPIVersion)
		defs.SetKind(kindComponentDefinition)
		return d.listDefinitions(ctx, defs, kindComponentDefinition, "")

	case "trait":
		defs.SetAPIVersion(definitionAPIVersion)
		defs.SetKind(kindTraitDefinition)
		return d.listDefinitions(ctx, defs, kindTraitDefinition, appliedWorkload)

	case "workflowstep":
		defs.SetAPIVersion(definitionAPIVersion)
		defs.SetKind(kindWorkflowStepDefinition)
		return d.listDefinitions(ctx, defs, kindWorkflowStepDefinition, "")

	case "policy":
		defs.SetAPIVersion(definitionAPIVersion)
		defs.SetKind(kindPolicyDefinition)
		return d.listDefinitions(ctx, defs, kindPolicyDefinition, "")

	default:
		return nil, bcode.ErrDefinitionTypeNotSupport
	}
}

func (d *definitionUsecaseImpl) listDefinitions(ctx context.Context, list *unstructured.UnstructuredList, cache, appliedWorkload string) ([]*apisv1.DefinitionBase, error) {
	if mc := d.caches.Get(cache); mc != nil && appliedWorkload == "" {
		return mc.([]*apisv1.DefinitionBase), nil
	}
	matchLabels := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      types.LabelDefinitionDeprecated,
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
			{
				Key:      types.LabelDefinitionHidden,
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(&matchLabels)
	if err != nil {
		return nil, err
	}
	if err := d.kubeClient.List(ctx, list, &client.ListOptions{
		LabelSelector: selector,
	}); err != nil {
		return nil, err
	}
	var defs []*apisv1.DefinitionBase
	for _, def := range list.Items {
		if appliedWorkload != "" {
			traitDef := &v1beta1.TraitDefinition{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(def.Object, traitDef); err != nil {
				return nil, errors.Wrap(err, "invalid trait definition")
			}
			filter := false
			for _, workload := range traitDef.Spec.AppliesToWorkloads {
				if workload == appliedWorkload || workload == "*" {
					filter = true
					break
				}
			}
			if !filter {
				continue
			}
		}
		definition := &apisv1.DefinitionBase{
			Name:        def.GetName(),
			Description: def.GetAnnotations()[types.AnnoDefinitionDescription],
			Icon:        def.GetAnnotations()[types.AnnoDefinitionIcon],
		}
		if cache == kindComponentDefinition {
			compDef := &v1beta1.ComponentDefinition{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(def.Object, compDef); err != nil {
				return nil, errors.Wrap(err, "invalid component definition")
			}
			definition.WorkloadType = compDef.Spec.Workload.Type
			definition.Component = &compDef.Spec
		}
		if cache == kindTraitDefinition {
			traitDef := &v1beta1.TraitDefinition{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(def.Object, traitDef); err != nil {
				return nil, errors.Wrap(err, "invalid trait definition")
			}
			definition.Trait = &traitDef.Spec
		}
		if cache == kindWorkflowStepDefinition {
			workflowStepDef := &v1beta1.WorkflowStepDefinition{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(def.Object, workflowStepDef); err != nil {
				return nil, errors.Wrap(err, "invalid trait definition")
			}
			definition.WorkflowStep = &workflowStepDef.Spec
		}
		if cache == kindPolicyDefinition {
			policyDef := &v1beta1.PolicyDefinition{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(def.Object, policyDef); err != nil {
				return nil, errors.Wrap(err, "invalid trait definition")
			}
			definition.Policy = &policyDef.Spec
		}
		defs = append(defs, definition)
	}
	if appliedWorkload == "" {
		d.caches.Put(cache, defs, time.Minute*3)
	}
	return defs, nil
}

// DetailDefinition get definition detail
func (d *definitionUsecaseImpl) DetailDefinition(ctx context.Context, name, defType string) (*apisv1.DetailDefinitionResponse, error) {
	if !utils.StringsContain([]string{"component", "trait", "workflowstep", "policy"}, defType) {
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
	defaultUISchema := renderDefaultUISchema(schema)
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

// AddDefinitionUISchema add definition custom ui schema config
func (d *definitionUsecaseImpl) AddDefinitionUISchema(ctx context.Context, name, defType, configRaw string) ([]*utils.UIParameter, error) {
	var uiParameters []*utils.UIParameter
	err := yaml.Unmarshal([]byte(configRaw), &uiParameters)
	if err != nil {
		log.Logger.Errorf("yaml unmarshal failure %s", err.Error())
		return nil, bcode.ErrInvalidDefinitionUISchema
	}
	dataBate, err := json.Marshal(uiParameters)
	if err != nil {
		log.Logger.Errorf("json marshal failure %s", err.Error())
		return nil, bcode.ErrInvalidDefinitionUISchema
	}
	var cm v1.ConfigMap
	if err := d.kubeClient.Get(ctx, k8stypes.NamespacedName{
		Namespace: types.DefaultKubeVelaNS,
		Name:      fmt.Sprintf("%s-uischema-%s", defType, name),
	}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			err = d.kubeClient.Create(ctx, &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: types.DefaultKubeVelaNS,
					Name:      fmt.Sprintf("%s-uischema-%s", defType, name),
				},
				Data: map[string]string{
					types.UISchema: string(dataBate),
				},
			})
		}
		if err != nil {
			return nil, err
		}
	} else {
		cm.Data[types.UISchema] = string(dataBate)
		err := d.kubeClient.Update(ctx, &cm)
		if err != nil {
			return nil, err
		}
	}
	return uiParameters, nil
}

func patchSchema(defaultSchema, customSchema []*utils.UIParameter) []*utils.UIParameter {
	var customSchemaMap = make(map[string]*utils.UIParameter, len(customSchema))
	for i, custom := range customSchema {
		customSchemaMap[custom.JSONKey] = customSchema[i]
	}
	if len(defaultSchema) == 0 {
		return customSchema
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
			if cusSchema.Additional != nil {
				dSchema.Additional = cusSchema.Additional
			}
			if cusSchema.Style != nil {
				dSchema.Style = cusSchema.Style
			}
			if cusSchema.Conditions != nil {
				dSchema.Conditions = cusSchema.Conditions
			}
		}
	}
	sort.Slice(defaultSchema, func(i, j int) bool {
		return defaultSchema[i].Sort < defaultSchema[j].Sort
	})
	return defaultSchema
}

func renderDefaultUISchema(apiSchema *openapi3.Schema) []*utils.UIParameter {
	if apiSchema == nil {
		return nil
	}
	var params []*utils.UIParameter
	for key, property := range apiSchema.Properties {
		if property.Value != nil {
			param := renderUIParameter(key, utils.FirstUpper(key), property, apiSchema.Required)
			params = append(params, param)
		}
	}
	sortDefaultUISchema(params)
	return params
}

// Sort Default UISchema
// 1.Check validate.required. It is True, the sort number will be lower.
// 2.Check subParameters. The more subparameters, the larger the sort number.
// 3.If validate.required or subParameters is equal, sort by Label
//
// The sort number starts with 100.
func sortDefaultUISchema(params []*utils.UIParameter) {
	sort.Slice(params, func(i, j int) bool {
		switch {
		case params[i].Validate.Required && !params[j].Validate.Required:
			return true
		case !params[i].Validate.Required && params[j].Validate.Required:
			return false
		default:
			switch {
			case len(params[i].SubParameters) < len(params[j].SubParameters):
				return true
			case len(params[i].SubParameters) > len(params[j].SubParameters):
				return false
			default:
				return params[i].Label < params[j].Label
			}
		}
	})
	for i, param := range params {
		param.Sort += uint(i)
	}
}

func renderUIParameter(key, label string, property *openapi3.SchemaRef, required []string) *utils.UIParameter {
	var parameter utils.UIParameter
	subType := ""
	if property.Value.Items != nil {
		if property.Value.Items.Value != nil {
			subType = property.Value.Items.Value.Type
		}
		parameter.SubParameters = renderDefaultUISchema(property.Value.Items.Value)
	}
	if property.Value.Properties != nil {
		parameter.SubParameters = renderDefaultUISchema(property.Value)
	}
	if property.Value.AdditionalProperties != nil {
		parameter.SubParameters = renderDefaultUISchema(property.Value.AdditionalProperties.Value)
		var enable = true
		value := property.Value.AdditionalProperties.Value
		parameter.AdditionalParameter = renderUIParameter(value.Title, utils.FirstUpper(value.Title), property.Value.AdditionalProperties, value.Required)
		parameter.Additional = &enable
	}
	parameter.Validate = &utils.Validate{}
	parameter.Validate.DefaultValue = property.Value.Default
	for _, enum := range property.Value.Enum {
		parameter.Validate.Options = append(parameter.Validate.Options, utils.Option{Label: utils.RenderLabel(enum), Value: enum})
	}
	parameter.JSONKey = key
	parameter.Description = property.Value.Description
	parameter.Label = label
	parameter.UIType = utils.GetDefaultUIType(property.Value.Type, len(parameter.Validate.Options) != 0, subType, len(property.Value.Properties) > 0)
	parameter.Validate.Max = property.Value.Max
	parameter.Validate.MaxLength = property.Value.MaxLength
	parameter.Validate.Min = property.Value.Min
	parameter.Validate.MinLength = property.Value.MinLength
	parameter.Validate.Pattern = property.Value.Pattern
	parameter.Validate.Required = utils.StringsContain(required, property.Value.Title)
	parameter.Sort = 100
	return &parameter
}
