package defclient

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// DefinitionClient is a interface
type DefinitionClient interface {
	GetWorkloadDefinition(name string) (*v1alpha2.WorkloadDefinition, error)
	GetTraitDefinition(name string) (*v1alpha2.TraitDefinition, error)
	GetScopeGVK(name string) (schema.GroupVersionKind, error)
}

// Factory can get wd|td|app
type Factory struct {
	client client.Client
	dm     discoverymapper.DiscoveryMapper
}

// NewDefinitionClient generate definition fetcher
func NewDefinitionClient(cli client.Client, dm discoverymapper.DiscoveryMapper) *Factory {
	f := &Factory{
		client: cli,
		dm:     dm,
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

// GetTraitDefinition Get TraitDefinition
func (f *Factory) GetTraitDefinition(name string) (*v1alpha2.TraitDefinition, error) {

	td := new(v1alpha2.TraitDefinition)
	if err := f.client.Get(context.Background(), client.ObjectKey{
		Name: name,
	}, td); err != nil {
		return nil, err
	}
	return td, nil
}

// GetScopeGVK Get ScopeDefinition
func (f *Factory) GetScopeGVK(name string) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind
	sd := new(v1alpha2.ScopeDefinition)
	if err := f.client.Get(context.Background(), client.ObjectKey{
		Name: name,
	}, sd); err != nil {
		return gvk, err
	}
	return util.GetGVKFromDefinition(f.dm, sd.Spec.Reference)
}
