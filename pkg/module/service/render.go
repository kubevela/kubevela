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
	"strings"

	"github.com/kubevela/pkg/util/singleton"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/module/naming"
	"github.com/oam-dev/kubevela/pkg/module/service/api"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/policy/placement"
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
	app, err := RenderApplication(mod, req.Namespace, req.Placement)
	if err != nil {
		return nil, err
	}
	return &api.ModuleResult{Application: app}, nil
}

// RenderApplication builds the module's owned Application. It is pure: given a
// parsed Module it touches no cluster and no registry, which is what lets the
// whole tier layout be unit-tested against a fixture.
//
// definitionNamespace is where the module's definitions install. The owned
// Application itself always lives in the system namespace: it is control-plane
// bookkeeping, so it stays with the control plane even when the capabilities it
// installs belong to a tenant namespace.
//
// Its name stays module-{module} with no namespace in it, because a module
// installs once cluster-wide. A second install into a different namespace is
// meant to fail, and it does: the owned Application is already controlled by
// the first install's deploy Application, so MustBeControlledByApp rejects the
// second (pkg/utils/apply/apply.go).
//
// place is the installer's placement request. Nil means they did not ask.
// An explicit placement marker on any auxiliary object, or a non-nil place,
// partitions each auxiliary tier by class and adds the topology policies and
// deploy steps that put each class where it belongs. An absent marker is hub,
// so it does not by itself follow the request. With neither an explicit
// marker nor a request, the Application is the flat shape this function has
// always returned.
func RenderApplication(mod *module.Module, definitionNamespace string, place *v1alpha1.Placement) (map[string]interface{}, error) {
	if mod == nil || mod.Name == "" {
		return nil, fmt.Errorf("render module: module has no name")
	}
	if errs := validation.IsDNS1123Label(mod.Name); len(errs) > 0 {
		return nil, fmt.Errorf("render module: invalid module name %q: %s", mod.Name, strings.Join(errs, "; "))
	}
	if definitionNamespace == "" {
		definitionNamespace = types.DefaultKubeVelaNS
	}

	triggered, err := partitionTriggered(mod, place)
	if err != nil {
		return nil, err
	}
	if triggered {
		return renderPartitioned(mod, definitionNamespace, place)
	}

	comps := []interface{}{}

	// The module-level auxiliary tier is module-wide and precedes every line
	// beneath it. Its objects install in source order whatever their kind, and
	// the tier reports healthy as soon as they are applied: the next tier never
	// waits for an object to become ready.
	moduleDep := ""
	if len(mod.Auxiliary) > 0 {
		moduleDep = mod.Name + "-aux"
		comps = append(comps, objectsTier(moduleDep, toObjects(mod.Auxiliary, definitionNamespace), ""))
	}

	for _, apiVersion := range enabledLines(mod) {
		line := mod.Lines[apiVersion]

		// Each line hangs off the module-level auxiliary, not off the previous
		// line: lines are siblings, so v2 must not wait on v1.
		dep := moduleDep
		if len(line.Auxiliary) > 0 {
			tier := fmt.Sprintf("%s-%s-aux", mod.Name, apiVersion)
			comps = append(comps, objectsTier(tier, toObjects(line.Auxiliary, definitionNamespace), dep))
			dep = tier
		}

		if len(line.Definitions) == 0 {
			continue
		}
		defs := make([]interface{}, 0, len(line.Definitions))
		for _, def := range line.Definitions {
			defs = append(defs, stampIdentity(def, mod.Name, apiVersion, definitionNamespace))
		}
		comps = append(comps, objectsTier(fmt.Sprintf("%s-%s-defs", mod.Name, apiVersion), defs, dep))
	}

	return map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "Application",
		"metadata": map[string]interface{}{
			"name":      "module-" + mod.Name,
			"namespace": types.DefaultKubeVelaNS,
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
		"spec": map[string]interface{}{
			"components": comps,
			"policies":   []interface{}{moduleStateKeepPolicy()},
		},
	}, nil
}

// modulePlacementNames are the policy and workflow step names the module
// renderer gives a partitioned Application. The placement package itself does
// not know they are for a module.
var modulePlacementNames = placement.Names{
	HubPolicy:       "module-hub",
	RuntimePolicy:   "module-runtime",
	HubOverride:     "module-hub-override",
	RuntimeOverride: "module-runtime-override",
	HubStep:         "deploy-module-hub",
	RuntimeStep:     "deploy-module-runtime",
}

// partitionTriggered reports whether this render leaves the flat shape.
// An explicit annotation triggers it, and so does a module or line default of
// runtime. The default class is hub, so an absent marker does not. An invalid
// placement marker is an error even when it is not what triggers the
// partition, so a bad value cannot render as if it were unmarked.
func partitionTriggered(mod *module.Module, place *v1alpha1.Placement) (bool, error) {
	found := false
	if mod.Placement != "" {
		class, err := placement.Normalize(mod.Placement)
		if err != nil {
			return false, fmt.Errorf("render module: %w", err)
		}
		if class == oam.PlacementRuntime {
			found = true
		}
	}
	for _, line := range mod.Lines {
		if line.Placement == "" {
			continue
		}
		class, err := placement.Normalize(line.Placement)
		if err != nil {
			return false, fmt.Errorf("render module: line %s: %w", line.APIVersion, err)
		}
		if line.Enabled && class == oam.PlacementRuntime {
			found = true
		}
	}
	groups := [][]map[string]interface{}{mod.Auxiliary}
	for _, line := range mod.Lines {
		groups = append(groups, line.Auxiliary)
	}
	for _, objs := range groups {
		for _, obj := range objs {
			marked, err := placement.Marked(obj)
			if err != nil {
				return false, fmt.Errorf("render module: %w", err)
			}
			if marked {
				found = true
			}
		}
	}
	return place != nil || found, nil
}

// renderPartitioned splits every auxiliary tier into the classes it actually
// contains and emits the two deploy steps that install them. Definitions are
// always hub, under the same tier name a flat render would have used.
// dependsOn stays inside one class: a hub tier depends on the hub tier above
// it, a runtime tier on the runtime tier above it.
func renderPartitioned(mod *module.Module, namespace string, place *v1alpha1.Placement) (map[string]interface{}, error) {
	comps := []interface{}{}
	var refs []placement.Component

	moduleRuntime, moduleHub := "", ""
	if len(mod.Auxiliary) > 0 {
		groups, err := placement.SplitAux(mod.Name+"-aux", mod.Auxiliary, mod.Placement)
		if err != nil {
			return nil, fmt.Errorf("render module: %w", err)
		}
		for _, g := range groups {
			comps = append(comps, objectsTier(g.Name, toObjects(g.Objects, namespace), ""))
			refs = append(refs, placement.Component{Name: g.Name, Class: g.Class})
			if g.Class == oam.PlacementHub {
				moduleHub = g.Name
			} else {
				moduleRuntime = g.Name
			}
		}
	}

	for _, apiVersion := range enabledLines(mod) {
		line := mod.Lines[apiVersion]
		lineHub := ""
		if len(line.Auxiliary) > 0 {
			groups, err := placement.SplitAux(fmt.Sprintf("%s-%s-aux", mod.Name, apiVersion), line.Auxiliary, line.Placement, mod.Placement)
			if err != nil {
				return nil, fmt.Errorf("render module: %w", err)
			}
			for _, g := range groups {
				dep := moduleRuntime
				if g.Class == oam.PlacementHub {
					dep = moduleHub
					lineHub = g.Name
				}
				comps = append(comps, objectsTier(g.Name, toObjects(g.Objects, namespace), dep))
				refs = append(refs, placement.Component{Name: g.Name, Class: g.Class})
			}
		}
		if len(line.Definitions) == 0 {
			continue
		}
		defs := make([]interface{}, 0, len(line.Definitions))
		for _, def := range line.Definitions {
			defs = append(defs, stampIdentity(def, mod.Name, apiVersion, namespace))
		}
		defName := fmt.Sprintf("%s-%s-defs", mod.Name, apiVersion)
		dep := lineHub
		if dep == "" {
			dep = moduleHub
		}
		comps = append(comps, objectsTier(defName, defs, dep))
		refs = append(refs, placement.Component{Name: defName, Class: oam.PlacementHub})
	}

	extraPolicies, steps := placement.Assemble(refs, place, modulePlacementNames)
	policies := []interface{}{moduleStateKeepPolicy()}
	for _, p := range extraPolicies {
		policies = append(policies, p)
	}
	stepList := make([]interface{}, len(steps))
	for i, step := range steps {
		stepList[i] = step
	}
	spec := map[string]interface{}{
		"components": comps,
		"policies":   policies,
		"workflow": map[string]interface{}{
			"steps": stepList,
		},
	}
	return map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "Application",
		"metadata": map[string]interface{}{
			"name":      "module-" + mod.Name,
			"namespace": types.DefaultKubeVelaNS,
			"labels": map[string]interface{}{
				types.LabelDefinitionModule: mod.Name,
			},
			"annotations": map[string]interface{}{
				types.AnnoDefinitionModuleVersion: mod.Version,
			},
		},
		"spec": spec,
	}, nil
}

// moduleStateKeepPolicyName names the apply-once policy every owned
// Application carries.
const moduleStateKeepPolicyName = "module-state-keep"

// moduleStateKeepPolicy is an apply-once policy that is switched off. A module
// installed through an addon inherits addons.oam.dev/name from the addon
// Application, and the resource keeper treats any Application with that label
// and no apply-once policy as apply-once, which turns state keep off: deleted
// definitions, auxiliary objects and CRDs would not come back until the next
// workflow run. Declaring the policy, switched off, keeps state keep running
// whatever labels the Application inherits, as the addon renderer does for the
// addon Application (ensureAddonComponentStateKeepPolicy).
func moduleStateKeepPolicy() map[string]interface{} {
	return map[string]interface{}{
		"name":       moduleStateKeepPolicyName,
		"type":       v1alpha1.ApplyOncePolicyType,
		"properties": map[string]interface{}{"enable": false},
	}
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
// preserving source order, and defaults each one's namespace to where the
// module's definitions install.
//
// The default preserves where these objects landed before the owned Application
// moved to the system namespace: they installed beside the definitions they
// support, and they still do. An object naming its own namespace keeps it, as
// before. A cluster-scoped object is unaffected in practice, because the
// resource keeper strips the namespace from cluster-scoped manifests before it
// dispatches or deletes them.
func toObjects(aux []map[string]interface{}, namespace string) []interface{} {
	out := make([]interface{}, 0, len(aux))
	for _, obj := range aux {
		// Copy for the same reason stampIdentity does: the parsed Module is
		// shared and may be cached.
		o := deepCopyMap(obj)
		meta, _ := o["metadata"].(map[string]interface{})
		if meta == nil {
			meta = map[string]interface{}{}
			o["metadata"] = meta
		}
		if ns, _ := meta["namespace"].(string); ns == "" {
			meta["namespace"] = namespace
		}
		out = append(out, o)
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
// {module}-{apiVersion}-{name} object name, the installing namespace, the
// definition identity labels, the full-name annotation, and the spec identity
// fields. It copies rather than mutates because the parsed Module is shared and
// may be cached.
//
// The namespace is written explicitly rather than inherited from the owned
// Application, because the two now differ: the Application is control-plane
// bookkeeping in the system namespace while its definitions install wherever
// the operator asked for them.
func stampIdentity(def map[string]interface{}, moduleName, apiVersion, namespace string) map[string]interface{} {
	out := deepCopyMap(def)

	meta, _ := out["metadata"].(map[string]interface{})
	if meta == nil {
		meta = map[string]interface{}{}
		out["metadata"] = meta
	}
	shortName, _ := meta["name"].(string)

	fullName := fmt.Sprintf("%s-%s-%s", moduleName, apiVersion, shortName)
	meta["name"] = naming.DefinitionName(moduleName, apiVersion, shortName)
	meta["namespace"] = namespace

	labels, _ := meta["labels"].(map[string]interface{})
	if labels == nil {
		labels = map[string]interface{}{}
		meta["labels"] = labels
	}
	labels[types.LabelDefinitionModule] = moduleName
	labels[types.LabelDefinitionModuleAPIVersion] = apiVersion
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
