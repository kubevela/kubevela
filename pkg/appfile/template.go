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

package appfile

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	// UsageTag is usage comment annotation
	UsageTag = "+usage="
	// ShortTag is the short alias annotation
	ShortTag = "+short"
	// InsertSecretToTag marks the value should be set as an context
	InsertSecretToTag = "+insertSecretTo="
)

// Template is a helper struct for processing capability including
// ComponentDefinition, TraitDefinition, ScopeDefinition.
// It mainly collects schematic and status data of a capability definition.
type Template struct {
	TemplateStr        string
	Health             string
	CustomStatus       string
	CapabilityCategory types.CapabilityCategory
	Reference          common.WorkloadGVK
	Helm               *common.Helm
	Kube               *common.Kube
	// TODO: Add scope definition too
	ComponentDefinition *v1beta1.ComponentDefinition
	WorkloadDefinition  *v1beta1.WorkloadDefinition
	TraitDefinition     *v1beta1.TraitDefinition
}

// LoadTemplate gets the capability definition from cluster and resolve it.
// It returns a helper struct, Template, which will be used for further
// processing.
func LoadTemplate(ctx context.Context, dm discoverymapper.DiscoveryMapper, cli client.Reader, capName string, capType types.CapType) (*Template, error) {
	// Application Controller only load template from ComponentDefinition and TraitDefinition
	switch capType {
	case types.TypeComponentDefinition:
		cd := new(v1beta1.ComponentDefinition)
		err := oamutil.GetDefinition(ctx, cli, cd, capName)
		if err != nil {
			if kerrors.IsNotFound(err) {
				wd := new(v1beta1.WorkloadDefinition)
				if err := oamutil.GetDefinition(ctx, cli, wd, capName); err != nil {
					return nil, errors.WithMessagef(err, "LoadTemplate from workloadDefinition [%s] ", capName)
				}
				tmpl, err := newTemplateOfWorkloadDefinition(wd)
				if err != nil {
					return nil, err
				}
				gvk, err := oamutil.GetGVKFromDefinition(dm, wd.Spec.Reference)
				if err != nil {
					return nil, errors.WithMessagef(err, "Get GVK from workload definition [%s]", capName)
				}
				tmpl.Reference = common.WorkloadGVK{
					APIVersion: gvk.GroupVersion().String(),
					Kind:       gvk.Kind,
				}
				return tmpl, nil
			}
			return nil, errors.WithMessagef(err, "LoadTemplate from ComponentDefinition [%s] ", capName)
		}
		tmpl, err := newTemplateOfCompDefinition(cd)
		if err != nil {
			return nil, err
		}
		return tmpl, nil

	case types.TypeTrait:
		td := new(v1beta1.TraitDefinition)
		err := oamutil.GetDefinition(ctx, cli, td, capName)
		if err != nil {
			return nil, errors.WithMessagef(err, "LoadTemplate [%s] ", capName)
		}
		tmpl, err := newTemplateOfTraitDefinition(td)
		if err != nil {
			return nil, err
		}
		return tmpl, nil
	case types.TypeScope:
		// TODO: add scope template support
	default:
		return nil, fmt.Errorf("kind(%s) of %s not supported", capType, capName)
	}
	return nil, fmt.Errorf("kind(%s) of %s not supported", capType, capName)
}

// DryRunTemplateLoader return a function that do the same work as
// LoadTemplate, but load template from provided ones before loading from
// cluster through LoadTemplate
func DryRunTemplateLoader(defs []oam.Object) TemplateLoaderFn {
	return TemplateLoaderFn(func(ctx context.Context, dm discoverymapper.DiscoveryMapper, r client.Reader, capName string, capType types.CapType) (*Template, error) {
		// retrieve provided cap definitions
		for _, def := range defs {
			if unstructDef, ok := def.(*unstructured.Unstructured); ok {
				if unstructDef.GetKind() == v1beta1.ComponentDefinitionKind &&
					capType == types.TypeComponentDefinition && unstructDef.GetName() == capName {
					compDef := &v1beta1.ComponentDefinition{}
					if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructDef.Object, compDef); err != nil {
						return nil, errors.Wrap(err, "invalid component definition")
					}
					tmpl, err := newTemplateOfCompDefinition(compDef)
					if err != nil {
						return nil, errors.WithMessagef(err, "cannot load template of component definition %q", capName)
					}
					return tmpl, nil
				}
				if unstructDef.GetKind() == v1beta1.TraitDefinitionKind &&
					capType == types.TypeTrait && unstructDef.GetName() == capName {
					traitDef := &v1beta1.TraitDefinition{}
					if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructDef.Object, traitDef); err != nil {
						return nil, errors.Wrap(err, "invalid trait definition")
					}
					tmpl, err := newTemplateOfTraitDefinition(traitDef)
					if err != nil {
						return nil, errors.WithMessagef(err, "cannot load template of trait definition %q", capName)
					}
					return tmpl, nil
				}
				// TODO(roywang) add support for ScopeDefinition
			}
		}
		// not found in provided cap definitions
		// then try to retrieve from cluster
		tmpl, err := LoadTemplate(ctx, dm, r, capName, capType)
		if err != nil {
			return nil, errors.WithMessagef(err, "cannot load template %q from cluster and provided ones", capName)
		}
		return tmpl, nil
	})
}

func newTemplateOfCompDefinition(compDef *v1beta1.ComponentDefinition) (*Template, error) {
	tmpl := &Template{
		Reference:           compDef.Spec.Workload.Definition,
		ComponentDefinition: compDef,
	}
	if err := loadSchematicToTemplate(tmpl, compDef.Spec.Status, compDef.Spec.Schematic, compDef.Spec.Extension); err != nil {
		return nil, errors.WithMessage(err, "cannot load template")
	}
	if compDef.Annotations["type"] == string(types.TerraformCategory) {
		tmpl.CapabilityCategory = types.TerraformCategory
	}
	return tmpl, nil
}

func newTemplateOfTraitDefinition(traitDef *v1beta1.TraitDefinition) (*Template, error) {
	tmpl := &Template{
		TraitDefinition: traitDef,
	}
	if err := loadSchematicToTemplate(tmpl, traitDef.Spec.Status, traitDef.Spec.Schematic, traitDef.Spec.Extension); err != nil {
		return nil, errors.WithMessage(err, "cannot load template")
	}
	return tmpl, nil
}

func newTemplateOfWorkloadDefinition(wlDef *v1beta1.WorkloadDefinition) (*Template, error) {
	tmpl := &Template{
		WorkloadDefinition: wlDef,
	}
	if err := loadSchematicToTemplate(tmpl, wlDef.Spec.Status, wlDef.Spec.Schematic, wlDef.Spec.Extension); err != nil {
		return nil, errors.WithMessage(err, "cannot load template")
	}
	return tmpl, nil
}

// loadSchematicToTemplate loads common data that all kind definitions have.
func loadSchematicToTemplate(tmpl *Template, sts *common.Status, schem *common.Schematic, ext *runtime.RawExtension) error {
	if sts != nil {
		tmpl.CustomStatus = sts.CustomStatus
		tmpl.Health = sts.HealthPolicy
	}

	if schem != nil {
		if schem.CUE != nil {
			tmpl.CapabilityCategory = types.CUECategory
			tmpl.TemplateStr = schem.CUE.Template
		}
		if schem.HELM != nil {
			tmpl.CapabilityCategory = types.HelmCategory
			tmpl.Helm = schem.HELM
			return nil
		}
		if schem.KUBE != nil {
			tmpl.CapabilityCategory = types.KubeCategory
			tmpl.Kube = schem.KUBE
			return nil
		}
	}

	if tmpl.TemplateStr == "" && ext != nil {
		tmpl.CapabilityCategory = types.CUECategory
		extension := map[string]interface{}{}
		if err := json.Unmarshal(ext.Raw, &extension); err != nil {
			return errors.Wrap(err, "cannot parse capability extension")
		}
		if extTemplate, ok := extension["template"]; ok {
			if tmpStr, ok := extTemplate.(string); ok {
				tmpl.TemplateStr = tmpStr
				return nil
			}
		}
	}
	return nil
}

// ConvertTemplateJSON2Object convert spec.extension to object
func ConvertTemplateJSON2Object(capabilityName string, in *runtime.RawExtension, schematic *common.Schematic) (types.Capability, error) {
	var t types.Capability
	t.Name = capabilityName
	if in != nil && in.Raw != nil {
		err := json.Unmarshal(in.Raw, &t)
		if err != nil {
			return t, errors.Wrapf(err, "parse extension fail")
		}
	}
	capTemplate := &Template{}
	if err := loadSchematicToTemplate(capTemplate, nil, schematic, in); err != nil {
		return t, errors.WithMessage(err, "cannot resolve schematic")
	}
	if capTemplate.TemplateStr != "" {
		t.CueTemplate = capTemplate.TemplateStr
	}
	return t, nil
}
