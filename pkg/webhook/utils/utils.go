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

	upstreamcuex "github.com/kubevela/pkg/cue/cuex"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/helm"

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

// ValidateCuexTemplate validate cueTemplate with CueX for types utilising it.
// Uses WorkloadCompiler so that templates referencing internal provider
// packages (e.g. "vela/helm") parse during admission validation.
//
// The compile runs under helm.WithDryRun so that any provider package that
// honors the dry-run signal short-circuits to a side-effect-free path.
// Without this, a ComponentDefinition whose CUE supplies fully concrete
// arguments to helm.#Render could trigger a real chart fetch and helm
// install during admission.
func ValidateCuexTemplate(ctx context.Context, cueTemplate string) error {
	return validateCuexTemplate(ctx, cueTemplate)
}

// ValidateCuexTemplateWithoutProviders validates a template's shape without
// executing the provider functions in it.
//
// A SourceDefinition's whole purpose is to fetch something, and admission runs
// with no parameters supplied - so every provider call is either handed a
// non-concrete value ("cannot convert incomplete value \"string\" to JSON") or,
// worse, actually performed. Performing it would do the source's I/O on every
// apply, which is the exact cost the cache exists to avoid, and would make
// admission depend on a remote service being reachable.
//
// The shape checks that matter - the schema block, the storage block, the
// generated key - are all static and unaffected.
func ValidateCuexTemplateWithoutProviders(ctx context.Context, cueTemplate string) error {
	return validateCuexTemplate(ctx, cueTemplate, upstreamcuex.DisableResolveProviderFunctions{})
}

// ValidateSourceTemplate validates a SourceDefinition's template against the
// packages a source is allowed to import.
//
// Compiling against SourceCompiler rather than WorkloadCompiler is what refuses
// an acting package at apply time. Without it a source importing vela/helm would
// be accepted and then install a chart on its first cache miss, since the render
// path sets no dry-run.
func ValidateSourceTemplate(ctx context.Context, cueTemplate string) error {
	val, err := velacuex.SourceCompiler.Get().CompileStringWithOptions(
		ctx, cueTemplate, upstreamcuex.DisableResolveProviderFunctions{})
	if err != nil {
		return err
	}
	if e := checkError(val.Err()); e != nil {
		return e
	}
	return checkError(val.Validate())
}

func validateCuexTemplate(ctx context.Context, cueTemplate string, opts ...upstreamcuex.CompileOption) error {
	ctx = helm.WithDryRun(ctx)
	val, err := velacuex.WorkloadCompiler.Get().CompileStringWithOptions(ctx, cueTemplate, opts...)
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
