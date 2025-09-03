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
	"fmt"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/features"
)

// ExtractResourceInfo extracts apiVersion and kind from CUE template without evaluation
func ExtractResourceInfo(cueTemplate string) ([]ResourceInfo, error) {
	file, err := parser.ParseFile("", cueTemplate, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CUE template: %w", err)
	}

	var resources []ResourceInfo

	// Walk through the AST to find output and outputs fields
	ast.Walk(file, func(node ast.Node) bool {
		if n, ok := node.(*ast.Field); ok {
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
	if e, ok := expr.(*ast.BasicLit); ok {
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
func ValidateOutputResourcesExist(cueTemplate string, mapper meta.RESTMapper, obj client.Object) error {
	// Check if feature gate is enabled FIRST before doing anything
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateResourcesExist) {
		return nil // Skip validation if feature is disabled
	}

	// Skip validation for addon definitions
	// Addons often bundle CRDs with definitions that reference them
	if IsAddonDefinition(obj) {
		return nil
	}

	// Only extract and validate resources if feature is enabled and not an addon
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
			ref := fmt.Sprintf("%s/%s", resource.APIVersion, resource.Kind)
			return fmt.Errorf("resource type not found on cluster: %s (%w)", ref, err)
		}
	}

	return nil
}

// IsAddonDefinition checks if the object is part of an addon installation
// This is a generic solution that works for ALL addons by checking owner references
func IsAddonDefinition(obj client.Object) bool {
	if obj == nil {
		return false
	}

	// Generic approach: Check if the object has an owner reference to an addon application
	// All addon definitions get owner references to their addon application (pattern: addon-{name})
	for _, ownerRef := range obj.GetOwnerReferences() {
		if ownerRef.Kind == "Application" &&
			ownerRef.APIVersion == "core.oam.dev/v1beta1" &&
			strings.HasPrefix(ownerRef.Name, "addon-") {
			return true
		}
	}

	// Fallback checks for edge cases where owner references might not be set yet
	labels := obj.GetLabels()
	if labels != nil {
		// Check if this is managed by an addon application
		appName, hasApp := labels["app.oam.dev/name"]
		if hasApp && strings.HasPrefix(appName, "addon-") {
			return true
		}
	}

	// Also check annotations for addon markers
	annotations := obj.GetAnnotations()
	if annotations != nil {
		if addonName, hasAddon := annotations["addons.oam.dev/name"]; hasAddon && addonName != "" {
			return true
		}
	}

	return false
}
