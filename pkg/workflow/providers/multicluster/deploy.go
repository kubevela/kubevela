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

package multicluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	pkgpolicy "github.com/oam-dev/kubevela/pkg/policy"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/utils"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
	"github.com/oam-dev/kubevela/pkg/utils/parallel"
	oamProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/oam"
)

// DeployWorkflowStepExecutor executor to run deploy workflow step
type DeployWorkflowStepExecutor interface {
	Deploy(ctx context.Context, policyNames []string, parallelism int) (healthy bool, reason string, err error)
}

// NewDeployWorkflowStepExecutor .
func NewDeployWorkflowStepExecutor(cli client.Client, af *appfile.Appfile, apply oamProvider.ComponentApply, healthCheck oamProvider.ComponentHealthCheck, renderer oamProvider.WorkloadRenderer, ignoreTerraformComponent bool) DeployWorkflowStepExecutor {
	return &deployWorkflowStepExecutor{
		cli:                      cli,
		af:                       af,
		apply:                    apply,
		healthCheck:              healthCheck,
		renderer:                 renderer,
		ignoreTerraformComponent: ignoreTerraformComponent,
	}
}

type deployWorkflowStepExecutor struct {
	cli                      client.Client
	af                       *appfile.Appfile
	apply                    oamProvider.ComponentApply
	healthCheck              oamProvider.ComponentHealthCheck
	renderer                 oamProvider.WorkloadRenderer
	ignoreTerraformComponent bool
}

// Deploy execute deploy workflow step
func (executor *deployWorkflowStepExecutor) Deploy(ctx context.Context, policyNames []string, parallelism int) (bool, string, error) {
	policies, err := selectPolicies(executor.af.Policies, policyNames)
	if err != nil {
		return false, "", err
	}
	components, err := loadComponents(ctx, executor.renderer, executor.cli, executor.af, executor.af.Components, executor.ignoreTerraformComponent)
	if err != nil {
		return false, "", err
	}
	placements, err := pkgpolicy.GetPlacementsFromTopologyPolicies(ctx, executor.cli, executor.af.Namespace, policies, resourcekeeper.AllowCrossNamespaceResource)
	if err != nil {
		return false, "", err
	}
	components, err = overrideConfiguration(policies, components)
	if err != nil {
		return false, "", err
	}
	return applyComponents(executor.apply, executor.healthCheck, components, placements, parallelism)
}

func selectPolicies(policies []v1beta1.AppPolicy, policyNames []string) ([]v1beta1.AppPolicy, error) {
	policyMap := make(map[string]v1beta1.AppPolicy)
	for _, policy := range policies {
		policyMap[policy.Name] = policy
	}
	var selectedPolicies []v1beta1.AppPolicy
	for _, policyName := range policyNames {
		if policy, found := policyMap[policyName]; found {
			selectedPolicies = append(selectedPolicies, policy)
		} else {
			return nil, errors.Errorf("policy %s not found", policyName)
		}
	}
	return selectedPolicies, nil
}

func loadComponents(ctx context.Context, renderer oamProvider.WorkloadRenderer, cli client.Client, af *appfile.Appfile, components []common.ApplicationComponent, ignoreTerraformComponent bool) ([]common.ApplicationComponent, error) {
	var loadedComponents []common.ApplicationComponent
	for _, comp := range components {
		loadedComp, err := af.LoadDynamicComponent(ctx, cli, comp.DeepCopy())
		if err != nil {
			return nil, err
		}
		if ignoreTerraformComponent {
			wl, err := renderer(comp)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to render component into workload")
			}
			if wl.CapabilityCategory == types.TerraformCategory {
				continue
			}
		}
		loadedComponents = append(loadedComponents, *loadedComp)
	}
	return loadedComponents, nil
}

func overrideConfiguration(policies []v1beta1.AppPolicy, components []common.ApplicationComponent) ([]common.ApplicationComponent, error) {
	var err error
	for _, policy := range policies {
		if policy.Type == v1alpha1.OverridePolicyType {
			overrideSpec := &v1alpha1.OverridePolicySpec{}
			if err := utils.StrictUnmarshal(policy.Properties.Raw, overrideSpec); err != nil {
				return nil, errors.Wrapf(err, "failed to parse override policy %s", policy.Name)
			}
			components, err = envbinding.PatchComponents(components, overrideSpec.Components, overrideSpec.Selector)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to apply override policy %s", policy.Name)
			}
		}
	}
	return components, nil
}

type applyTask struct {
	component common.ApplicationComponent
	placement v1alpha1.PlacementDecision
}

func (t *applyTask) key() string {
	return fmt.Sprintf("%s/%s/%s", t.placement.Cluster, t.placement.Namespace, t.component.Name)
}

func (t *applyTask) dependents() []string {
	var dependents []string
	for _, dependent := range t.component.DependsOn {
		dependents = append(dependents, fmt.Sprintf("%s/%s/%s", t.placement.Cluster, t.placement.Namespace, dependent))
	}
	return dependents
}

type applyTaskResult struct {
	healthy bool
	err     error
}

func applyComponents(apply oamProvider.ComponentApply, healthCheck oamProvider.ComponentHealthCheck, components []common.ApplicationComponent, placements []v1alpha1.PlacementDecision, parallelism int) (bool, string, error) {
	var tasks []*applyTask
	for _, comp := range components {
		for _, pl := range placements {
			tasks = append(tasks, &applyTask{component: comp, placement: pl})
		}
	}
	healthCheckResults := parallel.Run(func(task *applyTask) *applyTaskResult {
		healthy, err := healthCheck(task.component, nil, task.placement.Cluster, task.placement.Namespace, "")
		return &applyTaskResult{healthy: healthy, err: err}
	}, tasks, parallelism).([]*applyTaskResult)
	taskHealthyMap := map[string]bool{}
	for i, res := range healthCheckResults {
		taskHealthyMap[tasks[i].key()] = res.healthy
	}

	var pendingTasks []*applyTask
	var todoTasks []*applyTask
	for _, task := range tasks {
		if healthy, ok := taskHealthyMap[task.key()]; healthy && ok {
			continue
		}
		pending := false
		for _, dep := range task.dependents() {
			if healthy, ok := taskHealthyMap[dep]; ok && !healthy {
				pending = true
				break
			}
		}
		if pending {
			pendingTasks = append(pendingTasks, task)
		} else {
			todoTasks = append(todoTasks, task)
		}
	}
	var results []*applyTaskResult
	if len(todoTasks) > 0 {
		results = parallel.Run(func(task *applyTask) *applyTaskResult {
			_, _, healthy, err := apply(task.component, nil, task.placement.Cluster, task.placement.Namespace, "")
			return &applyTaskResult{healthy: healthy, err: err}
		}, todoTasks, parallelism).([]*applyTaskResult)
	}
	var errs []error
	var allHealthy = true
	var reasons []string
	for i, res := range results {
		if res.err != nil {
			errs = append(errs, res.err)
		}
		if !res.healthy {
			allHealthy = false
			reasons = append(reasons, fmt.Sprintf("%s is not healthy", todoTasks[i].key()))
		}
	}

	for _, t := range pendingTasks {
		reasons = append(reasons, fmt.Sprintf("%s is waiting dependents", t.key()))
	}

	return allHealthy && len(pendingTasks) == 0, strings.Join(reasons, ","), velaerrors.AggregateErrors(errs)
}
