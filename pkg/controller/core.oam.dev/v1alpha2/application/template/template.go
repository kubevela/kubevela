package template

import (
	"encoding/json"
	"fmt"

	"github.com/oam-dev/kubevela/apis/types"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"

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
type Handler func(key string, kind types.CapType) (string, error)

// Kind is template kind
type Kind = types.CapType

// LoadTemplate Get template according to key
func (m *manager) LoadTemplate(key string, kd types.CapType) (string, error) {
	switch kd {
	case types.TypeWorkload:
		wd, err := m.GetWorkloadDefinition(key)
		if err != nil {
			return "", errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		jsonRaw, err := getTemplate(wd.Spec.Extension.Raw)
		if err != nil {
			return "", errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		if jsonRaw == "" {
			return "", errors.New("no template found in definition")
		}
		return jsonRaw, nil

	case types.TypeTrait:
		td, err := m.GetTraitDefition(key)
		if err != nil && !kerrors.IsNotFound(err) {
			return "", errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		jsonRaw, err := getTemplate(td.Spec.Extension.Raw)
		if err != nil {
			return "", errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		if jsonRaw == "" {
			return "", errors.New("no template found in definition")
		}
		return jsonRaw, nil
	case types.TypeScope:
		// TODO: add scope template support
	}

	return "", fmt.Errorf("kind(%s) of %s not supported", kd, key)
}

func getTemplate(raw []byte) (string, error) {
	_tmp := map[string]interface{}{}
	if err := json.Unmarshal(raw, &_tmp); err != nil {
		return "", err
	}
	return fmt.Sprint(_tmp["template"]), nil
}
