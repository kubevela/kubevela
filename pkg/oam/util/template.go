package util

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
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
func GetScopeGVK(cli client.Client, dm discoverymapper.DiscoveryMapper,
	name string) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind
	sd := new(v1alpha2.ScopeDefinition)
	if err := cli.Get(context.Background(), client.ObjectKey{
		Name: name,
	}, sd); err != nil {
		return gvk, err
	}
	return GetGVKFromDefinition(dm, sd.Spec.Reference)
}

// LoadTemplate Get template according to key
func LoadTemplate(cli client.Reader, key string, kd types.CapType) (*Template, error) {
	switch kd {
	case types.TypeWorkload:
		wd, err := GetWorkloadDefinition(context.TODO(), cli, key)
		if err != nil {
			return nil, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		var capabilityCategory types.CapabilityCategory
		if wd.Annotations["type"] == string(types.TerraformCategory) {
			capabilityCategory = types.TerraformCategory
		}
		tmpl, err := getTempl(wd.Spec.Extension.Raw)
		if err != nil {
			return nil, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		if tmpl == nil {
			return nil, errors.New("no template found in definition")
		}
		tmpl.CapabilityCategory = capabilityCategory
		return tmpl, nil

	case types.TypeTrait:
		td, err := GetTraitDefinition(context.TODO(), cli, key)
		if err != nil {
			return nil, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		var capabilityCategory types.CapabilityCategory
		if td.Annotations["type"] == string(types.TerraformCategory) {
			capabilityCategory = types.TerraformCategory
		}
		tmpl, err := getTempl(td.Spec.Extension.Raw)
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

func getTempl(raw []byte) (*Template, error) {
	_tmp := map[string]interface{}{}
	if err := json.Unmarshal(raw, &_tmp); err != nil {
		return nil, err
	}
	var (
		health string
		status string
	)
	if _, ok := _tmp["healthPolicy"]; ok {
		health = fmt.Sprint(_tmp["healthPolicy"])
	}
	if _, ok := _tmp["customStatus"]; ok {
		status = fmt.Sprint(_tmp["customStatus"])
	}
	return &Template{
		TemplateStr:  fmt.Sprint(_tmp["template"]),
		Health:       health,
		CustomStatus: status}, nil
}
