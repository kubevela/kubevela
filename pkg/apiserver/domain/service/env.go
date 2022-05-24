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
	"errors"
	"reflect"
	"sort"

	apierror "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/repository"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/oam"
	util "github.com/oam-dev/kubevela/pkg/utils"
)

// EnvService defines the API of Env.
type EnvService interface {
	GetEnv(ctx context.Context, envName string) (*model.Env, error)
	ListEnvs(ctx context.Context, page, pageSize int, listOption apisv1.ListEnvOptions) (*apisv1.ListEnvResponse, error)
	ListEnvCount(ctx context.Context, listOption apisv1.ListEnvOptions) (int64, error)
	DeleteEnv(ctx context.Context, envName string) error
	CreateEnv(ctx context.Context, req apisv1.CreateEnvRequest) (*apisv1.Env, error)
	UpdateEnv(ctx context.Context, envName string, req apisv1.UpdateEnvRequest) (*apisv1.Env, error)
}

type envServiceImpl struct {
	Store          datastore.DataStore `inject:"datastore"`
	ProjectService ProjectService      `inject:""`
	KubeClient     client.Client       `inject:"kubeClient"`
}

// NewEnvService new env service
func NewEnvService() EnvService {
	return &envServiceImpl{}
}

// GetEnv get env
func (p *envServiceImpl) GetEnv(ctx context.Context, envName string) (*model.Env, error) {
	return repository.GetEnv(ctx, p.Store, envName)
}

// DeleteEnv delete an env by name
// the function assume applications contain in env already empty.
// it won't delete the namespace created by the Env, but it will update the label
func (p *envServiceImpl) DeleteEnv(ctx context.Context, envName string) error {
	env := &model.Env{}
	env.Name = envName

	if err := p.Store.Get(ctx, env); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}
	// reset the labels
	err := util.UpdateNamespace(ctx, p.KubeClient, env.Namespace, util.MergeOverrideLabels(map[string]string{
		oam.LabelNamespaceOfEnvName:         "",
		oam.LabelControlPlaneNamespaceUsage: "",
	}))
	if err != nil && apierror.IsNotFound(err) {
		return err
	}

	if err = p.Store.Delete(ctx, env); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}
	return nil
}

// ListEnvs list envs
func (p *envServiceImpl) ListEnvs(ctx context.Context, page, pageSize int, listOption apisv1.ListEnvOptions) (*apisv1.ListEnvResponse, error) {
	userName, ok := ctx.Value(&apisv1.CtxKeyUser).(string)
	if !ok {
		return nil, bcode.ErrUnauthorized
	}
	projects, err := p.ProjectService.ListUserProjects(ctx, userName)
	if err != nil {
		return nil, err
	}
	var availableProjectNames []string
	var projectNameAlias = make(map[string]string)
	for _, project := range projects {
		availableProjectNames = append(availableProjectNames, project.Name)
		projectNameAlias[project.Name] = project.Alias
	}
	if len(availableProjectNames) == 0 {
		return &apisv1.ListEnvResponse{Envs: []*apisv1.Env{}, Total: 0}, nil
	}
	if listOption.Project != "" {
		if !util.StringsContain(availableProjectNames, listOption.Project) {
			return &apisv1.ListEnvResponse{Envs: []*apisv1.Env{}, Total: 0}, nil
		}
	}
	projectNames := []string{listOption.Project}
	if listOption.Project == "" {
		projectNames = availableProjectNames
	}
	filter := datastore.FilterOptions{
		In: []datastore.InQueryOption{
			{
				Key:    "project",
				Values: projectNames,
			},
		},
	}
	entities, err := repository.ListEnvs(ctx, p.Store, &datastore.ListOptions{
		Page:          page,
		PageSize:      pageSize,
		SortBy:        []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
		FilterOptions: filter,
	})
	if err != nil {
		return nil, err
	}

	targets, err := repository.ListTarget(ctx, p.Store, listOption.Project, nil)
	if err != nil {
		return nil, err
	}

	var envs []*apisv1.Env
	for _, ee := range entities {
		envs = append(envs, convertEnvModel2Base(ee, targets))
	}

	for i := range envs {
		envs[i].Project.Alias = projectNameAlias[envs[i].Project.Name]
	}

	total, err := p.Store.Count(ctx, &model.Env{Project: listOption.Project}, &filter)
	if err != nil {
		return nil, err
	}
	return &apisv1.ListEnvResponse{Envs: envs, Total: total}, nil
}

func (p *envServiceImpl) ListEnvCount(ctx context.Context, listOption apisv1.ListEnvOptions) (int64, error) {
	return p.Store.Count(ctx, &model.Env{Project: listOption.Project}, nil)
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

func (p *envServiceImpl) updateAppWithNewEnv(ctx context.Context, envName string, env *model.Env) error {

	// List all apps inside the env
	apps, err := listApp(ctx, p.Store, apisv1.ListApplicationOptions{Env: envName})
	if err != nil {
		return err
	}
	for _, app := range apps {
		err = repository.UpdateEnvWorkflow(ctx, p.KubeClient, p.Store, app, env)
		if err != nil {
			return err
		}
	}
	return nil

}

// UpdateEnv update an env for request
func (p *envServiceImpl) UpdateEnv(ctx context.Context, name string, req apisv1.UpdateEnvRequest) (*apisv1.Env, error) {
	env := &model.Env{}
	env.Name = name
	err := p.Store.Get(ctx, env)
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

	targets, err := repository.ListTarget(ctx, p.Store, "", nil)
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
	if err := p.Store.Put(ctx, env); err != nil {
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
func (p *envServiceImpl) CreateEnv(ctx context.Context, req apisv1.CreateEnvRequest) (*apisv1.Env, error) {
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

	targets, err := repository.ListTarget(ctx, p.Store, "", nil)
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

	err = repository.CreateEnv(ctx, p.KubeClient, p.Store, newEnv)
	if err != nil {
		return nil, err
	}

	resp := convertEnvModel2Base(newEnv, targets)
	return resp, nil
}

// checkEnvTarget In one project, a delivery target can only belong to one env.
func (p *envServiceImpl) checkEnvTarget(ctx context.Context, project string, envName string, targets []string) (bool, error) {
	if len(targets) == 0 {
		return true, nil
	}
	entitys, err := p.Store.List(ctx, &model.Env{Project: project}, &datastore.ListOptions{})
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
