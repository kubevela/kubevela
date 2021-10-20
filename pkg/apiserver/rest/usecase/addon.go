package usecase

import (
	"context"
	"errors"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// AddonUsecase addon usecase
type AddonUsecase interface {
	ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistryMeta, error)
	GetAddonModel(ctx context.Context, name string) (*model.Addon, error)
	GetAddonRegistryModel(ctx context.Context, name string) (*model.AddonRegistry, error)
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

func (u *addonUsecaseImpl) CreateAddonRegistry(ctx context.Context, req apis.CreateAddonRegistryRequest) (*apis.AddonRegistryMeta, error) {
	r := addonRegistryModelFromCreateAddonRegistryRequest(req)
	t := time.Now()
	r.SetCreateTime(t)
	r.SetCreateTime(t)

	err := u.ds.Add(ctx, r)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrAddonExist
		}
		return nil, err
	}

	return &apis.AddonRegistryMeta{
		Name: r.Name,
		Git:  r.Git,
	}, nil

}

func (u *addonUsecaseImpl) GetAddonModel(ctx context.Context, name string) (*model.Addon, error) {
	var addon = model.Addon{
		Name: name,
	}
	err := u.ds.Get(ctx, &addon)
	if err != nil {
		return nil, err
	}
	return &addon, nil
}

func (u *addonUsecaseImpl) GetAddonRegistryModel(ctx context.Context, name string) (*model.AddonRegistry, error) {
	var r = model.AddonRegistry{
		Name: name,
	}
	err := u.ds.Get(ctx, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (u *addonUsecaseImpl) DetailAddon(ctx context.Context, addon *model.Addon) (apis.DetailAddonResponse, error) {
	panic("implement me")
}

func (u *addonUsecaseImpl) ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistryMeta, error) {
	var r = model.AddonRegistry{}
	entities, err := u.ds.List(ctx, &r, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apisv1.AddonRegistryMeta
	for _, entity := range entities {
		list = append(list, utils.ConvertAddonRegistryModel2AddonRegistryMeta(entity.(*model.AddonRegistry)))
	}
	return list, nil
}

func addonRegistryModelFromCreateAddonRegistryRequest(req apisv1.CreateAddonRegistryRequest) *model.AddonRegistry {
	return &model.AddonRegistry{
		Name: req.Name,
		Git:  req.Git,
	}
}
