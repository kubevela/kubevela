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
	"reflect"
	"sort"

	apierror "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	util "github.com/oam-dev/kubevela/pkg/utils"
)

// EnvUsecase defines the API of Env.
type EnvUsecase interface {
	GetEnv(ctx context.Context, envName string) (*model.Env, error)
	ListEnvs(ctx context.Context, page, pageSize int, listOption apisv1.ListEnvOptions) ([]*apisv1.Env, error)
	DeleteEnv(ctx context.Context, envName string) error
	CreateEnv(ctx context.Context, req apisv1.CreateEnvRequest) (*apisv1.Env, error)
	UpdateEnv(ctx context.Context, envName string, req apisv1.UpdateEnvRequest) (*apisv1.Env, error)
}

type envUsecaseImpl struct {
	ds         datastore.DataStore
	kubeClient client.Client
}

// NewEnvUsecase new env usecase
func NewEnvUsecase(ds datastore.DataStore) EnvUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &envUsecaseImpl{kubeClient: kubecli, ds: ds}
}

// GetEnv get env
func (p *envUsecaseImpl) GetEnv(ctx context.Context, envName string) (*model.Env, error) {
	return getEnv(ctx, p.ds, envName)
}

// DeleteEnv delete an env by name
// the function assume applications contain in env already empty.
// it won't delete the namespace created by the Env, but it will update the label
func (p *envUsecaseImpl) DeleteEnv(ctx context.Context, envName string) error {
	env := &model.Env{}
	env.Name = envName

	if err := p.ds.Get(ctx, env); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}
	// reset the labels
	err := util.UpdateNamespace(ctx, p.kubeClient, env.Namespace, util.MergeOverrideLabels(map[string]string{
		oam.LabelNamespaceOfEnvName:         "",
		oam.LabelControlPlaneNamespaceUsage: "",
	}))
	if err != nil && apierror.IsNotFound(err) {
		return err
	}

	if err = p.ds.Delete(ctx, env); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}
	return nil
}

// ListEnvs list envs
func (p *envUsecaseImpl) ListEnvs(ctx context.Context, page, pageSize int, listOption apisv1.ListEnvOptions) ([]*apisv1.Env, error) {
	entities, err := listEnvs(ctx, p.ds, &datastore.ListOptions{Page: page, PageSize: pageSize, SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
	if err != nil {
		return nil, err
	}

	Targets, err := listTarget(ctx, p.ds, nil)
	if err != nil {
		return nil, err
	}

	var envs []*apisv1.Env
	for _, ee := range entities {
		if listOption.Project != "" && ee.Project != listOption.Project {
			continue
		}
		envs = append(envs, convertEnvModel2Base(ee, Targets))
	}

	projects, err := listProjects(ctx, p.ds)
	if err != nil {
		return nil, err
	}
	for _, e := range envs {
		for _, pj := range projects {
			if e.Project.Name == pj.Name {
				e.Project.Alias = pj.Alias
				break
			}
		}
	}
	return envs, nil
}

func checkEqual(old, new []string) bool {
	if old == nil && new == nil {
		return true
	}
	if old == nil || new == nil {
		return false
	}
	sort.Strings(old)
	sort.Strings(new)
	return reflect.DeepEqual(old, new)
}

func (p *envUsecaseImpl) updateAppWithNewEnv(ctx context.Context, envName string, env *model.Env) error {

	// List all apps inside the env
	apps, err := listApp(ctx, p.ds, apisv1.ListApplicationOptions{Env: envName})
	if err != nil {
		return err
	}
	for _, app := range apps {
		err = UpdateEnvWorkflow(ctx, p.kubeClient, p.ds, app, env)
		if err != nil {
			return err
		}
	}
	return nil

}

// UpdateEnv update an env for request
func (p *envUsecaseImpl) UpdateEnv(ctx context.Context, name string, req apisv1.UpdateEnvRequest) (*apisv1.Env, error) {
	env := &model.Env{}
	env.Name = name
	err := p.ds.Get(ctx, env)
	if err != nil {
		log.Logger.Errorf("check if env name exists failure %s", err.Error())
		return nil, bcode.ErrEnvNotExisted
	}
	if req.Alias != "" {
		env.Alias = req.Alias
	}
	if req.Description != "" {
		env.Description = req.Description
	}
	var targetChanged bool
	if len(req.Targets) > 0 && checkEqual(env.Targets, req.Targets) {
		targetChanged = true
		env.Targets = req.Targets
	}

	// create namespace at first
	if err := p.ds.Put(ctx, env); err != nil {
		return nil, err
	}

	if targetChanged {
		if err = p.updateAppWithNewEnv(ctx, name, env); err != nil {
			log.Logger.Errorf("update envbinding failure %s", err.Error())
			return nil, err
		}
	}
	targets, err := listTarget(ctx, p.ds, nil)
	if err != nil {
		return nil, err
	}
	resp := convertEnvModel2Base(env, targets)
	return resp, nil
}

// CreateEnv create an env for request
func (p *envUsecaseImpl) CreateEnv(ctx context.Context, req apisv1.CreateEnvRequest) (*apisv1.Env, error) {
	newEnv := &model.Env{}
	newEnv.EnvBase = model.EnvBase(req)

	err := createEnv(ctx, p.kubeClient, p.ds, newEnv)
	if err != nil {
		return nil, err
	}

	Targets, err := listTarget(ctx, p.ds, nil)
	if err != nil {
		return nil, err
	}

	resp := convertEnvModel2Base(newEnv, Targets)
	return resp, nil
}

func convertEnvModel2Base(env *model.Env, targets []*model.Target) *apisv1.Env {
	data := apisv1.Env{
		Name:        env.Name,
		Alias:       env.Alias,
		Description: env.Description,
		Project:     apisv1.NameAlias{Name: env.Project},
		Namespace:   env.Namespace,
		CreateTime:  env.CreateTime,
		UpdateTime:  env.UpdateTime,
	}
	for _, dt := range env.Targets {
		for _, tg := range targets {
			if dt == tg.Name {
				data.Targets = append(data.Targets, apisv1.NameAlias{
					Name:  dt,
					Alias: tg.Alias,
				})
			}
		}
	}
	return &data
}
