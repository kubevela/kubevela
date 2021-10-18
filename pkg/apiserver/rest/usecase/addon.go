package usecase

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// AddonUsecase addon usecase
type AddonUsecase interface {
	ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistryMeta, error)
	GetAddonModel(ctx context.Context, name string) (*model.Addon, error)
	DetailAddon(ctx context.Context, addon *model.Addon) (apis.DetailAddonResponse, error)
	CreateAddonRegistry(ctx context.Context, req apis.CreateAddonRegistryRequest) (*apis.AddonRegistryMeta, error)
}

// NewAddonUsecase returns a addon usecase
func NewAddonUsecase(ds datastore.DataStore) AddonUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		panic(err)
	}
	return &addonUsecaseImpl{
		ds:         ds,
		kubeClient: kubecli,
		apply:      apply.NewAPIApplicator(kubecli),
	}
}

type addonUsecaseImpl struct {
	ds         datastore.DataStore
	kubeClient client.Client
	apply      apply.Applicator
}

func (a *addonUsecaseImpl) CreateAddonRegistry(ctx context.Context, req apis.CreateAddonRegistryRequest) (*apis.AddonRegistryMeta, error) {
	panic("implement me")
}

func (a *addonUsecaseImpl) GetAddonModel(ctx context.Context, name string) (*model.Addon, error) {
	panic("implement me")
}

func (a *addonUsecaseImpl) DetailAddon(ctx context.Context, addon *model.Addon) (apis.DetailAddonResponse, error) {
	panic("implement me")
}

func (a *addonUsecaseImpl) ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistryMeta, error) {
	panic("implement me")
}
