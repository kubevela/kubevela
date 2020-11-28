package defclient

import (
	"context"

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
	client  client.Client
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

