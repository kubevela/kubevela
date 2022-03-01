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
	"errors"
	"fmt"

	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	utils2 "github.com/oam-dev/kubevela/pkg/utils"
)

// UpdateEnvWorkflow will update env workflow internally
func UpdateEnvWorkflow(ctx context.Context, kubeClient client.Client, ds datastore.DataStore, app *model.Application, env *model.Env) error {
	// The existing step configuration should be maintained and the delivery target steps should be automatically updated.
	envSteps := GenEnvWorkflowSteps(ctx, kubeClient, ds, env, app)
	workflow, err := getWorkflowForApp(ctx, ds, app, convertWorkflowName(env.Name))
	if err != nil {
		// no workflow exist mean no need to update
		if errors.Is(err, bcode.ErrWorkflowNotExist) {
			return nil
		}
		return err
	}

	var envStepNames = env.Targets
	var workflowStepNames []string
	for _, step := range workflow.Steps {
		if isEnvStepType(step.Type) {
			workflowStepNames = append(workflowStepNames, step.Name)
		}
	}

	var filteredSteps []apisv1.WorkflowStep
	_, readyToDeleteSteps, readyToAddSteps := compareSlices(workflowStepNames, envStepNames)

	for _, step := range workflow.Steps {
		if isEnvStepType(step.Type) && utils.StringsContain(readyToDeleteSteps, step.Name) {
			continue
		}
		filteredSteps = append(filteredSteps, convertFromWorkflowStepModel(step))
	}

	for _, step := range envSteps {
		if isEnvStepType(step.Type) && utils.StringsContain(readyToAddSteps, step.Name) {
			filteredSteps = append(filteredSteps, step)
		}
	}
	modelSteps, err := convertAPIStep2ModelStep(filteredSteps)
	if err != nil {
		return err
	}
	err = updateWorkflowSteps(ctx, ds, workflow, modelSteps)
	if err != nil {
		return err
	}
	return nil
}

// GetComponentDefinition will get componentDefinition by kube client
func GetComponentDefinition(ctx context.Context, kubeClient client.Client, name string) (*v1beta1.ComponentDefinition, error) {
	var componentDefinition v1beta1.ComponentDefinition
	if err := kubeClient.Get(ctx, k8stypes.NamespacedName{Namespace: types.DefaultKubeVelaNS, Name: name}, &componentDefinition); err != nil {
		return nil, err
	}
	return &componentDefinition, nil
}

// GetSuitableDeployWay will get a workflow deploy strategy for workflow step
func GetSuitableDeployWay(ctx context.Context, kubeClient client.Client, ds datastore.DataStore, app *model.Application) string {
	components, err := ds.List(ctx, &model.ApplicationComponent{AppPrimaryKey: app.PrimaryKey()}, &datastore.ListOptions{PageSize: 1, Page: 1})
	if err != nil {
		log.Logger.Errorf("list application component list failure %s", err.Error())
	}
	if len(components) > 0 {
		component := components[0].(*model.ApplicationComponent)
		definition, err := GetComponentDefinition(ctx, kubeClient, component.Type)
		if err != nil {
			log.Logger.Errorf("get component definition %s failure %s", component.Type, err.Error())
			// using Deploy2Env by default
		}
		if definition != nil {
			if definition.Spec.Workload.Type == TerraformWorkloadType {
				return DeployCloudResource
			}
			if definition.Spec.Workload.Definition.Kind == TerraformWorkloadKind {
				return DeployCloudResource
			}
		}
	}
	return Deploy2Env
}

// GenEnvWorkflowSteps will generate workflow steps for an env and application
func GenEnvWorkflowSteps(ctx context.Context, kubeClient client.Client, ds datastore.DataStore, env *model.Env, app *model.Application) []apisv1.WorkflowStep {
	var workflowSteps []v1beta1.WorkflowStep
	for _, targetName := range env.Targets {
		step := v1beta1.WorkflowStep{
			Name: genPolicyEnvName(targetName),
			Type: GetSuitableDeployWay(ctx, kubeClient, ds, app),
			Properties: util.Object2RawExtension(map[string]string{
				"policy": genPolicyName(env.Name),
				"env":    genPolicyEnvName(targetName),
			}),
		}
		workflowSteps = append(workflowSteps, step)
	}
	var steps []apisv1.WorkflowStep
	for _, step := range workflowSteps {
		var propertyStr string
		if step.Properties != nil {
			properties, err := model.NewJSONStruct(step.Properties)
			if err != nil {
				log.Logger.Errorf("workflow %s step %s properties is invalid %s", utils2.Sanitize(app.Name), utils2.Sanitize(step.Name), err.Error())
				continue
			}
			propertyStr = properties.JSON()
		}
		steps = append(steps, apisv1.WorkflowStep{
			Name:        step.Name,
			Type:        step.Type,
			Alias:       fmt.Sprintf("Deploy To %s", step.Name),
			Description: fmt.Sprintf("deploy app to delivery target %s", step.Name),
			DependsOn:   step.DependsOn,
			Properties:  propertyStr,
			Inputs:      step.Inputs,
			Outputs:     step.Outputs,
		})
	}
	return steps
}

func getWorkflowForApp(ctx context.Context, ds datastore.DataStore, app *model.Application, workflowName string) (*model.Workflow, error) {
	var workflow = model.Workflow{
		Name:          workflowName,
		AppPrimaryKey: app.PrimaryKey(),
	}
	if err := ds.Get(ctx, &workflow); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrWorkflowNotExist
		}
		return nil, err
	}
	return &workflow, nil
}
