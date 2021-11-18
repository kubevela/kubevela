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
	"fmt"
	"sort"
	"strings"
	"time"

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
	// EnvBindingPolicy Multiple environment distribution policy
	EnvBindingPolicy PolicyType = "env-binding"

	// EnvBindingPolicyDefaultName default policy name
	EnvBindingPolicyDefaultName string = "env-bindings"
)

// ApplicationUsecase application usecase
type ApplicationUsecase interface {
	ListApplications(ctx context.Context, listOptions apisv1.ListApplicatioOptions) ([]*apisv1.ApplicationBase, error)
	GetApplication(ctx context.Context, appName string) (*model.Application, error)
	GetApplicationStatus(ctx context.Context, app *model.Application, envName string) (*common.AppStatus, error)
	DetailApplication(ctx context.Context, app *model.Application) (*apisv1.DetailApplicationResponse, error)
	PublishApplicationTemplate(ctx context.Context, app *model.Application) (*apisv1.ApplicationTemplateBase, error)
	CreateApplication(context.Context, apisv1.CreateApplicationRequest) (*apisv1.ApplicationBase, error)
	UpdateApplication(context.Context, *model.Application, apisv1.UpdateApplicationRequest) (*apisv1.ApplicationBase, error)
	DeleteApplication(ctx context.Context, app *model.Application) error
	Deploy(ctx context.Context, app *model.Application, req apisv1.ApplicationDeployRequest) (*apisv1.ApplicationDeployResponse, error)
	ListComponents(ctx context.Context, app *model.Application, op apisv1.ListApplicationComponentOptions) ([]*apisv1.ComponentBase, error)
	AddComponent(ctx context.Context, app *model.Application, com apisv1.CreateComponentRequest) (*apisv1.ComponentBase, error)
	DetailComponent(ctx context.Context, app *model.Application, componentName string) (*apisv1.DetailComponentResponse, error)
	DeleteComponent(ctx context.Context, app *model.Application, componentName string) error
	ListPolicies(ctx context.Context, app *model.Application) ([]*apisv1.PolicyBase, error)
	AddPolicy(ctx context.Context, app *model.Application, policy apisv1.CreatePolicyRequest) (*apisv1.PolicyBase, error)
	DetailPolicy(ctx context.Context, app *model.Application, policyName string) (*apisv1.DetailPolicyResponse, error)
	DeletePolicy(ctx context.Context, app *model.Application, policyName string) error
	UpdatePolicy(ctx context.Context, app *model.Application, policyName string, policy apisv1.UpdatePolicyRequest) (*apisv1.DetailPolicyResponse, error)
	CreateApplicationTrait(ctx context.Context, app *model.Application, component *model.ApplicationComponent, req apisv1.CreateApplicationTraitRequest) (*apisv1.ApplicationTrait, error)
	DeleteApplicationTrait(ctx context.Context, app *model.Application, component *model.ApplicationComponent, traitType string) error
	UpdateApplicationTrait(ctx context.Context, app *model.Application, component *model.ApplicationComponent, traitType string, req apisv1.UpdateApplicationTraitRequest) (*apisv1.ApplicationTrait, error)
	ListRevisions(ctx context.Context, appName, envName, status string, page, pageSize int) (*apisv1.ListRevisionsResponse, error)
	DetailRevision(ctx context.Context, appName, revisionName string) (*apisv1.DetailRevisionResponse, error)
}

type applicationUsecaseImpl struct {
	ds                    datastore.DataStore
	kubeClient            client.Client
	apply                 apply.Applicator
	workflowUsecase       WorkflowUsecase
	envBindingUsecase     EnvBindingUsecase
	deliveryTargetUsecase DeliveryTargetUsecase
}

// NewApplicationUsecase new application usecase
func NewApplicationUsecase(ds datastore.DataStore, workflowUsecase WorkflowUsecase, envBindingUsecase EnvBindingUsecase, deliveryTargetUsecase DeliveryTargetUsecase) ApplicationUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &applicationUsecaseImpl{
		ds:                    ds,
		workflowUsecase:       workflowUsecase,
		envBindingUsecase:     envBindingUsecase,
		deliveryTargetUsecase: deliveryTargetUsecase,
		kubeClient:            kubecli,
		apply:                 apply.NewAPIApplicator(kubecli),
	}
}

// ListApplications list applications
func (c *applicationUsecaseImpl) ListApplications(ctx context.Context, listOptions apisv1.ListApplicatioOptions) ([]*apisv1.ApplicationBase, error) {
	var app = model.Application{}
	if listOptions.Namespace != "" {
		app.Namespace = listOptions.Namespace
	}
	entitys, err := c.ds.List(ctx, &app, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apisv1.ApplicationBase
	for _, entity := range entitys {
		appBase := c.converAppModelToBase(entity.(*model.Application))
		if listOptions.Query != "" &&
			!(strings.Contains(appBase.Alias, listOptions.Query) ||
				strings.Contains(appBase.Name, listOptions.Query) ||
				strings.Contains(appBase.Description, listOptions.Query)) {
			continue
		}
		if listOptions.TargetName != "" {
			targetIsContain, _ := c.envBindingUsecase.CheckAppEnvBindingsContainTarget(ctx, &app, listOptions.TargetName)
			if targetIsContain {
				continue
			}
		}
		list = append(list, appBase)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].UpdateTime.Unix() > list[j].UpdateTime.Unix()
	})
	return list, nil
}

// GetApplication get application model
func (c *applicationUsecaseImpl) GetApplication(ctx context.Context, appName string) (*model.Application, error) {
	var app = model.Application{
		Name: appName,
	}
	if err := c.ds.Get(ctx, &app); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrApplicationNotExist
		}
		return nil, err
	}
	return &app, nil
}

// DetailApplication detail application  info
func (c *applicationUsecaseImpl) DetailApplication(ctx context.Context, app *model.Application) (*apisv1.DetailApplicationResponse, error) {
	base := c.converAppModelToBase(app)
	policys, err := c.queryApplicationPolicys(ctx, app)
	if err != nil {
		return nil, err
	}
	components, err := c.ListComponents(ctx, app, apisv1.ListApplicationComponentOptions{})
	envBindings, err := c.envBindingUsecase.GetEnvBindings(ctx, app)
	if err != nil {
		return nil, err
	}
	var policyNames []string
	var envBindingNames []string
	for _, p := range policys {
		policyNames = append(policyNames, p.Name)
	}
	for _, e := range envBindings {
		envBindingNames = append(envBindingNames, e.Name)
	}
	var detail = &apisv1.DetailApplicationResponse{
		ApplicationBase: *base,
		Policies:        policyNames,
		EnvBindings:     envBindingNames,
		ResourceInfo: apisv1.ApplicationResourceInfo{
			ComponentNum: len(components),
		},
		WorkflowStatus: []apisv1.WorkflowStepStatus{},
	}
	return detail, nil
}

// GetApplicationStatus get application status from controller cluster
func (c *applicationUsecaseImpl) GetApplicationStatus(ctx context.Context, appmodel *model.Application, envName string) (*common.AppStatus, error) {
	var app v1beta1.Application
	err := c.kubeClient.Get(ctx, types.NamespacedName{Namespace: appmodel.Namespace, Name: converAppName(appmodel, envName)}, &app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &app.Status, nil
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
			if err := SaveApplicationWorkflow(c.workflowUsecase, ctx, &application, oamApp.Spec.Workflow.Steps, application.Name); err != nil {
				log.Logger.Errorf("save applictaion polocies failure,%s", err.Error())
				return nil, err
			}
		}
		// TODO Waiting for Spec.EnvBinding support
	}

	// build-in create env binding
	if len(req.EnvBinding) > 0 {
		err := c.saveApplicationEnvBinding(ctx, application, req.EnvBinding)
		if err != nil {
			return nil, err
		}
	}

	if req.Component != nil {
		_, err = c.AddComponent(ctx, &application, *req.Component)
		if err != nil {
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
	base := c.converAppModelToBase(&application)
	return base, nil
}

func (c *applicationUsecaseImpl) genPolicyByEnv(ctx context.Context, app *model.Application, envName string) (v1beta1.AppPolicy, error) {
	appPolicy := v1beta1.AppPolicy{}
	envBinding, err := c.envBindingUsecase.GetEnvBinding(ctx, app, envName)
	if err != nil {
		return appPolicy, err
	}
	appPolicy.Name = genPolicyName(envBinding.Name)
	appPolicy.Type = string(EnvBindingPolicy)

	var envBindingSpec v1alpha1.EnvBindingSpec
	for _, targetName := range envBinding.TargetNames {
		target, err := c.deliveryTargetUsecase.GetDeliveryTarget(ctx, targetName)
		if err != nil || target == nil {
			return appPolicy, bcode.ErrFoundEnvbindingDeliveryTarget
		}
		envBindingSpec.Envs = append(envBindingSpec.Envs, createTargetClusterEnv(envBinding.EnvBindingBase, target))
	}
	properties, err := model.NewJSONStructByStruct(envBindingSpec)
	appPolicy.Properties = properties.RawExtension()
	return appPolicy, nil
}

func (c *applicationUsecaseImpl) saveApplicationEnvBinding(ctx context.Context, app model.Application, envBindings []*apisv1.EnvBinding) error {
	err := c.envBindingUsecase.BatchCreateEnvBinding(ctx, &app, envBindings)
	if err != nil {
		return err
	}
	return nil
}

func (c *applicationUsecaseImpl) UpdateApplication(ctx context.Context, app *model.Application, req apisv1.UpdateApplicationRequest) (*apisv1.ApplicationBase, error) {
	app.Alias = req.Alias
	app.Description = req.Description
	app.Labels = req.Labels
	app.Icon = req.Icon
	if err := c.ds.Put(ctx, app); err != nil {
		return nil, err
	}
	return c.converAppModelToBase(app), nil
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
	log.Logger.Infof("batch add %d components for app %s", len(componentModels), app.PrimaryKey())
	return c.ds.BatchAdd(ctx, componentModels)
}

func (c *applicationUsecaseImpl) ListComponents(ctx context.Context, app *model.Application, op apisv1.ListApplicationComponentOptions) ([]*apisv1.ComponentBase, error) {
	var component = model.ApplicationComponent{
		AppPrimaryKey: app.PrimaryKey(),
	}
	components, err := c.ds.List(ctx, &component, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	envComponents := map[string]bool{}
	componentSelectorDefine := false
	if op.EnvName != "" {
		envbindings, err := c.envBindingUsecase.GetEnvBindings(ctx, app)
		if err != nil && !errors.Is(err, bcode.ErrApplicationNotEnv) {
			log.Logger.Errorf("query app  env binding policy config failure %s", err.Error())
		}
		if len(envbindings) > 0 {
			for _, env := range envbindings {
				if env != nil && env.Name == op.EnvName {
					componentSelectorDefine = true
					for _, componentName := range env.ComponentSelector.Components {
						envComponents[componentName] = true
					}
				}
			}
		}
	}

	var list []*apisv1.ComponentBase
	for _, component := range components {
		pm := component.(*model.ApplicationComponent)
		if !componentSelectorDefine || envComponents[pm.Name] {
			list = append(list, c.converComponentModelToBase(pm))
		}
	}
	return list, nil
}

// DetailComponent detail app component
// TODO: Add status data about the component.
func (c *applicationUsecaseImpl) DetailComponent(ctx context.Context, app *model.Application, compName string) (*apisv1.DetailComponentResponse, error) {
	var component = model.ApplicationComponent{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          compName,
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
	var envbindingPolicy *model.ApplicationPolicy
	for _, policy := range policys {
		properties, err := model.NewJSONStruct(policy.Properties)
		if err != nil {
			log.Logger.Errorf("parse trait properties failire %w", err)
			return bcode.ErrInvalidProperties
		}
		appPolicy := &model.ApplicationPolicy{
			AppPrimaryKey: app.PrimaryKey(),
			Name:          policy.Name,
			Type:          policy.Type,
			Properties:    properties,
		}
		if policy.Type != string(EnvBindingPolicy) {
			policyModels = append(policyModels, appPolicy)
		} else {
			envbindingPolicy = appPolicy
		}
	}
	// If multiple configurations are configured, enable only the last one.
	if envbindingPolicy != nil {
		envbindingPolicy.Name = EnvBindingPolicyDefaultName
		policyModels = append(policyModels, envbindingPolicy)
		var envBindingSpec v1alpha1.EnvBindingSpec
		if err := json.Unmarshal([]byte(envbindingPolicy.Properties.JSON()), &envBindingSpec); err != nil {
			return fmt.Errorf("unmarshal env binding policy failure %w", err)
		}
		for _, env := range envBindingSpec.Envs {
			envBind := &model.EnvBinding{
				Name:        env.Name,
				Description: "",
			}
			if env.Selector != nil {
				envBind.ComponentSelector = (*model.ComponentSelector)(env.Selector)
			}
		}
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
	oamApp, err := c.renderOAMApplication(ctx, app, req.WorkflowName, version)
	if err != nil {
		return nil, err
	}
	configByte, _ := yaml.Marshal(oamApp)
	// step2: check and create deploy event
	if !req.Force {
		var lastVersion = model.ApplicationRevision{
			AppPrimaryKey: app.PrimaryKey(),
		}
		list, err := c.ds.List(ctx, &lastVersion, &datastore.ListOptions{PageSize: 1, Page: 1})
		if err != nil && !errors.Is(err, datastore.ErrRecordNotExist) {
			log.Logger.Errorf("query app latest revision failure %s", err.Error())
			return nil, bcode.ErrDeployConflict
		}
		if len(list) > 0 && list[0].(*model.ApplicationRevision).Status != model.RevisionStatusComplete {
			return nil, bcode.ErrDeployConflict
		}
	}

	workflow, err := c.workflowUsecase.GetWorkflow(ctx, oamApp.Annotations[oam.AnnotationWorkflowName])
	if err != nil {
		return nil, err
	}

	var appRevision = &model.ApplicationRevision{
		AppPrimaryKey:  app.PrimaryKey(),
		Version:        version,
		ApplyAppConfig: string(configByte),
		Status:         model.RevisionStatusInit,
		// TODO: Get user information from ctx and assign a value.
		DeployUser:   "",
		Note:         req.Note,
		TriggerType:  req.TriggerType,
		WorkflowName: oamApp.Annotations[oam.AnnotationWorkflowName],
		EnvName:      workflow.EnvName,
	}

	if err := c.ds.Add(ctx, appRevision); err != nil {
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
		appRevision.Status = model.RevisionStatusFail
		appRevision.Reason = err.Error()
		if err := c.ds.Put(ctx, appRevision); err != nil {
			log.Logger.Warnf("update deploy event failure %s", err.Error())
		}
		log.Logger.Errorf("deploy app %s failure %s", app.PrimaryKey(), err.Error())
		return nil, bcode.ErrDeployApplyFail
	}
	appRevision.Status = model.RevisionStatusRunning
	if err := c.ds.Put(ctx, appRevision); err != nil {
		log.Logger.Warnf("update deploy event failure %s", err.Error())
	}

	// step5: update deploy event status
	return &apisv1.ApplicationDeployResponse{
		ApplicationRevisionBase: apisv1.ApplicationRevisionBase{
			Version:     appRevision.Version,
			Status:      appRevision.Status,
			Reason:      appRevision.Reason,
			DeployUser:  appRevision.DeployUser,
			Note:        appRevision.Note,
			TriggerType: appRevision.TriggerType,
		},
	}, nil
}

func (c *applicationUsecaseImpl) renderOAMApplication(ctx context.Context, appModel *model.Application, reqWorkflowName, version string) (*v1beta1.Application, error) {
	var app = &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      appModel.Name,
			Namespace: appModel.Namespace,
			Labels:    appModel.Labels,
			Annotations: map[string]string{
				oam.AnnotationDeployVersion: version,
			},
		},
	}
	var component = model.ApplicationComponent{
		AppPrimaryKey: appModel.PrimaryKey(),
	}
	components, err := c.ds.List(ctx, &component, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if err != nil || len(components) == 0 {
		return nil, bcode.ErrNoComponent
	}

	var policy = model.ApplicationPolicy{
		AppPrimaryKey: appModel.PrimaryKey(),
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
		bc := common.ApplicationComponent{
			Name:             component.Name,
			Type:             component.Type,
			ExternalRevision: component.ExternalRevision,
			DependsOn:        component.DependsOn,
			Inputs:           component.Inputs,
			Outputs:          component.Outputs,
			Traits:           traits,
			Scopes:           component.Scopes,
			Properties:       component.Properties.RawExtension(),
		}
		if component.Properties != nil {
			bc.Properties = component.Properties.RawExtension()
		}
		app.Spec.Components = append(app.Spec.Components, bc)
	}

	for _, entity := range policies {
		policy := entity.(*model.ApplicationPolicy)
		apolicy := v1beta1.AppPolicy{
			Name: policy.Name,
			Type: policy.Type,
		}
		if policy.Properties != nil {
			apolicy.Properties = policy.Properties.RawExtension()
		}
		app.Spec.Policies = append(app.Spec.Policies, apolicy)
	}

	// Priority 1 uses the requested workflow as release .
	// Priority 2 uses the default workflow as release .
	var workflow *model.Workflow
	if reqWorkflowName != "" {
		workflow, err = c.workflowUsecase.GetWorkflow(ctx, reqWorkflowName)
		if err != nil {
			return nil, err
		}
		if workflow.EnvName != "" {
			envPolicy, err := c.genPolicyByEnv(ctx, appModel, workflow.EnvName)
			if err != nil {
				return nil, err
			}
			app.Spec.Policies = append(app.Spec.Policies, envPolicy)
		}

	} else {
		workflow, err = c.workflowUsecase.GetApplicationDefaultWorkflow(ctx, appModel)
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
			steps = append(steps, wstep)
		}
		app.Spec.Workflow = &v1beta1.Workflow{
			Steps: steps,
		}
	}

	return app, nil
}

func (c *applicationUsecaseImpl) converAppModelToBase(app *model.Application) *apisv1.ApplicationBase {
	appBase := &apisv1.ApplicationBase{
		Name:        app.Name,
		Alias:       app.Alias,
		Namespace:   app.Namespace,
		CreateTime:  app.CreateTime,
		UpdateTime:  app.UpdateTime,
		Description: app.Description,
		Icon:        app.Icon,
		Labels:      app.Labels,
	}
	return appBase
}

// DeleteApplication delete application
func (c *applicationUsecaseImpl) DeleteApplication(ctx context.Context, app *model.Application) error {
	// TODO: check app can be deleted

	// query all components to deleted
	components, err := c.ListComponents(ctx, app, apisv1.ListApplicationComponentOptions{})
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
		if err != nil && !errors.Is(err, datastore.ErrRecordNotExist) {
			log.Logger.Errorf("delete component %s in app %s failure %s", component.Name, app.Name, err.Error())
		}
	}

	for _, policy := range policies {
		err := c.ds.Delete(ctx, &model.ApplicationPolicy{AppPrimaryKey: app.PrimaryKey(), Name: policy.Name})
		if err != nil && errors.Is(err, datastore.ErrRecordNotExist) {
			log.Logger.Errorf("delete policy %s in app %s failure %s", policy.Name, app.Name, err.Error())
		}
	}

	if err := c.envBindingUsecase.BatchDeleteEnvBinding(ctx, app); err != nil {
		log.Logger.Errorf("delete envbindings in app %s failure %s", app.Name, err.Error())
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

func (c *applicationUsecaseImpl) CreateApplicationTrait(ctx context.Context, app *model.Application, component *model.ApplicationComponent, req apisv1.CreateApplicationTraitRequest) (*apisv1.ApplicationTrait, error) {
	var comp = model.ApplicationComponent{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          component.Name,
	}
	if err := c.ds.Get(ctx, &comp); err != nil {
		return nil, err
	}
	for _, trait := range comp.Traits {
		if trait.Type == req.Type {
			return nil, bcode.ErrTraitAlreadyExist
		}
	}
	properties, err := model.NewJSONStructByString(req.Properties)
	if err != nil {
		log.Logger.Errorf("new trait failure,%s", err.Error())
		return nil, bcode.ErrInvalidProperties
	}
	trait := model.ApplicationTrait{CreateTime: time.Now(), Type: req.Type, Properties: properties, Alias: req.Alias, Description: req.Description}
	comp.Traits = append(comp.Traits, trait)
	if err := c.ds.Put(ctx, &comp); err != nil {
		return nil, err
	}
	return &apisv1.ApplicationTrait{Type: trait.Type, Properties: properties, Alias: req.Alias, Description: req.Description, CreateTime: trait.CreateTime}, nil
}

func (c *applicationUsecaseImpl) DeleteApplicationTrait(ctx context.Context, app *model.Application, component *model.ApplicationComponent, traitType string) error {
	var comp = model.ApplicationComponent{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          component.Name,
	}
	if err := c.ds.Get(ctx, &comp); err != nil {
		return err
	}
	for i, trait := range comp.Traits {
		if trait.Type == traitType {
			comp.Traits = append(comp.Traits[:i], comp.Traits[i+1:]...)
			if err := c.ds.Put(ctx, &comp); err != nil {
				return err
			}
			return nil
		}
	}
	return bcode.ErrTraitNotExist
}

func (c *applicationUsecaseImpl) UpdateApplicationTrait(ctx context.Context, app *model.Application, component *model.ApplicationComponent, traitType string, req apisv1.UpdateApplicationTraitRequest) (*apisv1.ApplicationTrait, error) {
	var comp = model.ApplicationComponent{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          component.Name,
	}
	if err := c.ds.Get(ctx, &comp); err != nil {
		return nil, err
	}
	for i, trait := range comp.Traits {
		if trait.Type == traitType {
			properties, err := model.NewJSONStructByString(req.Properties)
			if err != nil {
				log.Logger.Errorf("update trait failure,%s", err.Error())
				return nil, bcode.ErrInvalidProperties
			}
			updatedTrait := model.ApplicationTrait{CreateTime: trait.CreateTime, UpdateTime: time.Now(), Properties: properties, Type: traitType, Alias: req.Alias, Description: req.Description}
			comp.Traits[i] = updatedTrait
			if err := c.ds.Put(ctx, &comp); err != nil {
				return nil, err
			}
			return &apisv1.ApplicationTrait{Type: trait.Type, Properties: properties,
				Alias: updatedTrait.Alias, Description: updatedTrait.Description, CreateTime: updatedTrait.CreateTime, UpdateTime: updatedTrait.UpdateTime}, nil
		}
	}
	return nil, bcode.ErrTraitNotExist
}

func (c *applicationUsecaseImpl) ListRevisions(ctx context.Context, appName, envName, status string, page, pageSize int) (*apisv1.ListRevisionsResponse, error) {
	var revision = model.ApplicationRevision{
		AppPrimaryKey: appName,
	}
	if envName != "" {
		revision.EnvName = envName
	}
	if status != "" {
		revision.Status = status
	}

	revisions, err := c.ds.List(ctx, &revision, &datastore.ListOptions{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, err
	}

	resp := &apisv1.ListRevisionsResponse{
		Revisions: []apisv1.ApplicationRevisionBase{},
	}
	for _, raw := range revisions {
		r, ok := raw.(*model.ApplicationRevision)
		if ok {
			resp.Revisions = append(resp.Revisions, apisv1.ApplicationRevisionBase{
				CreateTime:  r.CreateTime,
				Version:     r.Version,
				Status:      r.Status,
				Reason:      r.Reason,
				DeployUser:  r.DeployUser,
				Note:        r.Note,
				EnvName:     r.EnvName,
				TriggerType: r.TriggerType,
			})
		}
	}
	count, err := c.ds.Count(ctx, &revision)
	if err != nil {
		return nil, err
	}
	resp.Total = count

	return resp, nil
}

func (c *applicationUsecaseImpl) DetailRevision(ctx context.Context, appName, revisionName string) (*apisv1.DetailRevisionResponse, error) {
	var revision = model.ApplicationRevision{
		AppPrimaryKey: appName,
		Version:       revisionName,
	}
	if err := c.ds.Get(ctx, &revision); err != nil {
		return nil, err
	}
	return &apisv1.DetailRevisionResponse{
		ApplicationRevision: revision,
	}, nil
}

func createTargetClusterEnv(envBind apisv1.EnvBindingBase, target *model.DeliveryTarget) v1alpha1.EnvConfig {
	placement := v1alpha1.EnvPlacement{}
	var componentSelector *v1alpha1.EnvSelector
	if envBind.ComponentSelector != nil {
		componentSelector = &v1alpha1.EnvSelector{
			Components: envBind.ComponentSelector.Components,
		}
	}
	if target.Cluster != nil {
		placement.ClusterSelector = &common.ClusterSelector{Name: target.Cluster.ClusterName}
		placement.NamespaceSelector = &v1alpha1.NamespaceSelector{Name: target.Cluster.Namespace}
	}
	return v1alpha1.EnvConfig{
		Name:      genPolicyEnvName(target.Name),
		Placement: placement,
		Selector:  componentSelector,
	}
}

func SaveApplicationWorkflow(workflowUsecase WorkflowUsecase, ctx context.Context, application *model.Application, workflowSteps []v1beta1.WorkflowStep, workflowName string) error {
	var steps []apisv1.WorkflowStep
	for _, step := range workflowSteps {
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
	_, err := workflowUsecase.CreateWorkflow(ctx, application, apisv1.CreateWorkflowRequest{
		AppName:     application.PrimaryKey(),
		Name:        workflowName,
		Description: "Created automatically.",
		Steps:       steps,
		Default:     true,
	})
	if err != nil {
		return err
	}
	return nil
}

func converAppName(app *model.Application, envName string) string {
	return fmt.Sprintf("%s-%s", app.Name, envName)
}

func genPolicyName(envName string) string {
	return fmt.Sprintf("%s-%s", EnvBindingPolicyDefaultName, envName)
}

func genWorkflowName(app *model.Application, envName string) string {
	return fmt.Sprintf("%s-%s", app.Name, envName)
}

func genPolicyEnvName(targetName string) string {
	return targetName
}
