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

package service

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/version"
)

// SystemInfoService is service for systemInfoCollection
type SystemInfoService interface {
	Get(ctx context.Context) (*model.SystemInfo, error)
	GetSystemInfo(ctx context.Context) (*v1.SystemInfoResponse, error)
	UpdateSystemInfo(ctx context.Context, sysInfo v1.SystemInfoRequest) (*v1.SystemInfoResponse, error)
	Init(ctx context.Context) error
}

type systemInfoServiceImpl struct {
	Store      datastore.DataStore `inject:"datastore"`
	KubeClient client.Client       `inject:"kubeClient"`
}

// NewSystemInfoService return a systemInfoCollectionService
func NewSystemInfoService() SystemInfoService {
	return &systemInfoServiceImpl{}
}

func (u systemInfoServiceImpl) Get(ctx context.Context) (*model.SystemInfo, error) {
	// first get request will init systemInfoCollection{installId: {random}, enableCollection: true}
	info := &model.SystemInfo{}
	entities, err := u.Store.List(ctx, info, &datastore.ListOptions{})
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
	info.BaseModel = model.BaseModel{CreateTime: time.Now()}
	err = u.Store.Add(ctx, info)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (u systemInfoServiceImpl) GetSystemInfo(ctx context.Context) (*v1.SystemInfoResponse, error) {
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
		StatisticInfo: v1.StatisticInfo{
			AppCount:                   info.StatisticInfo.AppCount,
			ClusterCount:               info.StatisticInfo.ClusterCount,
			EnableAddonList:            info.StatisticInfo.EnabledAddon,
			ComponentDefinitionTopList: info.StatisticInfo.TopKCompDef,
			TraitDefinitionTopList:     info.StatisticInfo.TopKTraitDef,
			WorkflowDefinitionTopList:  info.StatisticInfo.TopKWorkflowStepDef,
			PolicyDefinitionTopList:    info.StatisticInfo.TopKPolicyDef,
			UpdateTime:                 info.StatisticInfo.UpdateTime,
		},
	}, nil
}

func (u systemInfoServiceImpl) UpdateSystemInfo(ctx context.Context, sysInfo v1.SystemInfoRequest) (*v1.SystemInfoResponse, error) {
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
			UpdateTime: time.Now(),
		},
		StatisticInfo:          info.StatisticInfo,
		DexUserDefaultProjects: sysInfo.DexUserDefaultProjects,
	}

	if sysInfo.LoginType == model.LoginTypeDex {
		admin := &model.User{Name: model.DefaultAdminUserName}
		if err := u.Store.Get(ctx, admin); err != nil {
			return nil, err
		}
		if admin.Email == "" {
			return nil, bcode.ErrEmptyAdminEmail
		}
		connectors, err := utils.GetDexConnectors(ctx, u.KubeClient)
		if err != nil {
			return nil, err
		}
		if len(connectors) < 1 {
			return nil, bcode.ErrNoDexConnector
		}
		if err := generateDexConfig(ctx, u.KubeClient, &model.UpdateDexConfig{
			VelaAddress: sysInfo.VelaAddress,
			Connectors:  connectors,
		}); err != nil {
			return nil, err
		}
	}
	err = u.Store.Put(ctx, &modifiedInfo)
	if err != nil {
		return nil, err
	}
	return &v1.SystemInfoResponse{
		SystemInfo: v1.SystemInfo{
			PlatformID:       modifiedInfo.InstallID,
			EnableCollection: modifiedInfo.EnableCollection,
			LoginType:        modifiedInfo.LoginType,
			// always use the initial createTime as system's installTime
			InstallTime: info.CreateTime,
		},
		SystemVersion: v1.SystemVersion{VelaVersion: version.VelaVersion, GitVersion: version.GitRevision},
	}, nil
}

func (u systemInfoServiceImpl) Init(ctx context.Context) error {
	info, err := u.Get(ctx)
	if err != nil {
		return err
	}
	signedKey = info.InstallID
	_, err = initDexConfig(ctx, u.KubeClient, "http://velaux.com")
	return err
}

func convertInfoToBase(info *model.SystemInfo) v1.SystemInfo {
	return v1.SystemInfo{
		PlatformID:             info.InstallID,
		EnableCollection:       info.EnableCollection,
		LoginType:              info.LoginType,
		InstallTime:            info.CreateTime,
		DexUserDefaultProjects: info.DexUserDefaultProjects,
	}
}
