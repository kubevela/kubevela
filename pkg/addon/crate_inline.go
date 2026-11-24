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
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/crate"
	craterender "github.com/oam-dev/kubevela/pkg/crate/service"
	"github.com/oam-dev/kubevela/pkg/features"
)

// readInlineCratesDir groups every crates/<name>/... item (everything
// under crates/ except crates/_imports.cue, which its own exact-path
// pattern already claims) by crate name, builds an in-memory crate.MapFS
// per group (the same fs.FS type the external OCI fetch path builds), and
// parses each group as an inline crate -- read from the addon's own bundled
// files via reader, no registry, no fetch. Every immediate subdirectory of
// crates/ is expected to be an inline crate; one with no _crate.cue at
// its root is a packaging mistake, not a legitimately-unrelated folder,
// so it fails --the same way the external path's ParseCrate/
// ParseCrateDir already fails loudly on a tree missing _crate.cue.
//
// rootPath is the addon's name: every real reader's RelativePath is rooted at
// it ("<addon>/crates/<name>/..."), the same rootPath GetPatternFromItem
// matches against. Pass "" for a reader whose paths are already addon-relative.
func readInlineCratesDir(a *InstallPackage, reader AsyncReader, items []Item, rootPath string) error {
	groups := map[string]craterender.MapFS{}
	var order []string

	for _, it := range items {
		readPath := reader.RelativePath(it)
		addonRel := filepath.ToSlash(readPath)
		if rootPath != "" {
			addonRel = strings.TrimPrefix(addonRel, rootPath+"/")
		}
		rel := strings.TrimPrefix(addonRel, CratesDirName+"/")
		if rel == addonRel {
			continue // not under crates/ at all; should not happen given the pattern match
		}
		sep := strings.IndexByte(rel, '/')
		if sep < 0 {
			// A file directly under crates/ with no subdirectory. The only
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
			fsys = craterender.MapFS{}
			groups[name] = fsys
			order = append(order, name)
		}
		data, err := reader.ReadFile(readPath)
		if err != nil {
			return fmt.Errorf("read inline crate %q file %s: %w", name, readPath, err)
		}
		fsys[inner] = []byte(data)
	}

	for _, name := range order {
		fsys := groups[name]
		if _, ok := fsys["_crate.cue"]; !ok {
			return fmt.Errorf("crates/%s: missing _crate.cue; every directory directly under "+
				"crates/ must be an inline crate (external references belong in "+
				"crates/_imports.cue instead)", name)
		}
		mod, err := crate.ParseCrate(fsys)
		if err != nil {
			return fmt.Errorf("parse inline crate %q: %w", name, err)
		}
		a.InlineCrates = append(a.InlineCrates, mod)
	}
	return nil
}

// crateDeclaredBy returns, for every crate name declared anywhere in the
// addon -- a hand-written type: crate component, an inline
// crates/<name>/, or a crates/_imports.cue entry -- every place that
// declares it. A disabled import declares nothing: it will not render, so it
// cannot collide with anything either.
func crateDeclaredBy(existingComponents []common2.ApplicationComponent, imports []CrateImport, inline []*crate.Crate) map[string][]string {
	declaredBy := map[string][]string{}
	for _, c := range existingComponents {
		if c.Type != "crate" {
			continue
		}
		name := c.Name
		if c.Properties != nil {
			var props struct {
				Crate string `json:"crate"`
			}
			if err := json.Unmarshal(c.Properties.Raw, &props); err == nil && props.Crate != "" {
				name = props.Crate
			}
		}
		declaredBy[name] = append(declaredBy[name], fmt.Sprintf("hand-written component %q", c.Name))
	}
	for _, imp := range imports {
		if !imp.Enabled {
			continue
		}
		declaredBy[imp.Crate] = append(declaredBy[imp.Crate], "crates/_imports.cue")
	}
	for _, mod := range inline {
		declaredBy[mod.Name] = append(declaredBy[mod.Name], fmt.Sprintf("inline crate \"crates/%s\"", mod.Name))
	}
	return declaredBy
}

// checkCrateNameCollisions fails on the first crate name declared in more
// than one place. No source silently wins over another: a name claimed
// twice is a packaging mistake, not an intentional override.
func checkCrateNameCollisions(declaredBy map[string][]string) error {
	names := make([]string, 0, len(declaredBy))
	for name := range declaredBy {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sources := declaredBy[name]
		if len(sources) > 1 {
			return fmt.Errorf("crate %q is declared more than once: %s", name, strings.Join(sources, ", "))
		}
	}
	return nil
}

// RenderInlineCrateComponents builds one type: k8s-objects ApplicationComponent
// per inline crate bundled in the addon (crates/<name>/_crate.cue), each
// wrapping the already-rendered owned Application as its single literal
// object. There is no registry to defer to for an inline crate, so it is
// parsed and rendered once, here, in Go, rather than lazily inside a
// type: crate component's own CueX render -- the same RenderApplication
// call the external path eventually reaches, just invoked eagerly instead of
// through the CueX provider. Every emitted component depends on every name
// in resourceComponentNames, so a crate's XRD/Compositions never apply
// before the addon's own operators and CRDs are healthy, the same ordering
// rule RenderCrateComponents already applies to external imports.
//
// namespace is the namespace of the Application that installs the addon, where
// the crates' definitions install; empty means vela-system.
//
// place is the addon's placement from the type: addon component. Each inline
// crate's runtime objects go to those clusters, the same as an imported
// crate that sets no placement of its own. Nil keeps everything on the hub.
func RenderInlineCrateComponents(addon *InstallPackage, existingComponents []common2.ApplicationComponent, resourceComponentNames []string, namespace string, place *v1alpha1.Placement) ([]common2.ApplicationComponent, error) {
	if len(addon.InlineCrates) == 0 {
		return nil, nil
	}
	// Mirrors the external path's own gate check (pkg/cue/cuex/providers/crate/crate.go),
	// which fires when the type: crate component's CueX render actually runs. An inline
	// crate has no CueX render to gate -- RenderApplication is called directly, below --
	// so without this check it would install regardless of EnableCrateComponent, silently
	// bypassing the same off-by-default safety property external imports already respect.
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCrateComponent) {
		return nil, fmt.Errorf("crate-as-component is disabled; enable the EnableCrateComponent feature gate to use inline crates")
	}

	declaredBy := crateDeclaredBy(existingComponents, addon.Imports, addon.InlineCrates)
	if err := checkCrateNameCollisions(declaredBy); err != nil {
		return nil, err
	}

	usedNames := make(map[string]bool, len(existingComponents))
	for _, c := range existingComponents {
		usedNames[c.Name] = true
	}

	var comps []common2.ApplicationComponent
	for _, mod := range addon.InlineCrates {
		app, err := craterender.RenderApplication(mod, namespace, place)
		if err != nil {
			return nil, fmt.Errorf("render inline crate %q for addon %q: %w", mod.Name, addon.Name, err)
		}
		raw, err := json.Marshal(map[string]interface{}{"objects": []interface{}{app}})
		if err != nil {
			return nil, fmt.Errorf("render inline crate %q for addon %q: %w", mod.Name, addon.Name, err)
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
