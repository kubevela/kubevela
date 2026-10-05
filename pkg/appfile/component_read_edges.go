/*
Copyright 2026 The KubeVela Authors.

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

package appfile

import (
	"encoding/json"
	"slices"

	workflowv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	wfmode "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// A component read beside the reader is an implied dependsOn, and is added where
// dependsOn between components already takes effect: a step named after the
// component, and a deploy step's own components. It is added only where it
// cannot leave the reader waiting on something that never comes; a read with no
// edge still waits at render until it can be answered.

// stepRef is a step, or a sub-step of the group at Top.
type stepRef struct{ Top, Sub int }

const topLevel = -1

// StepsWithReadDependencies returns a copy of the workflow's steps in which an
// apply-component step dependsOn the step named after each component its
// component reads beside itself, as that component's own dependsOn would.
//
// The edge is left out when no step has that name, when it would point at a
// later step in a workflow run in order, and when it would close a cycle.
func (af *Appfile) StepsWithReadDependencies(steps []workflowv1alpha1.WorkflowStep) []workflowv1alpha1.WorkflowStep {
	if !sources.ExpressionsEnabledFor(af.AppAnnotations) || len(steps) == 0 {
		return steps
	}
	out := make([]workflowv1alpha1.WorkflowStep, len(steps))
	for i := range steps {
		out[i] = *steps[i].DeepCopy()
	}
	steps = out

	named := map[string]stepRef{}
	for i, step := range steps {
		named[step.Name] = stepRef{i, topLevel}
		for j, sub := range step.SubSteps {
			named[sub.Name] = stepRef{i, j}
		}
	}
	base := func(r stepRef) *workflowv1alpha1.WorkflowStepBase {
		if r.Sub == topLevel {
			return &steps[r.Top].WorkflowStepBase
		}
		return &steps[r.Top].SubSteps[r.Sub]
	}
	comps := map[string]common.ApplicationComponent{}
	for _, c := range af.Components {
		comps[c.Name] = c
	}
	stepsInOrder, subStepsInOrder := true, false
	if af.WorkflowMode != nil {
		stepsInOrder = af.WorkflowMode.Steps != wfmode.WorkflowModeDAG
		subStepsInOrder = af.WorkflowMode.SubSteps == wfmode.WorkflowModeStep
	}

	for i, step := range steps {
		refs := []stepRef{{i, topLevel}}
		if len(step.SubSteps) > 0 {
			refs = refs[:0]
			for j := range step.SubSteps {
				refs = append(refs, stepRef{i, j})
			}
		}
		for _, from := range refs {
			reader, ok := appliedComponent(*base(from), comps)
			if !ok {
				continue
			}
			for _, producer := range af.readDependencies(reader.Name) {
				to, ok := named[producer]
				if !ok || to == from {
					continue
				}
				f, t := from, to
				if f.Top != t.Top {
					f, t = stepRef{f.Top, topLevel}, stepRef{t.Top, topLevel}
					if stepsInOrder && t.Top > f.Top {
						continue
					}
				} else if subStepsInOrder && t.Sub > f.Sub {
					continue
				}
				dependOn(steps, base(f), base(t).Name)
			}
		}
	}
	return steps
}

// appliedComponent is the component an apply-component step applies.
func appliedComponent(step workflowv1alpha1.WorkflowStepBase, comps map[string]common.ApplicationComponent) (common.ApplicationComponent, bool) {
	if step.Type != "apply-component" && step.Type != "builtin-apply-component" {
		return common.ApplicationComponent{}, false
	}
	var props struct {
		Component string `json:"component"`
	}
	if step.Properties == nil || json.Unmarshal(step.Properties.Raw, &props) != nil {
		return common.ApplicationComponent{}, false
	}
	c, ok := comps[props.Component]
	return c, ok
}

// ComponentsWithReadDependencies returns a copy of one deploy step's components
// in which each dependsOn the components of this step it reads beside itself, so
// that the step applies it in a placement once its producer is healthy there. A
// producer this step does not deploy gives no edge.
func (af *Appfile) ComponentsWithReadDependencies(components []common.ApplicationComponent) []common.ApplicationComponent {
	inStep := map[string]bool{}
	for _, c := range components {
		inStep[c.Name] = true
	}
	out := make([]common.ApplicationComponent, len(components))
	for i, c := range components {
		for _, p := range af.readDependencies(c.Name) {
			if inStep[p] && !slices.Contains(c.DependsOn, p) {
				c.DependsOn = append(slices.Clip(c.DependsOn), p)
			}
		}
		out[i] = c
	}
	return out
}

// readDependencies is the components a component reads beside itself, from the
// Appfile's dependencies: those an expression reads that order it.
func (af *Appfile) readDependencies(component string) []string {
	var out []string
	for _, d := range af.Dependencies {
		if d.Component == component && d.Source == common.DependencySourceExpression && sources.Orders(d) {
			out = append(out, d.DependsOn)
		}
	}
	return out
}

// dependOn adds name to step's dependsOn unless it is there already or would
// close a cycle.
func dependOn(steps []workflowv1alpha1.WorkflowStep, step *workflowv1alpha1.WorkflowStepBase, name string) {
	if slices.Contains(step.DependsOn, name) || waitsFor(steps, name, step.Name) {
		return
	}
	step.DependsOn = append(step.DependsOn, name)
}

// waitsFor reports whether the step named from cannot finish before the step
// named to has: through dependsOn, a group through its sub-steps, and a sub-step
// through what its group waits on to start.
func waitsFor(steps []workflowv1alpha1.WorkflowStep, from, to string) bool {
	edges := map[string][]string{}
	for _, step := range steps {
		edges[step.Name] = append(edges[step.Name], step.DependsOn...)
		for _, sub := range step.SubSteps {
			edges[step.Name] = append(edges[step.Name], sub.Name)
			edges[sub.Name] = append(edges[sub.Name], sub.DependsOn...)
			edges[sub.Name] = append(edges[sub.Name], step.DependsOn...)
		}
	}
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(n string) bool {
		if n == to {
			return true
		}
		if seen[n] {
			return false
		}
		seen[n] = true
		for _, next := range edges[n] {
			if visit(next) {
				return true
			}
		}
		return false
	}
	return visit(from)
}
