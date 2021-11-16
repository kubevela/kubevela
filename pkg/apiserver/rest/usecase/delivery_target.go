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

// DeliveryTargetUsecase deliveryTarget manage api
type DeliveryTargetUsecase interface {
	GetDeliveryTarget(ctx context.Context, deliveryTargetName string) (*model.DeliveryTarget, error)
	DetailDeliveryTarget(ctx context.Context, deliveryTarget *model.DeliveryTarget) (*apisv1.DetailDeliveryTargetResponse, error)
	DeleteDeliveryTarget(ctx context.Context, deliveryTargetName string) error
	CreateDeliveryTarget(ctx context.Context, req apisv1.CreateDeliveryTargetRequest) (*apisv1.DetailDeliveryTargetResponse, error)
	UpdateDeliveryTarget(ctx context.Context, deliveryTarget *model.DeliveryTarget, req apisv1.UpdateDeliveryTargetRequest) (*apisv1.DetailDeliveryTargetResponse, error)
	ListDeliveryTargets(ctx context.Context, page, pageSize int) (*apisv1.ListDeliveryTargetResponse, error)
}

// NewDeliveryTargetUsecase new DeliveryTarget usecase
func NewDeliveryTargetUsecase(ds datastore.DataStore) DeliveryTargetUsecase {
	return &deliveryTargetUsecaseImpl{ds: ds}
}

type deliveryTargetUsecaseImpl struct {
	ds datastore.DataStore
}

func (dt *deliveryTargetUsecaseImpl) ListDeliveryTargets(ctx context.Context, page, pageSize int) (*apisv1.ListDeliveryTargetResponse, error) {
	deliveryTarget := model.DeliveryTarget{}
	deliveryTargets, err := dt.ds.List(ctx, &deliveryTarget, &datastore.ListOptions{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, err
	}

	resp := &apisv1.ListDeliveryTargetResponse{
		DeliveryTargets: []apisv1.DeliveryTargetBase{},
	}
	for _, raw := range deliveryTargets {
		dt, ok := raw.(*model.DeliveryTarget)
		if ok {
			resp.DeliveryTargets = append(resp.DeliveryTargets, *convertFromDeliveryTargetModel(dt))
		}
	}
	count, err := dt.ds.Count(ctx, &deliveryTarget)
	if err != nil {
		return nil, err
	}
	resp.Total = count

	return resp, nil
}

// DeleteDeliveryTarget delete application DeliveryTarget
func (dt *deliveryTargetUsecaseImpl) DeleteDeliveryTarget(ctx context.Context, deliveryTargetName string) error {
	deliveryTarget := &model.DeliveryTarget{
		Name: deliveryTargetName,
	}
	if err := dt.ds.Delete(ctx, deliveryTarget); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrDeliveryTargetNotExist
		}
		return err
	}
	return nil
}

func (dt *deliveryTargetUsecaseImpl) CreateDeliveryTarget(ctx context.Context, req apisv1.CreateDeliveryTargetRequest) (*apisv1.DetailDeliveryTargetResponse, error) {
	deliveryTarget := convertCreateReqToDeliveryTargetModel(req)
	// check deliveryTarget name.
	exit, err := dt.ds.IsExist(ctx, &deliveryTarget)
	if err != nil {
		log.Logger.Errorf("check application name is exist failure %s", err.Error())
		return nil, bcode.ErrDeliveryTargetExist
	}
	if exit {
		return nil, bcode.ErrDeliveryTargetExist
	}
	if err := dt.ds.Add(ctx, &deliveryTarget); err != nil {
		return nil, err
	}
	return dt.DetailDeliveryTarget(ctx, &deliveryTarget)
}

func (dt *deliveryTargetUsecaseImpl) UpdateDeliveryTarget(ctx context.Context, deliveryTarget *model.DeliveryTarget, req apisv1.UpdateDeliveryTargetRequest) (*apisv1.DetailDeliveryTargetResponse, error) {
	deliveryTargetModel := convertUpdateReqToDeliveryTargetModel(deliveryTarget, req)
	if err := dt.ds.Put(ctx, deliveryTargetModel); err != nil {
		return nil, err
	}
	return dt.DetailDeliveryTarget(ctx, deliveryTargetModel)
}

// DetailDeliveryTarget detail DeliveryTarget
func (dt *deliveryTargetUsecaseImpl) DetailDeliveryTarget(ctx context.Context, deliveryTarget *model.DeliveryTarget) (*apisv1.DetailDeliveryTargetResponse, error) {
	return &apisv1.DetailDeliveryTargetResponse{
		DeliveryTargetBase: *convertFromDeliveryTargetModel(deliveryTarget),
	}, nil
}

// GetDeliveryTarget get DeliveryTarget model
func (dt *deliveryTargetUsecaseImpl) GetDeliveryTarget(ctx context.Context, deliveryTargetName string) (*model.DeliveryTarget, error) {
	deliveryTarget := &model.DeliveryTarget{
		Name: deliveryTargetName,
	}
	if err := dt.ds.Get(ctx, deliveryTarget); err != nil {
		return nil, err
	}
	return deliveryTarget, nil
}

func convertUpdateReqToDeliveryTargetModel(deliveryTarget *model.DeliveryTarget, req apisv1.UpdateDeliveryTargetRequest) *model.DeliveryTarget {
	deliveryTarget.Alias = req.Alias
	deliveryTarget.Description = req.Description
	deliveryTarget.Cluster = (*model.ClusterTarget)(req.Cluster)
	deliveryTarget.Variable = req.Variable
	return deliveryTarget
}

func convertCreateReqToDeliveryTargetModel(req apisv1.CreateDeliveryTargetRequest) model.DeliveryTarget {
	deliveryTarget := model.DeliveryTarget{
		Name:        req.Name,
		Namespace:   req.Namespace,
		Alias:       req.Alias,
		Description: req.Description,
		Cluster:     (*model.ClusterTarget)(req.Cluster),
		Variable:    req.Variable,
	}
	return deliveryTarget
}

func convertFromDeliveryTargetModel(deliveryTarget *model.DeliveryTarget) *apisv1.DeliveryTargetBase {
	return &apisv1.DeliveryTargetBase{
		Name:        deliveryTarget.Name,
		Namespace:   deliveryTarget.Namespace,
		Alias:       deliveryTarget.Alias,
		Description: deliveryTarget.Description,
		Cluster:     (*apisv1.ClusterTarget)(deliveryTarget.Cluster),
		Variable:    deliveryTarget.Variable,
		CreateTime:  deliveryTarget.CreateTime,
		UpdateTime:  deliveryTarget.UpdateTime,
	}
}
