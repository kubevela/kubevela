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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// ApplicationUsecase application usecase
type ApplicationUsecase interface {
	ListApplications(ctx context.Context) ([]*apisv1.ApplicationBase, error)
	GetApplication(ctx context.Context, appName string) (*model.Application, error)
	DetailApplication(ctx context.Context, app *model.Application) (*apisv1.DetailApplicationResponse, error)
	PublishApplicationTemplate(ctx context.Context, app *model.Application) (*apisv1.ApplicationTemplateBase, error)
	CreateApplication(context.Context, apisv1.CreateApplicationRequest) (*apisv1.ApplicationBase, error)
	DeleteApplication(ctx context.Context, app *model.Application) error
	Deploy(ctx context.Context, app *model.Application, req apisv1.ApplicationDeployRequest) (*apisv1.ApplicationDeployResponse, error)
	ListComponents(ctx context.Context, app *model.Application) ([]*apisv1.ComponentBase, error)
	AddComponent(ctx context.Context, app *model.Application, com apisv1.CreateComponentRequest) (*apisv1.ComponentBase, error)
	DetailComponent(ctx context.Context, app *model.Application, componentName string) (*apisv1.DetailComponentResponse, error)
	DeleteComponent(ctx context.Context, app *model.Application, componentName string) error
	ListPolicies(ctx context.Context, app *model.Application) ([]*apisv1.PolicyBase, error)
	AddPolicy(ctx context.Context, app *model.Application, policy apisv1.CreatePolicyRequest) (*apisv1.PolicyBase, error)
	DetailPolicy(ctx context.Context, app *model.Application, policyName string) (*apisv1.DetailPolicyResponse, error)
	DeletePolicy(ctx context.Context, app *model.Application, policyName string) error
	UpdatePolicy(ctx context.Context, app *model.Application, policyName string, policy apisv1.UpdatePolicyRequest) (*apisv1.DetailPolicyResponse, error)
}

type applicationUsecaseImpl struct {
	ds              datastore.DataStore
	kubeClient      client.Client
	apply           apply.Applicator
	workflowUsecase WorkflowUsecase
}

// NewApplicationUsecase new application usecase
func NewApplicationUsecase(ds datastore.DataStore, workflowUsecase WorkflowUsecase) ApplicationUsecase {
	kubecli, _ := clients.GetKubeClient()
	return &applicationUsecaseImpl{
		ds:              ds,
		workflowUsecase: workflowUsecase,
		kubeClient:      kubecli,
		apply:           apply.NewAPIApplicator(kubecli),
	}
}

// ListApplications list applications
func (c *applicationUsecaseImpl) ListApplications(ctx context.Context) ([]*apisv1.ApplicationBase, error) {
	var app = model.Application{}
	entitys, err := c.ds.List(ctx, &app, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apisv1.ApplicationBase
	for _, entity := range entitys {
		list = append(list, c.converAppModelToBase(entity.(*model.Application)))
	}
	return list, nil
}

// GetApplication get application model
func (c *applicationUsecaseImpl) GetApplication(ctx context.Context, appName string) (*model.Application, error) {
	var app = model.Application{
		Name: appName,
	}
	if err := c.ds.Get(ctx, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// DetailApplication detail application info
func (c *applicationUsecaseImpl) DetailApplication(ctx context.Context, app *model.Application) (*apisv1.DetailApplicationResponse, error) {
	base := c.converAppModelToBase(app)
	policys, err := c.queryApplicationPolicys(ctx, app)
	if err != nil {
		return nil, err
	}
	components, err := c.ListComponents(ctx, app)
	if err != nil {
		return nil, err
	}
	var policyNames []string
	for _, p := range policys {
		policyNames = append(policyNames, p.Name)
	}
	var detail = &apisv1.DetailApplicationResponse{
		ApplicationBase: *base,
		Policies:        policyNames,
		ResourceInfo: apisv1.ApplicationResourceInfo{
			ComponentNum: len(components),
		},
		WorkflowStatus: []apisv1.WorkflowStepStatus{},
	}
	return detail, nil
}

// PublishApplicationTemplate publish app template
func (c *applicationUsecaseImpl) PublishApplicationTemplate(ctx context.Context, app *model.Application) (*apisv1.ApplicationTemplateBase, error) {
	//TODO:
	return nil, nil
}

// CreateApplication create application
func (c *applicationUsecaseImpl) CreateApplication(ctx context.Context, req apisv1.CreateApplicationRequest) (*apisv1.ApplicationBase, error) {
	application := model.Application{
		Name:        req.Name,
		Description: req.Description,
		Namespace:   req.Namespace,
		Icon:        req.Icon,
		Labels:      req.Labels,
		ClusterList: req.ClusterList,
	}
	// check app name.
	exit, err := c.ds.IsExist(ctx, &application)
	if err != nil {
		log.Logger.Errorf("check application name is exist failure %s", err.Error())
		return nil, bcode.ErrApplicationExist
	}
	if exit {
		return nil, bcode.ErrApplicationExist
	}
	// check can deploy
	var canDeploy bool
	if req.YamlConfig != "" {
		var oamApp v1beta1.Application
		if err := yaml.Unmarshal([]byte(req.YamlConfig), &oamApp); err != nil {
			log.Logger.Errorf("application yaml config is invalid,%s", err.Error())
			return nil, bcode.ErrApplicationConfig
		}

		// split the configuration and store it in the database.
		if err := c.saveApplicationComponent(ctx, &application, oamApp.Spec.Components); err != nil {
			log.Logger.Errorf("save applictaion component failure,%s", err.Error())
			return nil, err
		}
		if len(oamApp.Spec.Policies) > 0 {
			if err := c.saveApplicationPolicy(ctx, &application, oamApp.Spec.Policies); err != nil {
				log.Logger.Errorf("save applictaion polocies failure,%s", err.Error())
				return nil, err
			}
		}
		if oamApp.Spec.Workflow != nil && len(oamApp.Spec.Workflow.Steps) > 0 {
			var steps []apisv1.WorkflowStep
			for _, step := range oamApp.Spec.Workflow.Steps {
				var propertyStr string
				if step.Properties != nil {
					properties, err := model.NewJSONStruct(step.Properties)
					if err != nil {
						log.Logger.Errorf("workflow %s step %s properties is invalid %s", application.Name, step.Name, err.Error())
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
			_, err := c.workflowUsecase.CreateOrUpdateWorkflow(ctx, apisv1.UpdateWorkflowRequest{
				Name:      application.Name,
				Namespace: application.Namespace,
				Steps:     steps,
				Enable:    true,
			})
			if err != nil {
				return nil, err
			}
		}
		// you can deploy only if the application contains components
		canDeploy = len(oamApp.Spec.Components) > 0
	}

	// add application to db.
	if err := c.ds.Add(ctx, &application); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrApplicationExist
		}
		return nil, err
	}
	// render app base info.
	base := c.converAppModelToBase(&application)
	// deploy to cluster if need.
	if req.Deploy && canDeploy {
		if _, err := c.Deploy(ctx, &application, apisv1.ApplicationDeployRequest{
			Commit:     "init create auto deploy",
			SourceType: "web",
		}); err != nil {
			return nil, err
		}
	}
	return base, nil
}

func (c *applicationUsecaseImpl) saveApplicationComponent(ctx context.Context, app *model.Application, components []common.ApplicationComponent) error {
	var componentModels []datastore.Entity
	for _, component := range components {
		// TODO: Check whether the component type is supported.
		var traits []model.ApplicationTrait
		for _, trait := range component.Traits {
			properties, err := model.NewJSONStruct(trait.Properties)
			if err != nil {
				log.Logger.Errorf("parse trait properties failire %w", err)
				return bcode.ErrInvalidProperties
			}
			traits = append(traits, model.ApplicationTrait{
				Type:       trait.Type,
				Properties: properties,
			})
		}
		properties, err := model.NewJSONStruct(component.Properties)
		if err != nil {
			log.Logger.Errorf("parse component properties failire %w", err)
			return bcode.ErrInvalidProperties
		}
		componentModel := model.ApplicationComponent{
			AppPrimaryKey:    app.PrimaryKey(),
			Name:             component.Name,
			Type:             component.Type,
			ExternalRevision: component.ExternalRevision,
			DependsOn:        component.DependsOn,
			Inputs:           component.Inputs,
			Outputs:          component.Outputs,
			Scopes:           component.Scopes,
			Traits:           traits,
			Properties:       properties,
		}
		componentModels = append(componentModels, &componentModel)
	}
	return c.ds.BatchAdd(ctx, componentModels)
}

func (c *applicationUsecaseImpl) ListComponents(ctx context.Context, app *model.Application) ([]*apisv1.ComponentBase, error) {
	var component = model.ApplicationComponent{
		AppPrimaryKey: app.PrimaryKey(),
	}
	components, err := c.ds.List(ctx, &component, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apisv1.ComponentBase
	for _, component := range components {
		pm := component.(*model.ApplicationComponent)
		list = append(list, c.converComponentModelToBase(pm))
	}
	return list, nil
}

// DetailComponent detail app component
// TODO: Add status data about the component.
func (c *applicationUsecaseImpl) DetailComponent(ctx context.Context, app *model.Application, policyName string) (*apisv1.DetailComponentResponse, error) {
	var component = model.ApplicationComponent{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          policyName,
	}
	err := c.ds.Get(ctx, &component)
	if err != nil {
		return nil, err
	}
	return &apisv1.DetailComponentResponse{
		ApplicationComponent: component,
	}, nil
}

func (c *applicationUsecaseImpl) converComponentModelToBase(m *model.ApplicationComponent) *apisv1.ComponentBase {
	return &apisv1.ComponentBase{
		Name:          m.Name,
		Description:   m.Description,
		Labels:        m.Labels,
		ComponentType: m.Type,
		Icon:          m.Icon,
		DependsOn:     m.DependsOn,
		Creator:       m.Creator,
		CreateTime:    m.CreateTime,
		UpdateTime:    m.UpdateTime,
	}
}

// ListPolicies list application policies
func (c *applicationUsecaseImpl) ListPolicies(ctx context.Context, app *model.Application) ([]*apisv1.PolicyBase, error) {
	policies, err := c.queryApplicationPolicys(ctx, app)
	if err != nil {
		return nil, err
	}
	var list []*apisv1.PolicyBase
	for _, policy := range policies {
		list = append(list, c.converPolicyModelToBase(policy))
	}
	return list, nil
}

func (c *applicationUsecaseImpl) converPolicyModelToBase(policy *model.ApplicationPolicy) *apisv1.PolicyBase {
	pb := &apisv1.PolicyBase{
		Name:        policy.Name,
		Type:        policy.Type,
		Properties:  policy.Properties,
		Description: policy.Description,
		Creator:     policy.Creator,
		CreateTime:  policy.CreateTime,
		UpdateTime:  policy.UpdateTime,
	}
	return pb
}

func (c *applicationUsecaseImpl) saveApplicationPolicy(ctx context.Context, app *model.Application, policys []v1beta1.AppPolicy) error {
	var policyModels []datastore.Entity
	for _, policy := range policys {
		properties, err := model.NewJSONStruct(policy.Properties)
		if err != nil {
			log.Logger.Errorf("parse trait properties failire %w", err)
			return bcode.ErrInvalidProperties
		}
		policyModels = append(policyModels, &model.ApplicationPolicy{
			AppPrimaryKey: app.PrimaryKey(),
			Name:          policy.Name,
			Type:          policy.Type,
			Properties:    properties,
		})
	}
	return c.ds.BatchAdd(ctx, policyModels)
}

func (c *applicationUsecaseImpl) queryApplicationPolicys(ctx context.Context, app *model.Application) (list []*model.ApplicationPolicy, err error) {
	var policy = model.ApplicationPolicy{
		AppPrimaryKey: app.PrimaryKey(),
	}
	policys, err := c.ds.List(ctx, &policy, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, policy := range policys {
		pm := policy.(*model.ApplicationPolicy)
		list = append(list, pm)
	}
	return
}

// DetailPolicy detail app policy
// TODO: Add status data about the policy.
func (c *applicationUsecaseImpl) DetailPolicy(ctx context.Context, app *model.Application, policyName string) (*apisv1.DetailPolicyResponse, error) {
	var policy = model.ApplicationPolicy{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          policyName,
	}
	err := c.ds.Get(ctx, &policy)
	if err != nil {
		return nil, err
	}
	return &apisv1.DetailPolicyResponse{
		PolicyBase: *c.converPolicyModelToBase(&policy),
	}, nil
}

// Deploy deploy app to cluster
// means to render oam application config and apply to cluster.
// An event record is generated for each deploy.
func (c *applicationUsecaseImpl) Deploy(ctx context.Context, app *model.Application, req apisv1.ApplicationDeployRequest) (*apisv1.ApplicationDeployResponse, error) {
	// step1: Render oam application
	version := utils.GenerateVersion("")
	oamApp, err := c.renderOAMApplication(ctx, app, version)
	if err != nil {
		return nil, err
	}
	configByte, _ := yaml.Marshal(oamApp)
	// step2: check and create deploy event
	if !req.Force {
		var lastEvent = model.DeployEvent{
			AppPrimaryKey: app.PrimaryKey(),
		}
		list, err := c.ds.List(ctx, &lastEvent, &datastore.ListOptions{PageSize: 1, Page: 1})
		if err != nil && !errors.Is(err, datastore.ErrRecordNotExist) {
			log.Logger.Errorf("query last app event failure %s", err.Error())
			return nil, bcode.ErrDeployEventConflict
		}
		if len(list) > 0 && list[0].(*model.DeployEvent).Status != model.DeployEventComplete {
			return nil, bcode.ErrDeployEventConflict
		}
	}

	var deployEvent = &model.DeployEvent{
		AppPrimaryKey:  app.PrimaryKey(),
		Version:        version,
		ApplyAppConfig: string(configByte),
		Status:         model.DeployEventInit,
		// TODO: Get user information from ctx and assign a value.
		DeployUser: "",
		Commit:     req.Commit,
		SourceType: req.SourceType,
	}

	if err := c.ds.Add(ctx, deployEvent); err != nil {
		return nil, err
	}
	// step3: check and create namespace
	var namespace corev1.Namespace
	if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: oamApp.Namespace}, &namespace); apierrors.IsNotFound(err) {
		namespace.Name = oamApp.Namespace
		if err := c.kubeClient.Create(ctx, &namespace); err != nil {
			log.Logger.Errorf("auto create namesapce failure %s", err.Error())
			return nil, bcode.ErrCreateNamespace
		}
	}
	// step4: apply to controller cluster
	err = c.apply.Apply(ctx, oamApp)
	if err != nil {
		deployEvent.Status = model.DeployEventFail
		deployEvent.Reason = err.Error()
		if err := c.ds.Put(ctx, deployEvent); err != nil {
			log.Logger.Warnf("update deploy event failure %s", err.Error())
		}
		log.Logger.Errorf("deploy app %s failure %s", app.PrimaryKey(), err.Error())
		return nil, bcode.ErrDeployApplyFail
	}
	deployEvent.Status = model.DeployEventRunning
	if err := c.ds.Put(ctx, deployEvent); err != nil {
		log.Logger.Warnf("update deploy event failure %s", err.Error())
	}

	// step5: update deploy event status
	return &apisv1.ApplicationDeployResponse{
		Version:    deployEvent.Version,
		Status:     deployEvent.Status,
		Reason:     deployEvent.Reason,
		DeployUser: deployEvent.DeployUser,
		Commit:     deployEvent.Commit,
		SourceType: deployEvent.SourceType,
	}, nil
}

func (c *applicationUsecaseImpl) renderOAMApplication(ctx context.Context, appMoel *model.Application, version string) (*v1beta1.Application, error) {
	var app = &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      appMoel.Name,
			Namespace: appMoel.Namespace,
			Labels:    appMoel.Labels,
			Annotations: map[string]string{
				"deploy_version": version,
			},
		},
	}
	var component = model.ApplicationComponent{
		AppPrimaryKey: appMoel.PrimaryKey(),
	}
	components, err := c.ds.List(ctx, &component, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if err != nil || len(components) == 0 {
		return nil, bcode.ErrNoComponent
	}

	var policy = model.ApplicationPolicy{
		AppPrimaryKey: appMoel.PrimaryKey(),
	}
	policies, err := c.ds.List(ctx, &policy, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, entity := range components {
		component := entity.(*model.ApplicationComponent)
		var traits []common.ApplicationTrait
		for _, trait := range component.Traits {
			aTrait := common.ApplicationTrait{
				Type: trait.Type,
			}
			if trait.Properties != nil {
				aTrait.Properties = trait.Properties.RawExtension()
			}
			traits = append(traits, aTrait)
		}
		app.Spec.Components = append(app.Spec.Components, common.ApplicationComponent{
			Name:             component.Name,
			Type:             component.Type,
			ExternalRevision: component.ExternalRevision,
			DependsOn:        component.DependsOn,
			Inputs:           component.Inputs,
			Outputs:          component.Outputs,
			Traits:           traits,
			Scopes:           component.Scopes,
		})
	}

	for _, entity := range policies {
		policy := entity.(*model.ApplicationPolicy)
		apolicy := v1beta1.AppPolicy{
			Name: component.Name,
			Type: component.Type,
		}
		if policy.Properties != nil {
			apolicy.Properties = policy.Properties.RawExtension()
		}
		app.Spec.Policies = append(app.Spec.Policies, apolicy)
	}
	workflow, err := c.workflowUsecase.GetWorkflow(ctx, appMoel.Name)
	if err != nil {
		return nil, err
	}
	if workflow != nil {
		var steps []v1beta1.WorkflowStep
		for _, step := range workflow.Steps {
			var wstep = v1beta1.WorkflowStep{
				Name:      step.Name,
				Type:      step.Type,
				DependsOn: step.DependsOn,
				Inputs:    step.Inputs,
				Outputs:   step.Outputs,
			}
			if step.Properties != nil {
				wstep.Properties = step.Properties.RawExtension()
			}
		}
		app.Spec.Workflow = &v1beta1.Workflow{
			Steps: steps,
		}
	}
	return app, nil
}

func (c *applicationUsecaseImpl) converAppModelToBase(app *model.Application) *apisv1.ApplicationBase {
	appBeas := &apisv1.ApplicationBase{
		Name:        app.Name,
		Namespace:   app.Namespace,
		CreateTime:  app.CreateTime,
		UpdateTime:  app.UpdateTime,
		Description: app.Description,
		Icon:        app.Icon,
		Labels:      app.Labels,
	}
	// TODO: get and render app status
	return appBeas
}

// DeleteApplication delete application
func (c *applicationUsecaseImpl) DeleteApplication(ctx context.Context, app *model.Application) error {
	// TODO: check app can be deleted

	// query all components to deleted
	components, err := c.ListComponents(ctx, app)
	if err != nil {
		return err
	}
	// query all policies to deleted
	policies, err := c.ListPolicies(ctx, app)
	if err != nil {
		return err
	}
	// delete workflow
	if err := c.workflowUsecase.DeleteWorkflow(ctx, app.Name); err != nil && !errors.Is(err, bcode.ErrWorkflowNotExist) {
		log.Logger.Errorf("delete workflow %s failure %s", app.Name, err.Error())
	}

	for _, component := range components {
		err := c.ds.Delete(ctx, &model.ApplicationComponent{AppPrimaryKey: app.PrimaryKey(), Name: component.Name})
		if err != nil && errors.Is(err, datastore.ErrRecordNotExist) {
			log.Logger.Errorf("delete component %s in app %s failure %s", component.Name, app.Name, err.Error())
		}
	}

	for _, policy := range policies {
		err := c.ds.Delete(ctx, &model.ApplicationPolicy{AppPrimaryKey: app.PrimaryKey(), Name: policy.Name})
		if err != nil && errors.Is(err, datastore.ErrRecordNotExist) {
			log.Logger.Errorf("delete policy %s in app %s failure %s", policy.Name, app.Name, err.Error())
		}
	}

	return c.ds.Delete(ctx, app)
}

func (c *applicationUsecaseImpl) AddComponent(ctx context.Context, app *model.Application, com apisv1.CreateComponentRequest) (*apisv1.ComponentBase, error) {
	componentModel := model.ApplicationComponent{
		AppPrimaryKey: app.PrimaryKey(),
		Description:   com.Description,
		Labels:        com.Labels,
		Icon:          com.Icon,
		// TODO: Get user information from ctx and assign a value.
		Creator:   "",
		Name:      com.Name,
		Type:      com.ComponentType,
		DependsOn: com.DependsOn,
	}
	properties, err := model.NewJSONStructByString(com.Properties)
	if err != nil {
		return nil, bcode.ErrInvalidProperties
	}
	componentModel.Properties = properties
	if err := c.ds.Add(ctx, &componentModel); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrApplicationComponetExist
		}
		log.Logger.Warnf("add component for app %s failure %s", app.PrimaryKey(), err.Error())
		return nil, err
	}
	return &apisv1.ComponentBase{
		Name:          componentModel.Name,
		Description:   componentModel.Description,
		Labels:        componentModel.Labels,
		ComponentType: componentModel.Type,
		Icon:          componentModel.Icon,
		DependsOn:     componentModel.DependsOn,
		Creator:       componentModel.Creator,
		CreateTime:    componentModel.CreateTime,
		UpdateTime:    componentModel.UpdateTime,
	}, nil
}

func (c *applicationUsecaseImpl) DeleteComponent(ctx context.Context, app *model.Application, componentName string) error {
	var component = model.ApplicationComponent{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          componentName,
	}
	if err := c.ds.Delete(ctx, &component); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrApplicationComponetNotExist
		}
		log.Logger.Warnf("delete app component %s failure %s", app.PrimaryKey(), err.Error())
		return err
	}
	return nil
}

func (c *applicationUsecaseImpl) AddPolicy(ctx context.Context, app *model.Application, createpolicy apisv1.CreatePolicyRequest) (*apisv1.PolicyBase, error) {
	policyModel := model.ApplicationPolicy{
		AppPrimaryKey: app.PrimaryKey(),
		Description:   createpolicy.Description,
		// TODO: Get user information from ctx and assign a value.
		Creator: "",
		Name:    createpolicy.Name,
		Type:    createpolicy.Type,
	}
	properties, err := model.NewJSONStructByString(createpolicy.Properties)
	if err != nil {
		return nil, bcode.ErrInvalidProperties
	}
	policyModel.Properties = properties
	if err := c.ds.Add(ctx, &policyModel); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrApplicationPolicyExist
		}
		log.Logger.Warnf("add policy for app %s failure %s", app.PrimaryKey(), err.Error())
		return nil, err
	}
	return &apisv1.PolicyBase{
		Name:        policyModel.Name,
		Description: policyModel.Description,
		Type:        policyModel.Type,
		Creator:     policyModel.Creator,
		CreateTime:  policyModel.CreateTime,
		UpdateTime:  policyModel.UpdateTime,
		Properties:  policyModel.Properties,
	}, nil
}

func (c *applicationUsecaseImpl) DeletePolicy(ctx context.Context, app *model.Application, policyName string) error {
	var policy = model.ApplicationPolicy{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          policyName,
	}
	if err := c.ds.Delete(ctx, &policy); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrApplicationPolicyNotExist
		}
		log.Logger.Warnf("delete app policy %s failure %s", app.PrimaryKey(), err.Error())
		return err
	}
	return nil
}

func (c *applicationUsecaseImpl) UpdatePolicy(ctx context.Context, app *model.Application, policyName string, policyUpdate apisv1.UpdatePolicyRequest) (*apisv1.DetailPolicyResponse, error) {
	var policy = model.ApplicationPolicy{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          policyName,
	}
	err := c.ds.Get(ctx, &policy)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrApplicationPolicyNotExist
		}
		log.Logger.Warnf("update app policy %s failure %s", app.PrimaryKey(), err.Error())
		return nil, err
	}
	policy.Type = policyUpdate.Type
	properties, err := model.NewJSONStructByString(policyUpdate.Properties)
	if err != nil {
		return nil, bcode.ErrInvalidProperties
	}
	policy.Properties = properties
	policy.Description = policyUpdate.Description

	if err := c.ds.Put(ctx, &policy); err != nil {
		return nil, err
	}
	return &apisv1.DetailPolicyResponse{
		PolicyBase: *c.converPolicyModelToBase(&policy),
	}, nil
}
