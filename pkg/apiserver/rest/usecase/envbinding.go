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
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

// EnvBindingUsecase envbinding usecase
type EnvBindingUsecase interface {
	GetEnvBindings(ctx context.Context, app *model.Application) ([]*apisv1.EnvBindingBase, error)
	CreateEnvBinding(ctx context.Context, app *model.Application, env apisv1.CreateApplicationEnvRequest) (*apisv1.EnvBinding, error)
	BatchCreateEnvBinding(ctx context.Context, app *model.Application, env apisv1.EnvBindingList) error
	UpdateEnvBinding(ctx context.Context, app *model.Application, envName string, diff apisv1.PutApplicationEnvRequest) (*apisv1.EnvBinding, error)
	DeleteEnvBinding(ctx context.Context, app *model.Application, envName string) error
	DetailEnvBinding(ctx context.Context, envBinding *model.EnvBinding) (*apisv1.DetailEnvBindingResponse, error)
}

type envBindingUsecaseImpl struct {
	ds datastore.DataStore
}

// NewEnvBindingUsecase new envBinding usecase
func NewEnvBindingUsecase(ds datastore.DataStore) EnvBindingUsecase {
	return &envBindingUsecaseImpl{
		ds: ds,
	}
}

func (c *envBindingUsecaseImpl) CreateEnvBinding(ctx context.Context, app *model.Application, envReq apisv1.CreateApplicationEnvRequest) (*apisv1.EnvBinding, error) {
	envBinding, err := c.getBindingByEnv(ctx, app, envReq.Name)
	if err != nil {
		if !errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, err
		}
	}
	envBindingModel := convertCreateReqToEnvBindingModel(app, envReq)
	if envBinding != nil {
		if err := c.ds.Put(ctx, &envBindingModel); err != nil {
			return nil, err
		}
	} else {
		if err := c.ds.Add(ctx, &envBindingModel); err != nil {
			return nil, err
		}
	}
	return &envReq.EnvBinding, nil
}

func (c *envBindingUsecaseImpl) BatchCreateEnvBinding(ctx context.Context, app *model.Application, envbindings apisv1.EnvBindingList) error {
	var envBindingModels []datastore.Entity
	for _, envBinding := range envbindings {
		envBindingModels = append(envBindingModels, createModelEnvBind(*envBinding))

	}
	return c.ds.BatchAdd(ctx, envBindingModels)
}

func (c *envBindingUsecaseImpl) GetEnvBindings(ctx context.Context, app *model.Application) ([]*apisv1.EnvBindingBase, error) {
	var envBinding = model.EnvBinding{
		AppPrimaryKey: app.PrimaryKey(),
	}
	envBindings, err := c.ds.List(ctx, &envBinding, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apisv1.EnvBindingBase
	for _, ebd := range envBindings {
		eb := ebd.(*model.EnvBinding)
		list = append(list, converEnvbindingModelToBase(eb))
	}
	return list, nil
}

func (c *envBindingUsecaseImpl) getBindingByEnv(ctx context.Context, app *model.Application, envName string) (*model.EnvBinding, error) {
	var envBinding = model.EnvBinding{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          envName,
	}
	err := c.ds.Get(ctx, &envBinding)
	if err != nil {
		return nil, err
	}
	return &envBinding, nil
}

func (c *envBindingUsecaseImpl) UpdateEnvBinding(ctx context.Context, app *model.Application,
	envName string, envUpdate apisv1.PutApplicationEnvRequest) (*apisv1.EnvBinding, error) {
	envBinding, err := c.getBindingByEnv(ctx, app, envName)
	if err != nil {
		return nil, err
	}
	var envBind model.EnvBinding
	// update env-binding base
	for i, env := range app.EnvBinding {
		if env.Name == envName {
			if envUpdate.Description != nil {
				app.EnvBinding[i].Description = *envUpdate.Description
			}
			if envUpdate.Alias != nil {
				app.EnvBinding[i].Alias = *envUpdate.Alias
			}
			if envUpdate.TargetNames != nil {
				app.EnvBinding[i].TargetNames = *envUpdate.TargetNames
			}
			if envUpdate.ComponentSelector == nil {
				app.EnvBinding[i].ComponentSelector = nil
			} else {
				app.EnvBinding[i].ComponentSelector = &model.ComponentSelector{
					Components: envUpdate.ComponentSelector.Components,
				}
			}
			envBind = *app.EnvBinding[i]
		}
	}
	// update envbinding
	if err := c.ds.Put(ctx, &envBind); err != nil {
		return nil, err
	}
	re := &apisv1.EnvBinding{
		Name:        envBinding.Name,
		Alias:       envBinding.Alias,
		Description: envBinding.Description,
		TargetNames: envBinding.TargetNames,
	}
	if envBinding.ComponentSelector != nil {
		re.ComponentSelector = (*apisv1.ComponentSelector)(envBinding.ComponentSelector)
	}
	return re, nil
}

func (c *envBindingUsecaseImpl) DeleteEnvBinding(ctx context.Context, app *model.Application, envName string) error {
	envBindings, err := c.GetEnvBindings(ctx, app)
	if err != nil {
		return err
	}
	for _, envBinding := range envBindings {
		if err := c.ds.Delete(ctx, &model.EnvBinding{AppPrimaryKey: app.PrimaryKey(), Name: envBinding.Name}); err != nil {
			return err
		}
	}
	return nil
}

func (c *envBindingUsecaseImpl) DetailEnvBinding(ctx context.Context, envBinding *model.EnvBinding) (*apisv1.DetailEnvBindingResponse, error) {
	return &apisv1.DetailEnvBindingResponse{
		EnvBindingBase: *converEnvbindingModelToBase(envBinding),
	}, nil
}

func convertCreateReqToEnvBindingModel(app *model.Application, req apisv1.CreateApplicationEnvRequest) model.EnvBinding {
	envBinding := model.EnvBinding{
		AppPrimaryKey:     app.Name,
		Name:              req.Name,
		Alias:             req.Alias,
		Description:       req.Description,
		TargetNames:       req.TargetNames,
		ComponentSelector: (*model.ComponentSelector)(req.ComponentSelector),
	}
	return envBinding
}

func converEnvbindingModelToBase(envBinding *model.EnvBinding) *apisv1.EnvBindingBase {
	ebb := &apisv1.EnvBindingBase{
		Name:              envBinding.Name,
		Alias:             envBinding.Alias,
		Description:       envBinding.Description,
		TargetNames:       envBinding.TargetNames,
		ComponentSelector: (*apisv1.ComponentSelector)(envBinding.ComponentSelector),
		CreateTime:        envBinding.CreateTime,
		UpdateTime:        envBinding.UpdateTime,
	}
	return ebb
}
