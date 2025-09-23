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

	"cuelang.org/go/cue/cuecontext"
	cueErrors "cuelang.org/go/cue/errors"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
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

// ValidateCuexTemplate validate cueTemplate with CueX for types utilising it
func ValidateCuexTemplate(ctx context.Context, cueTemplate string) error {
	val, err := cuex.DefaultCompiler.Get().CompileStringWithOptions(ctx, cueTemplate)
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

// ExtractResourceInfo extracts apiVersion and kind from CUE template without evaluation
func ExtractResourceInfo(cueTemplate string) ([]ResourceInfo, error) {
	file, err := parser.ParseFile("", cueTemplate, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CUE template: %w", err)
	}

	var resources []ResourceInfo

	// Walk through the AST to find output and outputs fields
	ast.Walk(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Field:
			label := extractLabel(n.Label)
			if label == "output" || label == "outputs" {
				if label == "output" {
					// Extract from single output field
					if resource := extractResourceFromStruct(n.Value); resource != nil {
						resources = append(resources, *resource)
					}
				} else if label == "outputs" {
					// Extract from outputs field (multiple resources)
					resources = append(resources, extractResourcesFromOutputs(n.Value)...)
				}
			}
		}
		return true
	}, nil)

	return resources, nil
}

type ResourceInfo struct {
	APIVersion string
	Kind       string
	Name       string // optional, for better error messages
}

func extractLabel(label ast.Label) string {
	switch l := label.(type) {
	case *ast.Ident:
		return l.Name
	case *ast.BasicLit:
		if l.Kind == token.STRING {
			// Remove quotes
			return strings.Trim(l.Value, `"`)
		}
	}
	return ""
}

func extractResourceFromStruct(expr ast.Expr) *ResourceInfo {
	structLit, ok := expr.(*ast.StructLit)
	if !ok {
		return nil
	}

	resource := &ResourceInfo{}

	for _, elt := range structLit.Elts {
		field, ok := elt.(*ast.Field)
		if !ok {
			continue
		}

		label := extractLabel(field.Label)
		value := extractStringValue(field.Value)

		switch label {
		case "apiVersion":
			resource.APIVersion = value
		case "kind":
			resource.Kind = value
		case "metadata":
			// Try to extract name from metadata.name
			if name := extractNameFromMetadata(field.Value); name != "" {
				resource.Name = name
			}
		}
	}

	// Only return if we have both apiVersion and kind
	if resource.APIVersion != "" && resource.Kind != "" {
		return resource
	}
	return nil
}

func extractResourcesFromOutputs(expr ast.Expr) []ResourceInfo {
	var resources []ResourceInfo

	structLit, ok := expr.(*ast.StructLit)
	if !ok {
		return resources
	}

	for _, elt := range structLit.Elts {
		field, ok := elt.(*ast.Field)
		if !ok {
			continue
		}

		if resource := extractResourceFromStruct(field.Value); resource != nil {
			// Use the field label as the resource name if not found in metadata
			if resource.Name == "" {
				resource.Name = extractLabel(field.Label)
			}
			resources = append(resources, *resource)
		}
	}

	return resources
}

func extractStringValue(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			return strings.Trim(e.Value, `"`)
		}
	}
	return ""
}

func extractNameFromMetadata(expr ast.Expr) string {
	structLit, ok := expr.(*ast.StructLit)
	if !ok {
		return ""
	}

	for _, elt := range structLit.Elts {
		field, ok := elt.(*ast.Field)
		if !ok {
			continue
		}

		if extractLabel(field.Label) == "name" {
			return extractStringValue(field.Value)
		}
	}
	return ""
}

// ValidateOutputResourcesExist validates that resources referenced in output/outputs fields exist on the cluster
func ValidateOutputResourcesExist(cueTemplate string, mapper meta.RESTMapper) error {
	resources, err := ExtractResourceInfo(cueTemplate)
	if err != nil {
		return fmt.Errorf("failed to extract resource info: %w", err)
	}

	for _, resource := range resources {
		gvk := schema.GroupVersionKind{
			Version: resource.APIVersion,
			Kind:    resource.Kind,
		}

		// Parse the apiVersion to get group and version
		if strings.Contains(resource.APIVersion, "/") {
			parts := strings.SplitN(resource.APIVersion, "/", 2)
			gvk.Group = parts[0]
			gvk.Version = parts[1]
		} else {
			// Core API resources (like v1) don't have a group
			gvk.Group = ""
			gvk.Version = resource.APIVersion
		}

		_, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			resourceName := resource.Name
			if resourceName == "" {
				resourceName = fmt.Sprintf("%s/%s", resource.APIVersion, resource.Kind)
			}
			return fmt.Errorf("resource type not found on cluster: %s (%s)", resourceName, err.Error())
		}
	}

	return nil
}
