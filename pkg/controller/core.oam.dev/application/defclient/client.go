package defclient

import (
	"context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"

	core "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefinitionClient is a interface
type DefinitionClient interface {
	GetWorkloadDefinition(name string) (*core.WorkloadDefinition, error)
	GetTraitDefition(name string) (*core.TraitDefinition, error)
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
func (f *Factory) GetWorkloadDefinition(name string) (*core.WorkloadDefinition, error) {

	wd := new(core.WorkloadDefinition)
	if err := f.client.Get(context.Background(), client.ObjectKey{
		Name: name,
	}, wd); err != nil {
		return nil, err
	}
	return wd, nil
}

// GetTraitDefition Get TraitDefition
func (f *Factory) GetTraitDefition(name string) (*core.TraitDefinition, error) {

	td := new(core.TraitDefinition)
	if err := f.client.Get(context.Background(), client.ObjectKey{
		Name: name,
	}, td); err != nil {
		return nil, err
	}
	return td, nil
}

// GetApplication Get Application
func (f *Factory) GetApplication(ctx context.Context, key client.ObjectKey) (*v1alpha2.Application, error) {
	app := new(v1alpha2.Application)

	if err := f.client.Get(ctx, key, app); err != nil {
		return nil, err
	}

	return app, nil
}
