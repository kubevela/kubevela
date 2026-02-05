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

// Package goloader provides functionality to load and process Go-based KubeVela definitions.
// It supports loading definitions from Go source files and converting them to CUE format.
package goloader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
	"github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
)

// GoExtension is the file extension for Go files
const GoExtension = ".go"

// DefinitionInfo holds information about a discovered Go definition
type DefinitionInfo struct {
	// Name is the definition name (e.g., "webservice", "daemon")
	Name string
	// Type is the definition type (e.g., "component", "trait", "policy", "workflow-step")
	Type string
	// FunctionName is the Go function that returns the definition
	FunctionName string
	// FilePath is the path to the Go file containing the definition
	FilePath string
	// PackageName is the Go package name
	PackageName string
	// Placement contains the definition-level placement constraints (if any)
	Placement *DefinitionPlacement
}

// DefinitionPlacement holds placement constraints for a definition.
// This is extracted from the definition's Go code.
type DefinitionPlacement struct {
	// RunOn specifies conditions that must be satisfied for the definition to run.
	RunOn []PlacementCondition `json:"runOn,omitempty"`
	// NotRunOn specifies conditions that exclude clusters from running the definition.
	NotRunOn []PlacementCondition `json:"notRunOn,omitempty"`
}

// PlacementCondition represents a single placement condition.
type PlacementCondition struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

// ToPlacementSpec converts DefinitionPlacement to placement.PlacementSpec.
func (p *DefinitionPlacement) ToPlacementSpec() placement.PlacementSpec {
	if p == nil {
		return placement.PlacementSpec{}
	}

	spec := placement.PlacementSpec{}

	for _, cond := range p.RunOn {
		spec.RunOn = append(spec.RunOn, &placement.LabelCondition{
			Key:      cond.Key,
			Operator: placement.Operator(cond.Operator),
			Values:   cond.Values,
		})
	}

	for _, cond := range p.NotRunOn {
		spec.NotRunOn = append(spec.NotRunOn, &placement.LabelCondition{
			Key:      cond.Key,
			Operator: placement.Operator(cond.Operator),
			Values:   cond.Values,
		})
	}

	return spec
}

// IsEmpty returns true if no placement constraints are defined.
func (p *DefinitionPlacement) IsEmpty() bool {
	return p == nil || (len(p.RunOn) == 0 && len(p.NotRunOn) == 0)
}

// LoadResult contains the result of loading Go definitions
type LoadResult struct {
	// CUE is the generated CUE string
	CUE string
	// YAML is the generated YAML string (Kubernetes CR format)
	YAML []byte
	// Definition contains metadata about the definition
	Definition DefinitionInfo
	// Error is set if loading failed for this definition
	Error error
}

// GeneratorEnvironment manages a reusable temp directory for CUE generation.
// This avoids running `go mod tidy` for each definition - it runs only ONCE per module.
type GeneratorEnvironment struct {
	// TempDir is the temporary directory with go.mod already set up
	TempDir string
	// ModuleRoot is the root directory of the user's Go module
	ModuleRoot string
	// ModuleName is the Go module name from go.mod
	ModuleName string
	// mu protects concurrent access during parallel generation
	mu sync.Mutex
}

// NewGeneratorEnvironment creates a new generator environment for a module.
// It sets up the temp directory and runs `go mod tidy` ONCE.
func NewGeneratorEnvironment(moduleRoot string) (*GeneratorEnvironment, error) {
	// Get the module name from go.mod
	moduleName, err := getModuleNameFromRoot(moduleRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get module name for %s", moduleRoot)
	}

	// Create a temporary directory for the generator
	tempDir, err := os.MkdirTemp("", "vela-def-gen-*")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp directory")
	}

	// Discover all Go packages in the module (components, traits, etc.)
	subPackages := discoverSubPackages(moduleRoot, moduleName)

	// Create go.mod that references the original module
	// First, try to copy replace directives from the source module's go.mod
	// This handles cases where the user has a local replace for kubevela
	sourceReplaces := getReplacesFromGoMod(moduleRoot)

	// If no kubevela replace in source, try to find it locally
	kubeVelaReplace := ""
	if !strings.Contains(sourceReplaces, "github.com/oam-dev/kubevela") {
		kubeVelaRoot := findKubeVelaRoot()
		if kubeVelaRoot != "" {
			kubeVelaReplace = fmt.Sprintf("replace github.com/oam-dev/kubevela => %s\n", kubeVelaRoot)
		}
	}

	goMod := fmt.Sprintf(`module vela-def-gen

go 1.21

require github.com/oam-dev/kubevela v0.0.0
require %s v0.0.0

%s%sreplace %s => %s
`, moduleName, sourceReplaces, kubeVelaReplace, moduleName, moduleRoot)

	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0600); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, errors.Wrap(err, "failed to write go.mod")
	}

	// Write a placeholder main.go that imports ALL subpackages
	// This ensures go mod tidy properly sets up requirements
	var imports strings.Builder
	imports.WriteString(fmt.Sprintf("\t_ \"%s\"\n", moduleName))
	for _, pkg := range subPackages {
		imports.WriteString(fmt.Sprintf("\t_ \"%s\"\n", pkg))
	}

	placeholderMain := fmt.Sprintf(`package main

import (
	"fmt"
%s)

func main() {
	fmt.Println("placeholder")
}
`, imports.String())

	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(placeholderMain), 0600); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, errors.Wrap(err, "failed to write placeholder main.go")
	}

	// Run go mod tidy ONCE for the entire module
	tidyCmd := exec.Command("go", "mod", "tidy", "-e")
	tidyCmd.Dir = tempDir
	tidyCmd.Env = append(os.Environ(), "GO111MODULE=on")
	if output, err := tidyCmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, errors.Wrapf(err, "go mod tidy failed: %s", string(output))
	}

	return &GeneratorEnvironment{
		TempDir:    tempDir,
		ModuleRoot: moduleRoot,
		ModuleName: moduleName,
	}, nil
}

// discoverSubPackages finds all Go packages under a module root
func discoverSubPackages(moduleRoot, moduleName string) []string {
	var packages []string
	seen := make(map[string]bool)

	filepath.Walk(moduleRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			// Skip hidden directories and common non-Go directories
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		// Check if this is a Go file (not test)
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Get the directory and compute import path
		dir := filepath.Dir(path)
		relPath, err := filepath.Rel(moduleRoot, dir)
		if err != nil {
			return nil
		}
		// Build the import path
		var importPath string
		if relPath == "." {
			importPath = moduleName
		} else {
			importPath = moduleName + "/" + filepath.ToSlash(relPath)
		}
		if !seen[importPath] {
			seen[importPath] = true
			if importPath != moduleName { // Don't duplicate the root
				packages = append(packages, importPath)
			}
		}
		return nil
	})

	return packages
}

// Close cleans up the temporary directory
func (env *GeneratorEnvironment) Close() error {
	if env.TempDir != "" {
		return os.RemoveAll(env.TempDir)
	}
	return nil
}

// GenerateCUE generates CUE for a single definition, reusing the pre-configured environment.
// This is much faster than GenerateCUEFromGoFile because it doesn't run `go mod tidy`.
// Note: This method is thread-safe but serializes access. For parallel processing,
// use GenerateCUEParallel which creates separate files for each worker.
func (env *GeneratorEnvironment) GenerateCUE(filePath string, defInfo DefinitionInfo) (string, error) {
	env.mu.Lock()
	defer env.mu.Unlock()

	return env.generateCUEInternal(filePath, defInfo, "main.go")
}

// generateCUEInternal is the internal implementation that generates CUE.
// The filename parameter allows parallel workers to use different files.
func (env *GeneratorEnvironment) generateCUEInternal(filePath string, defInfo DefinitionInfo, filename string) (string, error) {
	// Get the import path for this file
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get absolute path for %s", filePath)
	}

	_, importPath, err := findModuleInfo(filepath.Dir(absPath))
	if err != nil {
		return "", errors.Wrapf(err, "failed to find module info for %s", filePath)
	}

	// Generate the Go program for this definition
	genProgram := generateCUEGeneratorProgram(importPath, defInfo)

	// Write the generator program
	genFile := filepath.Join(env.TempDir, filename)
	if err := os.WriteFile(genFile, []byte(genProgram), 0600); err != nil {
		return "", errors.Wrap(err, "failed to write generator program")
	}

	// Run the generator (no go mod tidy needed - already done)
	runCmd := exec.Command("go", "run", filename)
	runCmd.Dir = env.TempDir
	runCmd.Env = append(os.Environ(), "GO111MODULE=on")
	var stdout, stderr bytes.Buffer
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr
	if err := runCmd.Run(); err != nil {
		return "", errors.Wrapf(err, "generator failed: %s", stderr.String())
	}

	return stdout.String(), nil
}

// GenerateCUEParallel generates CUE for multiple definitions in parallel.
// It uses worker goroutines to process definitions concurrently.
// Each worker uses a unique filename to avoid conflicts.
func (env *GeneratorEnvironment) GenerateCUEParallel(definitions []DefinitionInfo) []LoadResult {
	results := make([]LoadResult, len(definitions))

	// Use number of CPUs as worker count, but cap at number of definitions
	numWorkers := runtime.NumCPU()
	if numWorkers > len(definitions) {
		numWorkers = len(definitions)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	// Create work channel
	type workItem struct {
		index   int
		defInfo DefinitionInfo
	}
	workChan := make(chan workItem, len(definitions))
	var wg sync.WaitGroup

	// Start workers - each worker gets a unique ID for its filename
	for workerID := 0; workerID < numWorkers; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Each worker uses a unique filename to enable true parallelism
			filename := fmt.Sprintf("worker_%d.go", id)
			for work := range workChan {
				result := LoadResult{Definition: work.defInfo}
				cue, err := env.generateCUEInternal(work.defInfo.FilePath, work.defInfo, filename)
				if err != nil {
					result.Error = err
				} else {
					result.CUE = cue
				}
				results[work.index] = result
			}
		}(workerID)
	}

	// Send work
	for i, def := range definitions {
		workChan <- workItem{index: i, defInfo: def}
	}
	close(workChan)

	// Wait for all workers to finish
	wg.Wait()

	return results
}

// IsGoFile checks if a file is a Go source file (not a test file)
func IsGoFile(path string) bool {
	return strings.HasSuffix(path, GoExtension) && !strings.HasSuffix(path, "_test.go")
}

// IsGoDefinitionFile checks if a Go file likely contains defkit definitions
// by looking for defkit imports
func IsGoDefinitionFile(path string) (bool, error) {
	if !IsGoFile(path) {
		return false, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return false, errors.Wrapf(err, "failed to read file %s", path)
	}

	// Quick check for defkit import
	return strings.Contains(string(content), "github.com/oam-dev/kubevela/pkg/definition/defkit"), nil
}

// DiscoverDefinitions finds Go files that contain defkit definitions in a directory
func DiscoverDefinitions(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		isDefFile, err := IsGoDefinitionFile(path)
		if err != nil {
			return err
		}
		if isDefFile {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, errors.Wrapf(err, "failed to walk directory %s", dir)
	}

	return files, nil
}

// AnalyzeGoFile analyzes a Go file and extracts definition function information
func AnalyzeGoFile(filePath string) ([]DefinitionInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse Go file %s", filePath)
	}

	var definitions []DefinitionInfo
	packageName := node.Name.Name

	// Look for functions that return *defkit.ComponentDefinition or similar
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Skip methods (we only want package-level functions)
		if fn.Recv != nil {
			continue
		}

		// Check return type
		if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
			continue
		}

		retType := fn.Type.Results.List[0].Type
		defType := getDefinitionType(retType)
		if defType == "" {
			continue
		}

		// Extract definition name from function name
		defName := extractDefinitionName(fn.Name.Name, defType)

		definitions = append(definitions, DefinitionInfo{
			Name:         defName,
			Type:         defType,
			FunctionName: fn.Name.Name,
			FilePath:     filePath,
			PackageName:  packageName,
		})
	}

	return definitions, nil
}

// getDefinitionType extracts the definition type from a return type AST
func getDefinitionType(expr ast.Expr) string {
	// Handle pointer type: *defkit.ComponentDefinition
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	// Handle selector expression: defkit.ComponentDefinition
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}

	// Check if it's from defkit package
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != "defkit" {
		return ""
	}

	switch sel.Sel.Name {
	case "ComponentDefinition":
		return "component"
	case "TraitDefinition":
		return "trait"
	case "PolicyDefinition":
		return "policy"
	case "WorkflowStepDefinition":
		return "workflow-step"
	default:
		return ""
	}
}

// extractDefinitionName extracts a definition name from a function name
// e.g., "WebserviceComponent" -> "webservice", "Daemon" -> "daemon"
func extractDefinitionName(funcName, defType string) string {
	// Remove common suffixes
	suffixes := []string{"Component", "Trait", "Policy", "WorkflowStep", "Definition"}
	name := funcName
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			name = strings.TrimSuffix(name, suffix)
			break
		}
	}

	// Convert to lowercase (simple case conversion)
	return strings.ToLower(name)
}

// GenerateCUEFromGoFile generates CUE from a Go definition file
// This uses a code generation approach: it creates a temporary Go program
// that imports the definition and calls ToCue() on it
func GenerateCUEFromGoFile(filePath string, defInfo DefinitionInfo) (string, error) {
	// Get the absolute path and module information
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get absolute path for %s", filePath)
	}

	// Find the module root and import path
	moduleRoot, importPath, err := findModuleInfo(filepath.Dir(absPath))
	if err != nil {
		return "", errors.Wrapf(err, "failed to find module info for %s", filePath)
	}

	// Get the module name from the import path (the base module, not subpackage)
	moduleName, err := getModuleNameFromRoot(moduleRoot)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get module name for %s", moduleRoot)
	}

	// Create a temporary directory for the generator
	tempDir, err := os.MkdirTemp("", "vela-def-gen-*")
	if err != nil {
		return "", errors.Wrap(err, "failed to create temp directory")
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Generate the temporary Go program
	genProgram := generateCUEGeneratorProgram(importPath, defInfo)

	// Write the generator program
	genFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(genFile, []byte(genProgram), 0600); err != nil {
		return "", errors.Wrap(err, "failed to write generator program")
	}

	// Create go.mod that references the original module
	// We need to add replace directives for:
	// 1. github.com/oam-dev/kubevela -> the kubevela repo (for defkit) - only if found locally
	// 2. The user's module -> their local path (so go mod tidy doesn't try to fetch from remote)
	kubeVelaRoot := findKubeVelaRoot()
	var kubeVelaReplace string
	if kubeVelaRoot != "" {
		kubeVelaReplace = fmt.Sprintf("replace github.com/oam-dev/kubevela => %s\n", kubeVelaRoot)
	}
	goMod := fmt.Sprintf(`module vela-def-gen

go 1.21

require github.com/oam-dev/kubevela v0.0.0
require %s v0.0.0

%sreplace %s => %s
`, moduleName, kubeVelaReplace, moduleName, moduleRoot)

	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0600); err != nil {
		return "", errors.Wrap(err, "failed to write go.mod")
	}

	// Run go mod tidy with -e flag to ignore errors for missing modules
	// This is needed because the user's module may have dependencies we can't resolve
	tidyCmd := exec.Command("go", "mod", "tidy", "-e")
	tidyCmd.Dir = tempDir
	tidyCmd.Env = append(os.Environ(), "GO111MODULE=on")
	if output, err := tidyCmd.CombinedOutput(); err != nil {
		return "", errors.Wrapf(err, "go mod tidy failed: %s", string(output))
	}

	// Run the generator
	runCmd := exec.Command("go", "run", "main.go")
	runCmd.Dir = tempDir
	runCmd.Env = append(os.Environ(), "GO111MODULE=on")
	var stdout, stderr bytes.Buffer
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr
	if err := runCmd.Run(); err != nil {
		return "", errors.Wrapf(err, "generator failed: %s", stderr.String())
	}

	return stdout.String(), nil
}

// getModuleNameFromRoot reads the module name from go.mod in the given directory
func getModuleNameFromRoot(moduleRoot string) (string, error) {
	goModContent, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		return "", errors.Wrap(err, "failed to read go.mod")
	}

	lines := strings.Split(string(goModContent), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimPrefix(line, "module "), nil
		}
	}
	return "", errors.New("could not find module name in go.mod")
}

// getReplacesFromGoMod reads replace directives from a go.mod file
// and returns them as a string that can be included in another go.mod.
// This allows copying replace directives from the source module to the temp module.
func getReplacesFromGoMod(moduleRoot string) string {
	goModContent, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		return ""
	}

	var replaces strings.Builder
	lines := strings.Split(string(goModContent), "\n")
	inReplaceBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Handle replace block: replace ( ... )
		if trimmed == "replace (" {
			inReplaceBlock = true
			continue
		}
		if inReplaceBlock {
			if trimmed == ")" {
				inReplaceBlock = false
				continue
			}
			// Each line in the block is a replace directive
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				replaces.WriteString("replace " + trimmed + "\n")
			}
			continue
		}

		// Handle single-line replace: replace foo => bar
		if strings.HasPrefix(trimmed, "replace ") {
			replaces.WriteString(trimmed + "\n")
		}
	}

	return replaces.String()
}

// findKubeVelaRoot attempts to find the kubevela repository root
// It first checks if we're running from within kubevela, then falls back to GOPATH
func findKubeVelaRoot() string {
	// First, try to find kubevela in the current working directory ancestry
	cwd, err := os.Getwd()
	if err == nil {
		current := cwd
		for {
			goModPath := filepath.Join(current, "go.mod")
			if content, err := os.ReadFile(goModPath); err == nil {
				if strings.Contains(string(content), "module github.com/oam-dev/kubevela") {
					return current
				}
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	// Try GOPATH
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(os.Getenv("HOME"), "go")
	}
	kubevelaPath := filepath.Join(gopath, "src", "github.com", "oam-dev", "kubevela")
	if _, err := os.Stat(kubevelaPath); err == nil {
		return kubevelaPath
	}

	// Return empty string - kubevela will be resolved from module cache by go mod tidy
	return ""
}

// generateCUEGeneratorProgram creates a Go program that generates CUE from a definition
func generateCUEGeneratorProgram(importPath string, defInfo DefinitionInfo) string {
	return fmt.Sprintf(`package main

import (
	"fmt"

	def "%s"
)

func main() {
	component := def.%s()
	fmt.Print(component.ToCue())
}
`, importPath, defInfo.FunctionName)
}

// findModuleInfo finds the Go module root and import path for a directory
func findModuleInfo(dir string) (moduleRoot, importPath string, err error) {
	// Walk up to find go.mod
	current := dir
	for {
		goModPath := filepath.Join(current, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			moduleRoot = current
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", "", errors.New("could not find go.mod in parent directories")
		}
		current = parent
	}

	// Read go.mod to get module name
	goModContent, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		return "", "", errors.Wrap(err, "failed to read go.mod")
	}

	// Parse module name from go.mod
	lines := strings.Split(string(goModContent), "\n")
	var moduleName string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			moduleName = strings.TrimPrefix(line, "module ")
			break
		}
	}
	if moduleName == "" {
		return "", "", errors.New("could not find module name in go.mod")
	}

	// Calculate import path relative to module root
	relPath, err := filepath.Rel(moduleRoot, dir)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to calculate relative path")
	}

	if relPath == "." {
		importPath = moduleName
	} else {
		importPath = moduleName + "/" + filepath.ToSlash(relPath)
	}

	return moduleRoot, importPath, nil
}

// LoadFromFile loads a Go definition file and returns the generated CUE.
// If the file contains no definitions (e.g., helper files with shared types),
// it returns an empty slice without error.
func LoadFromFile(filePath string) ([]LoadResult, error) {
	// Analyze the file to find definitions
	definitions, err := AnalyzeGoFile(filePath)
	if err != nil {
		return nil, err
	}

	// If no definitions found, return empty slice (not an error)
	// This handles helper files like schemas.go that import defkit but don't define components
	if len(definitions) == 0 {
		return []LoadResult{}, nil
	}

	var results []LoadResult
	for _, defInfo := range definitions {
		result := LoadResult{Definition: defInfo}

		cue, err := GenerateCUEFromGoFile(filePath, defInfo)
		if err != nil {
			result.Error = err
		} else {
			result.CUE = cue
		}

		results = append(results, result)
	}

	return results, nil
}

// LoadFromDirectory loads all Go definition files from a directory
func LoadFromDirectory(dir string) ([]LoadResult, error) {
	files, err := DiscoverDefinitions(dir)
	if err != nil {
		return nil, err
	}

	var allResults []LoadResult
	for _, file := range files {
		results, err := LoadFromFile(file)
		if err != nil {
			// Add error result for the file (only for actual errors, not "no definitions")
			allResults = append(allResults, LoadResult{
				Definition: DefinitionInfo{FilePath: file},
				Error:      err,
			})
			continue
		}
		// Only add results if there are actual definitions
		// (empty slice means the file is a helper file with no definitions)
		allResults = append(allResults, results...)
	}

	return allResults, nil
}

// LoadFromFileWithEnv loads a Go definition file using a pre-configured generator environment.
// This is more efficient than LoadFromFile when loading multiple files from the same module.
func LoadFromFileWithEnv(env *GeneratorEnvironment, filePath string) ([]LoadResult, error) {
	// Analyze the file to find definitions
	definitions, err := AnalyzeGoFile(filePath)
	if err != nil {
		return nil, err
	}

	// If no definitions found, return empty slice (not an error)
	if len(definitions) == 0 {
		return []LoadResult{}, nil
	}

	var results []LoadResult
	for _, defInfo := range definitions {
		result := LoadResult{Definition: defInfo}

		cue, err := env.GenerateCUE(filePath, defInfo)
		if err != nil {
			result.Error = err
		} else {
			result.CUE = cue
		}

		results = append(results, result)
	}

	return results, nil
}

// LoadFromDirectoryWithEnv loads all Go definition files from a directory using a pre-configured environment.
// This is much faster than LoadFromDirectory because it runs `go mod tidy` only once.
func LoadFromDirectoryWithEnv(env *GeneratorEnvironment, dir string) ([]LoadResult, error) {
	files, err := DiscoverDefinitions(dir)
	if err != nil {
		return nil, err
	}

	var allResults []LoadResult
	for _, file := range files {
		results, err := LoadFromFileWithEnv(env, file)
		if err != nil {
			allResults = append(allResults, LoadResult{
				Definition: DefinitionInfo{FilePath: file},
				Error:      err,
			})
			continue
		}
		allResults = append(allResults, results...)
	}

	return allResults, nil
}

// LoadFromDirectoryOptimized loads all Go definition files from a directory with optimized performance.
// It creates a generator environment once, runs `go mod tidy` once, then processes all definitions.
// This is the recommended function for loading multiple definitions from a module.
func LoadFromDirectoryOptimized(dir string) ([]LoadResult, error) {
	// Find module root
	moduleRoot, _, err := findModuleInfo(dir)
	if err != nil {
		// Fall back to non-optimized loading if we can't find module root
		return LoadFromDirectory(dir)
	}

	// Create generator environment (runs go mod tidy once)
	env, err := NewGeneratorEnvironment(moduleRoot)
	if err != nil {
		// Fall back to non-optimized loading on error
		return LoadFromDirectory(dir)
	}
	defer func() { _ = env.Close() }()

	return LoadFromDirectoryWithEnv(env, dir)
}

// registryMainPath is the conventional path for the registry main program.
// Definition modules should have this file to enable efficient registry-based loading.
const registryMainPath = "cmd/register/main.go"

// LoadFromModuleWithRegistry loads definitions from a Go module using the registry pattern.
// This is the simplest and most efficient approach - it requires:
// 1. Each definition file has init() that calls defkit.Register()
// 2. Module has cmd/register/main.go that imports all packages and outputs JSON
//
// How it works:
// 1. Run `go run ./cmd/register` in the module directory
// 2. The main.go imports all definition packages (triggering init() functions)
// 3. init() functions call defkit.Register() for each definition
// 4. main() calls defkit.ToJSON() to output all registered definitions
// 5. CLI parses the JSON output
//
// This approach:
// - No temp directory needed
// - No go mod tidy needed (module is already set up)
// - Runs `go run` exactly ONCE
// - No AST parsing required
// - Fastest possible loading
func LoadFromModuleWithRegistry(moduleRoot string) ([]LoadResult, error) {
	// Check if module has cmd/register/main.go
	mainPath := filepath.Join(moduleRoot, registryMainPath)
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		return nil, errors.Errorf("module does not have %s - run 'vela def init-module' to create it", registryMainPath)
	}

	// Run go run ./cmd/register directly in the module
	// GOWORK=off ensures consistent behavior when parent directories have go.work files
	runCmd := exec.Command("go", "run", "./cmd/register")
	runCmd.Dir = moduleRoot
	runCmd.Env = append(os.Environ(), "GO111MODULE=on", "GOWORK=off")
	var stdout, stderr bytes.Buffer
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr
	if err := runCmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "registry generator failed: %s", stderr.String())
	}

	// Parse JSON output
	var registryOutput defkit.RegistryOutput
	if err := json.Unmarshal(stdout.Bytes(), &registryOutput); err != nil {
		return nil, errors.Wrapf(err, "failed to parse registry output: %s", stdout.String())
	}

	// Convert to LoadResults
	results := make([]LoadResult, 0, len(registryOutput.Definitions))
	for _, def := range registryOutput.Definitions {
		result := LoadResult{
			CUE: def.CUE,
			Definition: DefinitionInfo{
				Name: def.Name,
				Type: string(def.Type),
			},
		}

		// Convert placement if present
		if def.Placement != nil {
			result.Definition.Placement = &DefinitionPlacement{}
			for _, cond := range def.Placement.RunOn {
				result.Definition.Placement.RunOn = append(result.Definition.Placement.RunOn, PlacementCondition{
					Key:      cond.Key,
					Operator: cond.Operator,
					Values:   cond.Values,
				})
			}
			for _, cond := range def.Placement.NotRunOn {
				result.Definition.Placement.NotRunOn = append(result.Definition.Placement.NotRunOn, PlacementCondition{
					Key:      cond.Key,
					Operator: cond.Operator,
					Values:   cond.Values,
				})
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// SupportsRegistry checks if a module uses the registry pattern.
// It checks for the presence of cmd/register/main.go which is the conventional
// entry point for registry-based definition loading.
func SupportsRegistry(moduleRoot string) bool {
	mainPath := filepath.Join(moduleRoot, registryMainPath)
	_, err := os.Stat(mainPath)
	return err == nil
}

// LoadFromModuleAuto automatically selects the best loading strategy:
// - If the module uses defkit.Register(), use the registry approach
// - Otherwise, fall back to the AST-based approach
func LoadFromModuleAuto(moduleRoot string) ([]LoadResult, error) {
	if SupportsRegistry(moduleRoot) {
		return LoadFromModuleWithRegistry(moduleRoot)
	}
	return LoadFromDirectoryOptimized(moduleRoot)
}
