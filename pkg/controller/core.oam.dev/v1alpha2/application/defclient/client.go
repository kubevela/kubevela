package defclient

import (
	"context"
	"encoding/json"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kyaml "sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefinitionClient is a interface
type DefinitionClient interface {
	GetWorkloadDefinition(name string) (*v1alpha2.WorkloadDefinition, error)
	GetTraitDefition(name string) (*v1alpha2.TraitDefinition, error)
}

// Factory can get wd|td|app
type Factory struct {
	client client.Client
}

// NewDefinitionClient generate definition fetcher
func NewDefinitionClient(cli client.Client) *Factory {
	f := &Factory{
		client: cli,
	}
	return f
}

// GetWorkloadDefinition  Get WorkloadDefinition
func (f *Factory) GetWorkloadDefinition(name string) (*v1alpha2.WorkloadDefinition, error) {

	wd := new(v1alpha2.WorkloadDefinition)
	if err := f.client.Get(context.Background(), client.ObjectKey{
		Name: name,
	}, wd); err != nil {
		return nil, err
	}
	return wd, nil
}

// GetTraitDefition Get TraitDefition
func (f *Factory) GetTraitDefition(name string) (*v1alpha2.TraitDefinition, error) {

	td := new(v1alpha2.TraitDefinition)
	if err := f.client.Get(context.Background(), client.ObjectKey{
		Name: name,
	}, td); err != nil {
		return nil, err
	}
	return td, nil
}

// MockClient simulate the behavior of client
type MockClient struct {
	wds []*v1alpha2.WorkloadDefinition
	tds []*v1alpha2.TraitDefinition
}

// GetWorkloadDefinition  Get WorkloadDefinition
func (mock *MockClient) GetWorkloadDefinition(name string) (*v1alpha2.WorkloadDefinition, error) {
	for _, wd := range mock.wds {
		if wd.Name == name {
			return wd, nil
		}
	}
	return nil, kerrors.NewNotFound(schema.GroupResource{
		Group:    v1alpha2.Group,
		Resource: "WorkloadDefinition",
	}, name)
}

// GetTraitDefition Get TraitDefition
func (mock *MockClient) GetTraitDefition(name string) (*v1alpha2.TraitDefinition, error) {
	for _, td := range mock.tds {
		if td.Name == name {
			return td, nil
		}
	}
	return nil, kerrors.NewNotFound(schema.GroupResource{
		Group:    v1alpha2.Group,
		Resource: "TraitDefinition",
	}, name)
}

// AddWD  add workload definition to Mock Manager
func (mock *MockClient) AddWD(s string) error {
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

// AddTD add triat definition to Mock Manager
func (mock *MockClient) AddTD(s string) error {
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
