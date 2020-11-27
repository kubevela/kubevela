package cacheclient

import (
	"context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"

	core "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FastClient is a interface
type FastClient interface {
	GetWorkloadDefinition(ctx context.Context, ns, name string) (*core.WorkloadDefinition, error)
	GetTraitDefition(ctx context.Context, ns, name string) (*core.TraitDefinition, error)
	GetApplication(ctx context.Context, key client.ObjectKey) (*v1alpha2.Application, error)
}

// Factory can get wd|td|app
type Factory struct {
	cache.Cache
	client client.Client
}

// NewFastClient generate fast client
func NewFastClient(c cache.Cache, cli client.Client) *Factory {

	_, _ = c.GetInformer(context.Background(), &core.WorkloadDefinition{})
	_, _ = c.GetInformer(context.Background(), &core.TraitDefinition{})

	return &Factory{
		c,
		cli,
	}
}

// GetWorkloadDefinition  Get WorkloadDefinition
func (f *Factory) GetWorkloadDefinition(ctx context.Context, ns, name string) (*core.WorkloadDefinition, error) {
	wd := new(core.WorkloadDefinition)
	key := client.ObjectKey{
		Namespace: ns,
		Name:      name,
	}
	if err := f.Get(ctx, key, wd); err != nil {
		return nil, err
	}
	return wd, nil
}

// GetTraitDefition Get TraitDefition
func (f *Factory) GetTraitDefition(ctx context.Context, ns, name string) (*core.TraitDefinition, error) {
	td := new(core.TraitDefinition)
	key := client.ObjectKey{
		Namespace: ns,
		Name:      name,
	}
	if err := f.Get(ctx, key, td); err != nil {
		return nil, err
	}
	return td, nil
}

// GetApplication Get Application
func (f *Factory) GetApplication(ctx context.Context, key client.ObjectKey) (*v1alpha2.Application, error) {
	app := new(v1alpha2.Application)

	if err := f.Get(ctx, key, app); err != nil {
		if err := f.client.Get(ctx, key, app); err != nil {
			return nil, err
		}
	}
	return app, nil
}
