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
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/policy"
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
	var defaultWorkflow = true
	dataWf := model.Workflow{
		AppPrimaryKey: appPrimaryKey,
		// every namespace has a synced env
		EnvName: model.AutoGenEnvNamePrefix + app.Namespace,
		// every application has a synced workflow
		Name:        model.AutoGenWorkflowNamePrefix + appPrimaryKey,
		Alias:       model.AutoGenWorkflowNamePrefix + app.Name,
		Description: model.AutoGenDesc,
		Default:     &defaultWorkflow,
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
		ws := model.WorkflowStep{
			Name:      s.Name,
			Type:      s.Type,
			Inputs:    s.Inputs,
			Outputs:   s.Outputs,
			DependsOn: s.DependsOn,
		}
		if s.Properties != nil {
			properties, err := model.NewJSONStruct(s.Properties)
			if err != nil {
				return dataWf, nil, err
			}
			ws.Properties = properties
		}
		dataWf.Steps = append(dataWf.Steps, ws)
	}
	return dataWf, steps, nil
}

// FromCRTargets converts deployed Cluster/Namespace from Application CR Status into velaux data store
func FromCRTargets(ctx context.Context, cli client.Client, targetApp *v1beta1.Application, existTargets []datastore.Entity, project string) ([]*model.Target, map[string]string) {
	existTarget := make(map[string]*model.Target)
	for i := range existTargets {
		t := existTargets[i].(*model.Target)
		existTarget[fmt.Sprintf("%s-%s", t.Cluster.ClusterName, t.Cluster.Namespace)] = t
	}
	var targets []*model.Target
	var targetNames = map[string]string{}
	nc := make(map[string]struct{})
	// read the target from the topology policies
	placements, err := policy.GetPlacementsFromTopologyPolicies(ctx, cli, targetApp.Namespace, targetApp.Spec.Policies, true)
	if err != nil {
		log.Logger.Errorf("fail to get placements from topology policies %s", err.Error())
		return targets, targetNames
	}
	for _, placement := range placements {
		if placement.Cluster == "" {
			placement.Cluster = multicluster.ClusterLocalName
		}
		if placement.Namespace == "" {
			placement.Namespace = targetApp.Namespace
		}
		if placement.Namespace == "" {
			placement.Namespace = "default"
		}
		name := model.AutoGenTargetNamePrefix + placement.Cluster + "-" + placement.Namespace
		if _, ok := nc[name]; ok {
			continue
		}
		nc[name] = struct{}{}
		if target, ok := existTarget[fmt.Sprintf("%s-%s", placement.Cluster, placement.Namespace)]; ok {
			targetNames[target.Name] = target.Project
		} else {
			targetNames[name] = project
			targets = append(targets, &model.Target{
				Name:    name,
				Project: project,
				Cluster: &model.ClusterTarget{
					ClusterName: placement.Cluster,
					Namespace:   placement.Namespace,
				},
			})
		}
	}
	return targets, targetNames
}

// FromCRWorkflowRecord convert the workflow status to workflow record
func FromCRWorkflowRecord(app *v1beta1.Application, workflow model.Workflow, revision *model.ApplicationRevision) *model.WorkflowRecord {
	if app.Status.Workflow == nil || app.Status.Workflow.AppRevision == "" || revision == nil {
		return nil
	}
	steps := make([]model.WorkflowStepStatus, len(workflow.Steps))
	for i, step := range workflow.Steps {
		steps[i] = model.WorkflowStepStatus{
			Name:  step.Name,
			Alias: step.Alias,
			Type:  step.Type,
		}
	}
	return &model.WorkflowRecord{
		WorkflowName:       workflow.Name,
		WorkflowAlias:      workflow.Alias,
		AppPrimaryKey:      workflow.AppPrimaryKey,
		Name:               strings.Replace(app.Status.Workflow.AppRevision, ":", "-", 1),
		Namespace:          app.Namespace,
		Finished:           model.UnFinished,
		RevisionPrimaryKey: revision.Version,
		Steps:              steps,
		Status:             model.RevisionStatusRunning,
	}
}

// FromCRApplicationRevision convert the application revision to the revision in the data store
func FromCRApplicationRevision(ctx context.Context, cli client.Client, app *v1beta1.Application, workflow model.Workflow, envName string) *model.ApplicationRevision {
	if app.Status.Workflow == nil || app.Status.Workflow.AppRevision == "" {
		return nil
	}
	versions := strings.Split(app.Status.Workflow.AppRevision, ":")
	versionName := versions[0]
	var appRevision v1beta1.ApplicationRevision
	ctxTimeout, cancel := context.WithTimeout(ctx, time.Second*20)
	defer cancel()
	if err := cli.Get(ctxTimeout, types.NamespacedName{Namespace: app.Namespace, Name: versionName}, &appRevision); err != nil {
		log.Logger.Errorf("failed to get the application revision %s", err.Error())
		return nil
	}
	configByte, _ := yaml.Marshal(appRevision.Spec.Application)
	return &model.ApplicationRevision{
		BaseModel: model.BaseModel{
			CreateTime: appRevision.CreationTimestamp.Time,
			UpdateTime: time.Now(),
		},
		AppPrimaryKey:  workflow.AppPrimaryKey,
		RevisionCRName: appRevision.Name,
		WorkflowName:   workflow.Name,
		Version:        versionName,
		ApplyAppConfig: string(configByte),
		TriggerType:    "SyncFromCR",
		EnvName:        envName,
		Status:         model.RevisionStatusRunning,
	}
}
