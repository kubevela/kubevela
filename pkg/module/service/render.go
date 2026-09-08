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

package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/module/naming"
	"github.com/oam-dev/kubevela/pkg/module/service/api"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// rendererImpl fetches a module and renders its owned Application.
type rendererImpl struct {
	// fetchFn is a seam for tests. Production leaves it nil and the real
	// FetchModule is built per call, so no Kubernetes client is required at
	// construction time (init runs before the singletons are populated).
	fetchFn func(ctx context.Context, registry, moduleName, version string) (*module.Module, error)
}

// NewRenderer builds the module render service. It reads the Kubernetes client
// from the kubevela-pkg singleton at call time, not at construction, so callers
// may register it before the client is configured.
func NewRenderer() api.Renderer { return &rendererImpl{} }

// Register installs the render-only module service as the process-wide renderer
// used by the vela/module CueX provider.

func Register() { api.SetDefaultRenderer(NewRenderer()) }

// fetch resolves the module through the injected seam (tests) or the real
// registry-backed FetchModule (production).
func (r *rendererImpl) fetch(ctx context.Context, registry, moduleName, version string) (*module.Module, error) {
	if r.fetchFn != nil {
		return r.fetchFn(ctx, registry, moduleName, version)
	}
	return NewService(module.NewStore(singleton.KubeClient.Get())).FetchModule(ctx, registry, moduleName, version)
}

// RenderModule fetches the named module and renders its owned Application.
func (r *rendererImpl) RenderModule(ctx context.Context, req api.ModuleRequest) (*api.ModuleResult, error) {
	mod, err := r.fetch(ctx, req.Registry, req.Module, req.Version)
	if err != nil {
		return nil, err
	}
	app, err := RenderApplication(mod, req.Namespace)
	if err != nil {
		return nil, err
	}
	return &api.ModuleResult{Application: app}, nil
}

// RenderApplication builds the module's owned Application. It is pure: given a
// parsed Module it touches no cluster and no registry, which is what lets the
// whole tier layout be unit-tested against a fixture.
func RenderApplication(mod *module.Module, namespace string) (map[string]interface{}, error) {
	if mod == nil || mod.Name == "" {
		return nil, fmt.Errorf("render module: module has no name")
	}
	if namespace == "" {
		namespace = types.DefaultKubeVelaNS
	}

	comps := []interface{}{}

	// The module-level auxiliary tier is module-wide and precedes every line
	// beneath it. Its objects install in source order whatever their kind, and
	// the tier reports healthy as soon as they are applied: the next tier never
	// waits for an object to become ready.
	moduleDep := ""
	if len(mod.Auxiliary) > 0 {
		moduleDep = mod.Name + "-aux"
		comps = append(comps, objectsTier(moduleDep, toObjects(mod.Auxiliary), ""))
	}

	for _, apiVersion := range enabledLines(mod) {
		line := mod.Lines[apiVersion]

		// Each line hangs off the module-level auxiliary, not off the previous
		// line: lines are siblings, so v2 must not wait on v1.
		dep := moduleDep
		if len(line.Auxiliary) > 0 {
			tier := fmt.Sprintf("%s-%s-aux", mod.Name, apiVersion)
			comps = append(comps, objectsTier(tier, toObjects(line.Auxiliary), dep))
			dep = tier
		}

		if len(line.Definitions) == 0 {
			continue
		}
		defs := make([]interface{}, 0, len(line.Definitions))
		for _, def := range line.Definitions {
			defs = append(defs, stampIdentity(def, mod.Name, apiVersion))
		}
		comps = append(comps, objectsTier(fmt.Sprintf("%s-%s-defs", mod.Name, apiVersion), defs, dep))
	}

	return map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "Application",
		"metadata": map[string]interface{}{
			"name":      "module-" + mod.Name,
			"namespace": namespace,
			"labels": map[string]interface{}{
				types.LabelDefinitionModule: mod.Name,
			},
			"annotations": map[string]interface{}{
				// mod.Version is parsed from the fetched module's own _module.cue,
				// so it is always the concrete tag that was actually fetched --
				// including when the fetch request asked for "latest".
				types.AnnoDefinitionModuleVersion: mod.Version,
			},
		},
		"spec": map[string]interface{}{"components": comps},
	}, nil
}

// enabledLines returns the API versions to install: every line whose Enabled is
// true, sorted for a deterministic component order (and therefore a stable
// ApplicationRevision). Sorting is lexical, so v10 precedes v2; order between
// lines carries no meaning because lines are siblings under the XRD.
func enabledLines(mod *module.Module) []string {
	out := make([]string, 0, len(mod.Lines))
	for v, line := range mod.Lines {
		if !line.Enabled {
			continue
		}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// toObjects widens a scope's auxiliary objects for the k8s-objects properties,
// preserving source order.
func toObjects(aux []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(aux))
	for _, obj := range aux {
		out = append(out, obj)
	}
	return out
}

// objectsTier wraps objects in a k8s-objects component, healthy once applied.
// Every tier the renderer emits is one of these, so a tier's dependents wait
// only for its objects to be accepted by the API server, never for them to
// report ready.
func objectsTier(name string, objects []interface{}, dependsOn string) map[string]interface{} {
	c := map[string]interface{}{
		"name":       name,
		"type":       "k8s-objects",
		"properties": map[string]interface{}{"objects": objects},
	}
	if dependsOn != "" {
		c["dependsOn"] = []interface{}{dependsOn}
	}
	return c
}

// stampIdentity returns a copy of def carrying its module identity: the
// {module}-{apiVersion}-{name} object name, the definition identity labels, the
// full-name annotation, and the spec identity fields. It copies rather than
// mutates because the parsed Module is shared and may be cached.
func stampIdentity(def map[string]interface{}, moduleName, apiVersion string) map[string]interface{} {
	out := deepCopyMap(def)

	meta, _ := out["metadata"].(map[string]interface{})
	if meta == nil {
		meta = map[string]interface{}{}
		out["metadata"] = meta
	}
	shortName, _ := meta["name"].(string)

	fullName := fmt.Sprintf("%s-%s-%s", moduleName, apiVersion, shortName)
	meta["name"] = naming.DefinitionName(moduleName, apiVersion, shortName)

	labels, _ := meta["labels"].(map[string]interface{})
	if labels == nil {
		labels = map[string]interface{}{}
		meta["labels"] = labels
	}
	labels[types.LabelDefinitionModule] = moduleName
	labels[types.LabelDefinitionModuleAPIVersion] = apiVersion
	// The definition name can be up to the object-name limit, but a label value
	// caps at 63 chars, so bound it; the untruncated name lives on the full-name
	// annotation below.
	labels[types.LabelDefinitionName] = naming.TruncateWithHash(shortName, naming.MaxLabelValueLen)
	labels[oam.LabelAddonName] = moduleName

	annos, _ := meta["annotations"].(map[string]interface{})
	if annos == nil {
		annos = map[string]interface{}{}
		meta["annotations"] = annos
	}
	annos[types.AnnoDefinitionModuleFullName] = fullName

	spec, _ := out["spec"].(map[string]interface{})
	if spec == nil {
		spec = map[string]interface{}{}
		out["spec"] = spec
	}
	spec["module"] = moduleName
	spec["apiVersion"] = apiVersion

	return out
}

// deepCopyMap copies nested maps and slices so stamping never writes through to
// the fetched Module.
func deepCopyMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = deepCopyValue(v)
	}
	return out
}

func deepCopyValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		return deepCopyMap(t)
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = deepCopyValue(e)
		}
		return out
	case []map[string]interface{}:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = deepCopyMap(e)
		}
		return out
	default:
		return v
	}
}
