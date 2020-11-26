package cache_client

import (
	"context"
	core "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	v1alpha22 "github.com/oam-dev/kubevela/api/core.oam.dev/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FastClient interface {
	GetWorkloadDefinition(ctx context.Context, ns, name string) (*core.WorkloadDefinition, error)
	GetTraitDefition(ctx context.Context, ns, name string) (*core.TraitDefinition, error)
	GetApplication(ctx context.Context, key client.ObjectKey) (*v1alpha22.Application, error)
}

type factory struct {
	cache.Cache
	client client.Client
}

func NewFastClient(c cache.Cache, cli client.Client) *factory {

	c.GetInformer(context.Background(), &core.WorkloadDefinition{})
	c.GetInformer(context.Background(), &core.TraitDefinition{})

	return &factory{
		c,
		cli,
	}
}

func (f *factory) GetWorkloadDefinition(ctx context.Context, ns, name string) (*core.WorkloadDefinition, error) {
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

func (f *factory) GetTraitDefition(ctx context.Context, ns, name string) (*core.TraitDefinition, error) {
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

func (f *factory) GetApplication(ctx context.Context, key client.ObjectKey) (*v1alpha22.Application, error) {
	app := new(v1alpha22.Application)
	if err := f.Get(ctx, key, app); err != nil {
		if err := f.client.Get(ctx, key, app); err != nil {
			return nil, err
		}
	}
	return app, nil
}
