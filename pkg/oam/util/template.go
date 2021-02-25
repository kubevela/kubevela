package util

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

// Template includes its string, health and its category
type Template struct {
	TemplateStr        string
	Health             string
	CustomStatus       string
	CapabilityCategory types.CapabilityCategory
}

// GetScopeGVK Get ScopeDefinition
func GetScopeGVK(ctx context.Context, cli client.Reader, dm discoverymapper.DiscoveryMapper,
	name string) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind
	sd := new(v1alpha2.ScopeDefinition)
	err := GetDefinition(ctx, cli, sd, name)
	if err != nil {
		return gvk, err
	}

	return GetGVKFromDefinition(dm, sd.Spec.Reference)
}

// LoadTemplate Get template according to key
func LoadTemplate(ctx context.Context, cli client.Reader, key string, kd types.CapType) (*Template, error) {
	switch kd {
	case types.TypeWorkload:
		wd := new(v1alpha2.WorkloadDefinition)
		err := GetDefinition(ctx, cli, wd, key)
		if err != nil {
			return nil, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		var capabilityCategory types.CapabilityCategory
		if wd.Annotations["type"] == string(types.TerraformCategory) {
			capabilityCategory = types.TerraformCategory
		}
		tmpl, err := NewTemplate(wd.Spec.Schematic, wd.Spec.Status, wd.Spec.Extension)
		if err != nil {
			return nil, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		if tmpl == nil {
			return nil, errors.New("no template found in definition")
		}
		tmpl.CapabilityCategory = capabilityCategory
		return tmpl, nil

	case types.TypeTrait:
		td := new(v1alpha2.TraitDefinition)
		err := GetDefinition(ctx, cli, td, key)
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
		return tmpl, nil
	case types.TypeScope:
		// TODO: add scope template support
	}
	return nil, fmt.Errorf("kind(%s) of %s not supported", kd, key)
}

// NewTemplate will create CUE template for inner AbstractEngine using.
func NewTemplate(schematic *v1alpha2.Schematic, status *v1alpha2.Status, raw *runtime.RawExtension) (*Template, error) {
	var template string
	if schematic != nil && schematic.CUE != nil {
		template = schematic.CUE.Template
	}
	extension := map[string]interface{}{}
	tmp := &Template{
		TemplateStr: template,
	}
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
	if status != nil {
		tmp.CustomStatus = status.CustomStatus
		tmp.Health = status.HealthPolicy
	}
	return tmp, nil
}

// ConvertTemplateJSON2Object convert spec.extension to object
func ConvertTemplateJSON2Object(capabilityName string, in *runtime.RawExtension, schematic *v1alpha2.Schematic) (types.Capability, error) {
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
