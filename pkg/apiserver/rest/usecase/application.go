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
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
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
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	utils2 "github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/appfile/dryrun"
)

// PolicyType build-in policy type
type PolicyType string

const (
	// EnvBindingPolicy Multiple environment distribution policy
	EnvBindingPolicy PolicyType = "env-binding"

	// EnvBindingPolicyDefaultName default policy name
	EnvBindingPolicyDefaultName string = "env-bindings"

	defaultTokenLen int = 16
)

// ApplicationUsecase application usecase
type ApplicationUsecase interface {
	ListApplications(ctx context.Context, listOptions apisv1.ListApplicationOptions) ([]*apisv1.ApplicationBase, error)
	GetApplication(ctx context.Context, appName string) (*model.Application, error)
	GetApplicationStatus(ctx context.Context, app *model.Application, envName string) (*common.AppStatus, error)
	DetailApplication(ctx context.Context, app *model.Application) (*apisv1.DetailApplicationResponse, error)
	PublishApplicationTemplate(ctx context.Context, app *model.Application) (*apisv1.ApplicationTemplateBase, error)
	CreateApplication(context.Context, apisv1.CreateApplicationRequest) (*apisv1.ApplicationBase, error)
	UpdateApplication(context.Context, *model.Application, apisv1.UpdateApplicationRequest) (*apisv1.ApplicationBase, error)
	DeleteApplication(ctx context.Context, app *model.Application) error
	Deploy(ctx context.Context, app *model.Application, req apisv1.ApplicationDeployRequest) (*apisv1.ApplicationDeployResponse, error)
	GetApplicationComponent(ctx context.Context, app *model.Application, componentName string) (*model.ApplicationComponent, error)
	ListComponents(ctx context.Context, app *model.Application, op apisv1.ListApplicationComponentOptions) ([]*apisv1.ComponentBase, error)
	AddComponent(ctx context.Context, app *model.Application, com apisv1.CreateComponentRequest) (*apisv1.ComponentBase, error)
	DetailComponent(ctx context.Context, app *model.Application, componentName string) (*apisv1.DetailComponentResponse, error)
	DeleteComponent(ctx context.Context, app *model.Application, componentName string) error
	UpdateComponent(ctx context.Context, app *model.Application, component *model.ApplicationComponent, req apisv1.UpdateApplicationComponentRequest) (*apisv1.ComponentBase, error)
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
	Statistics(ctx context.Context, app *model.Application) (*apisv1.ApplicationStatisticsResponse, error)
	ListRecords(ctx context.Context, appName string) (*apisv1.ListWorkflowRecordsResponse, error)
	CompareAppWithLatestRevision(ctx context.Context, app *model.Application, compareReq apisv1.AppCompareReq) (*apisv1.AppCompareResponse, error)
	ResetAppToLatestRevision(ctx context.Context, appName string) (*apisv1.AppResetResponse, error)
	DryRunAppOrRevision(ctx context.Context, app *model.Application, dryRunReq apisv1.AppDryRunReq) (*apisv1.AppDryRunResponse, error)
	CreateApplicationTrigger(ctx context.Context, app *model.Application, req apisv1.CreateApplicationTriggerRequest) (*apisv1.ApplicationTriggerBase, error)
	ListApplicationTriggers(ctx context.Context, app *model.Application) ([]*apisv1.ApplicationTriggerBase, error)
	DeleteApplicationTrigger(ctx context.Context, app *model.Application, triggerName string) error
}

type applicationUsecaseImpl struct {
	ds                datastore.DataStore
	kubeClient        client.Client
	apply             apply.Applicator
	workflowUsecase   WorkflowUsecase
	envUsecase        EnvUsecase
	envBindingUsecase EnvBindingUsecase
	targetUsecase     TargetUsecase
	definitionUsecase DefinitionUsecase
	projectUsecase    ProjectUsecase
}

// NewApplicationUsecase new application usecase
func NewApplicationUsecase(ds datastore.DataStore,
	workflowUsecase WorkflowUsecase,
	envBindingUsecase EnvBindingUsecase,
	envUsecase EnvUsecase,
	targetUsecase TargetUsecase,
	definitionUsecase DefinitionUsecase,
	projectUsecase ProjectUsecase,
) ApplicationUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &applicationUsecaseImpl{
		ds:                ds,
		workflowUsecase:   workflowUsecase,
		envBindingUsecase: envBindingUsecase,
		targetUsecase:     targetUsecase,
		kubeClient:        kubecli,
		apply:             apply.NewAPIApplicator(kubecli),
		definitionUsecase: definitionUsecase,
		projectUsecase:    projectUsecase,
		envUsecase:        envUsecase,
	}
}

func listApp(ctx context.Context, ds datastore.DataStore, listOptions apisv1.ListApplicationOptions) ([]*model.Application, error) {
	var app = model.Application{}
	if listOptions.Project != "" {
		app.Project = listOptions.Project
	}
	var err error
	var envBinding []*apisv1.EnvBindingBase
	if listOptions.Env != "" || listOptions.TargetName != "" {
		envBinding, err = listFullEnvBinding(ctx, ds, envListOption{})
		if err != nil {
			log.Logger.Errorf("list envbinding for list application in env %s err %v", utils2.Sanitize(listOptions.Env), err)
			return nil, err
		}
	}

	entities, err := ds.List(ctx, &app, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*model.Application
	for _, entity := range entities {
		appModel, ok := entity.(*model.Application)
		if !ok {
			continue
		}
		if listOptions.Query != "" &&
			!(strings.Contains(appModel.Alias, listOptions.Query) ||
				strings.Contains(appModel.Name, listOptions.Query) ||
				strings.Contains(appModel.Description, listOptions.Query)) {
			continue
		}
		if listOptions.TargetName != "" {
			targetIsContain, _ := CheckAppEnvBindingsContainTarget(envBinding, listOptions.TargetName)
			if !targetIsContain {
				continue
			}
		}
		if len(envBinding) > 0 && listOptions.Env != "" {
			check := func() bool {
				for _, eb := range envBinding {
					if eb.Name == listOptions.Env && appModel.PrimaryKey() == eb.AppDeployName {
						return true
					}
				}
				return false
			}
			if !check() {
				continue
			}
		}
		list = append(list, appModel)
	}
	return list, nil
}

// ListApplications list applications
func (c *applicationUsecaseImpl) ListApplications(ctx context.Context, listOptions apisv1.ListApplicationOptions) ([]*apisv1.ApplicationBase, error) {
	apps, err := listApp(ctx, c.ds, listOptions)
	if err != nil {
		return nil, err
	}
	var list []*apisv1.ApplicationBase
	for _, app := range apps {
		appBase := c.converAppModelToBase(ctx, app)
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
	base := c.converAppModelToBase(ctx, app)
	policys, err := c.queryApplicationPolicies(ctx, app)
	if err != nil {
		return nil, err
	}
	componentNum, err := c.ds.Count(ctx, &model.ApplicationComponent{AppPrimaryKey: app.PrimaryKey()}, &datastore.FilterOptions{})
	if err != nil {
		return nil, err
	}
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
			ComponentNum: componentNum,
		},
		ApplicationType: func() string {
			if GetSuitableDeployWay(ctx, c.kubeClient, c.ds, app) == DeployCloudResource {
				return "cloud"
			}
			return "common"
		}(),
	}
	return detail, nil
}

// GetApplicationStatus get application status from controller cluster
func (c *applicationUsecaseImpl) GetApplicationStatus(ctx context.Context, appmodel *model.Application, envName string) (*common.AppStatus, error) {
	var app v1beta1.Application
	env, err := c.envUsecase.GetEnv(ctx, envName)
	if err != nil {
		return nil, err
	}
	err = c.kubeClient.Get(ctx, types.NamespacedName{Namespace: env.Namespace, Name: appmodel.Name}, &app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if !app.DeletionTimestamp.IsZero() {
		app.Status.Phase = "deleting"
	}
	return &app.Status, nil
}

// GetApplicationCR get application CR in cluster
func (c *applicationUsecaseImpl) GetApplicationCR(ctx context.Context, appModel *model.Application) (*v1beta1.ApplicationList, error) {
	var apps v1beta1.ApplicationList
	selector := labels.NewSelector()
	re, err := labels.NewRequirement(oam.AnnotationAppName, selection.Equals, []string{appModel.Name})
	if err != nil {
		return nil, err
	}
	selector = selector.Add(*re)
	err = c.kubeClient.List(ctx, &apps, &client.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &apps, nil
		}
		return nil, err
	}
	return &apps, nil
}

// PublishApplicationTemplate publish app template
func (c *applicationUsecaseImpl) PublishApplicationTemplate(ctx context.Context, app *model.Application) (*apisv1.ApplicationTemplateBase, error) {
	// TODO:
	return nil, nil
}

// CreateApplication create application
func (c *applicationUsecaseImpl) CreateApplication(ctx context.Context, req apisv1.CreateApplicationRequest) (*apisv1.ApplicationBase, error) {
	application := model.Application{
		Name:        req.Name,
		Alias:       req.Alias,
		Description: req.Description,
		Icon:        req.Icon,
		Labels:      req.Labels,
	}
	// check app name.
	exist, err := c.ds.IsExist(ctx, &application)
	if err != nil {
		log.Logger.Errorf("check application name is exist failure %s", err.Error())
		return nil, bcode.ErrApplicationExist
	}
	if exist {
		return nil, bcode.ErrApplicationExist
	}

	// check project
	project, err := c.projectUsecase.GetProject(ctx, req.Project)
	if err != nil {
		return nil, err
	}
	application.Project = project.Name

	if req.Component != nil {
		_, err = c.AddComponent(ctx, &application, *req.Component)
		if err != nil {
			return nil, err
		}
	}

	// build-in create env binding, it must after component added
	if len(req.EnvBinding) > 0 {
		err := c.saveApplicationEnvBinding(ctx, application, req.EnvBinding)
		if err != nil {
			return nil, err
		}
		if _, err := c.CreateApplicationTrigger(ctx, &application, apisv1.CreateApplicationTriggerRequest{
			Name:         fmt.Sprintf("%s-%s", application.Name, "default"),
			PayloadType:  model.PayloadTypeCustom,
			Type:         apisv1.TriggerTypeWebhook,
			WorkflowName: convertWorkflowName(req.EnvBinding[0].Name),
		}); err != nil {
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
	return base, nil
}

// CreateApplicationTrigger create application trigger
func (c *applicationUsecaseImpl) CreateApplicationTrigger(ctx context.Context, app *model.Application, req apisv1.CreateApplicationTriggerRequest) (*apisv1.ApplicationTriggerBase, error) {
	trigger := &model.ApplicationTrigger{
		AppPrimaryKey: app.Name,
		WorkflowName:  req.WorkflowName,
		Name:          req.Name,
		Alias:         req.Alias,
		Description:   req.Description,
		Type:          req.Type,
		PayloadType:   req.PayloadType,
		Token:         genWebhookToken(),
	}
	if err := c.ds.Add(ctx, trigger); err != nil {
		log.Logger.Errorf("failed to create application trigger, %s", err.Error())
		return nil, err
	}

	return &apisv1.ApplicationTriggerBase{
		WorkflowName:  req.WorkflowName,
		Name:          req.Name,
		Alias:         req.Alias,
		Description:   req.Description,
		Type:          req.Type,
		PayloadType:   req.PayloadType,
		Token:         trigger.Token,
		ComponentName: req.ComponentName,
		CreateTime:    trigger.CreateTime,
		UpdateTime:    trigger.UpdateTime,
	}, nil
}

// DeleteApplicationTrigger delete application trigger
func (c *applicationUsecaseImpl) DeleteApplicationTrigger(ctx context.Context, app *model.Application, token string) error {
	trigger := model.ApplicationTrigger{
		AppPrimaryKey: app.PrimaryKey(),
		Token:         token,
	}
	if err := c.ds.Delete(ctx, &trigger); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrApplicationTriggerNotExist
		}
		log.Logger.Warnf("delete app trigger failure %s", err.Error())
		return err
	}
	return nil
}

// ListApplicationTrigger list application triggers
func (c *applicationUsecaseImpl) ListApplicationTriggers(ctx context.Context, app *model.Application) ([]*apisv1.ApplicationTriggerBase, error) {
	trigger := &model.ApplicationTrigger{
		AppPrimaryKey: app.Name,
	}
	triggers, err := c.ds.List(ctx, trigger, &datastore.ListOptions{
		SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}},
	)
	if err != nil {
		log.Logger.Errorf("failed to list application triggers, %s", err.Error())
		return nil, err
	}

	resp := []*apisv1.ApplicationTriggerBase{}
	for _, raw := range triggers {
		trigger, ok := raw.(*model.ApplicationTrigger)
		if ok {
			resp = append(resp, &apisv1.ApplicationTriggerBase{
				WorkflowName: trigger.WorkflowName,
				Name:         trigger.Name,
				Alias:        trigger.Alias,
				Description:  trigger.Description,
				Type:         trigger.Type,
				PayloadType:  trigger.PayloadType,
				Token:        trigger.Token,
				UpdateTime:   trigger.UpdateTime,
				CreateTime:   trigger.CreateTime,
			})
		}
	}
	return resp, nil
}

func (c *applicationUsecaseImpl) genPolicyByEnv(ctx context.Context, app *model.Application, envName string, components []*model.ApplicationComponent) (v1beta1.AppPolicy, error) {
	appPolicy := v1beta1.AppPolicy{}
	envBinding, err := c.envBindingUsecase.GetEnvBinding(ctx, app, envName)
	if err != nil {
		return appPolicy, err
	}
	appPolicy.Name = genPolicyName(envBinding.Name)
	appPolicy.Type = string(EnvBindingPolicy)
	env, err := c.envUsecase.GetEnv(ctx, envName)
	if err != nil {
		return appPolicy, err
	}
	var envBindingSpec v1alpha1.EnvBindingSpec
	for _, targetName := range env.Targets {
		target, err := c.targetUsecase.GetTarget(ctx, targetName)
		if err != nil || target == nil {
			return appPolicy, bcode.ErrFoundEnvbindingDeliveryTarget
		}
		envBindingSpec.Envs = append(envBindingSpec.Envs, c.createTargetClusterEnv(ctx, envBinding, env, target, components))
	}
	properties, err := model.NewJSONStructByStruct(envBindingSpec)
	if err != nil {
		return appPolicy, bcode.ErrInvalidProperties
	}
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
	return c.converAppModelToBase(ctx, app), nil
}

// ListRecords list application record
func (c *applicationUsecaseImpl) ListRecords(ctx context.Context, appName string) (*apisv1.ListWorkflowRecordsResponse, error) {
	var record = model.WorkflowRecord{
		AppPrimaryKey: appName,
		Finished:      "false",
	}
	records, err := c.ds.List(ctx, &record, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		record.Finished = "true"
		records, err = c.ds.List(ctx, &record, &datastore.ListOptions{
			Page:     1,
			PageSize: 1,
			SortBy:   []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
		})
		if err != nil {
			return nil, err
		}
	}

	resp := &apisv1.ListWorkflowRecordsResponse{
		Records: []apisv1.WorkflowRecord{},
	}
	for _, raw := range records {
		record, ok := raw.(*model.WorkflowRecord)
		if ok {
			resp.Records = append(resp.Records, *convertFromRecordModel(record))
		}
	}
	resp.Total = int64(len(records))

	return resp, nil
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
	policies, err := c.queryApplicationPolicies(ctx, app)
	if err != nil {
		return nil, err
	}
	var list []*apisv1.PolicyBase
	for _, policy := range policies {
		list = append(list, convertPolicyModelToBase(policy))
	}
	return list, nil
}

func (c *applicationUsecaseImpl) queryApplicationPolicies(ctx context.Context, app *model.Application) (list []*model.ApplicationPolicy, err error) {
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
		PolicyBase: *convertPolicyModelToBase(&policy),
	}, nil
}

// Deploy deploy app to cluster
// means to render oam application config and apply to cluster.
// An event record is generated for each deploy.
func (c *applicationUsecaseImpl) Deploy(ctx context.Context, app *model.Application, req apisv1.ApplicationDeployRequest) (*apisv1.ApplicationDeployResponse, error) {
	// TODO: rollback to handle all the error case
	// step1: Render oam application
	version := utils.GenerateVersion("")
	oamApp, err := c.renderOAMApplication(ctx, app, req.WorkflowName, version)
	if err != nil {
		return nil, err
	}
	configByte, _ := yaml.Marshal(oamApp)

	workflow, err := c.workflowUsecase.GetWorkflow(ctx, app, oamApp.Annotations[oam.AnnotationWorkflowName])
	if err != nil {
		return nil, err
	}

	// step2: check and create deploy event
	if !req.Force {
		var lastVersion = model.ApplicationRevision{
			AppPrimaryKey: app.PrimaryKey(),
			EnvName:       workflow.EnvName,
		}
		list, err := c.ds.List(ctx, &lastVersion, &datastore.ListOptions{
			PageSize: 1, Page: 1, SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
		if err != nil && !errors.Is(err, datastore.ErrRecordNotExist) {
			log.Logger.Errorf("query app latest revision failure %s", err.Error())
			return nil, bcode.ErrDeployConflict
		}
		if len(list) > 0 {
			revision := list[0].(*model.ApplicationRevision)
			var status string
			if revision.Status == model.RevisionStatusRollback {
				rollbackRevision := &model.ApplicationRevision{
					AppPrimaryKey: revision.AppPrimaryKey,
					Version:       revision.RollbackVersion,
				}
				if err := c.ds.Get(ctx, rollbackRevision); err == nil {
					status = rollbackRevision.Status
				}
			} else {
				status = revision.Status
			}
			if status != model.RevisionStatusComplete && status != model.RevisionStatusTerminated {
				log.Logger.Warnf("last app revision can not complete %s/%s", list[0].(*model.ApplicationRevision).AppPrimaryKey, list[0].(*model.ApplicationRevision).Version)
				return nil, bcode.ErrDeployConflict
			}
		}
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
		CodeInfo:     req.CodeInfo,
		ImageInfo:    req.ImageInfo,
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

	// step5: create workflow record
	if err := c.workflowUsecase.CreateWorkflowRecord(ctx, app, oamApp, workflow); err != nil {
		log.Logger.Warnf("create workflow record failure %s", err.Error())
	}

	// step6: update app revision status
	appRevision.Status = model.RevisionStatusRunning
	if err := c.ds.Put(ctx, appRevision); err != nil {
		log.Logger.Warnf("update app revision failure %s", err.Error())
	}

	return &apisv1.ApplicationDeployResponse{
		ApplicationRevisionBase: c.converRevisionModelToBase(appRevision),
	}, nil
}

func (c *applicationUsecaseImpl) renderOAMApplication(ctx context.Context, appModel *model.Application, reqWorkflowName, version string) (*v1beta1.Application, error) {
	// Priority 1 uses the requested workflow as release .
	// Priority 2 uses the default workflow as release .
	var workflow *model.Workflow
	var err error
	if reqWorkflowName != "" {
		workflow, err = c.workflowUsecase.GetWorkflow(ctx, appModel, reqWorkflowName)
		if err != nil {
			return nil, err
		}
	} else {
		workflow, err = c.workflowUsecase.GetApplicationDefaultWorkflow(ctx, appModel)
		if err != nil && !errors.Is(err, bcode.ErrWorkflowNoDefault) {
			return nil, err
		}
	}
	if workflow == nil || workflow.EnvName == "" {
		return nil, bcode.ErrWorkflowNotExist
	}
	env, err := c.envUsecase.GetEnv(ctx, workflow.EnvName)
	if err != nil {
		return nil, err
	}
	labels := make(map[string]string)
	for key, value := range appModel.Labels {
		labels[key] = value
	}
	labels[oam.AnnotationAppName] = appModel.Name

	var app = &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      appModel.Name,
			Namespace: env.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				oam.AnnotationDeployVersion: version,
				// publish version is the identifier of workflow record
				oam.AnnotationPublishVersion: utils.GenerateVersion(reqWorkflowName),
				oam.AnnotationAppName:        appModel.Name,
				oam.AnnotationAppAlias:       appModel.Alias,
			},
		},
	}
	originalApp := &v1beta1.Application{}
	if err := c.kubeClient.Get(ctx, types.NamespacedName{
		Name:      appModel.Name,
		Namespace: env.Namespace,
	}, originalApp); err == nil {
		app.ResourceVersion = originalApp.ResourceVersion
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
	var componentModels []*model.ApplicationComponent
	for _, entity := range components {
		component := entity.(*model.ApplicationComponent)
		componentModels = append(componentModels, component)
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
	if workflow.EnvName != "" {
		envPolicy, err := c.genPolicyByEnv(ctx, appModel, workflow.EnvName, componentModels)
		if err != nil {
			return nil, err
		}
		app.Spec.Policies = append(app.Spec.Policies, envPolicy)
	}
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

	return app, nil
}

func (c *applicationUsecaseImpl) converAppModelToBase(ctx context.Context, app *model.Application) *apisv1.ApplicationBase {
	appBase := &apisv1.ApplicationBase{
		Name:        app.Name,
		Alias:       app.Alias,
		CreateTime:  app.CreateTime,
		UpdateTime:  app.UpdateTime,
		Description: app.Description,
		Icon:        app.Icon,
		Labels:      app.Labels,
	}
	project, err := c.projectUsecase.GetProject(ctx, app.Project)
	if err != nil {
		log.Logger.Errorf("query project info failure %s", err.Error())
	}
	if project != nil {
		appBase.Project = convertProjectModel2Base(project)
	}
	return appBase
}

func (c *applicationUsecaseImpl) converRevisionModelToBase(revision *model.ApplicationRevision) apisv1.ApplicationRevisionBase {
	return apisv1.ApplicationRevisionBase{
		Version:     revision.Version,
		Status:      revision.Status,
		Reason:      revision.Reason,
		DeployUser:  revision.DeployUser,
		Note:        revision.Note,
		TriggerType: revision.TriggerType,
		CreateTime:  revision.CreateTime,
		EnvName:     revision.EnvName,
		CodeInfo:    revision.CodeInfo,
		ImageInfo:   revision.ImageInfo,
	}
}

// DeleteApplication delete application
func (c *applicationUsecaseImpl) DeleteApplication(ctx context.Context, app *model.Application) error {
	// TODO: check app can be deleted
	crs, err := c.GetApplicationCR(ctx, app)
	if err != nil {
		return err
	}
	if len(crs.Items) > 0 {
		return bcode.ErrApplicationRefusedDelete
	}
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

	var revision = model.ApplicationRevision{
		AppPrimaryKey: app.PrimaryKey(),
	}
	revisions, err := c.ds.List(ctx, &revision, &datastore.ListOptions{})
	if err != nil {
		return err
	}

	triggers, err := c.ListApplicationTriggers(ctx, app)
	if err != nil {
		return err
	}

	// delete workflow
	if err := c.workflowUsecase.DeleteWorkflowByApp(ctx, app); err != nil && !errors.Is(err, bcode.ErrWorkflowNotExist) {
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

	for _, entity := range revisions {
		revision := entity.(*model.ApplicationRevision)
		if err := c.ds.Delete(ctx, &model.ApplicationRevision{AppPrimaryKey: app.PrimaryKey(), Version: revision.Version}); err != nil {
			log.Logger.Errorf("delete revision %s in app %s failure %s", revision.Version, app.Name, err.Error())
		}
	}

	for _, trigger := range triggers {
		if err := c.ds.Delete(ctx, &model.ApplicationTrigger{AppPrimaryKey: app.PrimaryKey(), Name: trigger.Name, Token: trigger.Token}); err != nil {
			log.Logger.Errorf("delete trigger %s in app %s failure %s", trigger.Name, app.Name, err.Error())
		}
	}

	if err := c.envBindingUsecase.BatchDeleteEnvBinding(ctx, app); err != nil {
		log.Logger.Errorf("delete envbindings in app %s failure %s", app.Name, err.Error())
	}

	return c.ds.Delete(ctx, app)
}

func (c *applicationUsecaseImpl) GetApplicationComponent(ctx context.Context, app *model.Application, componentName string) (*model.ApplicationComponent, error) {
	var component = model.ApplicationComponent{
		AppPrimaryKey: app.PrimaryKey(),
		Name:          componentName,
	}
	err := c.ds.Get(ctx, &component)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrApplicationComponetNotExist
		}
		return nil, err
	}
	return &component, nil
}

func (c *applicationUsecaseImpl) UpdateComponent(ctx context.Context, app *model.Application, component *model.ApplicationComponent, req apisv1.UpdateApplicationComponentRequest) (*apisv1.ComponentBase, error) {
	if req.Alias != nil {
		component.Alias = *req.Alias
	}
	if req.Description != nil {
		component.Description = *req.Description
	}
	if req.DependsOn != nil {
		component.DependsOn = *req.DependsOn
	}
	if req.Icon != nil {
		component.Icon = *req.Icon
	}
	if req.Labels != nil {
		component.Labels = *req.Labels
	}
	if req.Properties != nil {
		properties, err := model.NewJSONStructByString(*req.Properties)
		if err != nil {
			return nil, bcode.ErrInvalidProperties
		}
		component.Properties = properties
	}
	if err := c.ds.Put(ctx, component); err != nil {
		return nil, err
	}
	return converComponentModelToBase(component), nil
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
	// create default triat for component
	c.initCreateDefaultTrait(&componentModel)

	if err := c.ds.Add(ctx, &componentModel); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrApplicationComponetExist
		}
		log.Logger.Warnf("add component for app %s failure %s", utils2.Sanitize(app.PrimaryKey()), err.Error())
		return nil, err
	}
	return converComponentModelToBase(&componentModel), nil
}

func (c *applicationUsecaseImpl) initCreateDefaultTrait(component *model.ApplicationComponent) {
	replicationTrait := model.ApplicationTrait{
		Alias:       "Set Replicas",
		Type:        "scaler",
		Description: "Adjust the number of application instance.",
		Properties: &model.JSONStruct{
			"replicas": 1,
		},
		CreateTime: time.Now(),
		UpdateTime: time.Now(),
	}
	var initTraits = []model.ApplicationTrait{}
	if component.Type == "webservice" && len(component.Traits) == 0 {
		initTraits = append(initTraits, replicationTrait)
	}
	component.Traits = initTraits
}

func converComponentModelToBase(componentModel *model.ApplicationComponent) *apisv1.ComponentBase {
	if componentModel == nil {
		return nil
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
	}
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
		PolicyBase: *convertPolicyModelToBase(&policy),
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

	revisions, err := c.ds.List(ctx, &revision, &datastore.ListOptions{
		Page:     page,
		PageSize: pageSize,
		SortBy:   []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
	})
	if err != nil {
		return nil, err
	}

	resp := &apisv1.ListRevisionsResponse{
		Revisions: []apisv1.ApplicationRevisionBase{},
	}
	for _, raw := range revisions {
		r, ok := raw.(*model.ApplicationRevision)
		if ok {
			resp.Revisions = append(resp.Revisions, c.converRevisionModelToBase(r))
		}
	}
	count, err := c.ds.Count(ctx, &revision, nil)
	if err != nil {
		return nil, err
	}
	resp.Total = count

	return resp, nil
}

func (c *applicationUsecaseImpl) DetailRevision(ctx context.Context, appName, revisionVersion string) (*apisv1.DetailRevisionResponse, error) {
	var revision = model.ApplicationRevision{
		AppPrimaryKey: appName,
		Version:       revisionVersion,
	}
	if err := c.ds.Get(ctx, &revision); err != nil {
		return nil, err
	}
	return &apisv1.DetailRevisionResponse{
		ApplicationRevision: revision,
	}, nil
}

func (c *applicationUsecaseImpl) Statistics(ctx context.Context, app *model.Application) (*apisv1.ApplicationStatisticsResponse, error) {
	var targetMap = make(map[string]int)
	envbinding, err := c.envBindingUsecase.GetEnvBindings(ctx, app)
	if err != nil {
		log.Logger.Errorf("query app envbinding failure %s", err.Error())
	}
	for _, env := range envbinding {
		for _, target := range env.TargetNames {
			targetMap[target]++
		}
	}
	count, err := c.ds.Count(ctx, &model.ApplicationRevision{AppPrimaryKey: app.PrimaryKey()}, &datastore.FilterOptions{})
	if err != nil {
		return nil, err
	}
	return &apisv1.ApplicationStatisticsResponse{
		EnvCount:      int64(len(envbinding)),
		TargetCount:   int64(len(targetMap)),
		RevisonCount:  count,
		WorkflowCount: c.workflowUsecase.CountWorkflow(ctx, app),
	}, nil
}

// CompareAppWithLatestRevision compare application with last revision
func (c *applicationUsecaseImpl) CompareAppWithLatestRevision(ctx context.Context, appModel *model.Application, compareReq apisv1.AppCompareReq) (*apisv1.AppCompareResponse, error) {
	var reqWorkflowName string
	if compareReq.Env != "" {
		reqWorkflowName = convertWorkflowName(compareReq.Env)
	}
	newApp, err := c.renderOAMApplication(ctx, appModel, reqWorkflowName, "")
	if err != nil {
		return nil, err
	}
	ignoreSomeParams(newApp)
	newAppBytes, err := yaml.Marshal(newApp)
	if err != nil {
		return nil, err
	}

	oldApp, err := c.getAppFromLatestRevision(ctx, appModel.Name, compareReq.Env, "")
	if err != nil {
		if errors.Is(err, bcode.ErrApplicationRevisionNotExist) {
			return &apisv1.AppCompareResponse{IsDiff: false, NewAppYAML: string(newAppBytes)}, nil
		}
		return nil, err
	}
	ignoreSomeParams(oldApp)
	oldAppBytes, err := yaml.Marshal(oldApp)
	if err != nil {
		return nil, err
	}

	diffResult, buff, err := compare(ctx, newApp, oldApp)
	if err != nil {
		return &apisv1.AppCompareResponse{IsDiff: false, NewAppYAML: string(newAppBytes), OldAppYAML: string(oldAppBytes)}, err
	}
	return &apisv1.AppCompareResponse{IsDiff: diffResult.DiffType != "", DiffReport: buff.String(), NewAppYAML: string(newAppBytes), OldAppYAML: string(oldAppBytes)}, nil
}

// ResetAppToLatestRevision reset app's component to last revision
func (c *applicationUsecaseImpl) ResetAppToLatestRevision(ctx context.Context, appName string) (*apisv1.AppResetResponse, error) {
	targetApp, err := c.getAppFromLatestRevision(ctx, appName, "", "")
	if err != nil {
		return nil, err
	}
	return c.resetApp(ctx, targetApp)
}

// DryRunAppOrRevision dry-run application or revision
func (c *applicationUsecaseImpl) DryRunAppOrRevision(ctx context.Context, appModel *model.Application, dryRunReq apisv1.AppDryRunReq) (*apisv1.AppDryRunResponse, error) {
	var app *v1beta1.Application
	var err error
	if dryRunReq.DryRunType == "APP" {
		var reqWorkflowName string
		if dryRunReq.Env != "" {
			reqWorkflowName = convertWorkflowName(dryRunReq.Env)
		}
		app, err = c.renderOAMApplication(ctx, appModel, reqWorkflowName, "")
		if err != nil {
			return nil, err
		}
	} else {
		app, err = c.getAppFromLatestRevision(ctx, dryRunReq.AppName, dryRunReq.Env, dryRunReq.Version)
		if err != nil {
			return nil, err
		}
	}

	dryRunResult, err := dryRunApplication(ctx, app)
	if err != nil {
		return nil, err
	}
	return &apisv1.AppDryRunResponse{YAML: dryRunResult.String()}, nil
}

func (c *applicationUsecaseImpl) createTargetClusterEnv(ctx context.Context, envBind *model.EnvBinding, env *model.Env, target *model.Target, components []*model.ApplicationComponent) v1alpha1.EnvConfig {
	placement := v1alpha1.EnvPlacement{}
	if target.Cluster != nil {
		placement.ClusterSelector = &common.ClusterSelector{Name: target.Cluster.ClusterName}
		placement.NamespaceSelector = &v1alpha1.NamespaceSelector{Name: target.Cluster.Namespace}
	}
	var componentPatchs []v1alpha1.EnvComponentPatch
	// init cloud application region and provider info
	for _, component := range components {
		definition, err := GetComponentDefinition(ctx, c.kubeClient, component.Type)
		if err != nil {
			log.Logger.Errorf("get component definition %s failure %s", component.Type, err.Error())
			continue
		}
		if definition != nil {
			if definition.Spec.Workload.Type == TerraformWorkfloadType ||
				definition.Spec.Workload.Definition.Kind == TerraformWorkfloadKind {
				properties := model.JSONStruct{
					"providerRef": map[string]interface{}{
						"name": "default",
					},
					"writeConnectionSecretToRef": map[string]interface{}{
						"name":      fmt.Sprintf("%s-%s", component.Name, envBind.Name),
						"namespace": env.Namespace,
					},
				}
				if region, ok := target.Variable["region"]; ok {
					properties["region"] = region
				}
				if providerName, ok := target.Variable["providerName"]; ok {
					properties["providerRef"].(map[string]interface{})["name"] = providerName
				}
				if providerNamespace, ok := target.Variable["providerNamespace"]; ok {
					properties["providerRef"].(map[string]interface{})["namespace"] = providerNamespace
				}
				componentPatchs = append(componentPatchs, v1alpha1.EnvComponentPatch{
					Name:       component.Name,
					Properties: properties.RawExtension(),
					Type:       component.Type,
				})
			}
		}
	}

	return v1alpha1.EnvConfig{
		Name:      genPolicyEnvName(target.Name),
		Placement: placement,
		Patch: v1alpha1.EnvPatch{
			Components: componentPatchs,
		},
	}
}

func genPolicyName(envName string) string {
	return fmt.Sprintf("%s-%s", EnvBindingPolicyDefaultName, envName)
}

func genPolicyEnvName(targetName string) string {
	return targetName
}

func genWebhookToken() string {
	rand.Seed(time.Now().UnixNano())
	runes := []rune("abcdefghijklmnopqrstuvwxyz0123456789")

	b := make([]rune, defaultTokenLen)
	for i := range b {
		b[i] = runes[rand.Intn(len(runes))] // #nosec
	}
	return string(b)
}

func convertToModelComponent(appPrimaryKey string, component common.ApplicationComponent) (model.ApplicationComponent, error) {
	bc := model.ApplicationComponent{
		AppPrimaryKey:    appPrimaryKey,
		Name:             component.Name,
		Type:             component.Type,
		ExternalRevision: component.ExternalRevision,
		DependsOn:        component.DependsOn,
		Inputs:           component.Inputs,
		Outputs:          component.Outputs,
		Scopes:           component.Scopes,
	}
	if component.Properties != nil {
		properties, err := model.NewJSONStruct(component.Properties)
		if err != nil {
			return bc, err
		}
		bc.Properties = properties
	}
	for _, trait := range component.Traits {
		properties, err := model.NewJSONStruct(trait.Properties)
		if err != nil {
			return bc, err
		}
		bc.Traits = append(bc.Traits, model.ApplicationTrait{CreateTime: time.Now(), UpdateTime: time.Now(), Properties: properties, Type: trait.Type, Alias: trait.Type, Description: "auto gen"})
	}
	return bc, nil
}

func (c *applicationUsecaseImpl) getAppFromLatestRevision(ctx context.Context, appName string, envName string, version string) (*v1beta1.Application, error) {

	ar := &model.ApplicationRevision{AppPrimaryKey: appName}
	if envName != "" {
		ar.EnvName = envName
	}
	if version != "" {
		ar.Version = version
	}
	revisions, err := c.ds.List(ctx, ar, &datastore.ListOptions{
		Page:     1,
		PageSize: 1,
		SortBy:   []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
	})
	if err != nil || len(revisions) == 0 {
		return nil, bcode.ErrApplicationRevisionNotExist
	}
	latestRevisionRaw := revisions[0]
	latestRevision, ok := latestRevisionRaw.(*model.ApplicationRevision)
	if !ok {
		return nil, errors.New("convert application revision error")
	}
	oldApp := &v1beta1.Application{}
	if err := yaml.Unmarshal([]byte(latestRevision.ApplyAppConfig), oldApp); err != nil {
		return nil, err
	}
	return oldApp, nil
}

func (c *applicationUsecaseImpl) resetApp(ctx context.Context, targetApp *v1beta1.Application) (*apisv1.AppResetResponse, error) {
	appPrimaryKey := targetApp.Name

	originComps, err := c.ds.List(ctx, &model.ApplicationComponent{AppPrimaryKey: appPrimaryKey}, &datastore.ListOptions{})
	if err != nil {
		return nil, bcode.ErrApplicationComponetNotExist
	}

	var originCompNames []string
	for _, entity := range originComps {
		comp := entity.(*model.ApplicationComponent)
		originCompNames = append(originCompNames, comp.Name)
	}

	var targetCompNames []string
	targetComps := targetApp.Spec.Components
	for _, comp := range targetComps {
		targetCompNames = append(targetCompNames, comp.Name)
	}

	readyToUpdate, readyToDelete, readyToAdd := compareSlices(originCompNames, targetCompNames)

	// delete new app's components
	for _, compName := range readyToDelete {
		var component = model.ApplicationComponent{
			AppPrimaryKey: appPrimaryKey,
			Name:          compName,
		}
		if err := c.ds.Delete(ctx, &component); err != nil {
			if errors.Is(err, datastore.ErrRecordNotExist) {
				continue
			}
			log.Logger.Warnf("delete app %s comp %s failure %s", appPrimaryKey, compName, err.Error())
		}
	}

	for _, comp := range targetComps {
		// add or update new app's components from old app
		if utils.StringsContain(readyToAdd, comp.Name) || utils.StringsContain(readyToUpdate, comp.Name) {
			compModel, err := convertToModelComponent(appPrimaryKey, comp)
			if err != nil {
				return &apisv1.AppResetResponse{}, bcode.ErrInvalidProperties
			}
			properties, err := model.NewJSONStruct(comp.Properties)
			if err != nil {
				return &apisv1.AppResetResponse{}, bcode.ErrInvalidProperties
			}
			compModel.Properties = properties
			if err := c.ds.Add(ctx, &compModel); err != nil {
				if errors.Is(err, datastore.ErrRecordExist) {
					err := c.ds.Put(ctx, &compModel)
					if err != nil {
						log.Logger.Warnf("update comp %s  for app %s failure %s", comp.Name, utils2.Sanitize(appPrimaryKey), err.Error())
					}
					return &apisv1.AppResetResponse{IsReset: true}, err
				}
				log.Logger.Warnf("add comp %s  for app %s failure %s", comp.Name, utils2.Sanitize(appPrimaryKey), err.Error())
				return &apisv1.AppResetResponse{}, err
			}
		}
	}
	return &apisv1.AppResetResponse{IsReset: true}, nil
}

func dryRunApplication(ctx context.Context, app *v1beta1.Application) (bytes.Buffer, error) {
	c := common2.Args{
		Schema: common2.Scheme,
	}
	var buff = bytes.Buffer{}
	newClient, err := c.GetClient()
	if err != nil {
		return buff, err
	}
	var objs []oam.Object
	pd, err := c.GetPackageDiscover()
	if err != nil {
		return buff, err
	}
	config, err := c.GetConfig()
	if err != nil {
		return buff, err
	}
	dm, err := discoverymapper.New(config)
	if err != nil {
		return buff, err
	}
	dryRunOpt := dryrun.NewDryRunOption(newClient, dm, pd, objs)
	comps, err := dryRunOpt.ExecuteDryRun(ctx, app)
	if err != nil {
		return buff, errors.New("generate OAM objects")
	}
	var components = make(map[string]*unstructured.Unstructured)
	for _, comp := range comps {
		components[comp.Name] = comp.StandardWorkload
	}
	buff.Write([]byte(fmt.Sprintf("---\n# Application(%s) \n---\n\n", app.Name)))
	result, err := yaml.Marshal(app)
	if err != nil {
		return buff, errors.New("marshal app error")
	}
	buff.Write(result)
	buff.Write([]byte("\n---\n"))
	for _, c := range comps {
		buff.Write([]byte(fmt.Sprintf("---\n# Application(%s) -- Component(%s) \n---\n\n", app.Name, c.Name)))
		result, err := yaml.Marshal(components[c.Name])
		if err != nil {
			return buff, errors.New("marshal result for component " + c.Name + " object in yaml format")
		}
		buff.Write(result)
		buff.Write([]byte("\n---\n"))
		for _, t := range c.Traits {
			result, err := yaml.Marshal(t)
			if err != nil {
				return buff, errors.New("marshal result for component " + c.Name + " object in yaml format")
			}
			buff.Write(result)
			buff.Write([]byte("\n---\n"))
		}
		buff.Write([]byte("\n"))
	}
	return buff, nil
}

func ignoreSomeParams(o *v1beta1.Application) {
	// set default
	o.ResourceVersion = ""
	o.Spec.Workflow = nil
	newAnnotations := map[string]string{}
	annotations := o.GetAnnotations()
	for k, v := range annotations {
		if k == oam.AnnotationDeployVersion || k == oam.AnnotationPublishVersion || k == "kubectl.kubernetes.io/last-applied-configuration" {
			continue
		}
		newAnnotations[k] = v
	}
	o.SetAnnotations(newAnnotations)
}

func compare(ctx context.Context, newApp *v1beta1.Application, oldApp *v1beta1.Application) (*dryrun.DiffEntry, bytes.Buffer, error) {
	var buff = bytes.Buffer{}
	c := common2.Args{
		Schema: common2.Scheme,
	}
	_, err := c.GetClient()
	if err != nil {
		return nil, buff, err
	}
	pd, err := c.GetPackageDiscover()
	if err != nil {
		return nil, buff, err
	}
	config, err := c.GetConfig()
	if err != nil {
		return nil, buff, err
	}
	dm, err := discoverymapper.New(config)
	if err != nil {
		return nil, buff, err
	}
	var objs []oam.Object
	client, err := c.GetClient()
	if err != nil {
		return nil, buff, err
	}
	liveDiffOption := dryrun.NewLiveDiffOption(client, dm, pd, objs)
	diffResult, err := liveDiffOption.DiffApps(ctx, newApp, oldApp)
	if err != nil {
		return nil, buff, err
	}
	reportDiffOpt := dryrun.NewReportDiffOption(10, &buff)
	reportDiffOpt.PrintDiffReport(diffResult)
	return diffResult, buff, nil
}
