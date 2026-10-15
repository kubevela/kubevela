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

package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/workflow/pkg/cue/process"
	"github.com/kubevela/workflow/pkg/expressions"
	wftypes "github.com/kubevela/workflow/pkg/types"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// pinnedSourcesKey is the workflow context entry holding the sources a run has
// read, so every step of the run reads the value the first one did.
const pinnedSourcesKey = "kubevela-pinned-sources"

// pinnedSources is what a run has read of each source, and where each value
// came from. A workflow context outlives a run, so a value pinned in another run
// is not this one's.
type pinnedSources struct {
	Run    string                            `json:"run"`
	Values map[string]map[string]interface{} `json:"values"`
	From   map[string]pinnedFrom             `json:"from,omitempty"`
}

// pinnedFrom is the storage key a pinned value was resolved from, when that entry
// expires, and why it was served, such as a stale value served after a failed
// refresh, so a later step's report names the same resolution.
type pinnedFrom struct {
	Config    string `json:"config,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Message   string `json:"message,omitempty"`
}

// StepExpressions are the roots KubeVela answers for a workflow step's
// expressions, which the workflow resolves when the step runs: context and
// source, read as the step's own render would. stepContext is the render
// context of the step named, to which its name and type are added, and record
// is given what each step's sources resolved to.
//
// A source holds for the whole run: each is resolved the first time a step reads
// it, and every later step reads that value, typed again against its schema.
func StepExpressions(stepContext func(step, stepType string) process.Context,
	record func(step, stepType string, statuses map[string]SourceResolutionStatus)) wftypes.ExpressionOptions {
	resolverFor := func(ctx context.Context) (*sourceResolver, expressions.RunScope, error) {
		scope, ok := expressions.RunFrom(ctx)
		if !ok {
			return nil, scope, errors.New("a step expression is resolved only for a workflow step")
		}
		pCtx := stepContext(scope.Step, scope.StepType)
		pCtx.PushData(velaprocess.ContextStepName, scope.Step)
		pCtx.PushData(velaprocess.ContextStepType, scope.StepType)
		// The workflow knows a run by when it started, to the second. A revision
		// is always a new run, so pins are kept under both.
		if rev, _ := pCtx.GetData(velaprocess.ContextAppRevision).(string); rev != "" {
			scope.Run += "@" + rev
		}
		return newSourceResolver(ctx, contextValuesFor(pCtx), SurfaceWorkflowStep, sourceInputsFromContext(pCtx)), scope, nil
	}
	return wftypes.ExpressionOptions{
		Roots: []celengine.Root{rootNamed(propexpr.ContextIdent), rootNamed(propexpr.SourceIdent)},
		Resolvers: map[string]celengine.Resolver{
			propexpr.ContextIdent: celengine.ResolverFunc(func(ctx context.Context, _ []celengine.Read) (interface{}, error) {
				r, _, err := resolverFor(ctx)
				if err != nil {
					return nil, err
				}
				return r.expressionContext(), nil
			}),
			propexpr.SourceIdent: celengine.ResolverFunc(func(ctx context.Context, reads []celengine.Read) (interface{}, error) {
				r, scope, err := resolverFor(ctx)
				if err != nil {
					return nil, err
				}
				pins, err := readPins(scope)
				if err != nil {
					return nil, err
				}
				for name, values := range pins.Values {
					sourceType := r.sourceTypes[name]
					r.resolved[name] = r.typeToSchema(r.sourceTemplates[sourceType], values)
					from := pins.From[name]
					message := "pinned for this workflow run"
					if from.Message != "" {
						message = from.Message + " (" + message + ")"
					}
					r.setSourceStatus(name, sourceType, PhaseResolved, message, from.Config, from.ExpiresAt)
				}
				// What resolved is pinned even when a later read fails, so the
				// step's retry and every later step read the same value.
				out, readErr := r.resolveReads(ctx, reads)
				record(scope.Step, scope.StepType, r.statuses)
				for name, values := range r.resolved {
					pins.Values[name] = values
					if _, kept := pins.From[name]; kept {
						continue
					}
					if st, ok := r.statuses[name]; ok && (st.Config != "" || st.Message != "") {
						pins.From[name] = pinnedFrom{Config: st.Config, ExpiresAt: st.ExpiresAt, Message: st.Message}
					}
				}
				if err := writePins(scope, pins); err != nil {
					return nil, err
				}
				if readErr != nil {
					return nil, readErr
				}
				return out, nil
			}),
		},
	}
}

// rootNamed is KubeVela's declaration of the root.
func rootNamed(name string) celengine.Root {
	for _, r := range celexpr.Roots {
		if r.Name == name {
			return r
		}
	}
	return celengine.Root{Name: name}
}

func readPins(scope expressions.RunScope) (pinnedSources, error) {
	pins := pinnedSources{Run: scope.Run, Values: map[string]map[string]interface{}{}, From: map[string]pinnedFrom{}}
	raw := scope.Store.GetMutableValue(pinnedSourcesKey)
	if raw == "" {
		return pins, nil
	}
	var saved pinnedSources
	if err := json.Unmarshal([]byte(raw), &saved); err != nil {
		return pins, fmt.Errorf("read the sources pinned for this run: %w", err)
	}
	if saved.Run == scope.Run && saved.Values != nil {
		pins.Values = saved.Values
		if saved.From != nil {
			pins.From = saved.From
		}
	}
	return pins, nil
}

func writePins(scope expressions.RunScope, pins pinnedSources) error {
	raw, err := json.Marshal(pins)
	if err != nil {
		return err
	}
	scope.Store.SetMutableValue(string(raw), pinnedSourcesKey)
	return nil
}
