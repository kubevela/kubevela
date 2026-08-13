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

package defkit

import (
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// EvaluatedOutputs contains the concrete resources produced by evaluating a
// component's generated CUE template.
type EvaluatedOutputs struct {
	Primary   map[string]any
	Auxiliary map[string]map[string]any
}

// compileWithContextShim compiles generated CUE with a placeholder `context`
// declaration. KubeVela injects the runtime `context` field at render time,
// so a definition referencing context.* does not compile standalone; the
// shim matches how the field is actually supplied. Shared by
// ValidateGeneratedCUE and EvaluateCUE so both stay compiled the same way.
func compileWithContextShim(src string) cue.Value {
	return cuecontext.New().CompileString(src + "\ncontext: _\n")
}

// ValidateGeneratedCUE compiles a definition's generated CUE and returns CUE
// syntax and structural errors with their source positions. It does not
// evaluate the template, so no parameter fixtures are needed.
//
// This deliberately compiles rather than merely parses: cuelang.org/go's
// parser package documents itself as accepting a larger language than the
// CUE spec permits, for parser robustness, so a bare parse is not a
// reliable validity check. Compilation is the authoritative one.
func ValidateGeneratedCUE(def Definition) error {
	if def == nil {
		return fmt.Errorf("defkit: cannot validate a nil definition")
	}
	if err := compileWithContextShim(def.ToCue()).Err(); err != nil {
		return fmt.Errorf("defkit: generated CUE for %s %q: %w", def.DefType(), def.DefName(), err)
	}
	return nil
}

// EvaluateCUE evaluates the generated CUE for a component using the supplied
// test fixtures. Unlike Render, it executes CUE semantics, including schema
// defaults, validators, raw CUE, references, and comprehensions.
func (c *ComponentDefinition) EvaluateCUE(ctx *TestContextBuilder) (*EvaluatedOutputs, error) {
	if c == nil {
		return nil, fmt.Errorf("defkit: cannot evaluate a nil component")
	}
	if ctx == nil {
		return nil, fmt.Errorf("defkit: cannot evaluate component %q with a nil test context", c.DefName())
	}

	runtime := ctx.Build()
	value := compileWithContextShim(c.ToCue())

	value = value.FillPath(cue.ParsePath("context"), cueContextFixture(runtime))
	value = value.FillPath(cue.ParsePath("template.parameter"), runtime.params)
	if err := value.Err(); err != nil {
		return nil, fmt.Errorf("defkit: evaluate component %q with the supplied fixtures: %w", c.DefName(), err)
	}

	template := value.LookupPath(cue.ParsePath("template"))
	if err := template.Err(); err != nil {
		return nil, fmt.Errorf("defkit: evaluate template for component %q: %w", c.DefName(), err)
	}

	outputs := &EvaluatedOutputs{Auxiliary: make(map[string]map[string]any)}
	if primary := template.LookupPath(cue.ParsePath("output")); primary.Exists() {
		resource, err := decodeConcreteResource(primary)
		if err != nil {
			return nil, fmt.Errorf("defkit: evaluate primary output for component %q: %w", c.DefName(), err)
		}
		outputs.Primary = resource
	}

	auxiliary := template.LookupPath(cue.ParsePath("outputs"))
	if !auxiliary.Exists() {
		return outputs, nil
	}
	iterator, err := auxiliary.Fields(cue.Definitions(false), cue.Hidden(false), cue.Optional(true))
	if err != nil {
		return nil, fmt.Errorf("defkit: enumerate auxiliary outputs for component %q: %w", c.DefName(), err)
	}
	for iterator.Next() {
		name := iterator.Selector().Unquoted()
		resource, err := decodeConcreteResource(iterator.Value())
		if err != nil {
			return nil, fmt.Errorf("defkit: evaluate auxiliary output %q for component %q: %w", name, c.DefName(), err)
		}
		outputs.Auxiliary[name] = resource
	}
	return outputs, nil
}

func cueContextFixture(ctx *TestRuntimeContext) map[string]any {
	context := map[string]any{
		"name":        ctx.name,
		"namespace":   ctx.namespace,
		"appName":     ctx.appName,
		"appRevision": ctx.appRevision,
		"clusterVersion": map[string]any{
			"major": ctx.clusterMajor,
			"minor": ctx.clusterMinor,
		},
	}
	if ctx.outputStatus != nil {
		context["output"] = map[string]any{"status": ctx.outputStatus}
	}
	if len(ctx.outputsStatus) > 0 {
		outputs := make(map[string]any, len(ctx.outputsStatus))
		for name, status := range ctx.outputsStatus {
			outputs[name] = map[string]any{"status": status}
		}
		context["outputs"] = outputs
	}
	return context
}

func decodeConcreteResource(value cue.Value) (map[string]any, error) {
	if err := value.Validate(cue.Concrete(true)); err != nil {
		return nil, err
	}
	resource := map[string]any{}
	if err := value.Decode(&resource); err != nil {
		return nil, err
	}
	return resource, nil
}
