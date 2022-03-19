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

	"github.com/oam-dev/kubevela/version"

	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"

	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/oam-dev/kubevela/pkg/apiserver/model"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
)

// SystemInfoUsecase is usecase for systemInfoCollection
type SystemInfoUsecase interface {
	GetSystemInfo(ctx context.Context) (*v1.SystemInfoResponse, error)
	DeleteSystemInfo(ctx context.Context) error
	UpdateSystemInfo(ctx context.Context, sysInfo v1.SystemInfoRequest) (*v1.SystemInfoResponse, error)
}

type systemInfoUsecaseImpl struct {
	ds datastore.DataStore
}

// NewSystemInfoUsecase return a systemInfoCollectionUsecase
func NewSystemInfoUsecase(ds datastore.DataStore) SystemInfoUsecase {
	return &systemInfoUsecaseImpl{ds: ds}
}

func (u systemInfoUsecaseImpl) GetSystemInfo(ctx context.Context) (*v1.SystemInfoResponse, error) {
	// first get request will init systemInfoCollection{installId: {random}, enableCollection: true}
	info := &model.SystemInfo{}
	entities, err := u.ds.List(ctx, info, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(entities) != 0 {
		info := entities[0].(*model.SystemInfo)
		return &v1.SystemInfoResponse{SystemInfo: *info, SystemVersion: v1.SystemVersion{VelaVersion: version.VelaVersion, GitVersion: version.GitRevision}}, nil
	}
	installID := rand.String(16)
	info.InstallID = installID
	info.EnableCollection = true
	info.LoginType = model.LoginTypeLocal
	err = u.ds.Add(ctx, info)
	if err != nil {
		return nil, err
	}
	return &v1.SystemInfoResponse{SystemInfo: *info, SystemVersion: v1.SystemVersion{VelaVersion: version.VelaVersion, GitVersion: version.GitRevision}}, nil
}

func (u systemInfoUsecaseImpl) UpdateSystemInfo(ctx context.Context, sysInfo v1.SystemInfoRequest) (*v1.SystemInfoResponse, error) {
	info, err := u.GetSystemInfo(ctx)
	if err != nil {
		return nil, err
	}
	modifiedInfo := model.SystemInfo{
		InstallID:        info.InstallID,
		EnableCollection: sysInfo.EnableCollection,
		LoginType:        sysInfo.LoginType,
		BaseModel: model.BaseModel{
			CreateTime: info.CreateTime,
		}}
	err = u.ds.Put(ctx, &modifiedInfo)
	if err != nil {
		return nil, err
	}
	return &v1.SystemInfoResponse{SystemInfo: modifiedInfo, SystemVersion: v1.SystemVersion{VelaVersion: version.VelaVersion, GitVersion: version.GitRevision}}, nil
}

func (u systemInfoUsecaseImpl) DeleteSystemInfo(ctx context.Context) error {
	info, err := u.GetSystemInfo(ctx)
	if err != nil {
		return err
	}
	err = u.ds.Delete(ctx, info)
	if err != nil {
		return err
	}
	return nil
}
