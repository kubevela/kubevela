package template

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/defclient"
)

type manager struct {
	defclient.DefinitionClient
}

// GetHanler  get template handler
func GetHanler(cli defclient.DefinitionClient) Handler {
	m := &manager{
		DefinitionClient: cli,
	}
	return m.LoadTemplate
}

// Handler is template handler type
type Handler func(key string) (string, Kind, error)

// Kind is template kind
type Kind uint16

const (
	// WorkloadKind ...
	WorkloadKind Kind = (1 << iota)
	// TraitKind ...
	TraitKind
	// Unkownkind ...
	Unkownkind
)

// LoadTemplate Get template according to key
func (m *manager) LoadTemplate(key string) (string, Kind, error) {
	wd, err := m.GetWorkloadDefinition(key)
	if err != nil && !kerrors.IsNotFound(err) {
		return "", Unkownkind, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
	}
	if wd != nil {
		jsonRaw, err := getTemplate(wd.Spec.Extension.Raw)
		if err != nil {
			return "", Unkownkind, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}
		if jsonRaw != "" {
			return jsonRaw, WorkloadKind, nil
		}
	}
	td, err := m.GetTraitDefition(key)
	if err != nil && !kerrors.IsNotFound(err) {
		return "", Unkownkind, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
	}
	if td != nil {
		jsonRaw, err := getTemplate(td.Spec.Extension.Raw)
		if err != nil {
			return "", Unkownkind, errors.WithMessagef(err, "LoadTemplate [%s] ", key)
		}

		if jsonRaw != "" {
			return jsonRaw, TraitKind, nil
		}

	}
	return "", Unkownkind, nil
}

func getTemplate(raw []byte) (string, error) {
	_tmp := map[string]interface{}{}
	if err := json.Unmarshal(raw, &_tmp); err != nil {
		return "", err
	}
	return fmt.Sprint(_tmp["template"]), nil
}
