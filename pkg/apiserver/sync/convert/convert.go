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

package convert

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

// FromCRComponent concerts Application CR Component object into velaux data store component
func FromCRComponent(appPrimaryKey string, component common.ApplicationComponent) (model.ApplicationComponent, error) {
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

// FromCRPolicy converts Application CR Policy object into velaux data store policy
func FromCRPolicy(appPrimaryKey string, policyCR v1beta1.AppPolicy, creator string) (model.ApplicationPolicy, error) {
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

// FromCRWorkflow converts Application CR Workflow section into velaux data store workflow
func FromCRWorkflow(ctx context.Context, cli client.Client, appPrimaryKey string, app *v1beta1.Application) (model.Workflow, []v1beta1.WorkflowStep, error) {
	dataWf := model.Workflow{
		AppPrimaryKey: appPrimaryKey,
		// every namespace has a synced env
		EnvName: model.AutoGenEnvNamePrefix + app.Namespace,
		// every application has a synced workflow
		Name:        model.AutoGenWorkflowNamePrefix + appPrimaryKey,
		Alias:       model.AutoGenWorkflowNamePrefix + app.Name,
		Description: model.AutoGenDesc,
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
		steps = step.ConvertSteps(wf.Steps)
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

// FromCRTargets converts deployed Cluster/Namespace from Application CR Status into velaux data store
func FromCRTargets(targetApp *v1beta1.Application) []*model.Target {
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
