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
	ListEnvs(ctx context.Context, page, pageSize int, listOption apisv1.ListEnvOptions) (*apisv1.ListEnvResponse, error)
	ListEnvCount(ctx context.Context, listOption apisv1.ListEnvOptions) (int64, error)
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
func (p *envUsecaseImpl) ListEnvs(ctx context.Context, page, pageSize int, listOption apisv1.ListEnvOptions) (*apisv1.ListEnvResponse, error) {
	entities, err := listEnvs(ctx, p.ds, listOption.Project, &datastore.ListOptions{Page: page, PageSize: pageSize, SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
	if err != nil {
		return nil, err
	}

	Targets, err := listTarget(ctx, p.ds, "", nil)
	if err != nil {
		return nil, err
	}

	var envs []*apisv1.Env
	for _, ee := range entities {
		envs = append(envs, convertEnvModel2Base(ee, Targets))
	}

	projectResp, err := listProjects(ctx, p.ds, 0, 0)
	if err != nil {
		return nil, err
	}
	for _, e := range envs {
		for _, pj := range projectResp.Projects {
			if e.Project.Name == pj.Name {
				e.Project.Alias = pj.Alias
				break
			}
		}
	}
	total, err := p.ds.Count(ctx, &model.Env{Project: listOption.Project}, nil)
	if err != nil {
		return nil, err
	}
	return &apisv1.ListEnvResponse{Envs: envs, Total: total}, nil
}

func (p *envUsecaseImpl) ListEnvCount(ctx context.Context, listOption apisv1.ListEnvOptions) (int64, error) {
	return p.ds.Count(ctx, &model.Env{Project: listOption.Project}, nil)
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

	pass, err := p.checkEnvTarget(ctx, env.Project, env.Name, req.Targets)
	if err != nil || !pass {
		return nil, bcode.ErrEnvTargetConflict
	}

	var targetChanged bool
	if len(req.Targets) > 0 && !checkEqual(env.Targets, req.Targets) {
		targetChanged = true
		env.Targets = req.Targets
	}

	targets, err := listTarget(ctx, p.ds, "", nil)
	if err != nil {
		return nil, err
	}
	var targetMap = make(map[string]*model.Target, len(targets))
	for i, existTarget := range targets {
		targetMap[existTarget.Name] = targets[i]
	}
	for _, target := range req.Targets {
		if _, exist := targetMap[target]; !exist {
			return nil, bcode.ErrTargetNotExist
		}
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

	resp := convertEnvModel2Base(env, targets)
	return resp, nil
}

// CreateEnv create an env for request
func (p *envUsecaseImpl) CreateEnv(ctx context.Context, req apisv1.CreateEnvRequest) (*apisv1.Env, error) {
	newEnv := &model.Env{
		Name:        req.Name,
		Alias:       req.Alias,
		Description: req.Description,
		Namespace:   req.Namespace,
		Project:     req.Project,
		Targets:     req.Targets,
	}

	pass, err := p.checkEnvTarget(ctx, req.Project, req.Name, req.Targets)
	if err != nil || !pass {
		return nil, bcode.ErrEnvTargetConflict
	}

	targets, err := listTarget(ctx, p.ds, "", nil)
	if err != nil {
		return nil, err
	}

	var targetMap = make(map[string]*model.Target, len(targets))
	for i, existTarget := range targets {
		targetMap[existTarget.Name] = targets[i]
	}

	for _, target := range req.Targets {
		if _, exist := targetMap[target]; !exist {
			return nil, bcode.ErrTargetNotExist
		}
	}

	err = createEnv(ctx, p.kubeClient, p.ds, newEnv)
	if err != nil {
		return nil, err
	}

	resp := convertEnvModel2Base(newEnv, targets)
	return resp, nil
}

// checkEnvTarget In one project, a delivery target can only belong to one env.
func (p *envUsecaseImpl) checkEnvTarget(ctx context.Context, project string, envName string, targets []string) (bool, error) {
	if len(targets) == 0 {
		return true, nil
	}
	entitys, err := p.ds.List(ctx, &model.Env{Project: project}, &datastore.ListOptions{})
	if err != nil {
		return false, err
	}
	newMap := make(map[string]bool, len(targets))
	for _, new := range targets {
		newMap[new] = true
	}
	for _, entity := range entitys {
		env := entity.(*model.Env)
		for _, existTarget := range env.Targets {
			if ok := newMap[existTarget]; ok && env.Name != envName {
				return false, nil
			}
		}
	}
	return true, nil
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
