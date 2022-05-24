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

package v1

import (
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

// ConvertEnvBindingModelToBase assemble the DTO from EnvBinding model
func ConvertEnvBindingModelToBase(envBinding *model.EnvBinding, env *model.Env, targets []*model.Target) *apisv1.EnvBindingBase {
	var dtMap = make(map[string]*model.Target, len(targets))
	for _, dte := range targets {
		dtMap[dte.Name] = dte
	}
	var envBindingTargets []apisv1.EnvBindingTarget
	for _, targetName := range env.Targets {
		dt := dtMap[targetName]
		if dt != nil {
			ebt := apisv1.EnvBindingTarget{
				NameAlias: apisv1.NameAlias{Name: dt.Name, Alias: dt.Alias},
			}
			if dt.Cluster != nil {
				ebt.Cluster = &apisv1.ClusterTarget{
					ClusterName: dt.Cluster.ClusterName,
					Namespace:   dt.Cluster.Namespace,
				}
			}
			envBindingTargets = append(envBindingTargets, ebt)
		}
	}
	ebb := &apisv1.EnvBindingBase{
		Name:               envBinding.Name,
		Alias:              env.Alias,
		Description:        env.Description,
		TargetNames:        env.Targets,
		Targets:            envBindingTargets,
		CreateTime:         envBinding.CreateTime,
		UpdateTime:         envBinding.UpdateTime,
		AppDeployName:      envBinding.AppDeployName,
		AppDeployNamespace: env.Namespace,
	}
	return ebb
}

// ConvertAppModelToBase assemble the Application model to DTO
func ConvertAppModelToBase(app *model.Application, projects []*apisv1.ProjectBase) *apisv1.ApplicationBase {
	appBase := &apisv1.ApplicationBase{
		Name:        app.Name,
		Alias:       app.Alias,
		CreateTime:  app.CreateTime,
		UpdateTime:  app.UpdateTime,
		Description: app.Description,
		Icon:        app.Icon,
		Labels:      app.Labels,
		Project:     &apisv1.ProjectBase{Name: app.Project},
	}
	if app.IsSynced() {
		appBase.ReadOnly = true
	}
	for _, project := range projects {
		if project.Name == app.Project {
			appBase.Project = project
		}
	}
	return appBase
}

// ConvertComponentModelToBase assemble the ApplicationComponent model to DTO
func ConvertComponentModelToBase(componentModel *model.ApplicationComponent) *apisv1.ComponentBase {
	if componentModel == nil {
		return nil
	}
	return &apisv1.ComponentBase{
		Name:          componentModel.Name,
		Alias:         componentModel.Alias,
		Description:   componentModel.Description,
		Labels:        componentModel.Labels,
		ComponentType: componentModel.Type,
		Icon:          componentModel.Icon,
		DependsOn:     componentModel.DependsOn,
		Inputs:        componentModel.Inputs,
		Outputs:       componentModel.Outputs,
		Creator:       componentModel.Creator,
		Main:          componentModel.Main,
		CreateTime:    componentModel.CreateTime,
		UpdateTime:    componentModel.UpdateTime,
		Traits: func() (traits []*apisv1.ApplicationTrait) {
			for _, trait := range componentModel.Traits {
				traits = append(traits, &apisv1.ApplicationTrait{
					Type:        trait.Type,
					Properties:  trait.Properties,
					Alias:       trait.Alias,
					Description: trait.Description,
					CreateTime:  trait.CreateTime,
					UpdateTime:  trait.UpdateTime,
				})
			}
			return
		}(),
		WorkloadType: componentModel.WorkloadType,
	}
}

// ConvertRevisionModelToBase assemble the ApplicationRevision model to DTO
func ConvertRevisionModelToBase(revision *model.ApplicationRevision, user *model.User) apisv1.ApplicationRevisionBase {
	base := apisv1.ApplicationRevisionBase{
		Version:     revision.Version,
		Status:      revision.Status,
		Reason:      revision.Reason,
		Note:        revision.Note,
		TriggerType: revision.TriggerType,
		CreateTime:  revision.CreateTime,
		EnvName:     revision.EnvName,
		CodeInfo:    revision.CodeInfo,
		ImageInfo:   revision.ImageInfo,
		DeployUser:  &apisv1.NameAlias{Name: revision.DeployUser},
	}
	if user != nil {
		base.DeployUser.Alias = user.Alias
	}
	return base
}

// ConvertFromRecordModel assemble the WorkflowRecord model to DTO
func ConvertFromRecordModel(record *model.WorkflowRecord) *apisv1.WorkflowRecord {
	return &apisv1.WorkflowRecord{
		Name:                record.Name,
		Namespace:           record.Namespace,
		WorkflowName:        record.WorkflowName,
		WorkflowAlias:       record.WorkflowAlias,
		ApplicationRevision: record.RevisionPrimaryKey,
		StartTime:           record.StartTime,
		Status:              record.Status,
		Steps:               record.Steps,
	}
}

// ConvertFromWorkflowStepModel assemble the WorkflowStep model to DTO
func ConvertFromWorkflowStepModel(step model.WorkflowStep) apisv1.WorkflowStep {
	apiStep := apisv1.WorkflowStep{
		Name:        step.Name,
		Type:        step.Type,
		Alias:       step.Alias,
		Description: step.Description,
		Inputs:      step.Inputs,
		Outputs:     step.Outputs,
		Properties:  step.Properties.JSON(),
		DependsOn:   step.DependsOn,
	}
	if step.Properties != nil {
		apiStep.Properties = step.Properties.JSON()
	}
	return apiStep
}

// ConvertWorkflowBase assemble the Workflow model to DTO
func ConvertWorkflowBase(workflow *model.Workflow) apisv1.WorkflowBase {
	var steps []apisv1.WorkflowStep
	for _, step := range workflow.Steps {
		steps = append(steps, ConvertFromWorkflowStepModel(step))
	}
	return apisv1.WorkflowBase{
		Name:        workflow.Name,
		Alias:       workflow.Alias,
		Description: workflow.Description,
		Default:     convertBool(workflow.Default),
		EnvName:     workflow.EnvName,
		CreateTime:  workflow.CreateTime,
		UpdateTime:  workflow.UpdateTime,
		Steps:       steps,
	}
}

// ConvertPolicyModelToBase assemble the ApplicationPolicy model to DTO
func ConvertPolicyModelToBase(policy *model.ApplicationPolicy) *apisv1.PolicyBase {
	pb := &apisv1.PolicyBase{
		Name:        policy.Name,
		Type:        policy.Type,
		Properties:  policy.Properties,
		Description: policy.Description,
		Creator:     policy.Creator,
		CreateTime:  policy.CreateTime,
		UpdateTime:  policy.UpdateTime,
		EnvName:     policy.EnvName,
	}
	return pb
}

func convertBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
