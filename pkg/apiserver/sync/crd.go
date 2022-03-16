/*
 Copyright 2022 The KubeVela Authors.

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

package sync

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

const (
	// FromCR means the data source of truth is from k8s CR
	FromCR = "from-CR"
	// FromUX means the data source of truth is from velaux data store
	FromUX = "from-UX"
	// FromInner means the data source of truth is from KubeVela inner usage, such as addon or configuration that don't want to be synced
	FromInner = "from-inner"

	// SoT means the source of Truth from
	SoT = "SourceOfTruth"
)

// ConvertFromCRComponent concerts Application CR Component object into velaux data store component
func ConvertFromCRComponent(appPrimaryKey string, component common.ApplicationComponent) (model.ApplicationComponent, error) {
	bc := model.ApplicationComponent{
		AppPrimaryKey:    appPrimaryKey,
		Name:             component.Name,
		Type:             component.Type,
		ExternalRevision: component.ExternalRevision,
		DependsOn:        component.DependsOn,
		Inputs:           component.Inputs,
		Outputs:          component.Outputs,
		Scopes:           component.Scopes,
		Creator:          model.AutoGenComp,
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

// ConvertFromCRPolicy converts Application CR Policy object into velaux data store policy
func ConvertFromCRPolicy(appPrimaryKey string, policyCR v1beta1.AppPolicy, creator string) (model.ApplicationPolicy, error) {
	plc := model.ApplicationPolicy{
		AppPrimaryKey: appPrimaryKey,
		Name:          policyCR.Name,
		Type:          policyCR.Type,
		Creator:       creator,
	}
	if policyCR.Properties != nil {
		properties, err := model.NewJSONStruct(policyCR.Properties)
		if err != nil {
			return plc, err
		}
		plc.Properties = properties
	}
	return plc, nil
}

// ConvertFromCRWorkflow converts Application CR Workflow section into velaux data store workflow
func ConvertFromCRWorkflow(ctx context.Context, cli client.Client, appPrimaryKey string, app *v1beta1.Application) (model.Workflow, []v1beta1.WorkflowStep, error) {
	dataWf := model.Workflow{
		AppPrimaryKey: appPrimaryKey,
		// every namespace has a synced env
		EnvName: model.AutoGenEnvNamePrefix + app.Namespace,
		// every application has a synced workflow
		Name: model.AutoGenWorkflowNamePrefix + app.Name,
	}
	if app.Spec.Workflow == nil {
		return dataWf, nil, nil
	}
	var steps []v1beta1.WorkflowStep
	if app.Spec.Workflow.Ref != "" {
		dataWf.Name = app.Spec.Workflow.Ref
		wf := &v1alpha1.Workflow{}
		if err := cli.Get(ctx, types.NamespacedName{Namespace: app.GetNamespace(), Name: app.Spec.Workflow.Ref}, wf); err != nil {
			return dataWf, nil, err
		}
		steps = wf.Steps
	} else {
		steps = app.Spec.Workflow.Steps
	}
	for _, s := range steps {
		if s.Properties == nil {
			continue
		}
		properties, err := model.NewJSONStruct(s.Properties)
		if err != nil {
			return dataWf, nil, err
		}
		dataWf.Steps = append(dataWf.Steps, model.WorkflowStep{
			Name:       s.Name,
			Type:       s.Type,
			Inputs:     s.Inputs,
			Outputs:    s.Outputs,
			DependsOn:  s.DependsOn,
			Properties: properties,
		})
	}
	return dataWf, steps, nil
}

// ConvertFromCRTargets converts deployed Cluster/Namespace from Application CR Status into velaux data store
func ConvertFromCRTargets(targetApp *v1beta1.Application) []*model.Target {
	var targets []*model.Target
	nc := make(map[string]struct{})
	for _, v := range targetApp.Status.AppliedResources {
		var cluster = v.Cluster
		if cluster == "" {
			cluster = multicluster.ClusterLocalName
		}
		name := model.AutoGenTargetNamePrefix + cluster + "-" + v.Namespace
		if _, ok := nc[name]; ok {
			continue
		}
		nc[name] = struct{}{}
		targets = append(targets, &model.Target{
			Name: name,
			Cluster: &model.ClusterTarget{
				ClusterName: cluster,
				Namespace:   v.Namespace,
			},
		})
	}
	return targets
}

// ConvertApp2DatastoreApp will convert Application CR to datastore application related resources
func ConvertApp2DatastoreApp(ctx context.Context, cli client.Client, targetApp *v1beta1.Application, ds datastore.DataStore) (*model.DataStoreApp, error) {

	project := model.DefaultInitName
	if _, ok := targetApp.Labels[oam.LabelAddonName]; ok && strings.HasPrefix(targetApp.Name, "addon-") {
		project = model.DefaultAddonProject
	}
	appMeta := &model.Application{
		Name:        targetApp.Name,
		Description: model.AutoGenDesc,
		Alias:       targetApp.Name,
		Project:     project,
		Labels: map[string]string{
			model.LabelSyncNamespace:  targetApp.Namespace,
			model.LabelSyncGeneration: strconv.FormatInt(targetApp.Generation, 10),
		},
	}

	existApp := &model.Application{Name: targetApp.Name}
	err := ds.Get(ctx, existApp)
	if err == nil {
		namespace := existApp.Labels[model.LabelSyncNamespace]
		if namespace != targetApp.Namespace {
			appMeta.Name = formatAppComposedName(targetApp.Name, targetApp.Namespace)
		}
	}
	if err != nil && !errors.Is(err, datastore.ErrRecordNotExist) {
		return nil, err
	}

	// 1. convert app meta and env
	dsApp := &model.DataStoreApp{
		AppMeta: appMeta,
		Env: &model.Env{
			Name:        model.AutoGenEnvNamePrefix + targetApp.Namespace,
			Namespace:   targetApp.Namespace,
			Description: model.AutoGenDesc,
			Project:     project,
		},
		Eb: &model.EnvBinding{
			AppPrimaryKey: appMeta.PrimaryKey(),
			Name:          model.AutoGenEnvNamePrefix + targetApp.Namespace,
		},
	}

	// 2. convert component
	for _, cmp := range targetApp.Spec.Components {
		compModel, err := ConvertFromCRComponent(appMeta.PrimaryKey(), cmp)
		if err != nil {
			return nil, err
		}
		dsApp.Comps = append(dsApp.Comps, &compModel)
	}

	// 3. convert policy
	for _, plc := range targetApp.Spec.Policies {
		plcModel, err := ConvertFromCRPolicy(appMeta.PrimaryKey(), plc, model.AutoGenPolicy)
		if err != nil {
			return nil, err
		}
		dsApp.Policies = append(dsApp.Policies, &plcModel)
	}

	// 4. convert workflow
	wf, steps, err := ConvertFromCRWorkflow(ctx, cli, appMeta.PrimaryKey(), targetApp)
	if err != nil {
		return nil, err
	}
	dsApp.Workflow = &wf

	// 5. some policies are references in workflow step, we need to sync all the outside policy to make that work
	outsidePLC, err := step.LoadExternalPoliciesForWorkflow(ctx, cli, targetApp.Namespace, steps, targetApp.Spec.Policies)
	if err != nil {
		return nil, err
	}
	for _, plc := range outsidePLC {
		plcModel, err := ConvertFromCRPolicy(appMeta.PrimaryKey(), plc, model.AutoGenRefPolicy)
		if err != nil {
			return nil, err
		}
		dsApp.Policies = append(dsApp.Policies, &plcModel)
	}

	// 6. extract targets from status
	dsApp.Targets = ConvertFromCRTargets(targetApp)
	for _, t := range dsApp.Targets {
		dsApp.Env.Targets = append(dsApp.Env.Targets, t.Name)
	}
	return dsApp, nil
}

// CheckSoTFromCR will check the source of truth of the application
func CheckSoTFromCR(targetApp *v1beta1.Application) string {

	if _, innerUse := targetApp.Annotations[oam.AnnotationSOTFromInner]; innerUse {
		return FromInner
	}
	if _, appName := targetApp.Annotations[oam.AnnotationAppName]; appName {
		return FromUX
	}
	return FromCR
}

// CheckSoTFromAppMeta will check the source of truth marked in datastore
func CheckSoTFromAppMeta(ctx context.Context, ds datastore.DataStore, appName, namespace string, sotFromCR string) string {

	app := &model.Application{Name: formatAppComposedName(appName, namespace)}
	err := ds.Get(ctx, app)
	if err != nil {
		app = &model.Application{Name: appName}
		err = ds.Get(ctx, app)
		if err != nil {
			return sotFromCR
		}
	}
	if app.Labels == nil || app.Labels[SoT] == "" {
		return sotFromCR
	}
	return app.Labels[SoT]
}

// CR2UX provides the Add/Update/Delete method
type CR2UX struct {
	ds    datastore.DataStore
	cli   client.Client
	cache sync.Map
}

func formatAppComposedName(name, namespace string) string {
	return name + "-" + namespace
}

func (c *CR2UX) getAppMetaName(ctx context.Context, name, namespace string) string {
	existApp := &model.Application{Name: name}
	err := c.ds.Get(ctx, existApp)
	if err == nil {
		en := existApp.Labels[model.LabelSyncNamespace]
		if en != namespace {
			return formatAppComposedName(name, namespace)
		}
	}
	return name
}

// Init will initialize the cache
func (c *CR2UX) Init(ctx context.Context) error {
	appsRaw, err := c.ds.List(ctx, &model.Application{}, &datastore.ListOptions{})
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}
	for _, appR := range appsRaw {
		app, ok := appR.(*model.Application)
		if !ok {
			continue
		}
		gen, ok := app.Labels[model.LabelSyncGeneration]
		if !ok || gen == "" {
			continue
		}
		namespace := app.Labels[model.LabelSyncNamespace]
		var key = formatAppComposedName(app.Name, namespace)
		if strings.HasSuffix(app.Name, namespace) {
			key = app.Name
		}
		c.cache.Store(key, gen)
	}
	return nil
}

// AddOrUpdate will sync application CR to storage of VelaUX automatically
func (c *CR2UX) AddOrUpdate(ctx context.Context, targetApp *v1beta1.Application) error {
	ds := c.ds
	cli := c.cli

	if !c.shouldSync(ctx, targetApp, false) {
		return nil
	}

	dsApp, err := ConvertApp2DatastoreApp(ctx, cli, targetApp, ds)
	if err != nil {
		log.Logger.Errorf("Convert App to data store err %v", err)
		return err
	}

	if err = StoreProject(ctx, dsApp.AppMeta.Project, ds); err != nil {
		log.Logger.Errorf("get or create project for sync process err %v", err)
		return err
	}

	if err = StoreEnv(ctx, dsApp, ds); err != nil {
		log.Logger.Errorf("Store Env Metadata to data store err %v", err)
		return err
	}
	if err = StoreEnvBinding(ctx, dsApp.Eb, ds); err != nil {
		log.Logger.Errorf("Store EnvBinding Metadata to data store err %v", err)
		return err
	}
	if err = StoreComponents(ctx, dsApp.AppMeta.Name, dsApp.Comps, ds); err != nil {
		log.Logger.Errorf("Store Components Metadata to data store err %v", err)
		return err
	}
	if err = StorePolicy(ctx, dsApp.AppMeta.Name, dsApp.Policies, ds); err != nil {
		log.Logger.Errorf("Store Policy Metadata to data store err %v", err)
		return err
	}
	if err = StoreWorkflow(ctx, dsApp, ds); err != nil {
		log.Logger.Errorf("Store Workflow Metadata to data store err %v", err)
		return err
	}
	if err = StoreTargets(ctx, dsApp, ds); err != nil {
		log.Logger.Errorf("Store targets to data store err %v", err)
		return err
	}

	if err = StoreAppMeta(ctx, dsApp, ds); err != nil {
		log.Logger.Errorf("Store App Metadata to data store err %v", err)
		return err
	}

	// update cache
	c.updateCache(dsApp.AppMeta.PrimaryKey(), targetApp.Generation)
	return nil
}

// DeleteApp will delete the application as the CR was deleted
func (c *CR2UX) DeleteApp(ctx context.Context, targetApp *v1beta1.Application) error {
	ds := c.ds

	if !c.shouldSync(ctx, targetApp, true) {
		return nil
	}
	appName := c.getAppMetaName(ctx, targetApp.Name, targetApp.Namespace)

	_ = ds.Delete(ctx, &model.Application{Name: appName})

	cmps, err := ds.List(ctx, &model.ApplicationComponent{AppPrimaryKey: appName}, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	for _, entity := range cmps {
		comp := entity.(*model.ApplicationComponent)
		if comp.Creator == model.AutoGenComp {
			_ = ds.Delete(ctx, comp)
		}
	}

	plcs, err := ds.List(ctx, &model.ApplicationPolicy{AppPrimaryKey: appName}, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	for _, entity := range plcs {
		comp := entity.(*model.ApplicationPolicy)
		if comp.Creator == model.AutoGenPolicy {
			_ = ds.Delete(ctx, comp)
		}
	}

	_ = ds.Delete(ctx, &model.Workflow{Name: model.AutoGenWorkflowNamePrefix + appName})

	return nil
}
