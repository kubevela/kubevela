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
	"path/filepath"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/module"
	modulerender "github.com/oam-dev/kubevela/pkg/module/service"
)

// readInlineModulesDir groups every modules/<name>/... item (everything
// under modules/ except modules/_imports.cue, which its own exact-path
// pattern already claims) by module name, builds an in-memory module.MapFS
// per group (the same fs.FS type the external OCI fetch path builds), and
// parses each group as an inline module -- read from the addon's own bundled
// files via reader, no registry, no fetch. Every immediate subdirectory of
// modules/ is expected to be an inline module; one with no _module.cue at
// its root is a packaging mistake, not a legitimately-unrelated folder,
// so it fails --the same way the external path's ParseModule/
// ParseModuleDir already fails loudly on a tree missing _module.cue.
//
// rootPath is the addon's name: every real reader's RelativePath is rooted at
// it ("<addon>/modules/<name>/..."), the same rootPath GetPatternFromItem
// matches against. Pass "" for a reader whose paths are already addon-relative.
func readInlineModulesDir(a *InstallPackage, reader AsyncReader, items []Item, rootPath string) error {
	groups := map[string]modulerender.MapFS{}
	var order []string

	for _, it := range items {
		readPath := reader.RelativePath(it)
		addonRel := filepath.ToSlash(readPath)
		if rootPath != "" {
			addonRel = strings.TrimPrefix(addonRel, rootPath+"/")
		}
		rel := strings.TrimPrefix(addonRel, ModulesDirName+"/")
		if rel == addonRel {
			continue // not under modules/ at all; should not happen given the pattern match
		}
		sep := strings.IndexByte(rel, '/')
		if sep < 0 {
			// A file directly under modules/ with no subdirectory. The only
			// such file (_imports.cue) is claimed by its own pattern before
			// this one ever sees it, so anything reaching here is unexpected;
			// ignore rather than guess at its meaning.
			continue
		}
		name, inner := rel[:sep], rel[sep+1:]
		if inner == "" {
			continue // a bare directory marker, no file beneath it
		}

		fsys, ok := groups[name]
		if !ok {
			fsys = modulerender.MapFS{}
			groups[name] = fsys
			order = append(order, name)
		}
		data, err := reader.ReadFile(readPath)
		if err != nil {
			return fmt.Errorf("read inline module %q file %s: %w", name, readPath, err)
		}
		fsys[inner] = []byte(data)
	}

	for _, name := range order {
		fsys := groups[name]
		if _, ok := fsys["_module.cue"]; !ok {
			return fmt.Errorf("modules/%s: missing _module.cue; every directory directly under "+
				"modules/ must be an inline module (external references belong in "+
				"modules/_imports.cue instead)", name)
		}
		mod, err := module.ParseModule(fsys)
		if err != nil {
			return fmt.Errorf("parse inline module %q: %w", name, err)
		}
		a.InlineModules = append(a.InlineModules, mod)
	}
	return nil
}

// moduleDeclaredBy returns, for every module name declared anywhere in the
// addon -- a hand-written type: module component, an inline
// modules/<name>/, or a modules/_imports.cue entry -- every place that
// declares it. A disabled import declares nothing: it will not render, so it
// cannot collide with anything either.
func moduleDeclaredBy(existingComponents []common2.ApplicationComponent, imports []ModuleImport, inline []*module.Module) map[string][]string {
	declaredBy := map[string][]string{}
	for _, c := range existingComponents {
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
		declaredBy[name] = append(declaredBy[name], fmt.Sprintf("hand-written component %q", c.Name))
	}
	for _, imp := range imports {
		if !imp.Enabled {
			continue
		}
		declaredBy[imp.Module] = append(declaredBy[imp.Module], "modules/_imports.cue")
	}
	for _, mod := range inline {
		declaredBy[mod.Name] = append(declaredBy[mod.Name], fmt.Sprintf("inline module \"modules/%s\"", mod.Name))
	}
	return declaredBy
}

// checkModuleNameCollisions fails on the first module name declared in more
// than one place. No source silently wins over another: a name claimed
// twice is a packaging mistake, not an intentional override.
func checkModuleNameCollisions(declaredBy map[string][]string) error {
	names := make([]string, 0, len(declaredBy))
	for name := range declaredBy {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sources := declaredBy[name]
		if len(sources) > 1 {
			return fmt.Errorf("module %q is declared more than once: %s", name, strings.Join(sources, ", "))
		}
	}
	return nil
}

// RenderInlineModuleComponents builds one type: k8s-objects ApplicationComponent
// per inline module bundled in the addon (modules/<name>/_module.cue), each
// wrapping the already-rendered owned Application as its single literal
// object. There is no registry to defer to for an inline module, so it is
// parsed and rendered once, here, in Go, rather than lazily inside a
// type: module component's own CueX render -- the same RenderApplication
// call the external path eventually reaches, just invoked eagerly instead of
// through the CueX provider. Every emitted component depends on every name
// in resourceComponentNames, so a module's XRD/Compositions never apply
// before the addon's own operators and CRDs are healthy, the same ordering
// rule RenderModuleComponents already applies to external imports.
func RenderInlineModuleComponents(addon *InstallPackage, existingComponents []common2.ApplicationComponent, resourceComponentNames []string) ([]common2.ApplicationComponent, error) {
	if len(addon.InlineModules) == 0 {
		return nil, nil
	}
	// Mirrors the external path's own gate check (pkg/cue/cuex/providers/module/module.go),
	// which fires when the type: module component's CueX render actually runs. An inline
	// module has no CueX render to gate -- RenderApplication is called directly, below --
	// so without this check it would install regardless of EnableModuleComponent, silently
	// bypassing the same off-by-default safety property external imports already respect.
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableModuleComponent) {
		return nil, fmt.Errorf("module-as-component is disabled; enable the EnableModuleComponent feature gate to use inline modules")
	}

	declaredBy := moduleDeclaredBy(existingComponents, addon.Imports, addon.InlineModules)
	if err := checkModuleNameCollisions(declaredBy); err != nil {
		return nil, err
	}

	usedNames := make(map[string]bool, len(existingComponents))
	for _, c := range existingComponents {
		usedNames[c.Name] = true
	}

	var comps []common2.ApplicationComponent
	for _, mod := range addon.InlineModules {
		app, err := modulerender.RenderApplication(mod, "")
		if err != nil {
			return nil, fmt.Errorf("render inline module %q for addon %q: %w", mod.Name, addon.Name, err)
		}
		raw, err := json.Marshal(map[string]interface{}{"objects": []interface{}{app}})
		if err != nil {
			return nil, fmt.Errorf("render inline module %q for addon %q: %w", mod.Name, addon.Name, err)
		}
		componentName := uniqueImportedComponentName(mod.Name, usedNames)
		comps = append(comps, common2.ApplicationComponent{
			Name:       componentName,
			Type:       "k8s-objects",
			Properties: &runtime.RawExtension{Raw: raw},
			DependsOn:  append([]string{}, resourceComponentNames...),
		})
		usedNames[componentName] = true
	}
	return comps, nil
}
