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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
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

// Template includes its string, health and its category
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

// GetScopeGVK Get ScopeDefinition
func GetScopeGVK(ctx context.Context, cli client.Reader, dm discoverymapper.DiscoveryMapper,
	name string) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind
	sd := new(v1alpha2.ScopeDefinition)
	err := oamutil.GetDefinition(ctx, cli, sd, name)
	if err != nil {
		return gvk, err
	}

	return oamutil.GetGVKFromDefinition(dm, sd.Spec.Reference)
}

// LoadTemplate Get template according to key
func LoadTemplate(ctx context.Context, dm discoverymapper.DiscoveryMapper, cli client.Reader, key string, kd types.CapType) (*Template, error) {
	// Application Controller only load template from ComponentDefinition and TraitDefinition
	switch kd {
	case types.TypeComponentDefinition:
		var schematic *common.Schematic
		var status *common.Status
		var extension *runtime.RawExtension

		cd := new(v1beta1.ComponentDefinition)
		err := oamutil.GetDefinition(ctx, cli, cd, key)
		if err != nil {
			if kerrors.IsNotFound(err) {
				wd := new(v1beta1.WorkloadDefinition)
				if err := oamutil.GetDefinition(ctx, cli, wd, key); err != nil {
					return nil, errors.WithMessagef(err, "LoadTemplate from workloadDefinition [%s] ", key)
				}
				schematic, status, extension = wd.Spec.Schematic, wd.Spec.Status, wd.Spec.Extension
				tmpl, err := NewTemplate(schematic, status, extension)
				if err != nil {
					return nil, errors.WithMessagef(err, "Create template [%s] from workload definition", key)
				}
				if cd.Annotations["type"] == string(types.TerraformCategory) {
					tmpl.CapabilityCategory = types.TerraformCategory
				}
				tmpl.WorkloadDefinition = wd
				gvk, err := oamutil.GetGVKFromDefinition(dm, wd.Spec.Reference)
				if err != nil {
					return nil, errors.WithMessagef(err, "Get GVK from workload definition [%s]", key)
				}
				tmpl.Reference = common.WorkloadGVK{
					APIVersion: gvk.GroupVersion().String(),
					Kind:       gvk.Kind,
				}
				return tmpl, nil
			}
			return nil, errors.WithMessagef(err, "LoadTemplate from ComponentDefinition [%s] ", key)
		}
		schematic, status, extension = cd.Spec.Schematic, cd.Spec.Status, cd.Spec.Extension
		tmpl, err := NewTemplate(schematic, status, extension)
		if err != nil {
			return nil, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		if cd.Annotations["type"] == string(types.TerraformCategory) {
			tmpl.CapabilityCategory = types.TerraformCategory
		}
		tmpl.ComponentDefinition = cd
		tmpl.Reference = cd.Spec.Workload.Definition
		return tmpl, nil

	case types.TypeTrait:
		td := new(v1beta1.TraitDefinition)
		err := oamutil.GetDefinition(ctx, cli, td, key)
		if err != nil {
			return nil, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		var capabilityCategory types.CapabilityCategory
		if td.Annotations["type"] == string(types.TerraformCategory) {
			capabilityCategory = types.TerraformCategory
		}
		tmpl, err := NewTemplate(td.Spec.Schematic, td.Spec.Status, td.Spec.Extension)
		if err != nil {
			return nil, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		if tmpl == nil {
			return nil, errors.New("no template found in definition")
		}
		tmpl.CapabilityCategory = capabilityCategory
		tmpl.TraitDefinition = td
		return tmpl, nil
	case types.TypeScope:
		// TODO: add scope template support
	default:
		return nil, fmt.Errorf("kind(%s) of %s not supported", kd, key)
	}
	return nil, fmt.Errorf("kind(%s) of %s not supported", kd, key)
}

// NewTemplate will create template for inner AbstractEngine using.
func NewTemplate(schematic *common.Schematic, status *common.Status, raw *runtime.RawExtension) (*Template, error) {
	tmp := &Template{}

	if status != nil {
		tmp.CustomStatus = status.CustomStatus
		tmp.Health = status.HealthPolicy
	}
	if schematic != nil {
		if schematic.CUE != nil {
			tmp.TemplateStr = schematic.CUE.Template
			// CUE module has highest priority
			// no need to check other schematic types
			return tmp, nil
		}
		if schematic.HELM != nil {
			tmp.Helm = schematic.HELM
			tmp.CapabilityCategory = types.HelmCategory
			return tmp, nil
		}
		if schematic.KUBE != nil {
			tmp.Kube = schematic.KUBE
			tmp.CapabilityCategory = types.KubeCategory
		}
	}

	extension := map[string]interface{}{}
	if tmp.TemplateStr == "" && raw != nil {
		if err := json.Unmarshal(raw.Raw, &extension); err != nil {
			return nil, err
		}
		if extTemplate, ok := extension["template"]; ok {
			if tmpStr, ok := extTemplate.(string); ok {
				tmp.TemplateStr = tmpStr
			}
		}
	}
	return tmp, nil
}

// ConvertTemplateJSON2Object convert spec.extension to object
func ConvertTemplateJSON2Object(capabilityName string, in *runtime.RawExtension, schematic *common.Schematic) (types.Capability, error) {
	var t types.Capability
	t.Name = capabilityName
	capTemplate, err := NewTemplate(schematic, nil, in)

	if err != nil {
		return t, errors.Wrapf(err, "parse cue template")
	}
	if in != nil && in.Raw != nil {
		err := json.Unmarshal(in.Raw, &t)
		if err != nil {
			return t, errors.Wrapf(err, "parse extension fail")
		}
	}
	if capTemplate.TemplateStr != "" {
		t.CueTemplate = capTemplate.TemplateStr
	}
	return t, err
}
