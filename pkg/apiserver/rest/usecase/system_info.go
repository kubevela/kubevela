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
	"errors"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/version"
)

// SystemInfoUsecase is usecase for systemInfoCollection
type SystemInfoUsecase interface {
	Get(ctx context.Context) (*model.SystemInfo, error)
	GetSystemInfo(ctx context.Context) (*v1.SystemInfoResponse, error)
	UpdateSystemInfo(ctx context.Context, sysInfo v1.SystemInfoRequest) (*v1.SystemInfoResponse, error)
	Init(ctx context.Context) error
}

type systemInfoUsecaseImpl struct {
	ds         datastore.DataStore
	kubeClient client.Client
}

// NewSystemInfoUsecase return a systemInfoCollectionUsecase
func NewSystemInfoUsecase(ds datastore.DataStore) SystemInfoUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("failed to get kube client: %s", err.Error())
	}
	return &systemInfoUsecaseImpl{ds: ds, kubeClient: kubecli}
}

func (u systemInfoUsecaseImpl) Get(ctx context.Context) (*model.SystemInfo, error) {
	// first get request will init systemInfoCollection{installId: {random}, enableCollection: true}
	info := &model.SystemInfo{}
	entities, err := u.ds.List(ctx, info, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(entities) != 0 {
		info := entities[0].(*model.SystemInfo)
		if info.LoginType == "" {
			info.LoginType = model.LoginTypeLocal
		}
		return info, nil
	}
	installID := rand.String(16)
	info.InstallID = installID
	info.EnableCollection = true
	info.LoginType = model.LoginTypeLocal
	err = u.ds.Add(ctx, info)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (u systemInfoUsecaseImpl) GetSystemInfo(ctx context.Context) (*v1.SystemInfoResponse, error) {
	// first get request will init systemInfoCollection{installId: {random}, enableCollection: true}
	info, err := u.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.SystemInfoResponse{
		SystemInfo: convertInfoToBase(info),
		SystemVersion: v1.SystemVersion{
			VelaVersion: version.VelaVersion,
			GitVersion:  version.GitRevision,
		},
	}, nil
}

func (u systemInfoUsecaseImpl) UpdateSystemInfo(ctx context.Context, sysInfo v1.SystemInfoRequest) (*v1.SystemInfoResponse, error) {
	info, err := u.Get(ctx)
	if err != nil {
		return nil, err
	}
	modifiedInfo := model.SystemInfo{
		InstallID:        info.InstallID,
		EnableCollection: sysInfo.EnableCollection,
		LoginType:        sysInfo.LoginType,
		BaseModel: model.BaseModel{
			CreateTime: info.CreateTime,
		},
	}

	if sysInfo.LoginType == model.LoginTypeDex {
		if err := generateDexConfig(ctx, u.kubeClient, sysInfo.VelaAddress, &modifiedInfo); err != nil {
			return nil, err
		}
	}
	err = u.ds.Put(ctx, &modifiedInfo)
	if err != nil {
		return nil, err
	}
	return &v1.SystemInfoResponse{
		SystemInfo: v1.SystemInfo{
			InstallID:        modifiedInfo.InstallID,
			EnableCollection: modifiedInfo.EnableCollection,
			LoginType:        modifiedInfo.LoginType,
		},
		SystemVersion: v1.SystemVersion{VelaVersion: version.VelaVersion, GitVersion: version.GitRevision},
	}, nil
}

func (u systemInfoUsecaseImpl) Init(ctx context.Context) error {
	_, err := initDexConfig(ctx, u.kubeClient, "http://velaux.com", &model.SystemInfo{})
	return err
}

func convertInfoToBase(info *model.SystemInfo) v1.SystemInfo {
	return v1.SystemInfo{
		InstallID:        info.InstallID,
		EnableCollection: info.EnableCollection,
		LoginType:        info.LoginType,
	}
}

func generateDexConfig(ctx context.Context, kubeClient client.Client, velaAddress string, info *model.SystemInfo) error {
	secret, err := initDexConfig(ctx, kubeClient, velaAddress, info)
	if err != nil {
		return err
	}
	connectors, err := utils.GetDexConnectors(ctx, kubeClient)
	if err != nil {
		return err
	}
	config, err := model.NewJSONStructByStruct(info.DexConfig)
	if err != nil {
		return err
	}
	(*config)["connectors"] = connectors
	c, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(secret.Data[secretDexConfigKey], c) {
		secret.Data[secretDexConfigKey] = c
		if err := kubeClient.Update(ctx, secret); err != nil {
			return err
		}
		if err := restartDex(ctx, kubeClient); err != nil && !errors.Is(err, bcode.ErrDexNotFound) {
			return err
		}
	}
	return nil
}

func initDexConfig(ctx context.Context, kubeClient client.Client, velaAddress string, info *model.SystemInfo) (*corev1.Secret, error) {
	dexConfig := model.DexConfig{
		Issuer: fmt.Sprintf("%s/dex", velaAddress),
		Web: model.DexWeb{
			HTTP: "0.0.0.0:5556",
		},
		Storage: model.DexStorage{
			Type: "memory",
		},
		StaticClients: []model.DexStaticClient{
			{
				ID:           "velaux",
				Name:         "Vela UX",
				Secret:       "velaux-secret",
				RedirectURIs: []string{fmt.Sprintf("%s/callback", velaAddress)},
			},
		},
	}
	info.DexConfig = dexConfig

	secret := &corev1.Secret{}
	if err := kubeClient.Get(ctx, types.NamespacedName{
		Name:      dexConfigName,
		Namespace: velatypes.DefaultKubeVelaNS,
	}, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		config, err := yaml.Marshal(info.DexConfig)
		if err != nil {
			return nil, err
		}
		if err := kubeClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dexConfigName,
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				secretDexConfigKey: config,
			},
		}); err != nil {
			return nil, err
		}
	}
	return secret, nil
}
