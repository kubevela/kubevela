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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/repository"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	assembler "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/assembler/v1"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	pkgUtils "github.com/oam-dev/kubevela/pkg/utils"
)

// EnvBindingService envbinding service
type EnvBindingService interface {
	GetEnvBindings(ctx context.Context, app *model.Application) ([]*apisv1.EnvBindingBase, error)
	GetEnvBinding(ctx context.Context, app *model.Application, envName string) (*model.EnvBinding, error)
	CreateEnvBinding(ctx context.Context, app *model.Application, env apisv1.CreateApplicationEnvbindingRequest) (*apisv1.EnvBinding, error)
	BatchCreateEnvBinding(ctx context.Context, app *model.Application, env apisv1.EnvBindingList) error
	UpdateEnvBinding(ctx context.Context, app *model.Application, envName string, diff apisv1.PutApplicationEnvBindingRequest) (*apisv1.DetailEnvBindingResponse, error)
	DeleteEnvBinding(ctx context.Context, app *model.Application, envName string) error
	BatchDeleteEnvBinding(ctx context.Context, app *model.Application) error
	DetailEnvBinding(ctx context.Context, app *model.Application, envBinding *model.EnvBinding) (*apisv1.DetailEnvBindingResponse, error)
	ApplicationEnvRecycle(ctx context.Context, appModel *model.Application, envBinding *model.EnvBinding) error
}

type envBindingServiceImpl struct {
	Store             datastore.DataStore `inject:"datastore"`
	WorkflowService   WorkflowService     `inject:""`
	EnvService        EnvService          `inject:""`
	DefinitionService DefinitionService   `inject:""`
	KubeClient        client.Client       `inject:"kubeClient"`
}

// NewEnvBindingService new envBinding service
func NewEnvBindingService() EnvBindingService {
	return &envBindingServiceImpl{}
}

func (e *envBindingServiceImpl) GetEnvBindings(ctx context.Context, app *model.Application) ([]*apisv1.EnvBindingBase, error) {
	full, err := repository.ListFullEnvBinding(ctx, e.Store, repository.EnvListOption{AppPrimaryKey: app.PrimaryKey(), ProjectName: app.Project})
	if err != nil {
		log.Logger.Errorf("list envbinding for app %s err: %v\n", app.Name, err)
		return nil, err
	}
	return full, nil
}

func (e *envBindingServiceImpl) GetEnvBinding(ctx context.Context, app *model.Application, envName string) (*model.EnvBinding, error) {
	envBinding, err := e.getBindingByEnv(ctx, app, envName)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrEnvBindingsNotExist
		}
		return nil, err
	}
	return envBinding, nil
}

// CheckAppEnvBindingsContainTarget check envbinding contain target
func CheckAppEnvBindingsContainTarget(envBindings []*apisv1.EnvBindingBase, targetName string) (bool, error) {
	var filteredList []*apisv1.EnvBindingBase
	for _, envBinding := range envBindings {
		if utils.StringsContain(envBinding.TargetNames, targetName) {
			filteredList = append(filteredList, envBinding)
		}
	}
	return len(filteredList) > 0, nil
}

func (e *envBindingServiceImpl) CreateEnvBinding(ctx context.Context, app *model.Application, envReq apisv1.CreateApplicationEnvbindingRequest) (*apisv1.EnvBinding, error) {
	envBinding, err := e.getBindingByEnv(ctx, app, envReq.Name)
	if err != nil {
		if !errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, err
		}
	}
	if envBinding != nil {
		return nil, bcode.ErrEnvBindingExist
	}
	env, err := repository.GetEnv(ctx, e.Store, envReq.Name)
	if err != nil {
		return nil, err
	}
	envBindingModel := assembler.CreateEnvBindingModel(app, envReq)
	err = e.createEnvWorkflow(ctx, app, env, false)
	if err != nil {
		return nil, err
	}
	if err := e.Store.Add(ctx, &envBindingModel); err != nil {
		return nil, err
	}

	return &envReq.EnvBinding, nil
}

func (e *envBindingServiceImpl) BatchCreateEnvBinding(ctx context.Context, app *model.Application, envbindings apisv1.EnvBindingList) error {
	for i := range envbindings {
		envBindingModel := assembler.ConvertToEnvBindingModel(app, *envbindings[i])
		env, err := repository.GetEnv(ctx, e.Store, envBindingModel.Name)
		if err != nil {
			log.Logger.Errorf("get env failure %s", err.Error())
			continue
		}
		if err := e.Store.Add(ctx, envBindingModel); err != nil {
			log.Logger.Errorf("add envbinding %s failure %s", pkgUtils.Sanitize(envBindingModel.Name), err.Error())
			continue
		}
		err = e.createEnvWorkflow(ctx, app, env, i == 0)
		if err != nil {
			log.Logger.Errorf("create env workflow failure %s", err.Error())
			continue
		}
	}
	return nil
}

func (e *envBindingServiceImpl) getBindingByEnv(ctx context.Context, app *model.Application, envName string) (*model.EnvBinding, error) {
	var envBinding = model.EnvBinding{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          envName,
	}
	err := e.Store.Get(ctx, &envBinding)
	if err != nil {
		return nil, err
	}
	return &envBinding, nil
}

func (e *envBindingServiceImpl) UpdateEnvBinding(ctx context.Context, app *model.Application, envName string, _ apisv1.PutApplicationEnvBindingRequest) (*apisv1.DetailEnvBindingResponse, error) {
	envBinding, err := e.getBindingByEnv(ctx, app, envName)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrEnvBindingNotExist
		}
		return nil, err
	}
	env, err := repository.GetEnv(ctx, e.Store, envName)
	if err != nil {
		return nil, err
	}
	// update env
	if err := e.Store.Put(ctx, envBinding); err != nil {
		return nil, err
	}
	// update env workflow
	if err := repository.UpdateEnvWorkflow(ctx, e.KubeClient, e.Store, app, env); err != nil {
		return nil, bcode.ErrEnvBindingUpdateWorkflow
	}
	return e.DetailEnvBinding(ctx, app, envBinding)
}

func (e *envBindingServiceImpl) DeleteEnvBinding(ctx context.Context, appModel *model.Application, envName string) error {
	envBinding, err := e.getBindingByEnv(ctx, appModel, envName)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrEnvBindingNotExist
		}
		return err
	}
	env, err := repository.GetEnv(ctx, e.Store, envName)
	if err != nil && errors.Is(err, datastore.ErrRecordNotExist) {
		return err
	}
	if env != nil {
		var app v1beta1.Application
		err = e.KubeClient.Get(ctx, types.NamespacedName{Namespace: env.Namespace, Name: appModel.Name}, &app)
		if err == nil || !apierrors.IsNotFound(err) {
			return bcode.ErrApplicationEnvRefusedDelete
		}
		if err := e.Store.Delete(ctx, &model.EnvBinding{AppPrimaryKey: appModel.PrimaryKey(), Name: envBinding.Name}); err != nil {
			return err
		}
	}
	// delete env workflow
	if err := e.deleteEnvWorkflow(ctx, appModel, repository.ConvertWorkflowName(envBinding.Name)); err != nil {
		return fmt.Errorf("fail to clear the workflow belong to the env %w", err)
	}

	// delete the topology and env-bindings policies
	return repository.DeleteApplicationEnvPolicies(ctx, e.Store, appModel, envName)
}

func (e *envBindingServiceImpl) BatchDeleteEnvBinding(ctx context.Context, app *model.Application) error {
	envBindings, err := e.GetEnvBindings(ctx, app)
	if err != nil {
		return err
	}
	for _, envBinding := range envBindings {
		// delete env
		if err := e.Store.Delete(ctx, &model.EnvBinding{AppPrimaryKey: app.PrimaryKey(), Name: envBinding.Name}); err != nil {
			return err
		}
		// delete env workflow
		err := e.deleteEnvWorkflow(ctx, app, repository.ConvertWorkflowName(envBinding.Name))
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *envBindingServiceImpl) createEnvWorkflow(ctx context.Context, app *model.Application, env *model.Env, isDefault bool) error {
	steps, policies := repository.GenEnvWorkflowStepsAndPolicies(ctx, e.KubeClient, e.Store, env, app)
	workflow := &model.Workflow{
		Steps:         steps,
		Name:          repository.ConvertWorkflowName(env.Name),
		Alias:         fmt.Sprintf("%s Workflow", env.Alias),
		Description:   "Created automatically by envbinding.",
		Default:       &isDefault,
		EnvName:       env.Name,
		AppPrimaryKey: app.PrimaryKey(),
	}
	log.Logger.Infof("create workflow %s for app %s", pkgUtils.Sanitize(workflow.Name), pkgUtils.Sanitize(app.PrimaryKey()))
	if err := e.Store.Add(ctx, workflow); err != nil {
		return err
	}
	err := e.Store.BatchAdd(ctx, policies)
	if err != nil {
		if err := e.WorkflowService.DeleteWorkflow(ctx, app, repository.ConvertWorkflowName(env.Name)); err != nil {
			log.Logger.Errorf("fail to rollback the workflow after fail to create policies, %s", err.Error())
		}
		return fmt.Errorf("fail to create policies %w", err)
	}
	return nil
}

func (e *envBindingServiceImpl) deleteEnvWorkflow(ctx context.Context, app *model.Application, workflowName string) error {
	if err := e.WorkflowService.DeleteWorkflow(ctx, app, workflowName); err != nil {
		if !errors.Is(err, bcode.ErrWorkflowNotExist) {
			return err
		}
	}
	return nil
}

func (e *envBindingServiceImpl) DetailEnvBinding(ctx context.Context, app *model.Application, envBinding *model.EnvBinding) (*apisv1.DetailEnvBindingResponse, error) {
	targets, err := repository.ListTarget(ctx, e.Store, "", nil)
	if err != nil {
		return nil, err
	}
	env, err := repository.GetEnv(ctx, e.Store, envBinding.Name)
	if err != nil {
		return nil, err
	}
	return &apisv1.DetailEnvBindingResponse{
		EnvBindingBase: *assembler.ConvertEnvBindingModelToBase(envBinding, env, targets),
	}, nil
}

func (e *envBindingServiceImpl) ApplicationEnvRecycle(ctx context.Context, appModel *model.Application, envBinding *model.EnvBinding) error {
	env, err := repository.GetEnv(ctx, e.Store, envBinding.Name)
	if err != nil {
		return err
	}
	var app v1beta1.Application
	name := envBinding.AppDeployName
	if name == "" {
		name = appModel.Name
	}
	err = e.KubeClient.Get(ctx, types.NamespacedName{Namespace: env.Namespace, Name: name}, &app)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	if err == nil {
		if err := e.KubeClient.Delete(ctx, &app); err != nil {
			return err
		}
	}

	if err := resetRevisionsAndRecords(ctx, e.Store, appModel.Name, "", "", ""); err != nil {
		return err
	}
	log.Logger.Infof("Application %s(%s) recycle successfully from env %s", appModel.Name, name, env.Name)
	return nil
}
