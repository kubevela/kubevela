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
	"encoding/json"
	"errors"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// PolicyType build-in policy type
type PolicyType string

const (
	// EnvBindPolicy Multiple environment distribution policy
	EnvBindPolicy PolicyType = "env-binding"
)

// ApplicationUsecase application usecase
type ApplicationUsecase interface {
	ListApplications(ctx context.Context, listOptions apisv1.ListApplicatioPlanOptions) ([]*apisv1.ApplicationPlanBase, error)
	GetApplication(ctx context.Context, appName string) (*model.ApplicationPlan, error)
	DetailApplication(ctx context.Context, app *model.ApplicationPlan) (*apisv1.DetailApplicationPlanResponse, error)
	PublishApplicationTemplate(ctx context.Context, app *model.ApplicationPlan) (*apisv1.ApplicationTemplateBase, error)
	CreateApplication(context.Context, apisv1.CreateApplicationPlanRequest) (*apisv1.ApplicationPlanBase, error)
	DeleteApplication(ctx context.Context, app *model.ApplicationPlan) error
	Deploy(ctx context.Context, app *model.ApplicationPlan, req apisv1.ApplicationDeployRequest) (*apisv1.ApplicationDeployResponse, error)
	ListComponents(ctx context.Context, app *model.ApplicationPlan) ([]*apisv1.ComponentPlanBase, error)
	AddComponent(ctx context.Context, app *model.ApplicationPlan, com apisv1.CreateComponentPlanRequest) (*apisv1.ComponentPlanBase, error)
	DetailComponent(ctx context.Context, app *model.ApplicationPlan, componentName string) (*apisv1.DetailComponentPlanResponse, error)
	DeleteComponent(ctx context.Context, app *model.ApplicationPlan, componentName string) error
	ListPolicies(ctx context.Context, app *model.ApplicationPlan) ([]*apisv1.PolicyBase, error)
	AddPolicy(ctx context.Context, app *model.ApplicationPlan, policy apisv1.CreatePolicyRequest) (*apisv1.PolicyBase, error)
	DetailPolicy(ctx context.Context, app *model.ApplicationPlan, policyName string) (*apisv1.DetailPolicyResponse, error)
	DeletePolicy(ctx context.Context, app *model.ApplicationPlan, policyName string) error
	UpdatePolicy(ctx context.Context, app *model.ApplicationPlan, policyName string, policy apisv1.UpdatePolicyRequest) (*apisv1.DetailPolicyResponse, error)
}

type applicationUsecaseImpl struct {
	ds              datastore.DataStore
	kubeClient      client.Client
	apply           apply.Applicator
	workflowUsecase WorkflowUsecase
}

// NewApplicationUsecase new application usecase
func NewApplicationUsecase(ds datastore.DataStore, workflowUsecase WorkflowUsecase) ApplicationUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &applicationUsecaseImpl{
		ds:              ds,
		workflowUsecase: workflowUsecase,
		kubeClient:      kubecli,
		apply:           apply.NewAPIApplicator(kubecli),
	}
}

// ListApplications list applications
func (c *applicationUsecaseImpl) ListApplications(ctx context.Context, listOptions apisv1.ListApplicatioPlanOptions) ([]*apisv1.ApplicationPlanBase, error) {
	var app = model.ApplicationPlan{}
	if listOptions.Namespace != "" {
		app.Namespace = listOptions.Namespace
	}
	entitys, err := c.ds.List(ctx, &app, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apisv1.ApplicationPlanBase
	for _, entity := range entitys {
		appBase := c.converAppModelToBase(ctx, entity.(*model.ApplicationPlan))
		if listOptions.Query != "" &&
			!(strings.Contains(appBase.Alias, listOptions.Query) ||
				strings.Contains(appBase.Name, listOptions.Query) ||
				strings.Contains(appBase.Description, listOptions.Query)) {
			continue
		}
		if listOptions.Cluster != "" && !appBase.EnvBind.ContainCluster(listOptions.Cluster) {
			continue
		}
		list = append(list, appBase)
	}
	return list, nil
}

// GetApplication get application model
func (c *applicationUsecaseImpl) GetApplication(ctx context.Context, appName string) (*model.ApplicationPlan, error) {
	var app = model.ApplicationPlan{
		Name: appName,
	}
	if err := c.ds.Get(ctx, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// DetailApplication detail application info
func (c *applicationUsecaseImpl) DetailApplication(ctx context.Context, app *model.ApplicationPlan) (*apisv1.DetailApplicationPlanResponse, error) {
	base := c.converAppModelToBase(ctx, app)
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
	var detail = &apisv1.DetailApplicationPlanResponse{
		ApplicationPlanBase: *base,
		Policies:            policyNames,
		ResourceInfo: apisv1.ApplicationResourceInfo{
			ComponentNum: len(components),
		},
		WorkflowStatus: []apisv1.WorkflowStepStatus{},
	}
	return detail, nil
}

// PublishApplicationTemplate publish app template
func (c *applicationUsecaseImpl) PublishApplicationTemplate(ctx context.Context, app *model.ApplicationPlan) (*apisv1.ApplicationTemplateBase, error) {
	//TODO:
	return nil, nil
}

// CreateApplication create application
func (c *applicationUsecaseImpl) CreateApplication(ctx context.Context, req apisv1.CreateApplicationPlanRequest) (*apisv1.ApplicationPlanBase, error) {
	application := model.ApplicationPlan{
		Name:        req.Name,
		Alias:       req.Alias,
		Description: req.Description,
		Namespace:   req.Namespace,
		Icon:        req.Icon,
		Labels:      req.Labels,
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
			_, err := c.workflowUsecase.CreateWorkflow(ctx, &application, apisv1.CreateWorkflowPlanRequest{
				AppName:     application.PrimaryKey(),
				Name:        application.Name,
				Description: "Created automatically.",
				Steps:       steps,
				Enable:      true,
				Default:     true,
			})
			if err != nil {
				return nil, err
			}
		}
		// you can deploy only if the application contains components
		canDeploy = len(oamApp.Spec.Components) > 0
	}

	// build-in create env binding policy
	if len(req.EnvBind) > 0 {
		policy := model.ApplicationPolicyPlan{
			AppPrimaryKey: application.PrimaryKey(),
			Name:          "env-binds",
			Description:   "build-in create",
			Type:          string(EnvBindPolicy),
			Creator:       "",
		}
		var envBindingSpec v1alpha1.EnvBindingSpec
		for _, envBind := range req.EnvBind {
			placement := v1alpha1.EnvPlacement{
				ClusterSelector: &common.ClusterSelector{
					Name: envBind.ClusterSelector.Name,
				},
			}
			if envBind.ClusterSelector.Namespace != "" {
				placement.NamespaceSelector = &v1alpha1.NamespaceSelector{
					Name: envBind.ClusterSelector.Namespace,
				}
			}
			envBindingSpec.Envs = append(envBindingSpec.Envs, v1alpha1.EnvConfig{
				Name:      envBind.Name,
				Placement: placement,
			})
		}
		properties, err := model.NewJSONStructByStruct(envBindingSpec)
		if err != nil {
			log.Logger.Errorf("new env binding properties failure,%s", err.Error())
			return nil, bcode.ErrInvalidProperties
		}
		policy.Properties = properties
		if err := c.ds.Add(ctx, &policy); err != nil {
			log.Logger.Errorf("save env binding policy failure,%s", err.Error())
			return nil, err
		}
	}

	// add application to db.
	if err := c.ds.Add(ctx, &application); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrApplicationExist
		}
		return nil, err
	}
	// render app base info.
	base := c.converAppModelToBase(ctx, &application)
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

func (c *applicationUsecaseImpl) saveApplicationComponent(ctx context.Context, app *model.ApplicationPlan, components []common.ApplicationComponent) error {
	var componentModels []datastore.Entity
	for _, component := range components {
		// TODO: Check whether the component type is supported.
		var traits []model.ApplicationTraitPlan
		for _, trait := range component.Traits {
			properties, err := model.NewJSONStruct(trait.Properties)
			if err != nil {
				log.Logger.Errorf("parse trait properties failire %w", err)
				return bcode.ErrInvalidProperties
			}
			traits = append(traits, model.ApplicationTraitPlan{
				Type:       trait.Type,
				Properties: properties,
			})
		}
		properties, err := model.NewJSONStruct(component.Properties)
		if err != nil {
			log.Logger.Errorf("parse component properties failire %w", err)
			return bcode.ErrInvalidProperties
		}
		componentModel := model.ApplicationComponentPlan{
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
	log.Logger.Infof("batch add %d components for app %s", len(componentModels), app.PrimaryKey())
	return c.ds.BatchAdd(ctx, componentModels)
}

func (c *applicationUsecaseImpl) ListComponents(ctx context.Context, app *model.ApplicationPlan) ([]*apisv1.ComponentPlanBase, error) {
	var component = model.ApplicationComponentPlan{
		AppPrimaryKey: app.PrimaryKey(),
	}
	components, err := c.ds.List(ctx, &component, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apisv1.ComponentPlanBase
	for _, component := range components {
		log.Logger.Infof("component name %s", component.PrimaryKey())
		pm := component.(*model.ApplicationComponentPlan)
		list = append(list, c.converComponentModelToBase(pm))
	}
	return list, nil
}

// DetailComponent detail app component
// TODO: Add status data about the component.
func (c *applicationUsecaseImpl) DetailComponent(ctx context.Context, app *model.ApplicationPlan, policyName string) (*apisv1.DetailComponentPlanResponse, error) {
	var component = model.ApplicationComponentPlan{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          policyName,
	}
	err := c.ds.Get(ctx, &component)
	if err != nil {
		return nil, err
	}
	return &apisv1.DetailComponentPlanResponse{
		ApplicationComponentPlan: component,
	}, nil
}

func (c *applicationUsecaseImpl) converComponentModelToBase(m *model.ApplicationComponentPlan) *apisv1.ComponentPlanBase {
	return &apisv1.ComponentPlanBase{
		Name:          m.Name,
		Alias:         m.Alias,
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
func (c *applicationUsecaseImpl) ListPolicies(ctx context.Context, app *model.ApplicationPlan) ([]*apisv1.PolicyBase, error) {
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

func (c *applicationUsecaseImpl) converPolicyModelToBase(policy *model.ApplicationPolicyPlan) *apisv1.PolicyBase {
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

func (c *applicationUsecaseImpl) saveApplicationPolicy(ctx context.Context, app *model.ApplicationPlan, policys []v1beta1.AppPolicy) error {
	var policyModels []datastore.Entity
	for _, policy := range policys {
		properties, err := model.NewJSONStruct(policy.Properties)
		if err != nil {
			log.Logger.Errorf("parse trait properties failire %w", err)
			return bcode.ErrInvalidProperties
		}
		policyModels = append(policyModels, &model.ApplicationPolicyPlan{
			AppPrimaryKey: app.PrimaryKey(),
			Name:          policy.Name,
			Type:          policy.Type,
			Properties:    properties,
		})
	}
	return c.ds.BatchAdd(ctx, policyModels)
}

func (c *applicationUsecaseImpl) queryApplicationPolicys(ctx context.Context, app *model.ApplicationPlan) (list []*model.ApplicationPolicyPlan, err error) {
	var policy = model.ApplicationPolicyPlan{
		AppPrimaryKey: app.PrimaryKey(),
	}
	policys, err := c.ds.List(ctx, &policy, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, policy := range policys {
		pm := policy.(*model.ApplicationPolicyPlan)
		list = append(list, pm)
	}
	return
}

// DetailPolicy detail app policy
// TODO: Add status data about the policy.
func (c *applicationUsecaseImpl) DetailPolicy(ctx context.Context, app *model.ApplicationPlan, policyName string) (*apisv1.DetailPolicyResponse, error) {
	var policy = model.ApplicationPolicyPlan{
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
func (c *applicationUsecaseImpl) Deploy(ctx context.Context, app *model.ApplicationPlan, req apisv1.ApplicationDeployRequest) (*apisv1.ApplicationDeployResponse, error) {
	// step1: Render oam application
	version := utils.GenerateVersion("")
	oamApp, err := c.renderOAMApplication(ctx, app, req.WorkflowName, version)
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
		DeployUser:   "",
		Commit:       req.Commit,
		SourceType:   req.SourceType,
		WorkflowName: oamApp.Annotations[oam.AnnotationWorkflowName],
	}

	if err := c.ds.Add(ctx, deployEvent); err != nil {
		return nil, err
	}
	// step3: check and create namespace
	var namespace corev1.Namespace
	if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: oamApp.Namespace}, &namespace); apierrors.IsNotFound(err) {
		namespace.Name = oamApp.Namespace
		if err := c.kubeClient.Create(ctx, &namespace); err != nil {
			log.Logger.Errorf("auto create namespace failure %s", err.Error())
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

func (c *applicationUsecaseImpl) renderOAMApplication(ctx context.Context, appMoel *model.ApplicationPlan, reqWorkflowName, version string) (*v1beta1.Application, error) {
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
				oam.AnnotationDeployVersion: version,
			},
		},
	}
	var component = model.ApplicationComponentPlan{
		AppPrimaryKey: appMoel.PrimaryKey(),
	}
	components, err := c.ds.List(ctx, &component, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if err != nil || len(components) == 0 {
		return nil, bcode.ErrNoComponent
	}

	var policy = model.ApplicationPolicyPlan{
		AppPrimaryKey: appMoel.PrimaryKey(),
	}
	policies, err := c.ds.List(ctx, &policy, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, entity := range components {
		component := entity.(*model.ApplicationComponentPlan)
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
		policy := entity.(*model.ApplicationPolicyPlan)
		apolicy := v1beta1.AppPolicy{
			Name: policy.Name,
			Type: policy.Type,
		}
		if policy.Properties != nil {
			apolicy.Properties = policy.Properties.RawExtension()
		}
		app.Spec.Policies = append(app.Spec.Policies, apolicy)
	}

	// Priority 1 uses the requested workflow as release plan.
	// Priority 2 uses the default workflow as release plan.
	var workflow *model.WorkflowPlan
	if reqWorkflowName != "" {
		workflow, err = c.workflowUsecase.GetWorkflow(ctx, reqWorkflowName)
		if err != nil {
			return nil, err
		}
	} else {
		workflow, err = c.workflowUsecase.GetApplicationDefaultWorkflow(ctx, appMoel)
		if err != nil && !errors.Is(err, bcode.ErrWorkflowNoDefault) {
			return nil, err
		}
	}

	if workflow != nil {
		app.Annotations[oam.AnnotationWorkflowName] = workflow.Name
		var steps []v1beta1.WorkflowStep
		for _, step := range workflow.Steps {
			var wstep = v1beta1.WorkflowStep{
				Name:    step.Name,
				Type:    step.Type,
				Inputs:  step.Inputs,
				Outputs: step.Outputs,
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

func (c *applicationUsecaseImpl) converAppModelToBase(ctx context.Context, app *model.ApplicationPlan) *apisv1.ApplicationPlanBase {
	appBeas := &apisv1.ApplicationPlanBase{
		Name:        app.Name,
		Alias:       app.Alias,
		Namespace:   app.Namespace,
		CreateTime:  app.CreateTime,
		UpdateTime:  app.UpdateTime,
		Description: app.Description,
		Icon:        app.Icon,
		Labels:      app.Labels,
	}
	var policy = model.ApplicationPolicyPlan{
		AppPrimaryKey: app.PrimaryKey(),
		Type:          string(EnvBindPolicy),
	}
	policys, err := c.ds.List(ctx, &policy, &datastore.ListOptions{})
	if err != nil {
		log.Logger.Errorf("query application env binding policy failure %s", err.Error())
	}
	for _, policyEntity := range policys {
		policy := policyEntity.(*model.ApplicationPolicyPlan)
		if policy.Properties != nil {
			var envBindingSpec v1alpha1.EnvBindingSpec
			if err := json.Unmarshal([]byte(policy.Properties.JSON()), &envBindingSpec); err != nil {
				log.Logger.Errorf("unmarshal env binding policy failure %s", err.Error())
				continue
			}
			for _, env := range envBindingSpec.Envs {
				envBind := &apisv1.EnvBind{
					Name:        env.Name,
					Description: "",
				}
				if env.Placement.ClusterSelector != nil {
					envBind.ClusterSelector = &apisv1.ClusterSelector{
						Name: env.Placement.ClusterSelector.Name,
					}
				}
				if env.Placement.NamespaceSelector != nil && envBind.ClusterSelector != nil {
					envBind.ClusterSelector.Namespace = env.Placement.NamespaceSelector.Name
				}
				appBeas.EnvBind = append(appBeas.EnvBind, envBind)
			}
		}
	}
	// TODO: get and render app status
	return appBeas
}

// DeleteApplication delete application
func (c *applicationUsecaseImpl) DeleteApplication(ctx context.Context, app *model.ApplicationPlan) error {
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
		err := c.ds.Delete(ctx, &model.ApplicationComponentPlan{AppPrimaryKey: app.PrimaryKey(), Name: component.Name})
		if err != nil && !errors.Is(err, datastore.ErrRecordNotExist) {
			log.Logger.Errorf("delete component %s in app %s failure %s", component.Name, app.Name, err.Error())
		}
	}

	for _, policy := range policies {
		err := c.ds.Delete(ctx, &model.ApplicationPolicyPlan{AppPrimaryKey: app.PrimaryKey(), Name: policy.Name})
		if err != nil && errors.Is(err, datastore.ErrRecordNotExist) {
			log.Logger.Errorf("delete policy %s in app %s failure %s", policy.Name, app.Name, err.Error())
		}
	}

	return c.ds.Delete(ctx, app)
}

func (c *applicationUsecaseImpl) AddComponent(ctx context.Context, app *model.ApplicationPlan, com apisv1.CreateComponentPlanRequest) (*apisv1.ComponentPlanBase, error) {
	componentModel := model.ApplicationComponentPlan{
		AppPrimaryKey: app.PrimaryKey(),
		Description:   com.Description,
		Labels:        com.Labels,
		Icon:          com.Icon,
		// TODO: Get user information from ctx and assign a value.
		Creator:   "",
		Name:      com.Name,
		Type:      com.ComponentType,
		DependsOn: com.DependsOn,
		Alias:     com.Alias,
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
	return &apisv1.ComponentPlanBase{
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

func (c *applicationUsecaseImpl) DeleteComponent(ctx context.Context, app *model.ApplicationPlan, componentName string) error {
	var component = model.ApplicationComponentPlan{
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

func (c *applicationUsecaseImpl) AddPolicy(ctx context.Context, app *model.ApplicationPlan, createpolicy apisv1.CreatePolicyRequest) (*apisv1.PolicyBase, error) {
	policyModel := model.ApplicationPolicyPlan{
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

func (c *applicationUsecaseImpl) DeletePolicy(ctx context.Context, app *model.ApplicationPlan, policyName string) error {
	var policy = model.ApplicationPolicyPlan{
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

func (c *applicationUsecaseImpl) UpdatePolicy(ctx context.Context, app *model.ApplicationPlan, policyName string, policyUpdate apisv1.UpdatePolicyRequest) (*apisv1.DetailPolicyResponse, error) {
	var policy = model.ApplicationPolicyPlan{
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
