package template

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	fclient "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/application/cache-client"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	kyaml "sigs.k8s.io/yaml"
)

type manager struct {
	ns string
	fclient.FastClient
}

func GetHanler(ns string, cli fclient.FastClient) Handler {
	m := &manager{
		ns:         ns,
		FastClient: cli,
	}
	return m.LoadTemplate
}

type Handler func(key string) (string, Kind, error)

type Kind uint16

const (
	WorkloadKind Kind = (1 << iota)
	TraitKind
	Unkownkind
)

func (m *manager) LoadTemplate(key string) (string, Kind, error) {
	ctx := context.Background()
	wd, err := m.GetWorkloadDefinition(ctx, m.ns, key)
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
	td, err := m.GetTraitDefition(ctx, m.ns, key)
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

type MockManager struct {
	wds []*v1alpha2.WorkloadDefinition
	tds []*v1alpha2.TraitDefinition
}

func (mock *MockManager) LoadTemplate(key string) (string, Kind, error) {
	for _, wd := range mock.wds {
		if wd.Name == key {
			jsonRaw, err := getTemplate(wd.Spec.Extension.Raw)
			if err != nil {
				return "", Unkownkind, err
			}
			if jsonRaw != "" {
				return jsonRaw, WorkloadKind, nil
			}
		}
	}

	for _, td := range mock.tds {
		if td.Name == key {
			jsonRaw, err := getTemplate(td.Spec.Extension.Raw)
			if err != nil {
				return "", Unkownkind, err
			}
			if jsonRaw != "" {
				return jsonRaw, TraitKind, nil
			}
		}
	}

	return "", Unkownkind, nil
}

func (mock *MockManager) AddWD(s string) error {
	wd := &v1alpha2.WorkloadDefinition{}
	_body, err := kyaml.YAMLToJSON([]byte(s))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(_body, wd); err != nil {
		return err
	}

	if mock.wds == nil {
		mock.wds = []*v1alpha2.WorkloadDefinition{}
	}
	mock.wds = append(mock.wds, wd)
	return nil
}

func (mock *MockManager) AddTD(s string) error {
	td := &v1alpha2.TraitDefinition{}
	_body, err := kyaml.YAMLToJSON([]byte(s))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(_body, td); err != nil {
		return err
	}
	if mock.tds == nil {
		mock.tds = []*v1alpha2.TraitDefinition{}
	}
	mock.tds = append(mock.tds, td)
	return nil
}
