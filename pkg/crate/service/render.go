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
	"github.com/oam-dev/kubevela/pkg/crate"
	"github.com/oam-dev/kubevela/pkg/crate/naming"
	"github.com/oam-dev/kubevela/pkg/crate/service/api"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/policy/placement"
)

// rendererImpl fetches a crate and renders its owned Application.
type rendererImpl struct {
	// fetchFn is a seam for tests. Production leaves it nil and the real
	// FetchCrate is built per call, so no Kubernetes client is required at
	// construction time (init runs before the singletons are populated).
	fetchFn func(ctx context.Context, registry, crateName, version string) (*crate.Crate, error)
}

// NewRenderer builds the crate render service. It reads the Kubernetes client
// from the kubevela-pkg singleton at call time, not at construction, so callers
// may register it before the client is configured.
func NewRenderer() api.Renderer { return &rendererImpl{} }

// Register installs the render-only crate service as the process-wide renderer
// used by the vela/crate CueX provider.

func Register() { api.SetDefaultRenderer(NewRenderer()) }

// fetch resolves the crate through the injected seam (tests) or the real
// registry-backed FetchCrate (production).
func (r *rendererImpl) fetch(ctx context.Context, registry, crateName, version string) (*crate.Crate, error) {
	if r.fetchFn != nil {
		return r.fetchFn(ctx, registry, crateName, version)
	}
	return NewService(crate.NewStore(singleton.KubeClient.Get())).FetchCrate(ctx, registry, crateName, version)
}

// RenderCrate fetches the named crate and renders its owned Application.
func (r *rendererImpl) RenderCrate(ctx context.Context, req api.CrateRequest) (*api.CrateResult, error) {
	mod, err := r.fetch(ctx, req.Registry, req.Crate, req.Version)
	if err != nil {
		return nil, err
	}
	app, err := RenderApplication(mod, req.Namespace, req.Placement)
	if err != nil {
		return nil, err
	}
	return &api.CrateResult{Application: app}, nil
}

// RenderApplication builds the crate's owned Application. It is pure: given a
// parsed Crate it touches no cluster and no registry, which is what lets the
// whole tier layout be unit-tested against a fixture.
//
// definitionNamespace is where the crate's definitions install. The owned
// Application itself always lives in the system namespace: it is control-plane
// bookkeeping, so it stays with the control plane even when the capabilities it
// installs belong to a tenant namespace.
//
// Its name stays crate-{crate} with no namespace in it, because a crate
// installs once cluster-wide. A second install into a different namespace is
// meant to fail, and it does: the owned Application is already controlled by
// the first install's deploy Application, so MustBeControlledByApp rejects the
// second (pkg/utils/apply/apply.go).
//
// place is the installer's placement request. Nil means they did not ask.
// An explicit placement marker on a crate-level object or an enabled line,
// a runtime default on the crate or an enabled line, or a non-nil place,
// partitions each auxiliary tier by class and adds the topology policies and
// deploy steps that put each class where it belongs. A disabled line is
// checked for a bad marker and then ignored, because it installs nothing.
// An absent marker is hub, and so is an explicit hub default, so neither
// leaves the flat shape on its own. With neither a runtime marker nor a
// request, the Application is the flat shape this function has always
// returned.
func RenderApplication(mod *crate.Crate, definitionNamespace string, place *v1alpha1.Placement) (map[string]interface{}, error) {
	if mod == nil || mod.Name == "" {
		return nil, fmt.Errorf("render crate: crate has no name")
	}
	if errs := validation.IsDNS1123Label(mod.Name); len(errs) > 0 {
		return nil, fmt.Errorf("render crate: invalid crate name %q: %s", mod.Name, strings.Join(errs, "; "))
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

	// The crate-level auxiliary tier is crate-wide and precedes every line
	// beneath it. Its objects install in source order whatever their kind, and
	// the tier reports healthy as soon as they are applied: the next tier never
	// waits for an object to become ready.
	crateDep := ""
	if len(mod.Auxiliary) > 0 {
		crateDep = mod.Name + "-aux"
		comps = append(comps, objectsTier(crateDep, toObjects(mod.Auxiliary, definitionNamespace), ""))
	}

	for _, apiVersion := range enabledLines(mod) {
		line := mod.Lines[apiVersion]

		// Each line hangs off the crate-level auxiliary, not off the previous
		// line: lines are siblings, so v2 must not wait on v1.
		dep := crateDep
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
			"name":      "crate-" + mod.Name,
			"namespace": types.DefaultKubeVelaNS,
			"labels": map[string]interface{}{
				types.LabelDefinitionCrate: mod.Name,
			},
			"annotations": map[string]interface{}{
				// mod.Version is parsed from the fetched crate's own _crate.cue,
				// so it is always the concrete tag that was actually fetched --
				// including when the fetch request asked for "latest".
				types.AnnoDefinitionCrateVersion: mod.Version,
			},
		},
		"spec": map[string]interface{}{
			"components": comps,
			"policies":   []interface{}{crateStateKeepPolicy()},
		},
	}, nil
}

// cratePlacementNames are the policy and workflow step names the crate
// renderer gives a partitioned Application. The placement package itself does
// not know they are for a crate.
var cratePlacementNames = placement.Names{
	HubPolicy:       "crate-hub",
	RuntimePolicy:   "crate-runtime",
	HubOverride:     "crate-hub-override",
	RuntimeOverride: "crate-runtime-override",
	HubStep:         "deploy-crate-hub",
	RuntimeStep:     "deploy-crate-runtime",
}

// partitionTriggered reports whether this render leaves the flat shape.
// An explicit annotation on an installed object triggers it, and so does a
// crate or enabled-line default of runtime. The default class is hub, so an
// absent marker does not, and neither does an explicit hub default: the flat
// Application already installs on the local cluster. A disabled line is not
// installed, so its marker does not change the shape. An invalid placement
// marker is an error even on a disabled line, so a bad value cannot render
// as if it were unmarked.
func partitionTriggered(mod *crate.Crate, place *v1alpha1.Placement) (bool, error) {
	found := false
	if mod.Placement != "" {
		class, err := placement.Normalize(mod.Placement)
		if err != nil {
			return false, fmt.Errorf("render crate: %w", err)
		}
		if class == oam.PlacementRuntime {
			found = true
		}
	}
	marked, err := anyMarked(mod.Auxiliary)
	if err != nil {
		return false, err
	}
	if marked {
		found = true
	}
	for _, line := range mod.Lines {
		if line.Placement != "" {
			class, err := placement.Normalize(line.Placement)
			if err != nil {
				return false, fmt.Errorf("render crate: line %s: %w", line.APIVersion, err)
			}
			if line.Enabled && class == oam.PlacementRuntime {
				found = true
			}
		}
		marked, err := anyMarked(line.Auxiliary)
		if err != nil {
			return false, err
		}
		if line.Enabled && marked {
			found = true
		}
	}
	return place != nil || found, nil
}

func anyMarked(objs []map[string]interface{}) (bool, error) {
	found := false
	for _, obj := range objs {
		marked, err := placement.Marked(obj)
		if err != nil {
			return false, fmt.Errorf("render crate: %w", err)
		}
		if marked {
			found = true
		}
	}
	return found, nil
}

// renderPartitioned splits every auxiliary tier into the classes it actually
// contains and emits the two deploy steps that install them. Definitions are
// always hub, under the same tier name a flat render would have used.
// dependsOn stays inside one class: a hub tier depends on the hub tier above
// it, a runtime tier on the runtime tier above it.
func renderPartitioned(mod *crate.Crate, namespace string, place *v1alpha1.Placement) (map[string]interface{}, error) {
	comps := []interface{}{}
	var refs []placement.Component

	crateRuntime, crateHub := "", ""
	if len(mod.Auxiliary) > 0 {
		groups, err := placement.SplitAux(mod.Name+"-aux", mod.Auxiliary, mod.Placement)
		if err != nil {
			return nil, fmt.Errorf("render crate: %w", err)
		}
		for _, g := range groups {
			comps = append(comps, objectsTier(g.Name, toObjects(g.Objects, namespace), ""))
			refs = append(refs, placement.Component{Name: g.Name, Class: g.Class})
			if g.Class == oam.PlacementHub {
				crateHub = g.Name
			} else {
				crateRuntime = g.Name
			}
		}
	}

	for _, apiVersion := range enabledLines(mod) {
		line := mod.Lines[apiVersion]
		lineHub := ""
		if len(line.Auxiliary) > 0 {
			groups, err := placement.SplitAux(fmt.Sprintf("%s-%s-aux", mod.Name, apiVersion), line.Auxiliary, line.Placement, mod.Placement)
			if err != nil {
				return nil, fmt.Errorf("render crate: %w", err)
			}
			for _, g := range groups {
				dep := crateRuntime
				if g.Class == oam.PlacementHub {
					dep = crateHub
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
			dep = crateHub
		}
		comps = append(comps, objectsTier(defName, defs, dep))
		refs = append(refs, placement.Component{Name: defName, Class: oam.PlacementHub})
	}

	extraPolicies, steps := placement.Assemble(refs, place, cratePlacementNames)
	policies := []interface{}{crateStateKeepPolicy()}
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
			"name":      "crate-" + mod.Name,
			"namespace": types.DefaultKubeVelaNS,
			"labels": map[string]interface{}{
				types.LabelDefinitionCrate: mod.Name,
			},
			"annotations": map[string]interface{}{
				types.AnnoDefinitionCrateVersion: mod.Version,
			},
		},
		"spec": spec,
	}, nil
}

// crateStateKeepPolicyName names the apply-once policy every owned
// Application carries.
const crateStateKeepPolicyName = "crate-state-keep"

// crateStateKeepPolicy is an apply-once policy that is switched off. A crate
// installed through an addon inherits addons.oam.dev/name from the addon
// Application, and the resource keeper treats any Application with that label
// and no apply-once policy as apply-once, which turns state keep off: deleted
// definitions, auxiliary objects and CRDs would not come back until the next
// workflow run. Declaring the policy, switched off, keeps state keep running
// whatever labels the Application inherits, as the addon renderer does for the
// addon Application (ensureAddonComponentStateKeepPolicy).
func crateStateKeepPolicy() map[string]interface{} {
	return map[string]interface{}{
		"name":       crateStateKeepPolicyName,
		"type":       v1alpha1.ApplyOncePolicyType,
		"properties": map[string]interface{}{"enable": false},
	}
}

// enabledLines returns the API versions to install: every line whose Enabled is
// true, sorted for a deterministic component order (and therefore a stable
// ApplicationRevision). Sorting is lexical, so v10 precedes v2; order between
// lines carries no meaning because lines are siblings under the XRD.
func enabledLines(mod *crate.Crate) []string {
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
// crate's definitions install.
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
		// Copy for the same reason stampIdentity does: the parsed Crate is
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

// stampIdentity returns a copy of def carrying its crate identity: the
// {crate}-{apiVersion}-{name} object name, the installing namespace, the
// definition identity labels, the full-name annotation, and the spec identity
// fields. It copies rather than mutates because the parsed Crate is shared and
// may be cached.
//
// The namespace is written explicitly rather than inherited from the owned
// Application, because the two now differ: the Application is control-plane
// bookkeeping in the system namespace while its definitions install wherever
// the operator asked for them.
func stampIdentity(def map[string]interface{}, crateName, apiVersion, namespace string) map[string]interface{} {
	out := deepCopyMap(def)

	meta, _ := out["metadata"].(map[string]interface{})
	if meta == nil {
		meta = map[string]interface{}{}
		out["metadata"] = meta
	}
	shortName, _ := meta["name"].(string)

	fullName := fmt.Sprintf("%s-%s-%s", crateName, apiVersion, shortName)
	meta["name"] = naming.DefinitionName(crateName, apiVersion, shortName)
	meta["namespace"] = namespace

	labels, _ := meta["labels"].(map[string]interface{})
	if labels == nil {
		labels = map[string]interface{}{}
		meta["labels"] = labels
	}
	labels[types.LabelDefinitionCrate] = crateName
	labels[types.LabelDefinitionCrateAPIVersion] = apiVersion
	labels[types.LabelDefinitionName] = naming.TruncateWithHash(shortName, naming.MaxLabelValueLen)
	labels[oam.LabelAddonName] = crateName

	annos, _ := meta["annotations"].(map[string]interface{})
	if annos == nil {
		annos = map[string]interface{}{}
		meta["annotations"] = annos
	}
	annos[types.AnnoDefinitionCrateFullName] = fullName

	spec, _ := out["spec"].(map[string]interface{})
	if spec == nil {
		spec = map[string]interface{}{}
		out["spec"] = spec
	}
	spec["crate"] = crateName
	spec["apiVersion"] = apiVersion

	return out
}

// deepCopyMap copies nested maps and slices so stamping never writes through to
// the fetched Crate.
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
