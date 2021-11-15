package usecase

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	errors2 "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	restutils "github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// AddonUsecase addon usecase
type AddonUsecase interface {
	GetAddonRegistry(ctx context.Context, name string) (*model.AddonRegistry, error)
	CreateAddonRegistry(ctx context.Context, req apis.CreateAddonRegistryRequest) (*apis.AddonRegistryMeta, error)
	DeleteAddonRegistry(ctx context.Context, name string) error
	UpdateAddonRegistry(ctx context.Context, name string, req apis.UpdateAddonRegistryRequest) (*apis.AddonRegistryMeta, error)
	ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistryMeta, error)
	ListAddons(ctx context.Context, detailed bool, registry, query string) ([]*apis.DetailAddonResponse, error)
	StatusAddon(name string) (*apis.AddonStatusResponse, error)
	GetAddon(ctx context.Context, name string, registry string, detailed bool) (*apis.DetailAddonResponse, error)
	EnableAddon(ctx context.Context, name string, args apis.EnableAddonRequest) error
	DisableAddon(ctx context.Context, name string) error
}

// AddonImpl2AddonRes convert types.Addon to the type apiserver need
func AddonImpl2AddonRes(impl *types.Addon) *apis.DetailAddonResponse {
	return &apis.DetailAddonResponse{
		AddonMeta: impl.AddonMeta,
		APISchema: impl.APISchema,
		UISchema:  impl.UISchema,
		Detail:    impl.Detail,
	}
}

// NewAddonUsecase returns a addon usecase
func NewAddonUsecase(ds datastore.DataStore) AddonUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		panic(err)
	}
	return &addonUsecaseImpl{
		addonRegistryCache: make(map[string]*restutils.MemoryCache),
		addonRegistryDS:    ds,
		kubeClient:         kubecli,
		apply:              apply.NewAPIApplicator(kubecli),
	}
}

type addonUsecaseImpl struct {
	addonRegistryCache map[string]*restutils.MemoryCache
	addonRegistryDS    datastore.DataStore
	kubeClient         client.Client
	apply              apply.Applicator
}

// GetAddon will get addon information, if detailed is not set, addon's componennt and internal definition won't be returned
func (u *addonUsecaseImpl) GetAddon(ctx context.Context, name string, registry string, detailed bool) (*apis.DetailAddonResponse, error) {
	addons, err := u.ListAddons(ctx, detailed, registry, "")
	if err != nil {
		return nil, err
	}

	for _, addon := range addons {
		if addon.Name == name {
			return addon, nil
		}
	}
	return nil, bcode.ErrAddonNotExist
}

func (u *addonUsecaseImpl) StatusAddon(name string) (*apis.AddonStatusResponse, error) {
	var app v1beta1.Application
	err := u.kubeClient.Get(context.Background(), client.ObjectKey{
		Namespace: types.DefaultKubeVelaNS,
		Name:      pkgaddon.Convert2AppName(name),
	}, &app)
	if err != nil {
		if errors2.IsNotFound(err) {
			return &apis.AddonStatusResponse{
				Phase:            apis.AddonPhaseDisabled,
				EnablingProgress: nil,
			}, nil
		}
		return nil, bcode.ErrGetAddonApplication
	}

	switch app.Status.Phase {
	case common2.ApplicationRunning, common2.ApplicationWorkflowFinished:
		return &apis.AddonStatusResponse{
			Phase:            apis.AddonPhaseEnabled,
			EnablingProgress: nil,
		}, nil
	default:
		return &apis.AddonStatusResponse{
			Phase:            apis.AddonPhaseEnabling,
			EnablingProgress: nil,
		}, nil
	}
}

// getCacheKeyWithDetailFLag will get right cache key for given registry and detailed, to split different
func getCacheKeyWithDetailFLag(registry string, detailed bool) string {
	if detailed {
		return registry + "detailed"
	}
	return registry
}

func (u *addonUsecaseImpl) ListAddons(ctx context.Context, detailed bool, registry, query string) ([]*apis.DetailAddonResponse, error) {
	if u.isRegistryCacheUpToDate(getCacheKeyWithDetailFLag(registry, detailed)) {
		return u.getRegistryCache(getCacheKeyWithDetailFLag(registry, detailed)), nil
	}
	var addons []*types.Addon
	var listAddons []*types.Addon
	rs, err := u.ListAddonRegistries(ctx)
	if err != nil {
		return nil, err
	}

	for _, r := range rs {
		if registry != "" && r.Name != registry {
			continue
		}
		listAddons, err = pkgaddon.ListAddons(detailed, r.Git)
		if err != nil {
			log.Logger.Errorf("fail to get addons from registry %s", r.Name)
			continue
		}
		addons = mergeAddons(addons, listAddons)
	}

	if query != "" {
		var filtered []*types.Addon
		for i, addon := range addons {
			if strings.Contains(addon.Name, query) || strings.Contains(addon.Description, query) {
				filtered = append(filtered, addons[i])
			}
		}
		addons = filtered
	}

	sort.Slice(addons, func(i, j int) bool {
		return addons[i].Name < addons[j].Name
	})

	var addonRes []*apis.DetailAddonResponse
	for _, a := range addons {
		addonRes = append(addonRes, AddonImpl2AddonRes(a))
	}
	u.putRegistryCache(getCacheKeyWithDetailFLag(registry, detailed), addonRes)
	return addonRes, nil
}

func (u *addonUsecaseImpl) DeleteAddonRegistry(ctx context.Context, name string) error {
	return u.addonRegistryDS.Delete(ctx, &model.AddonRegistry{Name: name})
}

func (u *addonUsecaseImpl) CreateAddonRegistry(ctx context.Context, req apis.CreateAddonRegistryRequest) (*apis.AddonRegistryMeta, error) {
	r := addonRegistryModelFromCreateAddonRegistryRequest(req)

	err := u.addonRegistryDS.Add(ctx, r)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrAddonRegistryExist
		}
		return nil, err
	}

	return &apis.AddonRegistryMeta{
		Name: r.Name,
		Git:  r.Git,
	}, nil
}

func (u *addonUsecaseImpl) GetAddonRegistry(ctx context.Context, name string) (*model.AddonRegistry, error) {
	var r = model.AddonRegistry{
		Name: name,
	}
	err := u.addonRegistryDS.Get(ctx, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (u addonUsecaseImpl) UpdateAddonRegistry(ctx context.Context, name string, req apis.UpdateAddonRegistryRequest) (*apis.AddonRegistryMeta, error) {
	var r = model.AddonRegistry{
		Name: name,
	}
	err := u.addonRegistryDS.Get(ctx, &r)
	if err != nil {
		return nil, bcode.ErrAddonRegistryNotExist
	}
	r.Git = req.Git
	err = u.addonRegistryDS.Put(ctx, &r)
	if err != nil {
		return nil, err
	}

	return &apis.AddonRegistryMeta{
		Name: r.Name,
		Git:  r.Git,
	}, nil
}

func (u *addonUsecaseImpl) ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistryMeta, error) {
	var r = model.AddonRegistry{}

	var list []*apis.AddonRegistryMeta
	entities, err := u.addonRegistryDS.List(ctx, &r, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, entity := range entities {
		list = append(list, ConvertAddonRegistryModel2AddonRegistryMeta(entity.(*model.AddonRegistry)))
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list, nil
}

func (u *addonUsecaseImpl) EnableAddon(ctx context.Context, name string, args apis.EnableAddonRequest) error {
	registries, err := u.ListAddonRegistries(ctx)
	if err != nil {
		return err
	}
	for _, r := range registries {
		addon, err := pkgaddon.GetAddon(name, r.Git)

		if err != nil && err == bcode.ErrAddonNotExist {
			continue
		} else if err != nil {
			return bcode.WrapGithubRateLimitErr(err)
		}

		// render default ui schema
		addon.UISchema = renderDefaultUISchema(addon.APISchema)

		app, err := pkgaddon.RenderApplication(addon, args.Args)
		if err != nil {
			return err
		}
		err = u.kubeClient.Create(ctx, app)
		if err != nil {
			log.Logger.Errorf("apply application fail: %s", err.Error())
			return bcode.ErrAddonApply
		}
		return nil
	}
	return bcode.ErrAddonNotExist
}

func (u *addonUsecaseImpl) getRegistryCache(name string) []*apis.DetailAddonResponse {
	return u.addonRegistryCache[name].GetData().([]*apis.DetailAddonResponse)
}

func (u *addonUsecaseImpl) putRegistryCache(name string, addons []*apis.DetailAddonResponse) {
	u.addonRegistryCache[name] = restutils.NewMemoryCache(addons, time.Minute*3)
}

func (u *addonUsecaseImpl) isRegistryCacheUpToDate(name string) bool {
	d, ok := u.addonRegistryCache[name]
	if !ok {
		return false
	}
	return !d.IsExpired()
}

func (u *addonUsecaseImpl) DisableAddon(ctx context.Context, name string) error {
	app := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "Application"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pkgaddon.Convert2AppName(name),
			Namespace: types.DefaultKubeVelaNS,
		},
	}
	err := u.kubeClient.Delete(ctx, app)
	if err != nil {
		log.Logger.Errorf("delete application fail: %s", err.Error())
		return err
	}
	return nil
}

func addonRegistryModelFromCreateAddonRegistryRequest(req apis.CreateAddonRegistryRequest) *model.AddonRegistry {
	return &model.AddonRegistry{
		Name: req.Name,
		Git:  req.Git,
	}
}

func mergeAddons(a1, a2 []*types.Addon) []*types.Addon {
	for _, item := range a2 {
		if hasAddon(a1, item.Name) {
			continue
		}
		a1 = append(a1, item)
	}
	return a1
}

func hasAddon(addons []*types.Addon, name string) bool {
	for _, addon := range addons {
		if addon.Name == name {
			return true
		}
	}
	return false
}

// ConvertAddonRegistryModel2AddonRegistryMeta will convert from model to AddonRegistryMeta
func ConvertAddonRegistryModel2AddonRegistryMeta(r *model.AddonRegistry) *apis.AddonRegistryMeta {
	return &apis.AddonRegistryMeta{
		Name: r.Name,
		Git:  r.Git,
	}
}
