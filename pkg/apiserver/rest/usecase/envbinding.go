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
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// EnvBindingUsecase envbinding usecase
type EnvBindingUsecase interface {
	GetEnvBindings(ctx context.Context, app *model.Application) ([]*apisv1.EnvBindingBase, error)
	GetEnvBinding(ctx context.Context, app *model.Application, envName string) (*model.EnvBinding, error)
	CheckAppEnvBindingsContainTarget(ctx context.Context, app *model.Application, targetName string) (bool, error)
	CreateEnvBinding(ctx context.Context, app *model.Application, env apisv1.CreateApplicationEnvRequest) (*apisv1.EnvBinding, error)
	BatchCreateEnvBinding(ctx context.Context, app *model.Application, env apisv1.EnvBindingList) error
	UpdateEnvBinding(ctx context.Context, app *model.Application, envName string, diff apisv1.PutApplicationEnvRequest) (*apisv1.DetailEnvBindingResponse, error)
	DeleteEnvBinding(ctx context.Context, app *model.Application, envName string) error
	BatchDeleteEnvBinding(ctx context.Context, app *model.Application) error
	DetailEnvBinding(ctx context.Context, app *model.Application, envBinding *model.EnvBinding) (*apisv1.DetailEnvBindingResponse, error)
	ApplicationEnvRecycle(ctx context.Context, appModel *model.Application, envBinding *model.EnvBinding) error
}

type envBindingUsecaseImpl struct {
	ds              datastore.DataStore
	workflowUsecase WorkflowUsecase
	kubeClient      client.Client
}

// NewEnvBindingUsecase new envBinding usecase
func NewEnvBindingUsecase(ds datastore.DataStore, workflowUsecase WorkflowUsecase) EnvBindingUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &envBindingUsecaseImpl{
		ds:              ds,
		workflowUsecase: workflowUsecase,
		kubeClient:      kubecli,
	}
}

func (e *envBindingUsecaseImpl) GetEnvBindings(ctx context.Context, app *model.Application) ([]*apisv1.EnvBindingBase, error) {
	var envBinding = model.EnvBinding{
		AppPrimaryKey: app.PrimaryKey(),
	}
	envBindings, err := e.ds.List(ctx, &envBinding, &datastore.ListOptions{})
	if err != nil {
		return nil, bcode.ErrEnvBindingsNotExist
	}
	deliveryTarget := model.DeliveryTarget{
		Namespace: app.Namespace,
	}
	deliveryTargets, err := e.ds.List(ctx, &deliveryTarget, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apisv1.EnvBindingBase
	for _, ebd := range envBindings {
		eb := ebd.(*model.EnvBinding)
		list = append(list, convertEnvbindingModelToBase(app, eb, deliveryTargets))
	}
	return list, nil
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

func (e *envBindingUsecaseImpl) CheckAppEnvBindingsContainTarget(ctx context.Context, app *model.Application, targetName string) (bool, error) {
	envBindings, err := e.GetEnvBindings(ctx, app)
	if err != nil {
		return false, err
	}
	var filteredList []*apisv1.EnvBindingBase
	for _, envBinding := range envBindings {
		if utils.StringsContain(envBinding.TargetNames, targetName) {
			filteredList = append(filteredList, envBinding)
		}
	}
	return len(filteredList) > 0, nil
}

func (e *envBindingUsecaseImpl) CreateEnvBinding(ctx context.Context, app *model.Application, envReq apisv1.CreateApplicationEnvRequest) (*apisv1.EnvBinding, error) {
	envBinding, err := e.getBindingByEnv(ctx, app, envReq.Name)
	if err != nil {
		if !errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, err
		}
	}
	if envBinding != nil {
		return nil, bcode.ErrEnvBindingExist
	}
	envBindingModel := convertCreateReqToEnvBindingModel(app, envReq)
	if err := e.ds.Add(ctx, &envBindingModel); err != nil {
		return nil, err
	}
	err = e.createEnvWorkflow(ctx, app, &envBindingModel)
	if err != nil {
		return nil, err
	}
	return &envReq.EnvBinding, nil
}

func (e *envBindingUsecaseImpl) BatchCreateEnvBinding(ctx context.Context, app *model.Application, envbindings apisv1.EnvBindingList) error {
	for _, envBinding := range envbindings {
		envBindingModel := convertToEnvBindingModel(app, *envBinding)
		if err := e.ds.Add(ctx, envBindingModel); err != nil {
			return err
		}
		err := e.createEnvWorkflow(ctx, app, envBindingModel)
		if err != nil {
			return err
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

func (e *envBindingUsecaseImpl) UpdateEnvBinding(ctx context.Context, app *model.Application, envName string, envUpdate apisv1.PutApplicationEnvRequest) (*apisv1.DetailEnvBindingResponse, error) {
	envBinding, err := e.getBindingByEnv(ctx, app, envName)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrEnvBindingNotExist
		}
		return nil, err
	}
	convertUpdateReqToEnvBindingModel(envBinding, envUpdate)
	// update env
	if err := e.ds.Put(ctx, envBinding); err != nil {
		return nil, err
	}
	// update env workflow
	if err := e.updateEnvWorkflow(ctx, app, envBinding); err != nil {
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
	var app v1beta1.Application
	err = e.kubeClient.Get(ctx, types.NamespacedName{Namespace: appModel.Namespace, Name: convertAppName(appModel.Name, envBinding.Name)}, &app)
	if err == nil || !apierrors.IsNotFound(err) {
		return bcode.ErrApplicationEnvRefusedDelete
	}
	if err := e.ds.Delete(ctx, &model.EnvBinding{AppPrimaryKey: appModel.PrimaryKey(), Name: envBinding.Name}); err != nil {
		return err
	}
	// delete env workflow
	if err := e.deleteEnvWorkflow(ctx, appModel, envBinding.Name); err != nil {
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
		err := e.deleteEnvWorkflow(ctx, app, envBinding.Name)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *envBindingUsecaseImpl) createEnvWorkflow(ctx context.Context, app *model.Application, env *model.EnvBinding) error {
	steps := genEnvWorkflowSteps(env, app)
	_, err := e.workflowUsecase.CreateOrUpdateWorkflow(ctx, app, apisv1.CreateWorkflowRequest{
		AppName:     app.PrimaryKey(),
		Name:        env.Name,
		Alias:       fmt.Sprintf("%s env workflow", env.Alias),
		Description: "Created automatically by envbinding.",
		EnvName:     env.Name,
		Steps:       steps,
		Default:     false,
	})
	if err != nil {
		return err
	}
	return nil
}

func (e *envBindingUsecaseImpl) updateEnvWorkflow(ctx context.Context, app *model.Application, env *model.EnvBinding) error {
	steps := genEnvWorkflowSteps(env, app)
	workflow, err := e.workflowUsecase.GetWorkflow(ctx, app, env.Name)
	if err != nil {
		return err
	}
	_, err = e.workflowUsecase.UpdateWorkflow(ctx, workflow, apisv1.UpdateWorkflowRequest{
		Steps:       steps,
		Description: workflow.Description,
		EnvName:     workflow.EnvName,
	})
	if err != nil {
		return err
	}
	return nil
}

func (e *envBindingUsecaseImpl) deleteEnvWorkflow(ctx context.Context, app *model.Application, workflowName string) error {
	return e.workflowUsecase.DeleteWorkflow(ctx, app, workflowName)
}

func (e *envBindingUsecaseImpl) DetailEnvBinding(ctx context.Context, app *model.Application, envBinding *model.EnvBinding) (*apisv1.DetailEnvBindingResponse, error) {
	deliveryTarget := model.DeliveryTarget{
		Namespace: app.Namespace,
	}
	deliveryTargets, err := e.ds.List(ctx, &deliveryTarget, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	return &apisv1.DetailEnvBindingResponse{
		EnvBindingBase: *convertEnvbindingModelToBase(app, envBinding, deliveryTargets),
	}, nil
}

func (e *envBindingUsecaseImpl) ApplicationEnvRecycle(ctx context.Context, appModel *model.Application, envBinding *model.EnvBinding) error {
	var app v1beta1.Application
	err := e.kubeClient.Get(ctx, types.NamespacedName{Namespace: appModel.Namespace, Name: convertAppName(appModel.Name, envBinding.Name)}, &app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return e.kubeClient.Delete(ctx, &app)
}

func convertCreateReqToEnvBindingModel(app *model.Application, req apisv1.CreateApplicationEnvRequest) model.EnvBinding {
	envBinding := model.EnvBinding{
		AppPrimaryKey: app.Name,
		Name:          req.Name,
		Alias:         req.Alias,
		Description:   req.Description,
		TargetNames:   req.TargetNames,
	}
	return envBinding
}

func convertEnvbindingModelToBase(app *model.Application, envBinding *model.EnvBinding, deliveryTargets []datastore.Entity) *apisv1.EnvBindingBase {
	var dtMap = make(map[string]*model.DeliveryTarget, len(deliveryTargets))
	for _, dte := range deliveryTargets {
		dt := dte.(*model.DeliveryTarget)
		dtMap[dt.Name] = dt
	}
	var targets []apisv1.DeliveryTargetBase
	for _, targetName := range envBinding.TargetNames {
		dt := dtMap[targetName]
		if dt != nil {
			targets = append(targets, *convertFromDeliveryTargetModel(dt))
		}
	}
	ebb := &apisv1.EnvBindingBase{
		Name:              envBinding.Name,
		Alias:             envBinding.Alias,
		Description:       envBinding.Description,
		TargetNames:       envBinding.TargetNames,
		Targets:           targets,
		ComponentSelector: (*apisv1.ComponentSelector)(envBinding.ComponentSelector),
		CreateTime:        envBinding.CreateTime,
		UpdateTime:        envBinding.UpdateTime,
		AppDeployName:     convertAppName(app.Name, envBinding.Name),
	}
	return ebb
}

func convertUpdateReqToEnvBindingModel(envBinding *model.EnvBinding, envUpdate apisv1.PutApplicationEnvRequest) *model.EnvBinding {
	envBinding.Alias = envUpdate.Alias
	envBinding.Description = envUpdate.Description
	envBinding.TargetNames = envUpdate.TargetNames
	if envUpdate.ComponentSelector != nil {
		envBinding.ComponentSelector = (*model.ComponentSelector)(envUpdate.ComponentSelector)
	}
	return envBinding
}

func convertToEnvBindingModel(app *model.Application, envBind apisv1.EnvBinding) *model.EnvBinding {
	re := model.EnvBinding{
		AppPrimaryKey: app.Name,
		Name:          envBind.Name,
		Description:   envBind.Description,
		Alias:         envBind.Alias,
		TargetNames:   envBind.TargetNames,
	}
	if envBind.ComponentSelector != nil {
		re.ComponentSelector = (*model.ComponentSelector)(envBind.ComponentSelector)
	}
	return &re
}

func genEnvWorkflowSteps(env *model.EnvBinding, app *model.Application) []apisv1.WorkflowStep {
	var workflowSteps []v1beta1.WorkflowStep
	for _, targetName := range env.TargetNames {
		step := v1beta1.WorkflowStep{
			Name: genPolicyEnvName(targetName),
			Type: "deploy2env",
			Properties: util.Object2RawExtension(map[string]string{
				"policy": genPolicyName(env.Name),
				"env":    genPolicyEnvName(targetName),
			}),
		}
		workflowSteps = append(workflowSteps, step)
	}
	var steps []apisv1.WorkflowStep
	for _, step := range workflowSteps {
		var propertyStr string
		if step.Properties != nil {
			properties, err := model.NewJSONStruct(step.Properties)
			if err != nil {
				log.Logger.Errorf("workflow %s step %s properties is invalid %s", app.Name, step.Name, err.Error())
				continue
			}
			propertyStr = properties.JSON()
		}
		steps = append(steps, apisv1.WorkflowStep{
			Name:       step.Name,
			Type:       step.Type,
			DependsOn:  step.DependsOn,
			Properties: propertyStr,
			Inputs:     step.Inputs,
			Outputs:    step.Outputs,
		})
	}
	return steps
}
