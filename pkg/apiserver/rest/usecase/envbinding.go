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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	utils2 "github.com/oam-dev/kubevela/pkg/utils"
)

const (
	// Deploy2Env deploy app to target cluster, suitable for common applications
	Deploy2Env string = "deploy2env"
	// DeployCloudResource deploy app to local and copy secret to target cluster, suitable for cloud application.
	DeployCloudResource string = "deploy-cloud-resource"
	// TerraformWorkloadType cloud application
	TerraformWorkloadType string = "configurations.terraform.core.oam.dev"
	// TerraformWorkloadKind terraform workload kind
	TerraformWorkloadKind string = "Configuration"
)

// EnvBindingUsecase envbinding usecase
type EnvBindingUsecase interface {
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

type envBindingUsecaseImpl struct {
	ds                datastore.DataStore
	workflowUsecase   WorkflowUsecase
	envUsecase        EnvUsecase
	definitionUsecase DefinitionUsecase
	kubeClient        client.Client
}

// NewEnvBindingUsecase new envBinding usecase
func NewEnvBindingUsecase(ds datastore.DataStore, workflowUsecase WorkflowUsecase, definitionUsecase DefinitionUsecase, envUsecase EnvUsecase) EnvBindingUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &envBindingUsecaseImpl{
		ds:                ds,
		workflowUsecase:   workflowUsecase,
		definitionUsecase: definitionUsecase,
		kubeClient:        kubecli,
		envUsecase:        envUsecase,
	}
}

func pickEnv(envs []*model.Env, name string) (*model.Env, error) {
	for _, e := range envs {
		if e.Name == name {
			return e, nil
		}
	}
	return nil, bcode.ErrEnvNotExisted
}

func listFullEnvBinding(ctx context.Context, ds datastore.DataStore, option envListOption) ([]*apisv1.EnvBindingBase, error) {
	envBindings, err := listEnvBindings(ctx, ds, option)
	if err != nil {
		return nil, bcode.ErrEnvBindingsNotExist
	}
	targets, err := listTarget(ctx, ds, "", nil)
	if err != nil {
		return nil, err
	}
	var listOption *datastore.ListOptions
	if option.projectName != "" {
		listOption = &datastore.ListOptions{
			FilterOptions: datastore.FilterOptions{
				In: []datastore.InQueryOption{
					{
						Key:    "project",
						Values: []string{option.projectName},
					},
				},
			},
		}
	}
	envs, err := listEnvs(ctx, ds, listOption)
	if err != nil {
		return nil, err
	}
	var list []*apisv1.EnvBindingBase
	for _, eb := range envBindings {
		env, err := pickEnv(envs, eb.Name)
		if err != nil {
			log.Logger.Errorf("envbinding invalid %s", err.Error())
			continue
		}
		list = append(list, convertEnvBindingModelToBase(eb, env, targets))
	}
	return list, nil
}

func (e *envBindingUsecaseImpl) GetEnvBindings(ctx context.Context, app *model.Application) ([]*apisv1.EnvBindingBase, error) {
	full, err := listFullEnvBinding(ctx, e.ds, envListOption{appPrimaryKey: app.PrimaryKey(), projectName: app.Project})
	if err != nil {
		log.Logger.Errorf("list envbinding for app %s err: %v\n", app.Name, err)
		return nil, err
	}
	return full, nil
}

func (e *envBindingUsecaseImpl) GetEnvBinding(ctx context.Context, app *model.Application, envName string) (*model.EnvBinding, error) {
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

func (e *envBindingUsecaseImpl) CreateEnvBinding(ctx context.Context, app *model.Application, envReq apisv1.CreateApplicationEnvbindingRequest) (*apisv1.EnvBinding, error) {
	envBinding, err := e.getBindingByEnv(ctx, app, envReq.Name)
	if err != nil {
		if !errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, err
		}
	}
	if envBinding != nil {
		return nil, bcode.ErrEnvBindingExist
	}
	env, err := getEnv(ctx, e.ds, envReq.Name)
	if err != nil {
		return nil, err
	}
	envBindingModel := convertCreateReqToEnvBindingModel(app, envReq)
	err = e.createEnvWorkflow(ctx, app, env, false)
	if err != nil {
		return nil, err
	}
	if err := e.ds.Add(ctx, &envBindingModel); err != nil {
		return nil, err
	}

	return &envReq.EnvBinding, nil
}

func (e *envBindingUsecaseImpl) BatchCreateEnvBinding(ctx context.Context, app *model.Application, envbindings apisv1.EnvBindingList) error {
	for i := range envbindings {
		envBindingModel := convertToEnvBindingModel(app, *envbindings[i])
		env, err := getEnv(ctx, e.ds, envBindingModel.Name)
		if err != nil {
			log.Logger.Errorf("get env failure %s", err.Error())
			continue
		}
		if err := e.ds.Add(ctx, envBindingModel); err != nil {
			log.Logger.Errorf("add envbinding %s failure %s", utils2.Sanitize(envBindingModel.Name), err.Error())
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

func (e *envBindingUsecaseImpl) getBindingByEnv(ctx context.Context, app *model.Application, envName string) (*model.EnvBinding, error) {
	var envBinding = model.EnvBinding{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          envName,
	}
	err := e.ds.Get(ctx, &envBinding)
	if err != nil {
		return nil, err
	}
	return &envBinding, nil
}

func (e *envBindingUsecaseImpl) UpdateEnvBinding(ctx context.Context, app *model.Application, envName string, _ apisv1.PutApplicationEnvBindingRequest) (*apisv1.DetailEnvBindingResponse, error) {
	envBinding, err := e.getBindingByEnv(ctx, app, envName)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrEnvBindingNotExist
		}
		return nil, err
	}
	env, err := getEnv(ctx, e.ds, envName)
	if err != nil {
		return nil, err
	}
	// update env
	if err := e.ds.Put(ctx, envBinding); err != nil {
		return nil, err
	}
	// update env workflow
	if err := UpdateEnvWorkflow(ctx, e.kubeClient, e.ds, app, env); err != nil {
		return nil, bcode.ErrEnvBindingUpdateWorkflow
	}
	return e.DetailEnvBinding(ctx, app, envBinding)
}

func (e *envBindingUsecaseImpl) DeleteEnvBinding(ctx context.Context, appModel *model.Application, envName string) error {
	envBinding, err := e.getBindingByEnv(ctx, appModel, envName)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrEnvBindingNotExist
		}
		return err
	}
	env, err := getEnv(ctx, e.ds, envName)
	if err != nil && errors.Is(err, datastore.ErrRecordNotExist) {
		return err
	}
	if env != nil {
		var app v1beta1.Application
		err = e.kubeClient.Get(ctx, types.NamespacedName{Namespace: env.Namespace, Name: appModel.Name}, &app)
		if err == nil || !apierrors.IsNotFound(err) {
			return bcode.ErrApplicationEnvRefusedDelete
		}
		if err := e.ds.Delete(ctx, &model.EnvBinding{AppPrimaryKey: appModel.PrimaryKey(), Name: envBinding.Name}); err != nil {
			return err
		}
	}
	// delete env workflow
	if err := e.deleteEnvWorkflow(ctx, appModel, convertWorkflowName(envBinding.Name)); err != nil {
		return err
	}
	return nil
}

func (e *envBindingUsecaseImpl) BatchDeleteEnvBinding(ctx context.Context, app *model.Application) error {
	envBindings, err := e.GetEnvBindings(ctx, app)
	if err != nil {
		return err
	}
	for _, envBinding := range envBindings {
		// delete env
		if err := e.ds.Delete(ctx, &model.EnvBinding{AppPrimaryKey: app.PrimaryKey(), Name: envBinding.Name}); err != nil {
			return err
		}
		// delete env workflow
		err := e.deleteEnvWorkflow(ctx, app, convertWorkflowName(envBinding.Name))
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *envBindingUsecaseImpl) createEnvWorkflow(ctx context.Context, app *model.Application, env *model.Env, isDefault bool) error {
	steps := GenEnvWorkflowSteps(ctx, e.kubeClient, e.ds, env, app)
	_, err := e.workflowUsecase.CreateOrUpdateWorkflow(ctx, app, apisv1.CreateWorkflowRequest{
		Name:        convertWorkflowName(env.Name),
		Alias:       fmt.Sprintf("%s Workflow", env.Alias),
		Description: "Created automatically by envbinding.",
		EnvName:     env.Name,
		Steps:       steps,
		Default:     &isDefault,
	})
	if err != nil {
		return err
	}
	return nil
}

func (e *envBindingUsecaseImpl) deleteEnvWorkflow(ctx context.Context, app *model.Application, workflowName string) error {
	if err := e.workflowUsecase.DeleteWorkflow(ctx, app, workflowName); err != nil {
		if !errors.Is(err, bcode.ErrWorkflowNotExist) {
			return err
		}
	}
	return nil
}

func (e *envBindingUsecaseImpl) DetailEnvBinding(ctx context.Context, app *model.Application, envBinding *model.EnvBinding) (*apisv1.DetailEnvBindingResponse, error) {
	targets, err := listTarget(ctx, e.ds, "", nil)
	if err != nil {
		return nil, err
	}
	env, err := getEnv(ctx, e.ds, envBinding.Name)
	if err != nil {
		return nil, err
	}
	return &apisv1.DetailEnvBindingResponse{
		EnvBindingBase: *convertEnvBindingModelToBase(envBinding, env, targets),
	}, nil
}

func (e *envBindingUsecaseImpl) ApplicationEnvRecycle(ctx context.Context, appModel *model.Application, envBinding *model.EnvBinding) error {
	env, err := getEnv(ctx, e.ds, envBinding.Name)
	if err != nil {
		return err
	}
	var app v1beta1.Application
	err = e.kubeClient.Get(ctx, types.NamespacedName{Namespace: env.Namespace, Name: appModel.Name}, &app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := e.kubeClient.Delete(ctx, &app); err != nil {
		return err
	}

	if err := resetRevisionsAndRecords(ctx, e.ds, appModel.Name, "", "", ""); err != nil {
		return err
	}
	return nil
}

func convertCreateReqToEnvBindingModel(app *model.Application, req apisv1.CreateApplicationEnvbindingRequest) model.EnvBinding {
	envBinding := model.EnvBinding{
		AppPrimaryKey: app.Name,
		Name:          req.Name,
		AppDeployName: app.GetAppNameForSynced(),
	}
	return envBinding
}

func convertEnvBindingModelToBase(envBinding *model.EnvBinding, env *model.Env, targets []*model.Target) *apisv1.EnvBindingBase {
	var dtMap = make(map[string]*model.Target, len(targets))
	for _, dte := range targets {
		dtMap[dte.Name] = dte
	}
	var envBindingTargets []apisv1.EnvBindingTarget
	for _, targetName := range env.Targets {
		dt := dtMap[targetName]
		if dt != nil {
			ebt := apisv1.EnvBindingTarget{
				NameAlias: apisv1.NameAlias{Name: dt.Name, Alias: dt.Alias},
			}
			if dt.Cluster != nil {
				ebt.Cluster = &apisv1.ClusterTarget{
					ClusterName: dt.Cluster.ClusterName,
					Namespace:   dt.Cluster.Namespace,
				}
			}
			envBindingTargets = append(envBindingTargets, ebt)
		}
	}
	ebb := &apisv1.EnvBindingBase{
		Name:               envBinding.Name,
		Alias:              env.Alias,
		Description:        env.Description,
		TargetNames:        env.Targets,
		Targets:            envBindingTargets,
		CreateTime:         envBinding.CreateTime,
		UpdateTime:         envBinding.UpdateTime,
		AppDeployName:      envBinding.AppDeployName,
		AppDeployNamespace: env.Namespace,
	}
	return ebb
}

func convertToEnvBindingModel(app *model.Application, envBind apisv1.EnvBinding) *model.EnvBinding {
	re := model.EnvBinding{
		AppPrimaryKey: app.Name,
		Name:          envBind.Name,
		AppDeployName: app.GetAppNameForSynced(),
	}
	return &re
}

func convertWorkflowName(envName string) string {
	return fmt.Sprintf("workflow-%s", envName)
}

func isEnvStepType(stepType string) bool {
	return stepType == Deploy2Env || stepType == DeployCloudResource
}
