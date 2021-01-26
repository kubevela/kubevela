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
func LoadTemplate(cli client.Reader, key string, kd types.CapType) (string, string, error) {
	switch kd {
	case types.TypeWorkload:
		wd, err := GetWorkloadDefinition(cli, key)
		if err != nil {
			return "", "", errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		tmpl, health, err := getTemplAndHealth(wd.Spec.Extension.Raw)
		if err != nil {
			return "", "", errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		if tmpl == "" {
			return "", "", errors.New("no template found in definition")
		}
		return tmpl, health, nil

	case types.TypeTrait:
		td, err := GetTraitDefinition(cli, key)
		if err != nil {
			return "", "", errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		tmpl, health, err := getTemplAndHealth(td.Spec.Extension.Raw)
		if err != nil {
			return "", "", errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		if tmpl == "" {
			return "", "", errors.New("no template found in definition")
		}
		return tmpl, health, nil
	case types.TypeScope:
		// TODO: add scope template support
	}

	return "", "", fmt.Errorf("kind(%s) of %s not supported", kd, key)
}

func getTemplAndHealth(raw []byte) (string, string, error) {
	_tmp := map[string]interface{}{}
	if err := json.Unmarshal(raw, &_tmp); err != nil {
		return "", "", err
	}
	var health string
	if _, ok := _tmp["healthPolicy"]; ok {
		health = fmt.Sprint(_tmp["healthPolicy"])
	}
	return fmt.Sprint(_tmp["template"]), health, nil
}
