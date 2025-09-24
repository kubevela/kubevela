package utils

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
			ref := fmt.Sprintf("%s/%s", resource.APIVersion, resource.Kind)
			return fmt.Errorf("resource type not found on cluster: %s (%v)", ref, err)
		}
	}

	return nil
}
