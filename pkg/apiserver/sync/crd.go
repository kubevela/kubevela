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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	utils2 "github.com/oam-dev/kubevela/pkg/utils"
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
func ConvertFromCRWorkflow(ctx context.Context, cli client.Client, app *v1beta1.Application) (model.Workflow, []v1beta1.WorkflowStep, error) {
	dataWf := model.Workflow{
		AppPrimaryKey: app.Name,
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
			cluster = "local"
		}
		name := "auto-sync-" + cluster + "-" + v.Namespace
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
func ConvertApp2DatastoreApp(ctx context.Context, cli client.Client, targetApp *v1beta1.Application) (*model.DataStoreApp, error) {

	project := model.DefaultInitName

	if _, ok := targetApp.Labels[oam.LabelAddonName]; ok && strings.HasPrefix(targetApp.Name, "addon-") {
		project = model.DefaultAddonProject
	}

	// 1. convert app meta and env
	dsApp := &model.DataStoreApp{
		Name: targetApp.Name,
		AppMeta: &model.Application{
			Name:        targetApp.Name,
			Description: model.AutoGenDesc,
			Project:     project,
		},
		Env: &model.Env{
			Name:        model.AutoGenEnvNamePrefix + targetApp.Namespace,
			Namespace:   targetApp.Namespace,
			Description: model.AutoGenDesc,
			Project:     model.DefaultInitName,
		},
		Eb: &model.EnvBinding{
			AppPrimaryKey: targetApp.Name,
			Name:          model.AutoGenEnvNamePrefix + targetApp.Namespace,
		},
	}

	// 2. convert component
	for _, cmp := range targetApp.Spec.Components {
		compModel, err := ConvertFromCRComponent(targetApp.Name, cmp)
		if err != nil {
			return nil, err
		}
		dsApp.Comps = append(dsApp.Comps, &compModel)
	}

	// 3. convert policy
	for _, plc := range targetApp.Spec.Policies {
		plcModel, err := ConvertFromCRPolicy(targetApp.Name, plc, model.AutoGenPolicy)
		if err != nil {
			return nil, err
		}
		dsApp.Policies = append(dsApp.Policies, &plcModel)
	}

	// 4. convert workflow
	wf, steps, err := ConvertFromCRWorkflow(ctx, cli, targetApp)
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
		plcModel, err := ConvertFromCRPolicy(targetApp.Name, plc, model.AutoGenRefPolicy)
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
func CheckSoTFromAppMeta(ctx context.Context, ds datastore.DataStore, appName string, sotFromCR string) string {
	app := &model.Application{Name: appName}
	err := ds.Get(ctx, app)
	if err != nil {
		return sotFromCR
	}
	if app.Labels == nil || app.Labels[SoT] == "" {
		return sotFromCR
	}
	return app.Labels[SoT]
}

// StoreAppMeta will sync application metadata from CR to datastore
func StoreAppMeta(ctx context.Context, app *model.DataStoreApp, ds datastore.DataStore) error {
	err := ds.Get(ctx, &model.Application{Name: app.Name})
	if err == nil {
		// it means the record already exists, don't need to add anything
		return nil
	}
	if !errors.Is(err, datastore.ErrRecordNotExist) {
		// other database error, return it
		return err
	}
	return ds.Add(ctx, app.AppMeta)
}

// StoreEnv will sync application namespace from CR to datastore env, one namespace belongs to one env
func StoreEnv(ctx context.Context, app *model.DataStoreApp, ds datastore.DataStore) error {
	curEnv := &model.Env{Name: app.Env.Name}
	err := ds.Get(ctx, curEnv)
	if err == nil {
		// it means the record already exists, compare the targets
		if utils2.EqualSlice(curEnv.Targets, app.Env.Targets) {
			return nil
		}
		return ds.Put(ctx, app.Env)
	}
	if !errors.Is(err, datastore.ErrRecordNotExist) {
		// other database error, return it
		return err
	}
	return ds.Add(ctx, app.Env)
}

// StoreEnvBinding will add envbinding for application CR one application one envbinding
func StoreEnvBinding(ctx context.Context, eb *model.EnvBinding, ds datastore.DataStore) error {
	err := ds.Get(ctx, eb)
	if err == nil {
		// it means the record already exists, don't need to add anything
		return nil
	}
	if !errors.Is(err, datastore.ErrRecordNotExist) {
		// other database error, return it
		return err
	}
	return ds.Add(ctx, eb)
}

// StoreComponents will sync application components from CR to datastore
func StoreComponents(ctx context.Context, appPrimaryKey string, expComps []*model.ApplicationComponent, ds datastore.DataStore) error {

	// list the existing components in datastore
	originComps, err := ds.List(ctx, &model.ApplicationComponent{AppPrimaryKey: appPrimaryKey}, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	var originCompNames []string
	for _, entity := range originComps {
		comp := entity.(*model.ApplicationComponent)
		originCompNames = append(originCompNames, comp.Name)
	}

	var targetCompNames []string
	for _, comp := range expComps {
		targetCompNames = append(targetCompNames, comp.Name)
	}

	_, readyToDelete, readyToAdd := utils2.CompareSlices(originCompNames, targetCompNames)

	// delete the components that not belongs to the new app
	for _, entity := range originComps {
		comp := entity.(*model.ApplicationComponent)
		// we only compare for components that automatically generated by sync process.
		if comp.Creator != model.AutoGenComp {
			continue
		}
		if !utils.StringsContain(readyToDelete, comp.Name) {
			continue
		}
		if err := ds.Delete(ctx, comp); err != nil {
			if errors.Is(err, datastore.ErrRecordNotExist) {
				continue
			}
			log.Logger.Warnf("delete comp %s for app %s  failure %s", comp.Name, appPrimaryKey, err.Error())
		}
	}

	// add or update new app's components for datastore
	for _, comp := range expComps {
		if utils.StringsContain(readyToAdd, comp.Name) {
			err = ds.Add(ctx, comp)
		} else {
			err = ds.Put(ctx, comp)
		}
		if err != nil {
			log.Logger.Warnf("convert comp %s for app %s into datastore failure %s", comp.Name, utils2.Sanitize(appPrimaryKey), err.Error())
			return err
		}
	}
	return nil
}

// StorePolicy will add/update/delete policies, we don't delete ref policy
func StorePolicy(ctx context.Context, appPrimaryKey string, expPolicies []*model.ApplicationPolicy, ds datastore.DataStore) error {
	// list the existing policies for this app in datastore
	originPolicies, err := ds.List(ctx, &model.ApplicationPolicy{AppPrimaryKey: appPrimaryKey}, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	var originPolicyNames []string
	for _, entity := range originPolicies {
		plc := entity.(*model.ApplicationPolicy)
		originPolicyNames = append(originPolicyNames, plc.Name)
	}

	var targetPLCNames []string
	for _, plc := range expPolicies {
		targetPLCNames = append(targetPLCNames, plc.Name)
	}

	_, readyToDelete, readyToAdd := utils2.CompareSlices(originPolicyNames, targetPLCNames)

	// delete the components that not belongs to the new app
	for _, entity := range originPolicies {
		plc := entity.(*model.ApplicationPolicy)
		// we only compare for policies that automatically generated by sync process, and the policy should not be ref ones.
		if plc.Creator != model.AutoGenPolicy {
			continue
		}
		if !utils.StringsContain(readyToDelete, plc.Name) {
			continue
		}
		if err := ds.Delete(ctx, plc); err != nil {
			if errors.Is(err, datastore.ErrRecordNotExist) {
				continue
			}
			log.Logger.Warnf("delete policy %s for app %s failure %s", plc.Name, appPrimaryKey, err.Error())
		}
	}

	// add or update new app's policies for datastore
	for _, plc := range expPolicies {
		if utils.StringsContain(readyToAdd, plc.Name) {
			err = ds.Add(ctx, plc)
		} else {
			err = ds.Put(ctx, plc)
		}
		if err != nil {
			log.Logger.Warnf("convert policy %s for app %s into datastore failure %s", plc.Name, utils2.Sanitize(appPrimaryKey), err.Error())
			return err
		}
	}
	return nil
}

// StoreWorkflow will sync workflow application CR to datastore, it updates the only one workflow from the application with specified name
func StoreWorkflow(ctx context.Context, targetApp *model.DataStoreApp, ds datastore.DataStore) error {
	err := ds.Get(ctx, &model.Workflow{AppPrimaryKey: targetApp.Name, Name: targetApp.Workflow.Name})
	if err == nil {
		// it means the record already exists, update it
		return ds.Put(ctx, targetApp.Workflow)
	}
	if !errors.Is(err, datastore.ErrRecordNotExist) {
		// other database error, return it
		return err
	}
	return ds.Add(ctx, targetApp.Workflow)
}

// StoreTargets will sync targets from application CR to datastore
func StoreTargets(ctx context.Context, targetApp *model.DataStoreApp, ds datastore.DataStore) error {
	for _, t := range targetApp.Targets {
		err := ds.Get(ctx, t)
		if err == nil {
			continue
		}
		if !errors.Is(err, datastore.ErrRecordNotExist) {
			// other database error, return it
			return err
		}
		if err = ds.Add(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

// Store2UXAuto will sync application CR to storage of VelaUX automatically
func Store2UXAuto(ctx context.Context, cli client.Client, targetApp *v1beta1.Application, ds datastore.DataStore) error {
	sot := CheckSoTFromCR(targetApp)

	// This is a double check to make sure the app not be converted and un-deployed
	sot = CheckSoTFromAppMeta(ctx, ds, targetApp.Name, sot)

	switch sot {
	case FromUX, FromInner:
		// we don't sync if the application is not created from CR
		return nil
	default:
	}

	dsApp, err := ConvertApp2DatastoreApp(ctx, cli, targetApp)
	if err != nil {
		log.Logger.Errorf("Convert App to data store err %v", err)
		return err
	}

	if err = StoreAppMeta(ctx, dsApp, ds); err != nil {
		log.Logger.Errorf("Store App Metadata to data store err %v", err)
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
	if err = StoreComponents(ctx, dsApp.Name, dsApp.Comps, ds); err != nil {
		log.Logger.Errorf("Store Components Metadata to data store err %v", err)
		return err
	}
	if err = StorePolicy(ctx, dsApp.Name, dsApp.Policies, ds); err != nil {
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
	return nil
}

// DeleteApp will delete the application as the CR was deleted
func DeleteApp(ctx context.Context, targetApp *v1beta1.Application, ds datastore.DataStore) error {
	sot := CheckSoTFromCR(targetApp)

	// This is a double check to make sure the app not be converted and un-deployed
	sot = CheckSoTFromAppMeta(ctx, ds, targetApp.Name, sot)

	switch sot {
	case FromUX, FromInner:
		// we don't sync if the application is not created from CR
		return nil
	default:
	}

	_ = ds.Delete(ctx, &model.Application{Name: targetApp.Name})

	_ = ds.Delete(ctx, &model.Env{Name: model.AutoGenEnvNamePrefix + targetApp.Namespace})

	cmps, err := ds.List(ctx, &model.ApplicationComponent{AppPrimaryKey: targetApp.Name}, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	for _, entity := range cmps {
		comp := entity.(*model.ApplicationComponent)
		if comp.Creator == model.AutoGenComp {
			_ = ds.Delete(ctx, comp)
		}
	}

	plcs, err := ds.List(ctx, &model.ApplicationPolicy{AppPrimaryKey: targetApp.Name}, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	for _, entity := range plcs {
		comp := entity.(*model.ApplicationPolicy)
		if comp.Creator == model.AutoGenPolicy {
			_ = ds.Delete(ctx, comp)
		}
	}

	_ = ds.Delete(ctx, &model.Workflow{Name: model.AutoGenWorkflowNamePrefix + targetApp.Name})

	return nil
}
