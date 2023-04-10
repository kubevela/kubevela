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
	"sync"

	pkgmaps "github.com/kubevela/pkg/util/maps"
	"github.com/kubevela/pkg/util/slices"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam"
	pkgpolicy "github.com/oam-dev/kubevela/pkg/policy"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/utils"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
	oamProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/oam"
)

// DeployParameter is the parameter of deploy workflow step
type DeployParameter struct {
	// Declare the policies that used for this deployment. If not specified, the components will be deployed to the hub cluster.
	Policies []string `json:"policies,omitempty"`
	// Maximum number of concurrent delivered components.
	Parallelism int64 `json:"parallelism"`
	// If set false, this step will apply the components with the terraform workload.
	IgnoreTerraformComponent bool `json:"ignoreTerraformComponent"`
	// The policies that embeds in the `deploy` step directly
	InlinePolicies []v1beta1.AppPolicy `json:"inlinePolicies,omitempty"`
}

// DeployWorkflowStepExecutor executor to run deploy workflow step
type DeployWorkflowStepExecutor interface {
	Deploy(ctx context.Context) (healthy bool, reason string, err error)
}

// NewDeployWorkflowStepExecutor .
func NewDeployWorkflowStepExecutor(cli client.Client, af *appfile.Appfile, apply oamProvider.ComponentApply, healthCheck oamProvider.ComponentHealthCheck, renderer oamProvider.WorkloadRenderer, parameter DeployParameter) DeployWorkflowStepExecutor {
	return &deployWorkflowStepExecutor{
		cli:         cli,
		af:          af,
		apply:       apply,
		healthCheck: healthCheck,
		renderer:    renderer,
		parameter:   parameter,
	}
}

type deployWorkflowStepExecutor struct {
	cli         client.Client
	af          *appfile.Appfile
	apply       oamProvider.ComponentApply
	healthCheck oamProvider.ComponentHealthCheck
	renderer    oamProvider.WorkloadRenderer
	parameter   DeployParameter
}

// Deploy execute deploy workflow step
func (executor *deployWorkflowStepExecutor) Deploy(ctx context.Context) (bool, string, error) {
	policies, err := selectPolicies(executor.af.Policies, executor.parameter.Policies)
	if err != nil {
		return false, "", err
	}
	policies = append(policies, fillInlinePolicyNames(executor.parameter.InlinePolicies)...)
	components, err := loadComponents(ctx, executor.renderer, executor.cli, executor.af, executor.af.Components, executor.parameter.IgnoreTerraformComponent)
	if err != nil {
		return false, "", err
	}

	// Dealing with topology, override and replication policies in order.
	placements, err := pkgpolicy.GetPlacementsFromTopologyPolicies(ctx, executor.cli, executor.af.Namespace, policies, resourcekeeper.AllowCrossNamespaceResource)
	if err != nil {
		return false, "", err
	}
	components, err = overrideConfiguration(policies, components)
	if err != nil {
		return false, "", err
	}
	components, err = pkgpolicy.ReplicateComponents(policies, components)
	if err != nil {
		return false, "", err
	}
	return applyComponents(ctx, executor.apply, executor.healthCheck, components, placements, int(executor.parameter.Parallelism))
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

func fillInlinePolicyNames(policies []v1beta1.AppPolicy) []v1beta1.AppPolicy {
	for i := range policies {
		if policies[i].Name == "" {
			policies[i].Name = fmt.Sprintf("inline-%s-policy-%d", policies[i].Type, i)
		}
	}
	return policies
}

func loadComponents(ctx context.Context, renderer oamProvider.WorkloadRenderer, cli client.Client, af *appfile.Appfile, components []common.ApplicationComponent, ignoreTerraformComponent bool) ([]common.ApplicationComponent, error) {
	var loadedComponents []common.ApplicationComponent
	for _, comp := range components {
		loadedComp, err := af.LoadDynamicComponent(ctx, cli, comp.DeepCopy())
		if err != nil {
			return nil, err
		}
		if ignoreTerraformComponent {
			wl, err := renderer(ctx, comp)
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
			if policy.Properties == nil {
				return nil, fmt.Errorf("override policy %s must not have empty properties", policy.Name)
			}
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

type valueBuilder func(s string) (*value.Value, error)

type applyTask struct {
	component common.ApplicationComponent
	placement v1alpha1.PlacementDecision
	healthy   *bool
}

func (t *applyTask) key() string {
	return fmt.Sprintf("%s/%s/%s/%s", t.placement.Cluster, t.placement.Namespace, t.component.ReplicaKey, t.component.Name)
}

func (t *applyTask) varKey(v string) string {
	return fmt.Sprintf("%s/%s/%s/%s", t.placement.Cluster, t.placement.Namespace, t.component.ReplicaKey, v)
}

func (t *applyTask) varKeyWithoutReplica(v string) string {
	return fmt.Sprintf("%s/%s/%s/%s", t.placement.Cluster, t.placement.Namespace, "", v)
}

func (t *applyTask) getVar(from string, cache *pkgmaps.SyncMap[string, *value.Value]) *value.Value {
	key := t.varKey(from)
	keyWithNoReplica := t.varKeyWithoutReplica(from)
	var val *value.Value
	var ok bool
	if val, ok = cache.Get(key); !ok {
		if val, ok = cache.Get(keyWithNoReplica); !ok {
			return nil
		}
	}
	return val
}

func (t *applyTask) fillInputs(inputs *pkgmaps.SyncMap[string, *value.Value], build valueBuilder) error {
	if len(t.component.Inputs) == 0 {
		return nil
	}

	x, err := component2Value(t.component, build)
	if err != nil {
		return err
	}

	for _, input := range t.component.Inputs {
		var inputVal *value.Value
		if inputVal = t.getVar(input.From, inputs); inputVal == nil {
			return fmt.Errorf("input %s is not ready", input)
		}

		err = x.FillValueByScript(inputVal, fieldPathToComponent(input.ParameterKey))
		if err != nil {
			return errors.Wrap(err, "fill value to component")
		}
	}
	newComp, err := value2Component(x)
	if err != nil {
		return err
	}
	t.component = *newComp
	return nil
}

func (t *applyTask) generateOutput(output *unstructured.Unstructured, outputs []*unstructured.Unstructured, cache *pkgmaps.SyncMap[string, *value.Value], build valueBuilder) error {
	if len(t.component.Outputs) == 0 {
		return nil
	}

	var cueString string
	if output != nil {
		outputJSON, err := output.MarshalJSON()
		if err != nil {
			return errors.Wrap(err, "marshal output")
		}
		cueString += fmt.Sprintf("output:%s\n", string(outputJSON))
	}
	componentVal, err := build(cueString)
	if err != nil {
		return errors.Wrap(err, "create cue value from component")
	}

	for _, os := range outputs {
		name := os.GetLabels()[oam.TraitResource]
		if name != "" {
			if err := componentVal.FillObject(os.Object, "outputs", name); err != nil {
				return errors.WithMessage(err, "FillOutputs")
			}
		}
	}

	for _, o := range t.component.Outputs {
		pathToSetVar := t.varKey(o.Name)
		actualOutput, err := componentVal.LookupValue(o.ValueFrom)
		if err != nil {
			return errors.Wrap(err, "lookup output")
		}
		cache.Set(pathToSetVar, actualOutput)
	}
	return nil
}

func (t *applyTask) allDependsReady(healthyMap map[string]bool) bool {
	for _, d := range t.component.DependsOn {
		dKey := fmt.Sprintf("%s/%s/%s/%s", t.placement.Cluster, t.placement.Namespace, t.component.ReplicaKey, d)
		dKeyWithoutReplica := fmt.Sprintf("%s/%s/%s/%s", t.placement.Cluster, t.placement.Namespace, "", d)
		if !healthyMap[dKey] && !healthyMap[dKeyWithoutReplica] {
			return false
		}
	}
	return true
}

func (t *applyTask) allInputReady(cache *pkgmaps.SyncMap[string, *value.Value]) bool {
	for _, in := range t.component.Inputs {
		if val := t.getVar(in.From, cache); val == nil {
			return false
		}
	}

	return true
}

type applyTaskResult struct {
	healthy bool
	err     error
	task    *applyTask
}

// applyComponents will apply components to placements.
func applyComponents(ctx context.Context, apply oamProvider.ComponentApply, healthCheck oamProvider.ComponentHealthCheck, components []common.ApplicationComponent, placements []v1alpha1.PlacementDecision, parallelism int) (bool, string, error) {
	var tasks []*applyTask
	var cache = pkgmaps.NewSyncMap[string, *value.Value]()
	rootValue, err := value.NewValue("{}", nil, "")
	if err != nil {
		return false, "", err
	}
	var cueMutex sync.Mutex
	var makeValue = func(s string) (*value.Value, error) {
		cueMutex.Lock()
		defer cueMutex.Unlock()
		return rootValue.MakeValue(s)
	}

	taskHealthyMap := map[string]bool{}
	for _, comp := range components {
		for _, pl := range placements {
			tasks = append(tasks, &applyTask{component: comp, placement: pl})
		}
	}
	unhealthyResults := make([]*applyTaskResult, 0)
	maxHealthCheckTimes := len(tasks)
HealthCheck:
	for i := 0; i < maxHealthCheckTimes; i++ {
		checkTasks := make([]*applyTask, 0)
		for _, task := range tasks {
			if task.healthy == nil && task.allDependsReady(taskHealthyMap) && task.allInputReady(cache) {
				task.healthy = new(bool)
				err := task.fillInputs(cache, makeValue)
				if err != nil {
					taskHealthyMap[task.key()] = false
					unhealthyResults = append(unhealthyResults, &applyTaskResult{healthy: false, err: err, task: task})
					continue
				}
				checkTasks = append(checkTasks, task)
			}
		}
		if len(checkTasks) == 0 {
			break HealthCheck
		}
		checkResults := slices.ParMap[*applyTask, *applyTaskResult](checkTasks, func(task *applyTask) *applyTaskResult {
			healthy, output, outputs, err := healthCheck(ctx, task.component, nil, task.placement.Cluster, task.placement.Namespace, "")
			task.healthy = pointer.Bool(healthy)
			if healthy {
				err = task.generateOutput(output, outputs, cache, makeValue)
			}
			return &applyTaskResult{healthy: healthy, err: err, task: task}
		}, slices.Parallelism(parallelism))

		for _, res := range checkResults {
			taskHealthyMap[res.task.key()] = res.healthy
			if !res.healthy || res.err != nil {
				unhealthyResults = append(unhealthyResults, res)
			}
		}
	}

	var pendingTasks []*applyTask
	var todoTasks []*applyTask

	for _, task := range tasks {
		if healthy, ok := taskHealthyMap[task.key()]; healthy && ok {
			continue
		}
		if task.allDependsReady(taskHealthyMap) && task.allInputReady(cache) {
			todoTasks = append(todoTasks, task)
		} else {
			pendingTasks = append(pendingTasks, task)
		}
	}
	var results []*applyTaskResult
	if len(todoTasks) > 0 {
		results = slices.ParMap[*applyTask, *applyTaskResult](todoTasks, func(task *applyTask) *applyTaskResult {
			err := task.fillInputs(cache, makeValue)
			if err != nil {
				return &applyTaskResult{healthy: false, err: err, task: task}
			}
			_, _, healthy, err := apply(ctx, task.component, nil, task.placement.Cluster, task.placement.Namespace, "")
			if err != nil {
				return &applyTaskResult{healthy: healthy, err: err, task: task}
			}
			return &applyTaskResult{healthy: healthy, err: err, task: task}
		}, slices.Parallelism(parallelism))
	}
	var errs []error
	var allHealthy = true
	var reasons []string
	for _, res := range unhealthyResults {
		if res.err != nil {
			errs = append(errs, fmt.Errorf("error health check from %s: %w", res.task.key(), res.err))
		}
	}
	for _, res := range results {
		if res.err != nil {
			errs = append(errs, fmt.Errorf("error encountered in cluster %s: %w", res.task.placement.Cluster, res.err))
		}
		if !res.healthy {
			allHealthy = false
			reasons = append(reasons, fmt.Sprintf("%s is not healthy", res.task.key()))
		}
	}

	for _, t := range pendingTasks {
		reasons = append(reasons, fmt.Sprintf("%s is waiting dependents", t.key()))
	}

	return allHealthy && len(pendingTasks) == 0, strings.Join(reasons, ","), velaerrors.AggregateErrors(errs)
}

func fieldPathToComponent(input string) string {
	return fmt.Sprintf("properties.%s", strings.TrimSpace(input))
}

func component2Value(comp common.ApplicationComponent, build valueBuilder) (*value.Value, error) {
	x, err := build("")
	if err != nil {
		return nil, err
	}
	err = x.FillObject(comp, "")
	if err != nil {
		return nil, err
	}
	// Component.ReplicaKey have no json tag, so we need to set it manually
	err = x.FillObject(comp.ReplicaKey, "replicaKey")
	if err != nil {
		return nil, err
	}
	return x, nil
}

func value2Component(v *value.Value) (*common.ApplicationComponent, error) {
	var comp common.ApplicationComponent
	err := v.UnmarshalTo(&comp)
	if err != nil {
		return nil, err
	}
	if rk, err := v.GetString("replicaKey"); err == nil {
		comp.ReplicaKey = rk
	}
	return &comp, nil
}
