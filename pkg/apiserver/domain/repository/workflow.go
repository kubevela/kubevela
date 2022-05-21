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

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	utils2 "github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

type state int

const (
	// keepState means this step or policy does not need to change
	keepState state = iota
	// newState means this step or policy needs to add
	newState
	// updateState means this step or policy needs to update with new
	updateState
	// deleteState means this step or policy needs to delete
	deleteState
	// modifyState means this step need to change the policies
	modifyState
)

const (
	// Deploy2Env deploy app to target cluster, suitable for common applications
	Deploy2Env string = "deploy2env"
	// DeployCloudResource deploy app to local and copy secret to target cluster, suitable for cloud application.
	DeployCloudResource string = "deploy-cloud-resource"
	// TerraformWorkloadType cloud application
	TerraformWorkloadType string = "configurations.terraform.core.oam.dev"
	// TerraformWorkloadKind terraform workload kind
	TerraformWorkloadKind string = "Configuration"
)

type workflowStep struct {
	name     string
	stepType string
	policies []*policy
	state    state
}

type policy struct {
	name       string
	policyType string
	targets    []string
	state      state
}

type steps []*workflowStep

func (s steps) getSteps(new, exist []model.WorkflowStep) []model.WorkflowStep {
	var existMaps = make(map[string]model.WorkflowStep, len(exist))
	var newMaps = make(map[string]model.WorkflowStep, len(new))
	for i, s := range exist {
		existMaps[s.Name] = exist[i]
	}
	for i, s := range new {
		newMaps[s.Name] = new[i]
	}
	var result []model.WorkflowStep
	for _, step := range s {
		switch step.state {
		case keepState:
			result = append(result, existMaps[step.name])
		case updateState, newState:
			result = append(result, newMaps[step.name])
		case modifyState:
			if step.stepType == "deploy" {
				modelStep := existMaps[step.name]
				var policies []string
				for _, p := range step.policies {
					if p.state != deleteState {
						policies = append(policies, p.name)
					}
				}
				(*modelStep.Properties)["policies"] = policies
				result = append(result, modelStep)
			}
		default:
		}
	}
	return result
}

// getPolicies return the created, updated and deleted policies
func (s steps) getPolicies(existPolicies, policies []datastore.Entity) (c, u, d []datastore.Entity) {
	var created, updated, deleted []datastore.Entity
	var tagDeleted = make(map[string]string)
	var tagCreated = make(map[string]string)
	var tagUpdated = make(map[string]string)
	var tagKept = make(map[string]string)
	for _, step := range s {
		for _, p := range step.policies {
			switch p.state {
			case deleteState:
				// maybe the multiple target use the one policy, so if the policy is keep, can not delete it
				if p.policyType == v1alpha1.EnvBindingPolicyType && tagKept[p.name] != "" {
					continue
				}
				tagDeleted[p.name] = p.name
			case newState:
				tagCreated[p.name] = p.name
			case updateState:
				tagUpdated[p.name] = p.name
			default:
				tagKept[p.name] = p.name
				// maybe the multiple target use the one policy, so if the policy is keep, can not delete it
				if _, ok := tagDeleted[p.name]; ok && p.policyType == v1alpha1.EnvBindingPolicyType {
					delete(tagDeleted, p.name)
				}
			}
		}
	}
	for i, p := range existPolicies {
		policy := p.(*model.ApplicationPolicy)
		if _, ok := tagDeleted[policy.Name]; ok {
			deleted = append(deleted, existPolicies[i])
			delete(tagDeleted, policy.Name)
		}
	}
	for i, p := range policies {
		policy := p.(*model.ApplicationPolicy)
		// if the policy in updated and created, only set to updated
		if _, ok := tagUpdated[policy.Name]; ok {
			updated = append(updated, policies[i])
			delete(tagUpdated, policy.Name)
			delete(tagCreated, policy.Name)
			continue
		}
		if _, ok := tagCreated[policy.Name]; ok {
			created = append(created, policies[i])
			delete(tagCreated, policy.Name)
		}
	}
	return created, updated, deleted
}

func (s steps) String() string {
	strBuffer := strings.Builder{}
	for _, step := range s {
		strBuffer.WriteString(fmt.Sprintf("step: %s/%s state: %v \n", step.name, step.stepType, step.state))
		for _, p := range step.policies {
			strBuffer.WriteString(fmt.Sprintf("\t policy: %s/%s state: %v targets: %v\n", p.name, p.policyType, p.state, p.targets))
		}
	}
	return strBuffer.String()
}

func createWorkflowSteps(steps []model.WorkflowStep, policies []datastore.Entity) steps {
	var policyMap = make(map[string]*policy)
	var workflowSteps []*workflowStep
	for _, entity := range policies {
		p := entity.(*model.ApplicationPolicy)
		switch p.Type {
		case v1alpha1.TopologyPolicyType:
			var targets []string
			var topology v1alpha1.TopologyPolicySpec
			if err := json.Unmarshal([]byte(p.Properties.JSON()), &topology); err != nil {
				continue
			}
			for _, clu := range topology.Clusters {
				targets = append(targets, fmt.Sprintf("%s/%s", clu, topology.Namespace))
			}
			policyMap[p.Name] = &policy{
				name:       p.Name,
				policyType: p.Type,
				targets:    targets,
			}
		case v1alpha1.EnvBindingPolicyType:
			var envBinding v1alpha1.EnvBindingSpec
			if err := json.Unmarshal([]byte(p.Properties.JSON()), &envBinding); err != nil {
				continue
			}
			for _, env := range envBinding.Envs {
				targets := []string{fmt.Sprintf("%s/%s", env.Placement.ClusterSelector.Name, env.Placement.NamespaceSelector.Name)}
				policyMap[p.Name+"-"+env.Name] = &policy{
					name:       p.Name,
					policyType: p.Type,
					targets:    targets,
				}
			}

		}
	}
	for _, wStep := range steps {
		switch wStep.Type {
		case step.DeployWorkflowStep:
			var deploySpec step.DeployWorkflowStepSpec
			if err := json.Unmarshal([]byte(wStep.Properties.JSON()), &deploySpec); err != nil {
				continue
			}
			var policies []*policy
			for _, policyName := range deploySpec.Policies {
				if _, ok := policyMap[policyName]; ok {
					policies = append(policies, policyMap[policyName])
				}
			}
			workflowSteps = append(workflowSteps, &workflowStep{
				name:     wStep.Name,
				stepType: wStep.Type,
				policies: policies,
			})
		case Deploy2Env, DeployCloudResource:
			var deploySpec = make(map[string]interface{})
			if err := json.Unmarshal([]byte(wStep.Properties.JSON()), &deploySpec); err != nil {
				continue
			}
			policyName, _ := deploySpec["policy"].(string)
			envName, _ := deploySpec["env"].(string)
			if policyName != "" {
				if p, ok := policyMap[policyName+"-"+envName]; ok {
					workflowSteps = append(workflowSteps, &workflowStep{
						name:     wStep.Name,
						stepType: wStep.Type,
						policies: []*policy{p},
					})
				}
			}
		default:
			workflowSteps = append(workflowSteps, &workflowStep{
				name:     wStep.Name,
				stepType: wStep.Type,
				state:    keepState,
			})
		}
	}
	return workflowSteps
}

// compareWorkflowSteps compare the old workflow steps with new workflow steps
// will set the workflow step and policy state
func compareWorkflowSteps(old, new steps) steps {
	var oldTargets, newTargets []string
	cacheTarget := func(t string, targets []string) []string {
		if t == DeployCloudResource {
			var re []string
			for _, t := range targets {
				re = append(re, fmt.Sprintf("c-"+t))
			}
			return re
		}
		return targets
	}
	for _, step := range old {
		for _, p := range step.policies {
			oldTargets = append(oldTargets, cacheTarget(step.stepType, p.targets)...)
		}
	}
	for _, step := range new {
		for _, p := range step.policies {
			newTargets = append(newTargets, cacheTarget(step.stepType, p.targets)...)
		}
	}

	_, needDeleted, needAdded := utils2.ThreeWaySliceCompare(oldTargets, newTargets)
	var workflowSteps []*workflowStep

	var deployCloudResourcePolicyExist = false
	for j := range old {
		oldStep := old[j]
		var deletedPolicyCount = 0
		for i := range oldStep.policies {
			p := oldStep.policies[i]
			if utils2.SliceIncludeSlice(needDeleted, cacheTarget(oldStep.stepType, p.targets)) {
				p.state = deleteState
				deletedPolicyCount++
			}
		}
		if deletedPolicyCount != 0 && deletedPolicyCount == len(oldStep.policies) {
			oldStep.state = deleteState
		} else if deletedPolicyCount != 0 {
			oldStep.state = modifyState
		}
		workflowSteps = append(workflowSteps, oldStep)
		if oldStep.stepType == DeployCloudResource {
			deployCloudResourcePolicyExist = true
		}
	}
	for j := range new {
		newStep := new[j]
		for i := range newStep.policies {
			p := newStep.policies[i]
			if utils2.SliceIncludeSlice(needAdded, cacheTarget(newStep.stepType, p.targets)) {
				if p.policyType == v1alpha1.EnvBindingPolicyType && deployCloudResourcePolicyExist {
					p.state = updateState
				} else {
					p.state = newState
				}
				newStep.state = newState
				if newStep.stepType == DeployCloudResource {
					workflowSteps = append([]*workflowStep{newStep}, workflowSteps...)
				} else {
					workflowSteps = append(workflowSteps, newStep)
				}
			}
		}
	}
	return workflowSteps
}

// UpdateEnvWorkflow will update env workflow internally
func UpdateEnvWorkflow(ctx context.Context, kubeClient client.Client, ds datastore.DataStore, app *model.Application, env *model.Env) error {
	// The existing step configuration should be maintained and the delivery target steps should be automatically updated.
	envSteps, policies := GenEnvWorkflowStepsAndPolicies(ctx, kubeClient, ds, env, app)
	workflow, err := GetWorkflowForApp(ctx, ds, app, ConvertWorkflowName(env.Name))
	if err != nil {
		// no workflow exist mean no need to update
		if errors.Is(err, bcode.ErrWorkflowNotExist) {
			return nil
		}
		return err
	}

	existPolicies, err := ds.List(ctx, &model.ApplicationPolicy{AppPrimaryKey: app.PrimaryKey()}, &datastore.ListOptions{
		FilterOptions: datastore.FilterOptions{
			In: []datastore.InQueryOption{{
				Key:    "type",
				Values: []string{v1alpha1.TopologyPolicyType, v1alpha1.EnvBindingPolicyType},
			}},
		},
	})
	if err != nil {
		return err
	}

	workflowSteps := compareWorkflowSteps(createWorkflowSteps(workflow.Steps, existPolicies), createWorkflowSteps(envSteps, policies))

	// update the workflow
	if err := UpdateWorkflowSteps(ctx, ds, workflow, workflowSteps.getSteps(envSteps, workflow.Steps)); err != nil {
		return fmt.Errorf("fail to update the workflow steps %w", err)
	}

	// update the policies
	created, updated, deleted := workflowSteps.getPolicies(existPolicies, policies)
	for _, d := range deleted {
		if err := ds.Delete(ctx, d); err != nil {
			log.Logger.Errorf("fail to delete the policy %s", err.Error())
		}
		log.Logger.Infof("deleted a policy %s where update the workflow", d.PrimaryKey())
	}

	if err := ds.BatchAdd(ctx, created); err != nil {
		log.Logger.Errorf("fail to create the policy %s", err.Error())
	}

	for _, d := range updated {
		if err := ds.Put(ctx, d); err != nil {
			log.Logger.Errorf("fail to update the policy %s", err.Error())
		}
		log.Logger.Infof("updated a policy %s where update the workflow", d.PrimaryKey())
	}

	return nil
}

// UpdateAppEnvWorkflow will update the all env workflows internally of the specified app
func UpdateAppEnvWorkflow(ctx context.Context, kubeClient client.Client, ds datastore.DataStore, app *model.Application) error {
	envbindings, err := ListEnvBindings(ctx, ds, EnvListOption{AppPrimaryKey: app.PrimaryKey(), ProjectName: app.Project})
	if err != nil {
		return err
	}
	var envNames []string
	for _, binding := range envbindings {
		envNames = append(envNames, binding.Name)
	}
	if len(envNames) == 0 {
		return nil
	}
	envs, err := ListEnvs(ctx, ds, &datastore.ListOptions{
		FilterOptions: datastore.FilterOptions{
			In: []datastore.InQueryOption{
				{
					Key:    "name",
					Values: envNames,
				},
			},
		},
	})
	if err != nil {
		return err
	}
	for i := range envs {
		if err := UpdateEnvWorkflow(ctx, kubeClient, ds, app, envs[i]); err != nil {
			log.Logger.Errorf("fail to update the env workflow %s", envs[i].PrimaryKey())
		}
	}
	log.Logger.Infof("The env workflows of app %s updated successfully", app.PrimaryKey())
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

// HaveTerraformWorkload there is at least one component with terraform workload
func HaveTerraformWorkload(ctx context.Context, kubeClient client.Client, components []datastore.Entity) (terraformComponents []*model.ApplicationComponent) {
	getComponentDeployType := func(component *model.ApplicationComponent) string {
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
		return step.DeployWorkflowStep
	}
	for _, component := range components {
		if getComponentDeployType(component.(*model.ApplicationComponent)) == DeployCloudResource {
			terraformComponents = append(terraformComponents, component.(*model.ApplicationComponent))
		}
	}
	return terraformComponents
}

func createOverriteConfigForTerraformComponent(env *model.Env, target *model.Target, terraformComponents []*model.ApplicationComponent) v1alpha1.EnvConfig {
	placement := v1alpha1.EnvPlacement{}
	if target.Cluster != nil {
		placement.ClusterSelector = &common.ClusterSelector{Name: target.Cluster.ClusterName}
		placement.NamespaceSelector = &v1alpha1.NamespaceSelector{Name: target.Cluster.Namespace}
	}
	var componentPatchs []v1alpha1.EnvComponentPatch
	// init cloud application region and provider info
	for _, component := range terraformComponents {
		properties := model.JSONStruct{
			"providerRef": map[string]interface{}{
				"name": "default",
			},
			"writeConnectionSecretToRef": map[string]interface{}{
				"name":      fmt.Sprintf("%s-%s", component.Name, env.Name),
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

	return v1alpha1.EnvConfig{
		Name:      genPolicyEnvName(target.Name),
		Placement: placement,
		Patch: v1alpha1.EnvPatch{
			Components: componentPatchs,
		},
	}
}

// GenEnvWorkflowStepsAndPolicies will generate workflow steps and policies for an env and application
func GenEnvWorkflowStepsAndPolicies(ctx context.Context, kubeClient client.Client, ds datastore.DataStore, env *model.Env, app *model.Application) ([]model.WorkflowStep, []datastore.Entity) {
	var workflowSteps []v1beta1.WorkflowStep
	var policies []datastore.Entity
	components, err := ds.List(ctx, &model.ApplicationComponent{AppPrimaryKey: app.PrimaryKey()}, nil)
	if err != nil {
		log.Logger.Errorf("list application component list failure %s", err.Error())
	}
	userName, _ := ctx.Value(&apisv1.CtxKeyUser).(string)
	terraformComponents := HaveTerraformWorkload(ctx, kubeClient, components)

	var target = model.Target{Project: env.Project}
	targets, err := ds.List(ctx, &target, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{
		In: []datastore.InQueryOption{{
			Key:    "name",
			Values: env.Targets,
		}},
	}})
	if err != nil {
		log.Logger.Errorf("fail to get the targets detail info, %s", err.Error())
	}
	if len(terraformComponents) > 0 {
		appPolicy := &model.ApplicationPolicy{
			AppPrimaryKey: app.PrimaryKey(),
			Name:          genPolicyName(env.Name),
			Description:   "auto generated",
			Type:          v1alpha1.EnvBindingPolicyType,
			Creator:       userName,
			EnvName:       env.Name,
		}
		var envs []v1alpha1.EnvConfig
		// gen workflow step and policies for all targets
		for i := range targets {
			target := targets[i].(*model.Target)
			step := v1beta1.WorkflowStep{
				Name: target.Name + "-cloud-resource",
				Type: DeployCloudResource,
				Properties: util.Object2RawExtension(map[string]string{
					"policy": genPolicyName(env.Name),
					"env":    genPolicyEnvName(target.Name),
				}),
			}
			workflowSteps = append(workflowSteps, step)
			envs = append(envs, createOverriteConfigForTerraformComponent(env, target, terraformComponents))
		}
		properties, err := model.NewJSONStructByStruct(v1alpha1.EnvBindingSpec{
			Envs: envs,
		})
		if err != nil {
			log.Logger.Errorf("fail to create the properties of the topology policy, %s", err.Error())
		} else {
			appPolicy.Properties = properties
			policies = append(policies, appPolicy)
		}
	}
	if len(components) > len(terraformComponents) || len(components) == 0 {
		// gen workflow step and policies for all targets
		for i := range targets {
			target := targets[i].(*model.Target)
			if target.Cluster == nil {
				continue
			}
			step := v1beta1.WorkflowStep{
				Name: target.Name,
				Type: step.DeployWorkflowStep,
				Properties: util.Object2RawExtension(map[string]interface{}{
					"policies": []string{target.Name},
				}),
			}
			workflowSteps = append(workflowSteps, step)
			appPolicy := &model.ApplicationPolicy{
				AppPrimaryKey: app.PrimaryKey(),
				Name:          target.Name,
				Description:   fmt.Sprintf("auto generated by the target %s", target.Name),
				Type:          v1alpha1.TopologyPolicyType,
				Creator:       userName,
				EnvName:       env.Name,
			}
			properties, err := model.NewJSONStructByStruct(v1alpha1.TopologyPolicySpec{
				Placement: v1alpha1.Placement{
					Clusters: []string{target.Cluster.ClusterName},
				},
				Namespace: target.Cluster.Namespace,
			})
			if err != nil {
				log.Logger.Errorf("fail to create the properties of the topology policy, %s", err.Error())
				continue
			}
			appPolicy.Properties = properties
			policies = append(policies, appPolicy)
		}
	}
	var steps []model.WorkflowStep
	for _, step := range workflowSteps {
		targetName := strings.Replace(step.Name, "-cloud-resource", "", 1)
		s := model.WorkflowStep{
			Name:        step.Name,
			Type:        step.Type,
			Alias:       fmt.Sprintf("Deploy To %s", targetName),
			Description: fmt.Sprintf("deploy app to delivery target %s", targetName),
			DependsOn:   step.DependsOn,
			Inputs:      step.Inputs,
			Outputs:     step.Outputs,
		}
		if step.Properties != nil {
			properties, err := model.NewJSONStruct(step.Properties)
			if err != nil {
				log.Logger.Errorf("workflow %s step %s properties is invalid %s", utils2.Sanitize(app.Name), utils2.Sanitize(step.Name), err.Error())
				continue
			}
			s.Properties = properties
		}
		steps = append(steps, s)
	}
	return steps, policies
}

// UpdateWorkflowSteps will update workflow with new steps
func UpdateWorkflowSteps(ctx context.Context, ds datastore.DataStore, workflow *model.Workflow, steps []model.WorkflowStep) error {
	workflow.Steps = steps
	return ds.Put(ctx, workflow)
}

// GetWorkflowForApp get the specified workflow of the application
func GetWorkflowForApp(ctx context.Context, ds datastore.DataStore, app *model.Application, workflowName string) (*model.Workflow, error) {
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

// ConvertWorkflowName generate the workflow name
func ConvertWorkflowName(envName string) string {
	return fmt.Sprintf("workflow-%s", envName)
}

func genPolicyName(envName string) string {
	return fmt.Sprintf("%s-%s", EnvBindingPolicyDefaultName, envName)
}

func genPolicyEnvName(targetName string) string {
	return targetName
}
