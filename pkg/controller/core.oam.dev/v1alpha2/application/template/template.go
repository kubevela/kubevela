package template

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/defclient"
)

type manager struct {
	defclient.DefinitionClient
}

// GetHandler  get template handler
func GetHandler(cli defclient.DefinitionClient) Handler {
	m := &manager{
		DefinitionClient: cli,
	}
	return m.LoadTemplate
}

// Handler is template handler type
type Handler func(key string, kind types.CapType) (string, string, error)

// Kind is template kind
type Kind = types.CapType

// LoadTemplate Get template according to key
func (m *manager) LoadTemplate(key string, kd types.CapType) (string, string, error) {
	switch kd {
	case types.TypeWorkload:
		wd, err := m.GetWorkloadDefinition(key)
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
		td, err := m.GetTraitDefinition(key)
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
