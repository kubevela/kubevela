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

package utils

import (
	"context"
	"fmt"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
)

// ValidateInheritedTemplate validates a definition that extends another. It
// replaces ValidateCuexTemplate for one, since `$super` is declared nowhere in
// the child's template and compiling it alone always fails.
//
// Refused: never calling the parent, omitting a parameter it requires with no
// default, a call it would not accept. Warned: a name it does not declare, and a
// published parameter that reaches neither the parent nor the child's output.
//
// alsoRead are the healthPolicy, customStatus and details, so a parameter read
// only there is not reported as unused.
func ValidateInheritedTemplate(ctx context.Context, name, template string, ancestors []inherit.Level, surface inherit.Surface, alsoRead ...string) ([]string, error) {
	chain := make([]inherit.Level, 0, len(ancestors)+1)
	chain = append(chain, inherit.Level{Name: name, Template: template})
	chain = append(chain, ancestors...)

	if err := validateOwnTemplate(ctx, name, template); err != nil {
		return nil, err
	}
	return inherit.CheckCall(ctx, chain, surface, admissionCompiler, alsoRead...)
}

// validateOwnTemplate checks the child's own CUE, which the call check does
// not: that judges only the `$super` block against the parent.
//
// The chain is not resolved here, so `$super` is left open. What it holds is the
// call check's business.
func validateOwnTemplate(ctx context.Context, name, template string) error {
	val, err := velacuex.WorkloadCompiler.Get().CompileStringWithOptions(
		ctx, openTemplate(template)+"\n"+inherit.SuperField+": _\n",
		cuex.DisableResolveProviderFunctions{})
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := val.Err(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := val.Validate(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// StatusSources returns the CUE a definition carries outside its template.
func StatusSources(status *common.Status) []string {
	if status == nil {
		return nil
	}
	return []string{status.HealthPolicy, status.CustomStatus, status.Details}
}

// admissionCompiler compiles a template for validation, with provider functions
// off so admission has no side effects: a template may call `vela/helm`.
// `context` and `parameter` are opened, or a parent reading `context.name` does
// not resolve and its parameters cannot be read.
//
// The compiler is the render path's own, since a definition that imports a
// provider package has to validate as the thing that will render it.
func admissionCompiler(ctx context.Context, src string) (cue.Value, error) {
	val, err := velacuex.WorkloadCompiler.Get().CompileStringWithOptions(ctx, openTemplate(src), cuex.DisableResolveProviderFunctions{})
	if err != nil {
		return val, err
	}
	return val, val.Err()
}

// openTemplate closes a template over the fields a render supplies, so it
// compiles as a file in its own right.
func openTemplate(src string) string {
	return src + "\ncontext: _\nparameter: _\n"
}

// ValidateExtendsHasTemplate refuses `extends` on a definition with no CUE
// template.
//
// A chain is composed by rendering the child's template against its parent's,
// so a child with no template calls nothing: it inherits no surface and
// describes nothing of its own. Judged on write, because at render the
// complaint is about a missing `$super` block rather than about the definition
// being empty, and because everything else the handler checks about `extends`
// needs a template and so is skipped without one.
func ValidateExtendsHasTemplate(kind, name, extends string, schematic *common.Schematic) error {
	if extends == "" || (schematic != nil && schematic.CUE != nil) {
		return nil
	}
	return fmt.Errorf(
		"%s %s extends %s but has no CUE template to call it from; "+
			"add a `%s: properties: {...}` block",
		kind, name, extends, inherit.SuperField)
}
