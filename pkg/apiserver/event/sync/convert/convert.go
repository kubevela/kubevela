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

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/policy"
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
func FromCRWorkflow(ctx context.Context, cli client.Client, appPrimaryKey string, app *v1beta1.Application) (model.Workflow, []workflowv1alpha1.WorkflowStep, error) {
	var defaultWorkflow = true
	name := app.Annotations[oam.AnnotationWorkflowName]
	if name == "" {
		name = model.AutoGenWorkflowNamePrefix + appPrimaryKey
	}
	dataWf := model.Workflow{
		AppPrimaryKey: appPrimaryKey,
		// every namespace has a synced env
		EnvName: model.AutoGenEnvNamePrefix + app.Namespace,
		// every application has a synced workflow
		Name:        name,
		Alias:       model.AutoGenWorkflowNamePrefix + app.Name,
		Description: model.AutoGenDesc,
		Default:     &defaultWorkflow,
	}
	if app.Spec.Workflow == nil {
		return dataWf, nil, nil
	}
	var steps []workflowv1alpha1.WorkflowStep
	if app.Spec.Workflow.Ref != "" {
		dataWf.Name = app.Spec.Workflow.Ref
		wf := &workflowv1alpha1.Workflow{}
		if err := cli.Get(ctx, types.NamespacedName{Namespace: app.GetNamespace(), Name: app.Spec.Workflow.Ref}, wf); err != nil {
			return dataWf, nil, err
		}
		steps = wf.Steps
	} else {
		steps = app.Spec.Workflow.Steps
	}
	for _, s := range steps {
		base, err := FromCRWorkflowStepBase(s.WorkflowStepBase)
		if err != nil {
			return dataWf, nil, err
		}
		ws := model.WorkflowStep{
			WorkflowStepBase: *base,
			SubSteps:         make([]model.WorkflowStepBase, 0),
		}
		for _, sub := range s.SubSteps {
			subBase, err := FromCRWorkflowStepBase(sub)
			if err != nil {
				return dataWf, nil, err
			}
			ws.SubSteps = append(ws.SubSteps, *subBase)
		}
		dataWf.Steps = append(dataWf.Steps, ws)
	}
	return dataWf, steps, nil
}

// FromCRWorkflowStepBase convert cr to model
func FromCRWorkflowStepBase(step workflowv1alpha1.WorkflowStepBase) (*model.WorkflowStepBase, error) {
	base := &model.WorkflowStepBase{
		Name:      step.Name,
		Type:      step.Type,
		Inputs:    step.Inputs,
		Outputs:   step.Outputs,
		DependsOn: step.DependsOn,
		Meta:      step.Meta,
		If:        step.If,
		Timeout:   step.Timeout,
	}
	if step.Properties != nil {
		properties, err := model.NewJSONStruct(step.Properties)
		if err != nil {
			return nil, err
		}
		base.Properties = properties
	}
	return base, nil
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
		klog.Errorf("fail to get placements from topology policies %s", err.Error())
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
			StepStatus: model.StepStatus{
				Name:  step.Name,
				Alias: step.Alias,
				Type:  step.Type,
			},
		}
	}
	return &model.WorkflowRecord{
		WorkflowName:       workflow.Name,
		WorkflowAlias:      workflow.Alias,
		AppPrimaryKey:      workflow.AppPrimaryKey,
		Name:               strings.Replace(app.Status.Workflow.AppRevision, ":", "-", 1),
		Namespace:          app.Namespace,
		Finished:           model.UnFinished,
		StartTime:          app.Status.Workflow.StartTime.Time,
		EndTime:            app.Status.Workflow.EndTime.Time,
		RevisionPrimaryKey: revision.Version,
		Steps:              steps,
		Status:             model.RevisionStatusRunning,
	}
}

// FromCRWorkflowStepStatus convert the workflow step status to workflow step status
func FromCRWorkflowStepStatus(stepStatus workflowv1alpha1.StepStatus, alias string) model.StepStatus {
	return model.StepStatus{
		Name:             stepStatus.Name,
		Alias:            alias,
		ID:               stepStatus.ID,
		Type:             stepStatus.Type,
		Message:          stepStatus.Message,
		Reason:           stepStatus.Reason,
		Phase:            stepStatus.Phase,
		FirstExecuteTime: stepStatus.FirstExecuteTime.Time,
		LastExecuteTime:  stepStatus.LastExecuteTime.Time,
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
		klog.Errorf("failed to get the application revision %s", err.Error())
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
