/*
Copyright 2025 The KubeVela Authors.

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

package addon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/definition/goloader"
)

// ReadGoDefFolder reads the godef/ folder from an addon directory and returns GoDefModule info
func ReadGoDefFolder(addonPath string) (*GoDefModule, error) {
	godefPath := filepath.Join(addonPath, GoDefDirName)

	// Check if godef/ directory exists
	info, err := os.Stat(godefPath)
	if os.IsNotExist(err) {
		return nil, nil // No godef folder, not an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat godef directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("godef is not a directory")
	}

	module := &GoDefModule{
		Path: godefPath,
	}

	// Read module.yaml if it exists
	moduleYAMLPath := filepath.Join(godefPath, GoDefModuleFileName)
	if data, err := os.ReadFile(moduleYAMLPath); err == nil {
		module.ModuleYAML = string(data)
	}

	// Read go.mod if it exists
	goModPath := filepath.Join(godefPath, "go.mod")
	if data, err := os.ReadFile(goModPath); err == nil {
		module.GoMod = string(data)
	}

	// Read go.sum if it exists
	goSumPath := filepath.Join(godefPath, "go.sum")
	if data, err := os.ReadFile(goSumPath); err == nil {
		module.GoSum = string(data)
	}

	// Read all .go files recursively
	err = filepath.Walk(godefPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read Go file %s: %w", path, err)
			}
			relPath, _ := filepath.Rel(godefPath, path)
			module.GoFiles = append(module.GoFiles, ElementFile{
				Name: relPath,
				Data: string(data),
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read Go files: %w", err)
	}

	return module, nil
}

// CompileGoDefinitions compiles Go-based definitions from a godef/ folder
// and returns the compiled CUE definitions.
// Returns an error if any definition fails to compile or if no definitions are found.
func CompileGoDefinitions(ctx context.Context, godefPath string) ([]ElementFile, error) {
	klog.V(2).Infof("Compiling Go definitions from %s", godefPath)

	// Check if the path exists
	if _, err := os.Stat(godefPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("godef path does not exist: %s", godefPath)
	}

	// Load the module using the goloader
	opts := goloader.DefaultModuleLoadOptions()
	module, err := goloader.LoadModule(ctx, godefPath, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load Go definition module: %w", err)
	}

	// Check if any definitions were discovered
	if len(module.Definitions) == 0 {
		return nil, fmt.Errorf("no Go definitions found in %s. Ensure your Go files define functions that return *defkit.ComponentDefinition, *defkit.TraitDefinition, etc.", godefPath)
	}

	// Compile all definitions to CUE, collecting errors
	var compiledDefs []ElementFile
	var compilationErrors []string

	for _, result := range module.Definitions {
		defName := result.Definition.Name
		if defName == "" {
			defName = result.Definition.FilePath
		}

		if result.Error != nil {
			compilationErrors = append(compilationErrors, fmt.Sprintf("%s: %v", defName, result.Error))
			continue
		}

		if result.CUE == "" {
			compilationErrors = append(compilationErrors, fmt.Sprintf("%s: no CUE output generated (template may be empty)", defName))
			continue
		}

		compiledDefs = append(compiledDefs, ElementFile{
			Name: fmt.Sprintf("%s-%s.cue", result.Definition.Type, result.Definition.Name),
			Data: result.CUE,
		})
	}

	// If there were compilation errors, return them
	if len(compilationErrors) > 0 {
		return nil, fmt.Errorf("failed to compile Go definitions:\n  - %s\n\nHint: Ensure go.mod has a 'replace' directive pointing to a local kubevela checkout, e.g.:\n  replace github.com/oam-dev/kubevela => /path/to/kubevela", strings.Join(compilationErrors, "\n  - "))
	}

	klog.V(2).Infof("Compiled %d definitions from Go sources", len(compiledDefs))
	return compiledDefs, nil
}

// HasGoDefFolder checks if an addon directory has a godef/ folder
func HasGoDefFolder(addonPath string) bool {
	godefPath := filepath.Join(addonPath, GoDefDirName)
	info, err := os.Stat(godefPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// CompileGoDefinitionsFromAddon compiles Go-based definitions from an addon directory
// This is a convenience function that validates and compiles in one step
func CompileGoDefinitionsFromAddon(ctx context.Context, addonPath string) ([]ElementFile, error) {
	godefPath := filepath.Join(addonPath, GoDefDirName)

	// Validate the godef module structure
	if err := ValidateGoDefModule(godefPath); err != nil {
		return nil, fmt.Errorf("invalid godef module: %w", err)
	}

	// Compile the Go definitions
	return CompileGoDefinitions(ctx, godefPath)
}

// DetectDefinitionConflicts checks for conflicting definition names between CUE and Go definitions
// Returns a list of conflicting definition names
func DetectDefinitionConflicts(cueDefinitions, goDefinitions []ElementFile) []string {
	cueDefNames := make(map[string]bool)
	for _, def := range cueDefinitions {
		name := extractDefinitionName(def)
		if name != "" {
			cueDefNames[name] = true
		}
	}

	var conflicts []string
	for _, def := range goDefinitions {
		name := extractDefinitionName(def)
		if name != "" && cueDefNames[name] {
			conflicts = append(conflicts, name)
		}
	}
	return conflicts
}

// extractDefinitionName extracts the definition name from an ElementFile
// For CUE files, it looks for the definition name in the file content
// For compiled Go definitions, the name is typically in the format "type-name.cue"
func extractDefinitionName(def ElementFile) string {
	// For compiled Go definitions, the filename format is "type-name.cue"
	// e.g., "component-my-webservice.cue" -> "my-webservice"
	name := strings.TrimSuffix(def.Name, ".cue")
	name = strings.TrimSuffix(name, ".yaml")
	name = strings.TrimSuffix(name, ".yml")

	// Remove type prefix if present (component-, trait-, policy-, workflow-step-)
	prefixes := []string{"component-", "trait-", "policy-", "workflow-step-"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return strings.TrimPrefix(name, prefix)
		}
	}

	// For CUE definitions without type prefix, try to extract from content
	// Look for patterns like `"my-component": { ... type: "component"`
	// This is a simplified extraction - the actual name is usually the filename
	return name
}

// ValidateGoDefModule validates the Go definition module structure
func ValidateGoDefModule(godefPath string) error {
	// Check for go.mod
	goModPath := filepath.Join(godefPath, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		return fmt.Errorf("godef/ folder must contain go.mod file")
	}

	// Check for at least one .go file
	hasGoFile := false
	err := filepath.Walk(godefPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			hasGoFile = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return fmt.Errorf("failed to check for Go files: %w", err)
	}
	if !hasGoFile {
		return fmt.Errorf("godef/ folder must contain at least one .go file (non-test)")
	}

	return nil
}
