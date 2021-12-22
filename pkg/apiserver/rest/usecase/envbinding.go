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
	utils2 "github.com/oam-dev/kubevela/pkg/utils"
)

const (
	// Deploy2Env deploy app to target cluster, suitable for common applications
	Deploy2Env string = "deploy2env"
	// DeployCloudResource deploy app to local and copy secret to target cluster, suitable for cloud application.
	DeployCloudResource string = "deploy-cloud-resource"
	// TerraformWorkfloadType cloud application
	TerraformWorkfloadType string = "configurations.terraform.core.oam.dev"
	// TerraformWorkfloadKind terraform workload kind
	TerraformWorkfloadKind string = "Configuration"
)

// EnvBindingUsecase envbinding usecase
type EnvBindingUsecase interface {
	GetEnvBindings(ctx context.Context, app *model.Application) ([]*apisv1.EnvBindingBase, error)
	GetEnvBinding(ctx context.Context, app *model.Application, envName string) (*model.EnvBinding, error)
	CheckAppEnvBindingsContainTarget(ctx context.Context, app *model.Application, targetName string) (bool, error)
	CreateEnvBinding(ctx context.Context, app *model.Application, env apisv1.CreateApplicationEnvbindingRequest) (*apisv1.EnvBinding, error)
	BatchCreateEnvBinding(ctx context.Context, app *model.Application, env apisv1.EnvBindingList) error
	UpdateEnvBinding(ctx context.Context, app *model.Application, envName string, diff apisv1.PutApplicationEnvBindingRequest) (*apisv1.DetailEnvBindingResponse, error)
	DeleteEnvBinding(ctx context.Context, app *model.Application, envName string) error
	BatchDeleteEnvBinding(ctx context.Context, app *model.Application) error
	DetailEnvBinding(ctx context.Context, app *model.Application, envBinding *model.EnvBinding) (*apisv1.DetailEnvBindingResponse, error)
	ApplicationEnvRecycle(ctx context.Context, appModel *model.Application, envBinding *model.EnvBinding) error
	GetSuitableType(ctx context.Context, app *model.Application) string
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

func (e *envBindingUsecaseImpl) GetEnvBindings(ctx context.Context, app *model.Application) ([]*apisv1.EnvBindingBase, error) {
	var envBinding = model.EnvBinding{
		AppPrimaryKey: app.PrimaryKey(),
	}
	envBindings, err := e.ds.List(ctx, &envBinding, &datastore.ListOptions{})
	if err != nil {
		return nil, bcode.ErrEnvBindingsNotExist
	}
	targets, err := e.ds.List(ctx, &model.Target{}, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apisv1.EnvBindingBase
	for _, ebd := range envBindings {
		eb := ebd.(*model.EnvBinding)
		env, err := e.envUsecase.GetEnv(ctx, eb.Name)
		if err != nil {
			log.Logger.Errorf("app %s envbinding %s is invalid %s", app.Name, ebd.PrimaryKey(), err.Error())
			continue
		}
		list = append(list, convertEnvbindingModelToBase(app, eb, env, targets))
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
	env, err := e.envUsecase.GetEnv(ctx, envReq.Name)
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
		env, err := e.envUsecase.GetEnv(ctx, envBindingModel.Name)
		if err != nil {
			log.Logger.Errorf("get env failure %s", err.Error())
			continue
		}
		if err := e.ds.Add(ctx, envBindingModel); err != nil {
			log.Logger.Errorf("add envbinding %s failure %s", envBindingModel.Name, err.Error())
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

func (e *envBindingUsecaseImpl) UpdateEnvBinding(ctx context.Context, app *model.Application, envName string, envUpdate apisv1.PutApplicationEnvBindingRequest) (*apisv1.DetailEnvBindingResponse, error) {
	envBinding, err := e.getBindingByEnv(ctx, app, envName)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrEnvBindingNotExist
		}
		return nil, err
	}
	convertUpdateReqToEnvBindingModel(envBinding, envUpdate)
	env, err := e.envUsecase.GetEnv(ctx, envName)
	if err != nil {
		return nil, err
	}
	// update env
	if err := e.ds.Put(ctx, envBinding); err != nil {
		return nil, err
	}
	// update env workflow
	if err := e.updateEnvWorkflow(ctx, app, env); err != nil {
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
	env, err := e.envUsecase.GetEnv(ctx, envName)
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
	steps := e.genEnvWorkflowSteps(ctx, env, app)
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

func (e *envBindingUsecaseImpl) updateEnvWorkflow(ctx context.Context, app *model.Application, env *model.Env) error {
	// The existing step configuration should be maintained and the delivery target steps should be automatically updated.
	envSteps := e.genEnvWorkflowSteps(ctx, env, app)
	workflow, err := e.workflowUsecase.GetWorkflow(ctx, app, convertWorkflowName(env.Name))
	if err != nil {
		return err
	}

	var envStepNames = env.Targets
	var workflowStepNames []string
	for _, step := range workflow.Steps {
		if isEnvStepType(step.Type) {
			workflowStepNames = append(workflowStepNames, step.Name)
		}
	}

	var filteredSteps []apisv1.WorkflowStep
	_, readyToDeleteSteps, readyToAddSteps := compareSlices(workflowStepNames, envStepNames)

	for _, step := range workflow.Steps {
		if isEnvStepType(step.Type) && utils.StringsContain(readyToDeleteSteps, step.Name) {
			continue
		}
		filteredSteps = append(filteredSteps, convertFromWorkflowStepModel(step))
	}

	for _, step := range envSteps {
		if isEnvStepType(step.Type) && utils.StringsContain(readyToAddSteps, step.Name) {
			filteredSteps = append(filteredSteps, step)
		}
	}

	_, err = e.workflowUsecase.UpdateWorkflow(ctx, workflow, apisv1.UpdateWorkflowRequest{
		Steps:       filteredSteps,
		Description: workflow.Description,
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
	target := model.Target{}
	targets, err := e.ds.List(ctx, &target, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	env, err := e.envUsecase.GetEnv(ctx, envBinding.Name)
	if err != nil {
		return nil, err
	}
	return &apisv1.DetailEnvBindingResponse{
		EnvBindingBase: *convertEnvbindingModelToBase(app, envBinding, env, targets),
	}, nil
}

func (e *envBindingUsecaseImpl) ApplicationEnvRecycle(ctx context.Context, appModel *model.Application, envBinding *model.EnvBinding) error {
	env, err := e.envUsecase.GetEnv(ctx, envBinding.Name)
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
	}
	return envBinding
}

func convertEnvbindingModelToBase(app *model.Application, envBinding *model.EnvBinding, env *model.Env, targets []datastore.Entity) *apisv1.EnvBindingBase {
	var dtMap = make(map[string]*model.Target, len(targets))
	for _, dte := range targets {
		dt := dte.(*model.Target)
		dtMap[dt.Name] = dt
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
		AppDeployName:      app.Name,
		AppDeployNamespace: env.Namespace,
	}
	return ebb
}

func convertUpdateReqToEnvBindingModel(envBinding *model.EnvBinding, envUpdate apisv1.PutApplicationEnvBindingRequest) *model.EnvBinding {
	return envBinding
}

func convertToEnvBindingModel(app *model.Application, envBind apisv1.EnvBinding) *model.EnvBinding {
	re := model.EnvBinding{
		AppPrimaryKey: app.Name,
		Name:          envBind.Name,
	}
	return &re
}

func (e *envBindingUsecaseImpl) GetSuitableType(ctx context.Context, app *model.Application) string {
	components, err := e.ds.List(ctx, &model.ApplicationComponent{AppPrimaryKey: app.PrimaryKey()}, &datastore.ListOptions{PageSize: 1, Page: 1})
	if err != nil {
		log.Logger.Errorf("list application component list failure %s", err.Error())
	}
	if len(components) > 0 {
		component := components[0].(*model.ApplicationComponent)
		definition, err := e.definitionUsecase.GetComponentDefinition(ctx, component.Type)
		if err != nil {
			log.Logger.Errorf("get component definition %s failure %s", component.Type, err.Error())
		}
		if definition != nil {
			if definition.Spec.Workload.Type == TerraformWorkfloadType {
				return DeployCloudResource
			}
			if definition.Spec.Workload.Definition.Kind == TerraformWorkfloadKind {
				return DeployCloudResource
			}
		}
	}
	return Deploy2Env
}

func (e *envBindingUsecaseImpl) genEnvWorkflowSteps(ctx context.Context, env *model.Env, app *model.Application) []apisv1.WorkflowStep {
	var workflowSteps []v1beta1.WorkflowStep
	for _, targetName := range env.Targets {
		step := v1beta1.WorkflowStep{
			Name: genPolicyEnvName(targetName),
			Type: e.GetSuitableType(ctx, app),
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
				log.Logger.Errorf("workflow %s step %s properties is invalid %s", utils2.Sanitize(app.Name), utils2.Sanitize(step.Name), err.Error())
				continue
			}
			propertyStr = properties.JSON()
		}
		steps = append(steps, apisv1.WorkflowStep{
			Name:        step.Name,
			Type:        step.Type,
			Alias:       fmt.Sprintf("Deploy To %s", step.Name),
			Description: fmt.Sprintf("deploy app to delivery target %s", step.Name),
			DependsOn:   step.DependsOn,
			Properties:  propertyStr,
			Inputs:      step.Inputs,
			Outputs:     step.Outputs,
		})
	}
	return steps
}

func convertWorkflowName(envName string) string {
	return fmt.Sprintf("workflow-%s", envName)
}

func compareSlices(a []string, b []string) ([]string, []string, []string) {
	m := make(map[string]uint8)
	for _, k := range a {
		m[k] |= 1 << 0
	}
	for _, k := range b {
		m[k] |= 1 << 1
	}

	var inAAndB, inAButNotB, inBButNotA []string
	for k, v := range m {
		a := v&(1<<0) != 0
		b := v&(1<<1) != 0
		switch {
		case a && b:
			inAAndB = append(inAAndB, k)
		case a && !b:
			inAButNotB = append(inAButNotB, k)
		case !a && b:
			inBButNotA = append(inBButNotA, k)
		}
	}
	return inAAndB, inAButNotB, inBButNotA
}

func isEnvStepType(stepType string) bool {
	return stepType == Deploy2Env || stepType == DeployCloudResource
}
