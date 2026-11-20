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
	"strconv"
	"strings"

	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/workflow/pkg/cue/process"
	"github.com/kubevela/workflow/pkg/expressions"
	wftypes "github.com/kubevela/workflow/pkg/types"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// PinnedSourcesKey is the workflow context entry holding the sources a run has
// read, so every step of the run reads the value the first one did.
const PinnedSourcesKey = "kubevela-pinned-sources"

// pinnedSources is what a run has read of each source, and where each value
// came from. A workflow context outlives a run, so a value pinned in another run
// is not this one's.
type pinnedSources struct {
	Run    string                            `json:"run"`
	Values map[string]map[string]interface{} `json:"values"`
	From   map[string]pinnedFrom             `json:"from,omitempty"`
	// Sealed are the fields of each source kept in SensitivePins, by their path in its value,
	// list indices included. Each holds, here, the pointer SensitivePins gives it.
	Sealed map[string][][]string `json:"sealed,omitempty"`
}

// pinnedFrom is the storage key a pinned value was resolved from, when that entry
// expires, and why it was served, such as a stale value served after a failed
// refresh, so a later step's report names the same resolution.
type pinnedFrom struct {
	Config    string `json:"config,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Message   string `json:"message,omitempty"`
}

// SensitivePins keeps the sensitive fields of a run's pinned sources, which the workflow
// context, a ConfigMap, must not hold. A field is stored under its key: the source's name and
// the field's path, dot-joined. Load returns nothing for another run.
type SensitivePins interface {
	// Ref is what the workflow context holds in place of the field stored under key.
	Ref(key string) string
	Load(ctx context.Context, run string) (map[string]interface{}, error)
	Save(ctx context.Context, run string, fields map[string]interface{}) error
}

// StepExpressions are the roots KubeVela answers for a workflow step's
// expressions, which the workflow resolves when the step runs: context and
// source, read as the step's own render would. stepContext is the render
// context of the step named, to which its name and type are added, and record
// is given what each step's sources resolved to.
//
// A source holds for the whole run: each is resolved the first time a step reads
// it, and every later step reads that value, typed again against its schema. Its
// sensitive fields are pinned in vault; with no vault a source that has any is not
// pinned, and each step resolves it.
func StepExpressions(stepContext func(step, stepType string) process.Context,
	record func(step, stepType string, statuses map[string]SourceResolutionStatus), vault SensitivePins) wftypes.ExpressionOptions {
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
				pins, err := readPins(ctx, scope, vault)
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
				if err := writePins(ctx, scope, vault, pins, r.sensitiveMasks); err != nil {
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

// readPins reads what this run has pinned, putting each sealed field's value from vault back in
// place of its pointer. A source with a field the vault no longer has is dropped, so it is
// resolved again rather than read with the pointer.
func readPins(ctx context.Context, scope expressions.RunScope, vault SensitivePins) (pinnedSources, error) {
	pins := pinnedSources{Run: scope.Run, Values: map[string]map[string]interface{}{}, From: map[string]pinnedFrom{}}
	raw := scope.Store.GetMutableValue(PinnedSourcesKey)
	if raw == "" {
		return pins, nil
	}
	var saved pinnedSources
	if err := json.Unmarshal([]byte(raw), &saved); err != nil {
		return pins, fmt.Errorf("read the sources pinned for this run: %w", err)
	}
	if saved.Run != scope.Run || saved.Values == nil {
		return pins, nil
	}
	pins.Values = saved.Values
	if saved.From != nil {
		pins.From = saved.From
	}
	if len(saved.Sealed) == 0 {
		return pins, nil
	}
	var fields map[string]interface{}
	if vault != nil {
		var err error
		if fields, err = vault.Load(ctx, scope.Run); err != nil {
			return pins, fmt.Errorf("read the sensitive sources pinned for this run: %w", err)
		}
	}
	for name, paths := range saved.Sealed {
		for _, path := range paths {
			v, ok := fields[sealedKey(name, path)]
			if ok {
				ok = setAt(pins.Values[name], path, v)
			}
			if !ok {
				delete(pins.Values, name)
				delete(pins.From, name)
				break
			}
		}
	}
	return pins, nil
}

// writePins pins the run's sources in the workflow context, except their sensitive fields:
// those go to vault, and the context holds the pointer vault gives each. With no vault, a
// source with sensitive fields is not pinned.
func writePins(ctx context.Context, scope expressions.RunScope, vault SensitivePins, pins pinnedSources,
	masksOf func(source string) map[string]struct{}) error {
	public := pinnedSources{Run: pins.Run, Values: map[string]map[string]interface{}{}, From: map[string]pinnedFrom{}}
	fields := map[string]interface{}{}
	for name, values := range pins.Values {
		masks := masksOf(name)
		if len(masks) > 0 && vault == nil {
			continue
		}
		if len(masks) > 0 {
			var sealed [][]string
			values, _ = seal(values, nil, "", masks, func(path []string, v interface{}) interface{} {
				key := sealedKey(name, path)
				fields[key] = v
				sealed = append(sealed, path)
				return vault.Ref(key)
			}).(map[string]interface{})
			if len(sealed) > 0 {
				if public.Sealed == nil {
					public.Sealed = map[string][][]string{}
				}
				public.Sealed[name] = sealed
			}
		}
		public.Values[name] = values
		if from, ok := pins.From[name]; ok {
			public.From[name] = from
		}
	}
	if len(fields) > 0 {
		if err := vault.Save(ctx, pins.Run, fields); err != nil {
			return fmt.Errorf("pin the sensitive sources for this run: %w", err)
		}
	}
	raw, err := json.Marshal(public)
	if err != nil {
		return err
	}
	scope.Store.SetMutableValue(string(raw), PinnedSourcesKey)
	return nil
}

// seal copies v with every field marked sensitive replaced by what keep returns for it. path
// is where v sits, list indices included; markPath is the same without them, as marks are
// written.
func seal(v interface{}, path []string, markPath string, masks map[string]struct{},
	keep func(path []string, v interface{}) interface{}) interface{} {
	if markPath != "" && MaskedPath(markPath, masks) {
		return keep(append([]string{}, path...), v)
	}
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, child := range val {
			out[k] = seal(child, append(path, k), joinMaskPath(markPath, k), masks, keep)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, child := range val {
			out[i] = seal(child, append(path, strconv.Itoa(i)), markPath, masks, keep)
		}
		return out
	default:
		return v
	}
}

// setAt puts v at path inside root, reporting whether the path was there to put it.
func setAt(root map[string]interface{}, path []string, v interface{}) bool {
	if root == nil || len(path) == 0 {
		return false
	}
	var parent interface{} = root
	for i, seg := range path {
		last := i == len(path)-1
		switch node := parent.(type) {
		case map[string]interface{}:
			if _, ok := node[seg]; !ok {
				return false
			}
			if last {
				node[seg] = v
				return true
			}
			parent = node[seg]
		case []interface{}:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return false
			}
			if last {
				node[idx] = v
				return true
			}
			parent = node[idx]
		default:
			return false
		}
	}
	return false
}

func sealedKey(source string, path []string) string {
	return source + "." + strings.Join(path, ".")
}
