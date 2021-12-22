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

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// TargetUsecase Target manage api
type TargetUsecase interface {
	GetTarget(ctx context.Context, TargetName string) (*model.Target, error)
	DetailTarget(ctx context.Context, Target *model.Target) (*apisv1.DetailTargetResponse, error)
	DeleteTarget(ctx context.Context, TargetName string) error
	CreateTarget(ctx context.Context, req apisv1.CreateTargetRequest) (*apisv1.DetailTargetResponse, error)
	UpdateTarget(ctx context.Context, Target *model.Target, req apisv1.UpdateTargetRequest) (*apisv1.DetailTargetResponse, error)
	ListTargets(ctx context.Context, page, pageSize int) (*apisv1.ListTargetResponse, error)
}

type targetUsecaseImpl struct {
	ds             datastore.DataStore
	projectUsecase ProjectUsecase
}

// NewTargetUsecase new Target usecase
func NewTargetUsecase(ds datastore.DataStore, projectUsecase ProjectUsecase) TargetUsecase {
	return &targetUsecaseImpl{
		ds:             ds,
		projectUsecase: projectUsecase,
	}
}

func (dt *targetUsecaseImpl) ListTargets(ctx context.Context, page, pageSize int) (*apisv1.ListTargetResponse, error) {
	Target := model.Target{}
	Targets, err := dt.ds.List(ctx, &Target, &datastore.ListOptions{Page: page, PageSize: pageSize, SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
	if err != nil {
		return nil, err
	}
	resp := &apisv1.ListTargetResponse{
		Targets: []apisv1.TargetBase{},
	}
	for _, raw := range Targets {
		target, ok := raw.(*model.Target)
		if ok {
			resp.Targets = append(resp.Targets, *(dt.convertFromTargetModel(ctx, target)))
		}
	}
	count, err := dt.ds.Count(ctx, &Target, nil)
	if err != nil {
		return nil, err
	}
	resp.Total = count

	return resp, nil
}

// DeleteTarget delete application Target
func (dt *targetUsecaseImpl) DeleteTarget(ctx context.Context, targetName string) error {
	Target := &model.Target{
		Name: targetName,
	}
	if err := dt.ds.Delete(ctx, Target); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrTargetNotExist
		}
		return err
	}
	return nil
}

func (dt *targetUsecaseImpl) CreateTarget(ctx context.Context, req apisv1.CreateTargetRequest) (*apisv1.DetailTargetResponse, error) {
	Target := convertCreateReqToTargetModel(req)

	// check Target name.
	exit, err := dt.ds.IsExist(ctx, &Target)
	if err != nil {
		log.Logger.Errorf("check application name is exist failure %s", err.Error())
		return nil, bcode.ErrTargetExist
	}
	if exit {
		return nil, bcode.ErrTargetExist
	}

	if err := dt.ds.Add(ctx, &Target); err != nil {
		return nil, err
	}
	return dt.DetailTarget(ctx, &Target)
}

func (dt *targetUsecaseImpl) UpdateTarget(ctx context.Context, target *model.Target, req apisv1.UpdateTargetRequest) (*apisv1.DetailTargetResponse, error) {
	TargetModel := convertUpdateReqToTargetModel(target, req)
	if err := dt.ds.Put(ctx, TargetModel); err != nil {
		return nil, err
	}
	return dt.DetailTarget(ctx, TargetModel)
}

// DetailTarget detail Target
func (dt *targetUsecaseImpl) DetailTarget(ctx context.Context, target *model.Target) (*apisv1.DetailTargetResponse, error) {
	return &apisv1.DetailTargetResponse{
		TargetBase: *dt.convertFromTargetModel(ctx, target),
	}, nil
}

// GetTarget get Target model
func (dt *targetUsecaseImpl) GetTarget(ctx context.Context, targetName string) (*model.Target, error) {
	Target := &model.Target{
		Name: targetName,
	}
	if err := dt.ds.Get(ctx, Target); err != nil {
		return nil, err
	}
	return Target, nil
}

func convertUpdateReqToTargetModel(target *model.Target, req apisv1.UpdateTargetRequest) *model.Target {
	target.Alias = req.Alias
	target.Description = req.Description
	target.Cluster = (*model.ClusterTarget)(req.Cluster)
	target.Variable = req.Variable
	return target
}

func convertCreateReqToTargetModel(req apisv1.CreateTargetRequest) model.Target {
	Target := model.Target{
		Name:        req.Name,
		Alias:       req.Alias,
		Description: req.Description,
		Cluster:     (*model.ClusterTarget)(req.Cluster),
		Variable:    req.Variable,
	}
	return Target
}

func (dt *targetUsecaseImpl) convertFromTargetModel(ctx context.Context, target *model.Target) *apisv1.TargetBase {
	var appNum int64 = 0
	// TODO: query app num in target
	targetBase := &apisv1.TargetBase{
		Name:        target.Name,
		Alias:       target.Alias,
		Description: target.Description,
		Cluster:     (*apisv1.ClusterTarget)(target.Cluster),
		Variable:    target.Variable,
		CreateTime:  target.CreateTime,
		UpdateTime:  target.UpdateTime,
		AppNum:      appNum,
	}

	if targetBase.Cluster != nil && targetBase.Cluster.ClusterName != "" {
		cluster, err := _getClusterFromDataStore(ctx, dt.ds, target.Cluster.ClusterName)
		if err != nil {
			log.Logger.Errorf("query cluster info failure %s", err.Error())
		}
		if cluster != nil {
			targetBase.ClusterAlias = cluster.Alias
		}
	}

	return targetBase
}
