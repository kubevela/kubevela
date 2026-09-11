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

package addon

import (
	"encoding/json"
	"fmt"
	"slices"

	"cuelang.org/go/cue/cuecontext"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// ModuleImport is one modules/_imports.cue entry: an external module the
// addon references by name, resolved from a module registry when the addon's
// rendered type: module component reconciles.
type ModuleImport struct {
	// Module is the module name; also the emitted component's name.
	Module string
	// Enabled gates whether this import is installed. Absent in the source
	// file defaults to true.
	Enabled bool
	// Registry is the named module registry to resolve Module from; empty
	// means search every configured registry.
	Registry string
	// Version is the exact module package version to install; empty means
	// latest. addon:build/addon:publish already reject a semver range here
	// (RFC-109b), so this value is trusted as an exact pin by the time it
	// reaches render time.
	Version string
}

// rawModuleImports is the modules/_imports.cue file shape.
type rawModuleImports struct {
	Imports []rawModuleImport `json:"imports"`
}

// rawModuleImport is one raw imports[] entry before validation/flattening.
type rawModuleImport struct {
	Module  string            `json:"module"`
	Enabled *bool             `json:"enabled"`
	Sources []rawModuleSource `json:"sources"`
}

// rawModuleSource is one raw sources[] entry. Only registry/version/versions
// are read; oci/git/apiLine sources are not supported in this story.
type rawModuleSource struct {
	Registry string   `json:"registry"`
	Version  string   `json:"version"`
	Versions []string `json:"versions"`
}

// parseModuleImports parses a modules/_imports.cue file's contents into its
// ModuleImport entries. Every entry must declare exactly one sources[] entry
// _imports.cue is evaluated as literal CUE data only -- no
// context.*/parameter.* expressions.
func parseModuleImports(data string) ([]ModuleImport, error) {
	val := cuecontext.New().CompileString(data)
	if err := val.Err(); err != nil {
		return nil, fmt.Errorf("compile modules/_imports.cue: %w", err)
	}
	var raw rawModuleImports
	if err := val.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode modules/_imports.cue: %w", err)
	}

	imports := make([]ModuleImport, 0, len(raw.Imports))
	for _, imp := range raw.Imports {
		if imp.Module == "" {
			return nil, fmt.Errorf("modules/_imports.cue: import missing required field \"module\"")
		}
		if len(imp.Sources) != 1 {
			return nil, fmt.Errorf(
				"modules/_imports.cue: module %q must declare exactly one sources[] entry (found %d); multi-source imports are not supported yet",
				imp.Module, len(imp.Sources))
		}
		enabled := true
		if imp.Enabled != nil {
			enabled = *imp.Enabled
		}
		src := imp.Sources[0]
		if len(src.Versions) > 0 {
			klog.Warningf(
				"modules/_imports.cue: module %q source declares versions %v, but the API-line filter is not enforced yet; every enabled line in the fetched module will install",
				imp.Module, src.Versions)
		}
		imports = append(imports, ModuleImport{
			Module:   imp.Module,
			Enabled:  enabled,
			Registry: src.Registry,
			Version:  src.Version,
		})
	}
	return imports, nil
}

// readModuleImportsFile reads modules/_imports.cue (when present) and parses
// it into InstallPackage.Imports.
func readModuleImportsFile(a *InstallPackage, reader AsyncReader, readPath string) error {
	data, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	imports, err := parseModuleImports(data)
	if err != nil {
		return err
	}
	a.Imports = imports
	return nil
}

// existingModuleNames returns the set of module names already declared as
// type: module components -- e.g. hand-authored directly in the addon's
// template.cue -- keyed by the module each targets: properties.module when
// set, else the component's own name, mirroring the type: module component
// template's own default (module: *context.name | string).
func existingModuleNames(components []common2.ApplicationComponent) map[string]bool {
	names := make(map[string]bool, len(components))
	for _, c := range components {
		if c.Type != "module" {
			continue
		}
		name := c.Name
		if c.Properties != nil {
			var props struct {
				Module string `json:"module"`
			}
			if err := json.Unmarshal(c.Properties.Raw, &props); err == nil && props.Module != "" {
				name = props.Module
			}
		}
		names[name] = true
	}
	return names
}

// RenderModuleComponents builds one type: module ApplicationComponent per
// enabled modules/_imports.cue entry that is not already declared as a
// type: module component in existingComponents -- an addon author's own
// template.cue component always wins over an auto-generated one, silently.
// Each emitted component depends on every name in resourceComponentNames, so
// the module's XRD/Compositions never apply before the addon's own operators
// and CRDs are healthy.
func RenderModuleComponents(addon *InstallPackage, existingComponents []common2.ApplicationComponent, resourceComponentNames []string) ([]common2.ApplicationComponent, error) {
	already := existingModuleNames(existingComponents)

	var comps []common2.ApplicationComponent
	for _, imp := range addon.Imports {
		if !imp.Enabled || already[imp.Module] {
			continue
		}
		properties := map[string]interface{}{"module": imp.Module}
		if imp.Registry != "" {
			properties["registry"] = imp.Registry
		}
		if imp.Version != "" {
			properties["version"] = imp.Version
		}
		raw, err := json.Marshal(properties)
		if err != nil {
			return nil, fmt.Errorf("render module component %q for addon %q: %w", imp.Module, addon.Name, err)
		}
		comps = append(comps, common2.ApplicationComponent{
			Name:       imp.Module,
			Type:       "module",
			Properties: &runtime.RawExtension{Raw: raw},
			DependsOn:  slices.Clone(resourceComponentNames),
		})
		already[imp.Module] = true
	}
	return comps, nil
}
