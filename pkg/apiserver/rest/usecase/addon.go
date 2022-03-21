/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	velaerr "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// AddonHandler handle CRUD and installation of addons
type AddonHandler interface {
	GetAddonRegistry(ctx context.Context, name string) (*apis.AddonRegistry, error)
	CreateAddonRegistry(ctx context.Context, req apis.CreateAddonRegistryRequest) (*apis.AddonRegistry, error)
	DeleteAddonRegistry(ctx context.Context, name string) error
	UpdateAddonRegistry(ctx context.Context, name string, req apis.UpdateAddonRegistryRequest) (*apis.AddonRegistry, error)
	ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistry, error)
	ListAddons(ctx context.Context, registry, query string) ([]*apis.DetailAddonResponse, error)
	StatusAddon(ctx context.Context, name string) (*apis.AddonStatusResponse, error)
	GetAddon(ctx context.Context, name string, registry string) (*apis.DetailAddonResponse, error)
	EnableAddon(ctx context.Context, name string, args apis.EnableAddonRequest) error
	DisableAddon(ctx context.Context, name string, force bool) error
	ListEnabledAddon(ctx context.Context) ([]*apis.AddonBaseStatus, error)
	UpdateAddon(ctx context.Context, name string, args apis.EnableAddonRequest) error
}

// AddonImpl2AddonRes convert pkgaddon.UIData to the type apiserver need
func AddonImpl2AddonRes(impl *pkgaddon.UIData) (*apis.DetailAddonResponse, error) {
	var defs []*apis.AddonDefinition
	for _, def := range impl.Definitions {
		obj := &unstructured.Unstructured{}
		dec := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
		_, _, err := dec.Decode([]byte(def.Data), nil, obj)
		if err != nil {
			return nil, fmt.Errorf("convert %s file content to definition fail", def.Name)
		}
		defs = append(defs, &apis.AddonDefinition{
			Name:        obj.GetName(),
			DefType:     obj.GetKind(),
			Description: obj.GetAnnotations()["definition.oam.dev/description"],
		})
	}
	return &apis.DetailAddonResponse{
		Meta:         impl.Meta,
		APISchema:    impl.APISchema,
		UISchema:     impl.UISchema,
		Detail:       impl.Detail,
		Definitions:  defs,
		RegistryName: impl.RegistryName,
	}, nil
}

// NewAddonUsecase returns an addon usecase
func NewAddonUsecase(cacheTime time.Duration) AddonHandler {
	config, err := clients.GetKubeConfig()
	if err != nil {
		panic(err)
	}
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		panic(err)
	}
	dc, err := clients.GetDiscoveryClient()
	if err != nil {
		panic(err)
	}
	ds := pkgaddon.NewRegistryDataStore(kubecli)
	cache := pkgaddon.NewCache(ds)

	// TODO(@wonderflow): it's better to add a close channel here, but it should be fine as it's only invoke once in APIServer.
	go cache.DiscoverAndRefreshLoop(cacheTime)

	return &defaultAddonHandler{
		addonRegistryCache: cache,
		addonRegistryDS:    ds,
		kubeClient:         kubecli,
		config:             config,
		apply:              apply.NewAPIApplicator(kubecli),
		mutex:              new(sync.RWMutex),
		discoveryClient:    dc,
	}
}

type defaultAddonHandler struct {
	addonRegistryCache *pkgaddon.Cache
	addonRegistryDS    pkgaddon.RegistryDataStore
	kubeClient         client.Client
	config             *rest.Config
	apply              apply.Applicator
	discoveryClient    *discovery.DiscoveryClient

	mutex *sync.RWMutex
}

// GetAddon will get addon information
func (u *defaultAddonHandler) GetAddon(ctx context.Context, name string, registry string) (*apis.DetailAddonResponse, error) {
	var addon *pkgaddon.UIData
	var err error
	if registry == "" {
		registries, err := u.addonRegistryDS.ListRegistries(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range registries {
			addon, err = u.addonRegistryCache.GetUIData(r, name)
			if err != nil && !errors.Is(err, pkgaddon.ErrNotExist) {
				return nil, err
			}
			if addon != nil {
				break
			}
		}
	} else {
		addonRegistry, err := u.addonRegistryDS.GetRegistry(ctx, registry)
		if err != nil {
			return nil, err
		}
		addon, err = u.addonRegistryCache.GetUIData(addonRegistry, name)
		if err != nil && !errors.Is(err, pkgaddon.ErrNotExist) {
			return nil, err
		}
	}

	if addon == nil {
		return nil, bcode.ErrAddonNotExist
	}

	addon.UISchema = renderAddonCustomUISchema(ctx, u.kubeClient, name, renderDefaultUISchema(addon.APISchema))

	a, err := AddonImpl2AddonRes(addon)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (u *defaultAddonHandler) StatusAddon(ctx context.Context, name string) (*apis.AddonStatusResponse, error) {
	status, err := pkgaddon.GetAddonStatus(ctx, u.kubeClient, name)
	if err != nil {
		return nil, bcode.ErrGetAddonApplication
	}

	if status.AddonPhase == string(apis.AddonPhaseDisabled) {
		return &apis.AddonStatusResponse{
			AddonBaseStatus: apis.AddonBaseStatus{
				Name:  name,
				Phase: apis.AddonPhase(status.AddonPhase),
			},
		}, nil
	}

	res := apis.AddonStatusResponse{
		AddonBaseStatus: apis.AddonBaseStatus{
			Name:  name,
			Phase: apis.AddonPhase(status.AddonPhase),
		},
		AppStatus: *status.AppStatus,
		Clusters:  status.Clusters,
	}

	if res.Phase != apis.AddonPhaseEnabled {
		return &res, nil
	}
	var sec v1.Secret
	err = u.kubeClient.Get(ctx, client.ObjectKey{
		Namespace: types.DefaultKubeVelaNS,
		Name:      pkgaddon.Convert2SecName(name),
	}, &sec)
	if err != nil && !errors2.IsNotFound(err) {
		return nil, bcode.ErrAddonSecretGet
	} else if errors2.IsNotFound(err) {
		return &res, nil
	}

	res.Args, err = pkgaddon.FetchArgsFromSecret(&sec)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func (u *defaultAddonHandler) ListAddons(ctx context.Context, registry, query string) ([]*apis.DetailAddonResponse, error) {
	var addons []*pkgaddon.UIData
	rs, err := u.addonRegistryDS.ListRegistries(ctx)
	if err != nil {
		return nil, err
	}

	var gatherErr velaerr.ErrorList

	for _, r := range rs {
		if registry != "" && r.Name != registry {
			continue
		}
		listAddons, err := u.addonRegistryCache.ListUIData(r)
		if err != nil {
			gatherErr = append(gatherErr, err)
			continue
		}
		addons = mergeAddons(addons, listAddons)
	}

	for i, a := range addons {
		if a.Invisible {
			addons = append(addons[:i], addons[i+1:]...)
		}
	}

	if query != "" {
		var filtered []*pkgaddon.UIData
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

	for _, addon := range addons {
		// render default ui schema
		addon.UISchema = renderDefaultUISchema(addon.APISchema)
	}

	var addonResources []*apis.DetailAddonResponse
	for _, a := range addons {
		addonRes, err := AddonImpl2AddonRes(a)
		if err != nil {
			log.Logger.Errorf("err while converting AddonImpl to DetailAddonResponse: %v", err)
			continue
		}
		addonResources = append(addonResources, addonRes)
	}
	if gatherErr.HasError() {
		return addonResources, gatherErr
	}
	return addonResources, nil
}

func (u *defaultAddonHandler) DeleteAddonRegistry(ctx context.Context, name string) error {
	return u.addonRegistryDS.DeleteRegistry(ctx, name)
}

func (u *defaultAddonHandler) CreateAddonRegistry(ctx context.Context, req apis.CreateAddonRegistryRequest) (*apis.AddonRegistry, error) {
	r := addonRegistryModelFromCreateAddonRegistryRequest(req)

	err := u.addonRegistryDS.AddRegistry(ctx, r)
	if err != nil {
		return nil, err
	}

	return &apis.AddonRegistry{
		Name:  r.Name,
		Git:   r.Git,
		OSS:   r.OSS,
		Gitee: r.Gitee,
	}, nil
}

func (u *defaultAddonHandler) GetAddonRegistry(ctx context.Context, name string) (*apis.AddonRegistry, error) {
	r, err := u.addonRegistryDS.GetRegistry(ctx, name)
	if err != nil {
		return nil, err
	}
	return &apis.AddonRegistry{
		Name: r.Name,
		Git:  r.Git,
		OSS:  r.OSS,
	}, nil
}

func (u defaultAddonHandler) UpdateAddonRegistry(ctx context.Context, name string, req apis.UpdateAddonRegistryRequest) (*apis.AddonRegistry, error) {
	r, err := u.addonRegistryDS.GetRegistry(ctx, name)
	if err != nil {
		return nil, bcode.ErrAddonRegistryNotExist
	}
	r.Git = req.Git
	r.OSS = req.Oss
	err = u.addonRegistryDS.UpdateRegistry(ctx, r)
	if err != nil {
		return nil, err
	}

	return &apis.AddonRegistry{
		Name: r.Name,
		Git:  r.Git,
		OSS:  r.OSS,
	}, nil
}

func (u *defaultAddonHandler) ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistry, error) {

	var list []*apis.AddonRegistry
	registries, err := u.addonRegistryDS.ListRegistries(ctx)
	if err != nil {
		// the storage configmap still not exist, don't return error add registry will create the configmap
		if errors2.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, registry := range registries {
		r := ConvertAddonRegistryModel2AddonRegistryMeta(registry)
		list = append(list, &r)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list, nil
}

func (u *defaultAddonHandler) EnableAddon(ctx context.Context, name string, args apis.EnableAddonRequest) error {
	var err error
	registries, err := u.addonRegistryDS.ListRegistries(ctx)
	if err != nil {
		return err
	}
	for _, r := range registries {
		err = pkgaddon.EnableAddon(ctx, name, u.kubeClient, u.discoveryClient, u.apply, u.config, r, args.Args, u.addonRegistryCache)
		if err == nil {
			return nil
		}

		// if reach this line error must is not nil
		if errors.Is(err, pkgaddon.ErrNotExist) {
			// one registry return addon not exist error, should not break other registry func
			continue
		}

		// wrap this error with special bcode
		if errors.Is(err, pkgaddon.ErrVersionMismatch) {
			return bcode.ErrAddonSystemVersionMismatch
		}
		// except `addon not found`, other errors should return directly
		return err
	}
	return bcode.ErrAddonNotExist
}

func (u *defaultAddonHandler) DisableAddon(ctx context.Context, name string, force bool) error {
	err := pkgaddon.DisableAddon(ctx, u.kubeClient, name, u.config, force)
	if err != nil {
		log.Logger.Errorf("delete application fail: %s", err.Error())
		return err
	}
	return nil
}

func (u *defaultAddonHandler) ListEnabledAddon(ctx context.Context) ([]*apis.AddonBaseStatus, error) {
	apps := &v1beta1.ApplicationList{}
	if err := u.kubeClient.List(ctx, apps, client.InNamespace(types.DefaultKubeVelaNS), client.HasLabels{oam.LabelAddonName}); err != nil {
		return nil, err
	}
	var response []*apis.AddonBaseStatus
	for _, application := range apps.Items {
		if addonName := application.Labels[oam.LabelAddonName]; addonName != "" {
			if application.Status.Phase != common2.ApplicationRunning {
				continue
			}
			response = append(response, &apis.AddonBaseStatus{
				Name:  addonName,
				Phase: convertAppStateToAddonPhase(application.Status.Phase),
			})
		}
	}
	return response, nil
}

func (u *defaultAddonHandler) UpdateAddon(ctx context.Context, name string, args apis.EnableAddonRequest) error {

	var app v1beta1.Application
	// check addon application whether exist
	err := u.kubeClient.Get(ctx, client.ObjectKey{
		Namespace: types.DefaultKubeVelaNS,
		Name:      pkgaddon.Convert2AppName(name),
	}, &app)
	if err != nil {
		return err
	}

	registries, err := u.addonRegistryDS.ListRegistries(ctx)
	if err != nil {
		return err
	}

	for _, r := range registries {
		err = pkgaddon.EnableAddon(ctx, name, u.kubeClient, u.discoveryClient, u.apply, u.config, r, args.Args, u.addonRegistryCache)
		if err == nil {
			return nil
		}

		if errors.Is(err, pkgaddon.ErrNotExist) {
			continue
		}

		// wrap this error with special bcode
		if errors.Is(err, pkgaddon.ErrVersionMismatch) {
			return bcode.ErrAddonSystemVersionMismatch
		}
		// except `addon not found`, other errors should return directly
		return err
	}
	return bcode.ErrAddonNotExist
}

func addonRegistryModelFromCreateAddonRegistryRequest(req apis.CreateAddonRegistryRequest) pkgaddon.Registry {
	return pkgaddon.Registry{
		Name:  req.Name,
		Git:   req.Git,
		OSS:   req.Oss,
		Gitee: req.Gitee,
	}
}

func mergeAddons(a1, a2 []*pkgaddon.UIData) []*pkgaddon.UIData {
	for _, item := range a2 {
		if hasAddon(a1, item.Name) {
			continue
		}
		a1 = append(a1, item)
	}
	return a1
}

func hasAddon(addons []*pkgaddon.UIData, name string) bool {
	if name == "" {
		return true
	}
	for _, addon := range addons {
		if addon.Name == name {
			return true
		}
	}
	return false
}

func convertAppStateToAddonPhase(state common2.ApplicationPhase) apis.AddonPhase {
	switch state {
	case common2.ApplicationRunning:
		return apis.AddonPhaseEnabled
	default:
		return apis.AddonPhaseEnabling
	}
}

func renderAddonCustomUISchema(ctx context.Context, cli client.Client, addonName string, defaultSchema []*utils.UIParameter) []*utils.UIParameter {
	var cm v1.ConfigMap
	if err := cli.Get(ctx, k8stypes.NamespacedName{
		Namespace: types.DefaultKubeVelaNS,
		Name:      fmt.Sprintf("addon-uischema-%s", addonName),
	}, &cm); err != nil {
		if !errors2.IsNotFound(err) {
			log.Logger.Errorf("find uischema configmap from cluster failure %s", err.Error())
		}
		return defaultSchema
	}
	data, ok := cm.Data[types.UISchema]
	if !ok {
		return defaultSchema
	}
	schema := []*utils.UIParameter{}
	if err := json.Unmarshal([]byte(data), &schema); err != nil {
		log.Logger.Errorf("unmarshal ui schema failure %s", err.Error())
		return defaultSchema
	}
	return patchSchema(defaultSchema, schema)
}

// ConvertAddonRegistryModel2AddonRegistryMeta will convert from model to AddonRegistry
func ConvertAddonRegistryModel2AddonRegistryMeta(r pkgaddon.Registry) apis.AddonRegistry {
	return apis.AddonRegistry{
		Name: r.Name,
		Git:  r.Git,
		OSS:  r.OSS,
	}
}
