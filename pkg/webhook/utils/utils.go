/*
 Copyright 2021. The KubeVela Authors.

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
	"regexp"
	"strconv"
	"strings"

	"github.com/kubevela/pkg/cue/cuex"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	workflowproviders "github.com/oam-dev/kubevela/pkg/workflow/providers"

	"cuelang.org/go/cue/cuecontext"
	cueErrors "cuelang.org/go/cue/errors"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/core"
)

// ContextRegex to match '**: reference "context" not found'
var ContextRegex = `^.+:\sreference\s\"context\"\snot\sfound$`

// ValidateDefinitionRevision validate whether definition will modify the immutable object definitionRevision
func ValidateDefinitionRevision(ctx context.Context, cli client.Client, def runtime.Object, defRevNamespacedName types.NamespacedName) error {
	if errs := validation.IsQualifiedName(defRevNamespacedName.Name); len(errs) != 0 {
		return errors.Errorf("invalid definitionRevision name %s:%s", defRevNamespacedName.Name, strings.Join(errs, ","))
	}
	defRev := new(v1beta1.DefinitionRevision)
	if err := cli.Get(ctx, defRevNamespacedName, defRev); err != nil {
		return client.IgnoreNotFound(err)
	}

	newRev, _, err := core.GatherRevisionInfo(def)
	if err != nil {
		return err
	}
	if defRev.Spec.RevisionHash != newRev.Spec.RevisionHash {
		return errors.New("the definition's spec is different with existing definitionRevision's spec")
	}
	if !core.DeepEqualDefRevision(defRev, newRev) {
		return errors.New("the definition's spec is different with existing definitionRevision's spec")
	}
	return nil
}

// ValidateCueTemplate validate cueTemplate
func ValidateCueTemplate(cueTemplate string) error {
	val := cuecontext.New().CompileString(cueTemplate)
	if e := checkError(val.Err()); e != nil {
		return e
	}
	err := val.Validate()
	return checkError(err)
}

// ValidateCuexTemplate validates a ComponentDefinition or TraitDefinition CUE
// template with CueX, using the workload compiler that renders those kinds.
//
// The compiler must match the one that renders the definition. Three import
// paths, "vela/kube", "vela/http" and "vela/config", are registered by both the
// workload and the workflow compiler but resolve to different packages with
// different selectors and different $params schemas. Compiling against the wrong
// flavor does not raise an error: a selector the flavor does not define is
// silently accepted, and the $params schema check that would have caught a
// misspelled or mistyped argument disappears with it. Validating a component
// template against the workflow compiler therefore turns a checked template into
// an unchecked one, quietly. See ValidateWorkflowStepCuexTemplate for the
// counterpart used by workflow steps.
func ValidateCuexTemplate(ctx context.Context, cueTemplate string) error {
	return validateCuexTemplateWith(ctx, velacuex.WorkloadCompiler.Get(), cueTemplate)
}

// ValidateWorkflowStepCuexTemplate validates a WorkflowStepDefinition CUE
// template against the workflow provider compiler.
//
// Step templates import workflow-only builtin packages that the workload
// compiler does not register, "vela/op" above all. Validating them with
// ValidateCuexTemplate rejects 26 of the 36 bundled step definitions with
// `builtin package "vela/op" undefined`, so the two kinds cannot share a
// compiler. They also cannot share the workflow one, for the reason described on
// ValidateCuexTemplate.
func ValidateWorkflowStepCuexTemplate(ctx context.Context, cueTemplate string) error {
	return validateCuexTemplateWith(ctx, workflowproviders.DefaultCompiler.Get(), cueTemplate)
}

// validateCuexTemplateWith compiles cueTemplate against the given compiler and
// reports whether it is a valid definition template.
//
// Compiles with DisableResolveProviderFunctions so provider functions are never
// executed. Admission is a type check, and resolving would let a definition
// whose CUE supplies fully concrete arguments trigger real side effects (a chart
// fetch and helm install, a cluster read) merely by being submitted. Skipping
// resolution also removes the need for the previous helm.WithDryRun guard, which
// only protected the one provider that honored the signal; its other caller in
// pkg/appfile stays, so the helper is not orphaned.
//
// Provider function arguments are still type-checked, because imports and
// package schemas are resolved by BuildInstance before resolution would run.
// Unresolved provider outputs ($returns) are left incomplete rather than
// erroring, since Validate is called without cue.Concrete and templates are
// legitimately incomplete until an Application supplies parameter values.
func validateCuexTemplateWith(ctx context.Context, compiler *cuex.Compiler, cueTemplate string) error {
	val, err := compiler.CompileStringWithOptions(ctx, cueTemplate, cuex.DisableResolveProviderFunctions{})
	if err != nil {
		return err
	}
	if e := checkError(val.Err()); e != nil {
		return e
	}
	err = val.Validate()
	return checkError(err)
}

func checkError(err error) error {
	re := regexp.MustCompile(ContextRegex)
	if err != nil {
		// ignore context not found error
		for _, e := range cueErrors.Errors(err) {
			if !re.MatchString(e.Error()) {
				return cueErrors.New(e.Error())
			}
		}
	}
	return nil
}

// ValidateSemanticVersion validates if a Definition's version includes all of
// major,minor & patch version values.
func ValidateSemanticVersion(version string) error {
	if version != "" {
		versionParts := strings.Split(version, ".")
		if len(versionParts) != 3 {
			return errors.New("Not a valid version")
		}

		for _, versionPart := range versionParts {
			if _, err := strconv.Atoi(versionPart); err != nil {
				return errors.New("Not a valid version")
			}
		}
	}
	return nil
}

// ValidateMultipleDefVersionsNotPresent validates that both Name Annotation Revision and Spec.Version are not present
func ValidateMultipleDefVersionsNotPresent(version, revisionName, objectType string) error {
	if version != "" && revisionName != "" {
		return fmt.Errorf("%s has both spec.version and revision name annotation. Only one can be present", objectType)
	}
	return nil
}
