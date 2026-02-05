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

package goloader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
)

// ModuleMetadata contains metadata about a definition module.
// This is read from module.yaml in the module root.
type ModuleMetadata struct {
	// APIVersion is the metadata API version
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	// Kind should be "DefinitionModule"
	Kind string `yaml:"kind" json:"kind"`
	// Metadata contains name and version
	Metadata ModuleObjectMeta `yaml:"metadata" json:"metadata"`
	// Spec contains module configuration
	Spec ModuleSpec `yaml:"spec" json:"spec"`
}

// ModuleObjectMeta contains module identification
type ModuleObjectMeta struct {
	// Name is the module name
	Name string `yaml:"name" json:"name"`
}

// ModuleSpec contains module specification
type ModuleSpec struct {
	// Description is a human-readable description
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Maintainers lists the module maintainers
	Maintainers []Maintainer `yaml:"maintainers,omitempty" json:"maintainers,omitempty"`
	// MinVelaVersion is the minimum KubeVela version required
	MinVelaVersion string `yaml:"minVelaVersion,omitempty" json:"minVelaVersion,omitempty"`
	// MinDefkitVersion is the minimum defkit SDK version
	MinDefkitVersion string `yaml:"minDefkitVersion,omitempty" json:"minDefkitVersion,omitempty"`
	// Categories for organization
	Categories []string `yaml:"categories,omitempty" json:"categories,omitempty"`
	// Dependencies lists other definition modules this depends on
	Dependencies []ModuleDependency `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	// Exclude patterns for files to skip
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	// NameOverrides maps definition paths to custom names
	NameOverrides map[string]string `yaml:"nameOverrides,omitempty" json:"nameOverrides,omitempty"`
	// Placement defines default placement constraints for all definitions in this module.
	// Individual definitions can override these constraints.
	Placement *ModulePlacement `yaml:"placement,omitempty" json:"placement,omitempty"`
	// Hooks defines lifecycle hooks for the module
	Hooks *ModuleHooks `yaml:"hooks,omitempty" json:"hooks,omitempty"`
}

// ModuleHooks defines lifecycle hooks for module application
type ModuleHooks struct {
	// PreApply hooks run before definitions are applied
	PreApply []Hook `yaml:"pre-apply,omitempty" json:"pre-apply,omitempty"`
	// PostApply hooks run after definitions are applied
	PostApply []Hook `yaml:"post-apply,omitempty" json:"post-apply,omitempty"`
}

// Hook represents a single hook action
type Hook struct {
	// Path is a directory containing YAML manifests to apply
	// Files are processed in alphabetical order (use numeric prefixes for ordering)
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
	// Script is a path to a shell script to execute
	Script string `yaml:"script,omitempty" json:"script,omitempty"`
	// Wait indicates whether to wait for resources to be ready (for Path hooks)
	Wait bool `yaml:"wait,omitempty" json:"wait,omitempty"`
	// WaitFor specifies the readiness condition to wait for.
	// Can be a simple condition name (e.g., "Ready", "Established") or
	// a CUE expression (e.g., 'status.phase == "Running"').
	// Only used when Wait is true.
	WaitFor string `yaml:"waitFor,omitempty" json:"waitFor,omitempty"`
	// Optional indicates whether hook failure should stop the process
	Optional bool `yaml:"optional,omitempty" json:"optional,omitempty"`
	// Timeout for the hook execution (default: 5m for wait, 30s for scripts)
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// IsEmpty returns true if no hooks are defined
func (h *ModuleHooks) IsEmpty() bool {
	return h == nil || (len(h.PreApply) == 0 && len(h.PostApply) == 0)
}

// HasPreApply returns true if pre-apply hooks are defined
func (h *ModuleHooks) HasPreApply() bool {
	return h != nil && len(h.PreApply) > 0
}

// HasPostApply returns true if post-apply hooks are defined
func (h *ModuleHooks) HasPostApply() bool {
	return h != nil && len(h.PostApply) > 0
}

// Validate checks that the hook configuration is valid
func (hook *Hook) Validate() error {
	if hook.Path == "" && hook.Script == "" {
		return errors.New("hook must specify either 'path' or 'script'")
	}
	if hook.Path != "" && hook.Script != "" {
		return errors.New("hook cannot specify both 'path' and 'script'")
	}
	if hook.Wait && hook.Script != "" {
		return errors.New("'wait' is only valid for 'path' hooks, not 'script' hooks")
	}
	if hook.WaitFor != "" && !hook.Wait {
		return errors.New("'waitFor' requires 'wait: true'")
	}
	if hook.WaitFor != "" && hook.Script != "" {
		return errors.New("'waitFor' is only valid for 'path' hooks, not 'script' hooks")
	}
	return nil
}

// ModulePlacement defines placement constraints at the module level.
// This is a YAML-parseable representation that gets converted to placement.PlacementSpec.
type ModulePlacement struct {
	// RunOn specifies conditions that must be satisfied for definitions to be applied.
	RunOn []ModulePlacementCondition `yaml:"runOn,omitempty" json:"runOn,omitempty"`
	// NotRunOn specifies conditions that exclude clusters from running definitions.
	NotRunOn []ModulePlacementCondition `yaml:"notRunOn,omitempty" json:"notRunOn,omitempty"`
}

// ModulePlacementCondition represents a single placement condition in YAML format.
type ModulePlacementCondition struct {
	// Key is the cluster label key to match against.
	Key string `yaml:"key" json:"key"`
	// Operator is the comparison operator (Eq, Ne, In, NotIn, Exists, NotExists).
	Operator string `yaml:"operator" json:"operator"`
	// Values are the values to compare against.
	Values []string `yaml:"values,omitempty" json:"values,omitempty"`
}

// IsEmpty returns true if no placement constraints are defined.
func (p *ModulePlacement) IsEmpty() bool {
	return p == nil || (len(p.RunOn) == 0 && len(p.NotRunOn) == 0)
}

// ToPlacementSpec converts the YAML-parsed ModulePlacement to a placement.PlacementSpec
// that can be used for evaluation.
func (p *ModulePlacement) ToPlacementSpec() placement.PlacementSpec {
	if p == nil {
		return placement.PlacementSpec{}
	}

	spec := placement.PlacementSpec{}

	for _, cond := range p.RunOn {
		spec.RunOn = append(spec.RunOn, cond.ToCondition())
	}

	for _, cond := range p.NotRunOn {
		spec.NotRunOn = append(spec.NotRunOn, cond.ToCondition())
	}

	return spec
}

// ToCondition converts a ModulePlacementCondition to a placement.Condition.
func (c ModulePlacementCondition) ToCondition() placement.Condition {
	return &placement.LabelCondition{
		Key:      c.Key,
		Operator: placement.Operator(c.Operator),
		Values:   c.Values,
	}
}

// Maintainer represents a module maintainer
type Maintainer struct {
	Name  string `yaml:"name" json:"name"`
	Email string `yaml:"email,omitempty" json:"email,omitempty"`
}

// ModuleDependency represents a dependency on another module
type ModuleDependency struct {
	// Module is the Go module path
	Module string `yaml:"module" json:"module"`
	// Version is the required version (supports semver constraints)
	Version string `yaml:"version" json:"version"`
}

// LoadedModule represents a loaded definition module
type LoadedModule struct {
	// Metadata is the module metadata
	Metadata ModuleMetadata
	// Path is the local filesystem path to the module
	Path string
	// ModulePath is the Go module path (e.g., github.com/myorg/defs)
	ModulePath string
	// Version is the resolved version
	Version string
	// Definitions contains all discovered definitions
	Definitions []LoadResult
	// Dependencies are the resolved dependencies
	Dependencies []*LoadedModule
}

// ModuleLoadOptions configures module loading
type ModuleLoadOptions struct {
	// Version specifies the version to load (for remote modules)
	Version string
	// Types filters which definition types to load
	Types []string
	// NamePrefix adds a prefix to all definition names
	NamePrefix string
	// IncludeTests includes test files
	IncludeTests bool
	// ResolveDependencies resolves and loads dependencies
	ResolveDependencies bool
}

// DefaultModuleLoadOptions returns default options
func DefaultModuleLoadOptions() ModuleLoadOptions {
	return ModuleLoadOptions{
		Types:               []string{"component", "trait", "policy", "workflow-step"},
		ResolveDependencies: true,
	}
}

// LoadModule loads a definition module from a path or Go module reference.
// Supports:
//   - Local paths: ./my-defs, /absolute/path/to/defs
//   - Go modules: github.com/myorg/defs@v1.0.0
func LoadModule(ctx context.Context, moduleRef string, opts ModuleLoadOptions) (*LoadedModule, error) {
	// Determine if this is a local path or Go module
	if isLocalPath(moduleRef) {
		return loadModuleFromPath(ctx, moduleRef, opts)
	}
	return loadModuleFromGoMod(ctx, moduleRef, opts)
}

// isLocalPath checks if a reference is a local filesystem path
func isLocalPath(ref string) bool {
	// Check for absolute path, relative path indicators, or path separators
	if strings.HasPrefix(ref, "/") ||
		strings.HasPrefix(ref, "./") ||
		strings.HasPrefix(ref, "../") ||
		strings.HasPrefix(ref, ".\\") ||
		strings.HasPrefix(ref, "..\\") {
		return true
	}
	// Check if it exists as a local path
	_, err := os.Stat(ref)
	return err == nil
}

// loadModuleFromPath loads a module from a local filesystem path
func loadModuleFromPath(ctx context.Context, modulePath string, opts ModuleLoadOptions) (*LoadedModule, error) {
	absPath, err := filepath.Abs(modulePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to resolve absolute path for %s", modulePath)
	}

	// Check if path exists
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, errors.Wrapf(err, "module path does not exist: %s", absPath)
	}
	if !info.IsDir() {
		return nil, errors.Errorf("module path must be a directory: %s", absPath)
	}

	module := &LoadedModule{
		Path: absPath,
	}

	// Load module metadata if exists
	metadata, err := loadModuleMetadata(absPath)
	if err == nil {
		module.Metadata = *metadata
	} else if !os.IsNotExist(errors.Cause(err)) {
		// Only fail if error is not "file not found"
		return nil, errors.Wrap(err, "failed to load module metadata")
	}

	// Determine Go module path from go.mod
	goModPath, _, err := findModuleInfo(absPath)
	if err == nil {
		module.ModulePath = goModPath
	}

	// Derive version from git
	module.Version = deriveVersionFromGit(absPath)

	// Discover and load definitions
	definitions, err := discoverAndLoadDefinitions(ctx, absPath, opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load definitions from module")
	}
	module.Definitions = definitions

	// Apply name prefix if specified
	if opts.NamePrefix != "" {
		for i := range module.Definitions {
			if module.Definitions[i].Definition.Name != "" {
				module.Definitions[i].Definition.Name = opts.NamePrefix + module.Definitions[i].Definition.Name
			}
		}
	}

	return module, nil
}

// deriveVersionFromGit attempts to derive a version from git.
// Resolution order:
//  1. Git tag (e.g., "v1.2.0")
//  2. Git commit hash (e.g., "v0.0.0-dev+abc1234")
//  3. Fallback to "v0.0.0-local" if not a git repo
func deriveVersionFromGit(modulePath string) string {
	// Try to get version from git describe (tags)
	cmd := exec.Command("git", "describe", "--tags", "--exact-match", "HEAD")
	cmd.Dir = modulePath
	if output, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(output))
	}

	// Try to get the latest tag with distance
	cmd = exec.Command("git", "describe", "--tags", "--always")
	cmd.Dir = modulePath
	if output, err := cmd.Output(); err == nil {
		desc := strings.TrimSpace(string(output))
		// If it's just a commit hash (no tags), format as dev version
		if !strings.Contains(desc, "-") && len(desc) <= 12 {
			return fmt.Sprintf("v0.0.0-dev+%s", desc)
		}
		// Has tag with distance, e.g., "v1.0.0-3-gabc1234"
		return desc
	}

	// Try to get just the commit hash
	cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = modulePath
	if output, err := cmd.Output(); err == nil {
		return fmt.Sprintf("v0.0.0-dev+%s", strings.TrimSpace(string(output)))
	}

	// Not a git repo or git not available
	return "v0.0.0-local"
}

// loadModuleFromGoMod loads a module from a Go module reference
func loadModuleFromGoMod(ctx context.Context, moduleRef string, opts ModuleLoadOptions) (*LoadedModule, error) {
	// Parse module reference: github.com/myorg/defs@v1.0.0
	modulePath, version := parseModuleRef(moduleRef)
	if opts.Version != "" {
		version = opts.Version
	}

	// Download the module using go mod download
	localPath, err := downloadGoModule(ctx, modulePath, version)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download module %s", moduleRef)
	}

	// Load from the downloaded path
	module, err := loadModuleFromPath(ctx, localPath, opts)
	if err != nil {
		return nil, err
	}

	module.ModulePath = modulePath
	module.Version = version

	return module, nil
}

// parseModuleRef parses a module reference into path and version
func parseModuleRef(ref string) (modulePath, version string) {
	if idx := strings.LastIndex(ref, "@"); idx != -1 {
		return ref[:idx], ref[idx+1:]
	}
	return ref, "latest"
}

// downloadGoModule downloads a Go module and returns its local path
func downloadGoModule(ctx context.Context, modulePath, version string) (string, error) {
	// Use go mod download to get the module
	// Always include version specifier - without it, go mod download may skip
	// the download if it thinks the module is the main module
	if version == "" {
		version = "latest"
	}
	moduleSpec := modulePath + "@" + version

	cmd := exec.CommandContext(ctx, "go", "mod", "download", "-json", moduleSpec)
	cmd.Env = append(os.Environ(), "GO111MODULE=on")

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", errors.Errorf("go mod download failed: %s", string(exitErr.Stderr))
		}
		return "", errors.Wrap(err, "go mod download failed")
	}

	// Parse the JSON output to get the Dir field
	// Output format: {"Path":"...","Version":"...","Dir":"..."}
	var result struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`
		Dir     string `json:"Dir"`
		Error   string `json:"Error"`
	}

	// Check for empty output (can happen if go thinks module is main module)
	if len(output) == 0 {
		return "", errors.Errorf("go mod download returned empty output for %s; ensure the module exists and is accessible", moduleSpec)
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return "", errors.Wrapf(err, "failed to parse go mod download output: %s", string(output))
	}

	// Check for error in JSON response
	if result.Error != "" {
		return "", errors.Errorf("go mod download failed for %s: %s", moduleSpec, result.Error)
	}

	if result.Dir == "" {
		return "", errors.New("go mod download did not return a directory")
	}

	return result.Dir, nil
}

// loadModuleMetadata loads module.yaml from a module directory
func loadModuleMetadata(modulePath string) (*ModuleMetadata, error) {
	metadataPath := filepath.Join(modulePath, "module.yaml")
	content, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read module.yaml")
	}

	var metadata ModuleMetadata
	if err := yaml.Unmarshal(content, &metadata); err != nil {
		return nil, errors.Wrap(err, "failed to parse module.yaml")
	}

	return &metadata, nil
}

// discoverAndLoadDefinitions discovers and loads all definitions from a module path.
// This is optimized to:
// 1. Try registry-based loading first (most efficient - single program execution)
// 2. Fall back to AST-based parallel loading if registry not available
// 3. Run `go mod tidy` only ONCE for the entire module
// 4. Process definitions in PARALLEL using goroutines
func discoverAndLoadDefinitions(ctx context.Context, modulePath string, opts ModuleLoadOptions) ([]LoadResult, error) {
	// Try registry-based loading first (most efficient approach)
	// This works when the module uses defkit.Register() in init() functions
	if SupportsRegistry(modulePath) {
		results, err := LoadFromModuleWithRegistry(modulePath)
		if err == nil && len(results) > 0 {
			// Filter results by type if specified
			if len(opts.Types) > 0 {
				var filtered []LoadResult
				for _, r := range results {
					for _, t := range opts.Types {
						if r.Definition.Type == t {
							filtered = append(filtered, r)
							break
						}
					}
				}
				return filtered, nil
			}
			return results, nil
		}
		// If registry approach failed, fall through to AST-based loading
	}

	// Create generator environment ONCE for the entire module
	// This runs `go mod tidy` only once, then reuses the environment for all definitions
	env, err := NewGeneratorEnvironment(modulePath)
	if err != nil {
		// Fall back to non-optimized loading if environment creation fails
		return discoverAndLoadDefinitionsFallback(ctx, modulePath, opts)
	}
	defer func() { _ = env.Close() }()

	// Define conventional directories to scan
	conventionalDirs := []struct {
		dir      string
		defTypes []string
	}{
		{"components", []string{"component"}},
		{"traits", []string{"trait"}},
		{"policies", []string{"policy"}},
		{"workflows", []string{"workflow-step"}},
		{".", nil}, // Also scan root for mixed definitions
	}

	// First pass: discover all definitions to load
	var allDefinitions []DefinitionInfo
	seenFiles := make(map[string]bool) // Track files to avoid duplicates from overlapping scans

	for _, conv := range conventionalDirs {
		dirPath := filepath.Join(modulePath, conv.dir)

		// Skip if directory doesn't exist
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			continue
		}

		// Check if this type is in the filter
		if len(opts.Types) > 0 && len(conv.defTypes) > 0 {
			found := false
			for _, t := range conv.defTypes {
				for _, allowed := range opts.Types {
					if t == allowed {
						found = true
						break
					}
				}
			}
			if !found {
				continue
			}
		}

		// Discover definitions in this directory
		files, err := DiscoverDefinitions(dirPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to discover definitions in %s", dirPath)
		}

		for _, file := range files {
			// Skip files we've already processed (from overlapping directory scans)
			if seenFiles[file] {
				continue
			}
			seenFiles[file] = true

			// Skip test files unless requested
			if !opts.IncludeTests && strings.HasSuffix(file, "_test.go") {
				continue
			}

			// Analyze the file to find definitions
			defs, err := AnalyzeGoFile(file)
			if err != nil {
				continue // Skip files that can't be analyzed
			}

			// Filter by type if specified
			for _, def := range defs {
				if len(opts.Types) > 0 {
					found := false
					for _, t := range opts.Types {
						if def.Type == t {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				}
				allDefinitions = append(allDefinitions, def)
			}
		}
	}

	// If no definitions found, return empty slice
	if len(allDefinitions) == 0 {
		return []LoadResult{}, nil
	}

	// Second pass: load all definitions in PARALLEL using the shared environment
	results := env.GenerateCUEParallel(allDefinitions)

	return results, nil
}

// discoverAndLoadDefinitionsFallback is the non-optimized fallback that runs
// `go mod tidy` for each definition. Used when the optimized path fails.
func discoverAndLoadDefinitionsFallback(ctx context.Context, modulePath string, opts ModuleLoadOptions) ([]LoadResult, error) {
	var allResults []LoadResult
	seenFiles := make(map[string]bool) // Track files to avoid duplicates from overlapping scans

	conventionalDirs := []struct {
		dir      string
		defTypes []string
	}{
		{"components", []string{"component"}},
		{"traits", []string{"trait"}},
		{"policies", []string{"policy"}},
		{"workflows", []string{"workflow-step"}},
		{".", nil},
	}

	for _, conv := range conventionalDirs {
		dirPath := filepath.Join(modulePath, conv.dir)

		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			continue
		}

		if len(opts.Types) > 0 && len(conv.defTypes) > 0 {
			found := false
			for _, t := range conv.defTypes {
				for _, allowed := range opts.Types {
					if t == allowed {
						found = true
						break
					}
				}
			}
			if !found {
				continue
			}
		}

		files, err := DiscoverDefinitions(dirPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to discover definitions in %s", dirPath)
		}

		for _, file := range files {
			// Skip files we've already processed (from overlapping directory scans)
			if seenFiles[file] {
				continue
			}
			seenFiles[file] = true

			if !opts.IncludeTests && strings.HasSuffix(file, "_test.go") {
				continue
			}

			results, err := LoadFromFile(file)
			if err != nil {
				allResults = append(allResults, LoadResult{
					Definition: DefinitionInfo{FilePath: file},
					Error:      err,
				})
				continue
			}

			for _, result := range results {
				if len(opts.Types) > 0 {
					found := false
					for _, t := range opts.Types {
						if result.Definition.Type == t {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				}
				allResults = append(allResults, result)
			}
		}
	}

	return allResults, nil
}

// ListModuleDefinitions returns information about definitions in a module without loading them
func ListModuleDefinitions(ctx context.Context, moduleRef string, opts ModuleLoadOptions) ([]DefinitionInfo, error) {
	// Determine path
	var modulePath string
	if isLocalPath(moduleRef) {
		absPath, err := filepath.Abs(moduleRef)
		if err != nil {
			return nil, err
		}
		modulePath = absPath
	} else {
		path, version := parseModuleRef(moduleRef)
		if opts.Version != "" {
			version = opts.Version
		}
		var err error
		modulePath, err = downloadGoModule(ctx, path, version)
		if err != nil {
			return nil, err
		}
	}

	// Find all definition files
	var allDefs []DefinitionInfo

	conventionalDirs := []string{"components", "traits", "policies", "workflows", "."}
	for _, dir := range conventionalDirs {
		dirPath := filepath.Join(modulePath, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			continue
		}

		files, err := DiscoverDefinitions(dirPath)
		if err != nil {
			continue
		}

		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}

			defs, err := AnalyzeGoFile(file)
			if err != nil {
				continue
			}
			allDefs = append(allDefs, defs...)
		}
	}

	return allDefs, nil
}

// ValidateModule validates a module for correctness and compatibility
func ValidateModule(module *LoadedModule, velaVersion string) []error {
	var errs []error

	// Check for definition errors
	for _, def := range module.Definitions {
		if def.Error != nil {
			errs = append(errs, errors.Wrapf(def.Error, "definition %s failed to load", def.Definition.FilePath))
		}
	}

	// Validate placement operators
	if module.Metadata.Spec.Placement != nil {
		errs = append(errs, validatePlacementConditions("runOn", module.Metadata.Spec.Placement.RunOn)...)
		errs = append(errs, validatePlacementConditions("notRunOn", module.Metadata.Spec.Placement.NotRunOn)...)
	}

	// Check minimum Vela version using semver comparison
	if module.Metadata.Spec.MinVelaVersion != "" && velaVersion != "" {
		minVersion, minErr := semver.NewVersion(module.Metadata.Spec.MinVelaVersion)
		currentVersion, curErr := semver.NewVersion(velaVersion)
		if minErr == nil && curErr == nil {
			if minVersion.GreaterThan(currentVersion) {
				errs = append(errs, errors.Errorf(
					"module requires KubeVela %s or later, but cluster has %s",
					module.Metadata.Spec.MinVelaVersion,
					velaVersion,
				))
			}
		}
	}

	// Validate hooks
	if module.Metadata.Spec.Hooks != nil {
		errs = append(errs, validateHooks("pre-apply", module.Metadata.Spec.Hooks.PreApply, module.Path)...)
		errs = append(errs, validateHooks("post-apply", module.Metadata.Spec.Hooks.PostApply, module.Path)...)
	}

	return errs
}

// validateHooks validates a list of hooks
func validateHooks(phase string, hooks []Hook, modulePath string) []error {
	var errs []error
	for i, hook := range hooks {
		if err := hook.Validate(); err != nil {
			errs = append(errs, errors.Wrapf(err, "hooks.%s[%d]", phase, i))
			continue
		}

		// Check that paths exist
		if hook.Path != "" {
			fullPath := filepath.Join(modulePath, hook.Path)
			if info, err := os.Stat(fullPath); err != nil {
				errs = append(errs, errors.Wrapf(err, "hooks.%s[%d].path %q does not exist", phase, i, hook.Path))
			} else if !info.IsDir() {
				errs = append(errs, errors.Errorf("hooks.%s[%d].path %q must be a directory", phase, i, hook.Path))
			}
		}
		if hook.Script != "" {
			fullPath := filepath.Join(modulePath, hook.Script)
			if info, err := os.Stat(fullPath); err != nil {
				errs = append(errs, errors.Wrapf(err, "hooks.%s[%d].script %q does not exist", phase, i, hook.Script))
			} else if info.IsDir() {
				errs = append(errs, errors.Errorf("hooks.%s[%d].script %q must be a file, not a directory", phase, i, hook.Script))
			}
		}
	}
	return errs
}

// validatePlacementConditions validates that all conditions have valid operators
func validatePlacementConditions(field string, conditions []ModulePlacementCondition) []error {
	var errs []error
	for i, cond := range conditions {
		op := placement.Operator(cond.Operator)
		if !op.IsValid() {
			errs = append(errs, errors.Errorf(
				"invalid operator %q in placement.%s[%d] (key=%q); valid operators: Eq, Ne, In, NotIn, Exists, NotExists",
				cond.Operator, field, i, cond.Key,
			))
		}
	}
	return errs
}

// GetDefinitionsByType returns definitions filtered by type
func (m *LoadedModule) GetDefinitionsByType(defType string) []LoadResult {
	var results []LoadResult
	for _, def := range m.Definitions {
		if def.Definition.Type == defType {
			results = append(results, def)
		}
	}
	return results
}

// GetComponents returns all component definitions
func (m *LoadedModule) GetComponents() []LoadResult {
	return m.GetDefinitionsByType("component")
}

// GetTraits returns all trait definitions
func (m *LoadedModule) GetTraits() []LoadResult {
	return m.GetDefinitionsByType("trait")
}

// GetPolicies returns all policy definitions
func (m *LoadedModule) GetPolicies() []LoadResult {
	return m.GetDefinitionsByType("policy")
}

// GetWorkflowSteps returns all workflow step definitions
func (m *LoadedModule) GetWorkflowSteps() []LoadResult {
	return m.GetDefinitionsByType("workflow-step")
}

// Summary returns a summary of the module contents
func (m *LoadedModule) Summary() string {
	components := len(m.GetComponents())
	traits := len(m.GetTraits())
	policies := len(m.GetPolicies())
	workflows := len(m.GetWorkflowSteps())

	var errors int
	for _, d := range m.Definitions {
		if d.Error != nil {
			errors++
		}
	}

	summary := fmt.Sprintf("Module: %s\n", m.ModulePath)
	if m.Version != "" {
		summary += fmt.Sprintf("Version: %s\n", m.Version)
	}
	if m.Metadata.Spec.Description != "" {
		summary += fmt.Sprintf("Description: %s\n", m.Metadata.Spec.Description)
	}
	summary += "\nDefinitions:\n"
	summary += fmt.Sprintf("  Components:     %d\n", components)
	summary += fmt.Sprintf("  Traits:         %d\n", traits)
	summary += fmt.Sprintf("  Policies:       %d\n", policies)
	summary += fmt.Sprintf("  Workflow Steps: %d\n", workflows)
	if errors > 0 {
		summary += fmt.Sprintf("  Errors:         %d\n", errors)
	}

	return summary
}
