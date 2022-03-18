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
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
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
		Name:  model.AutoGenWorkflowNamePrefix + app.Name,
		Alias: "Synced",
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
func (c *CR2UX) ConvertApp2DatastoreApp(ctx context.Context, targetApp *v1beta1.Application) (*model.DataStoreApp, error) {
	cli := c.cli

	appName := c.getAppMetaName(ctx, targetApp.Name, targetApp.Namespace)

	project := model.DefaultInitName
	if _, ok := targetApp.Labels[oam.LabelAddonName]; ok && strings.HasPrefix(targetApp.Name, "addon-") {
		project = model.DefaultAddonProject
	}
	appMeta := &model.Application{
		Name:        appName,
		Description: model.AutoGenDesc,
		Alias:       targetApp.Name,
		Project:     project,
		Labels: map[string]string{
			model.LabelSyncNamespace:  targetApp.Namespace,
			model.LabelSyncGeneration: strconv.FormatInt(targetApp.Generation, 10),
		},
	}

	// 1. convert app meta and env
	dsApp := &model.DataStoreApp{
		AppMeta: appMeta,
		Env: &model.Env{
			Name:        model.AutoGenEnvNamePrefix + targetApp.Namespace,
			Namespace:   targetApp.Namespace,
			Description: model.AutoGenDesc,
			Project:     project,
			Alias:       "Synced",
		},
		Eb: &model.EnvBinding{
			AppPrimaryKey: appMeta.PrimaryKey(),
			Name:          model.AutoGenEnvNamePrefix + targetApp.Namespace,
			AppDeployName: appMeta.GetAppNameForSynced(),
		},
	}

	// 2. convert component and trait
	for _, cmp := range targetApp.Spec.Components {
		compModel, err := ConvertFromCRComponent(appMeta.PrimaryKey(), cmp)
		if err != nil {
			return nil, err
		}
		dsApp.Comps = append(dsApp.Comps, &compModel)
	}

	// 3. convert workflow
	wf, steps, err := ConvertFromCRWorkflow(ctx, cli, appMeta.PrimaryKey(), targetApp)
	if err != nil {
		return nil, err
	}
	dsApp.Workflow = &wf

	// 4. convert policy, some policies are references in workflow step, we need to sync all the outside policy to make that work
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

	// TODO(wonderflow): we don't sync targets as it can't be judged well in velaux env
	// if we want to sync, we can extract targets from status, like below:
	/*
		dsApp.Targets = ConvertFromCRTargets(targetApp)
		for _, t := range dsApp.Targets {
			dsApp.Env.Targets = append(dsApp.Env.Targets, t.Name)
		}
	*/
	return dsApp, nil
}
