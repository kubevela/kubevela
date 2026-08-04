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

// Package service provides a render-only addon service: it resolves an addon
// from the registry and renders its Application plus auxiliaries, without
// dispatching anything to the cluster.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/addon/service/api"
	"github.com/oam-dev/kubevela/pkg/oam"
)

type rendererImpl struct {
	cli    client.Client
	config *rest.Config

	// cache maps explicitly versioned requests to their rendered
	// *api.AddonResult, so repeat requests skip registry I/O and CUE rendering.
	// Unpinned requests are deliberately not cached: their empty version means
	// "latest", whose resolution can change without any request input changing.
	cache sync.Map

	// resolveFn is a seam for tests: it defaults to r.resolveAndRender and is
	// only overridden by unit tests that count invocations. Production always
	// uses the default.
	resolveFn func(ctx context.Context, req api.AddonRequest) (*api.AddonResult, error)

	// findPackagesFn lets unit tests supply a package without registry I/O.
	// Production uses pkgaddon.FindAddonPackagesDetailFromRegistry.
	findPackagesFn func(context.Context, client.Client, []string, []string) ([]*pkgaddon.WholeAddonPackage, error)
}

// NewRenderer builds a render-only addon service. It reads the Kubernetes
// client and rest config from the kubevela-pkg singletons at call time, so no
// startup injection is required.
func NewRenderer() api.Renderer {
	return &rendererImpl{}
}

// init registers the default renderer so a blank import of this package
// (from cmd/core) wires the addon CueX provider, instead of injecting it in
// server.go.
func init() { api.SetDefaultRenderer(NewRenderer()) }

// client returns the injected client (tests) or the shared singleton (production).
func (r *rendererImpl) client() client.Client {
	if r.cli != nil {
		return r.cli
	}
	return singleton.KubeClient.Get()
}

// restConfig returns the injected config (tests) or the shared singleton (production).
func (r *rendererImpl) restConfig() *rest.Config {
	if r.config != nil {
		return r.config
	}
	return singleton.KubeConfig.Get()
}

// cacheKey builds the resolve-once cache key from every input that can change
// the rendered output, including SkipVersionValidate so a validated request and
// a skipped request never alias to the same cached result.
func cacheKey(req api.AddonRequest) string {
	return fmt.Sprintf("%s|%s|%s|%t|%s", req.Name, req.Version, req.Registry, req.SkipVersionValidate, hashProperties(req.Properties))
}

// hashProperties returns a stable SHA-256 hex digest of the properties map.
// json.Marshal sorts map keys, so the marshaled bytes are canonical and the
// digest is order-independent.
func hashProperties(properties map[string]interface{}) string {
	b, err := json.Marshal(properties)
	if err != nil {
		// A non-marshalable value cannot produce a stable key; fall back to a
		// sentinel that is distinct from any real digest so such requests are
		// never served a stale cache entry.
		return "unhashable"
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// RenderAddon resolves and renders an addon. Explicit versions are immutable
// cache keys; an empty version is resolved every time so a newly published
// registry version is not hidden by a stale in-process result.
func (r *rendererImpl) RenderAddon(ctx context.Context, req api.AddonRequest) (*api.AddonResult, error) {
	resolve := r.resolveFn
	if resolve == nil {
		resolve = r.resolveAndRender
	}
	if req.Version == "" {
		return resolve(ctx, req)
	}

	key := cacheKey(req)
	if cached, ok := r.cache.Load(key); ok {
		return cached.(*api.AddonResult), nil
	}
	res, err := resolve(ctx, req)
	if err != nil {
		return nil, err
	}
	r.cache.Store(key, res)
	return res, nil
}

func (r *rendererImpl) resolveAndRender(ctx context.Context, req api.AddonRequest) (*api.AddonResult, error) {
	regs := []string{}
	if req.Registry != "" {
		regs = []string{req.Registry}
	}
	findPackages := r.findPackagesFn
	if findPackages == nil {
		findPackages = pkgaddon.FindAddonPackagesDetailFromRegistry
	}
	pkgs, err := findPackages(ctx, r.client(), []string{req.Name}, regs)
	if err != nil {
		return nil, fmt.Errorf("addon %q not found in registries %v: %w", req.Name, regs, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("addon %q not found in registries %v", req.Name, regs)
	}
	whole := pkgs[0]
	installPkg := &whole.InstallPackage
	registryName := whole.RegistryName

	// FindAddonPackagesDetailFromRegistry loads a concrete package (the latest for
	// versioned registries). If the caller pinned an exact version that differs,
	// fetch that specific version.
	resolved := installPkg.Version
	if req.Version != "" && req.Version != resolved {
		exact, err := r.fetchExactVersion(ctx, registryName, req.Name, req.Version)
		if err != nil {
			return nil, fmt.Errorf("fetch addon %q version %q: %w", req.Name, req.Version, err)
		}
		installPkg = exact
		resolved = exact.Version
	}

	if !req.SkipVersionValidate {
		if err := r.validateSystemRequirements(ctx, req.Name, installPkg); err != nil {
			return nil, err
		}
	}

	app, aux, err := pkgaddon.RenderApp(ctx, installPkg, r.client(), req.Properties)
	if err != nil {
		return nil, fmt.Errorf("render addon %q: %w", req.Name, err)
	}

	groups, err := r.auxComponents(ctx, installPkg, req.Properties)
	if err != nil {
		return nil, fmt.Errorf("render auxiliaries for addon %q: %w", req.Name, err)
	}
	// The addon template's own outputs (RenderApp's aux) have no fixed category,
	// so they go into a catch-all component.
	groups = append(groups, auxComponent{name: "addon-auxiliaries", objects: aux})

	setAddonRegistryLabel(app, registryName)

	appMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(app)
	if err != nil {
		return nil, err
	}
	appMap["apiVersion"] = "core.oam.dev/v1beta1"
	appMap["kind"] = "Application"

	appendAuxComponents(appMap, groups)
	ensureAddonComponentStateKeepPolicy(appMap)
	sanitizeManifest(appMap)
	suppressLastAppliedConfig(appMap)

	return &api.AddonResult{
		ResolvedVersion: resolved,
		Registry:        registryName,
		Application:     appMap,
	}, nil
}

func setAddonRegistryLabel(app metav1.Object, registryName string) {
	labels := app.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[oam.LabelAddonRegistry] = registryName
	app.SetLabels(labels)
}

// sanitizeManifest removes the root status subresource and every
// metadata.creationTimestamp (recursively, including nested objects such as the
// resources inside a k8s-objects component) so the manifest completes cleanly
// as CUE.
func sanitizeManifest(m map[string]interface{}) {
	delete(m, "status")
	stripCreationTimestamp(m)
}

func stripCreationTimestamp(v interface{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		if meta, ok := t["metadata"].(map[string]interface{}); ok {
			delete(meta, "creationTimestamp")
		}
		for _, val := range t {
			stripCreationTimestamp(val)
		}
	case []interface{}:
		for _, item := range t {
			stripCreationTimestamp(item)
		}
	}
}

// suppressLastAppliedConfig marks the rendered Application so the dispatcher
// skips recording app.oam.dev/last-applied-configuration on it. The full
// desired spec is already durably captured per-generation in the
// ApplicationRevision, so this second dispatch-time copy is redundant - and
// for a resource-heavy addon (e.g. one bundling several CRDs), folding every
// manifest into a single Application can make that copy alone exceed
// Kubernetes' 256KiB per-object annotation limit, failing the apply outright.
func suppressLastAppliedConfig(m map[string]interface{}) {
	metadata, ok := m["metadata"].(map[string]interface{})
	if !ok {
		metadata = map[string]interface{}{}
		m["metadata"] = metadata
	}
	annotations, ok := metadata["annotations"].(map[string]interface{})
	if !ok {
		annotations = map[string]interface{}{}
		metadata["annotations"] = annotations
	}
	annotations[oam.AnnotationLastAppliedConfig] = "skip"
}

const addonComponentStateKeepPolicyName = "addon-component-state-keep"

// ensureAddonComponentStateKeepPolicy disables the legacy implicit apply-once
// behavior for component-installed addons unless the addon declares its own.
func ensureAddonComponentStateKeepPolicy(m map[string]interface{}) {
	spec, ok := m["spec"].(map[string]interface{})
	if !ok {
		spec = map[string]interface{}{}
		m["spec"] = spec
	}
	policies, _ := spec["policies"].([]interface{})
	usedNames := make(map[string]struct{}, len(policies))
	for _, item := range policies {
		policy, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if policyType, _ := policy["type"].(string); policyType == v1alpha1.ApplyOncePolicyType {
			return
		}
		if name, _ := policy["name"].(string); name != "" {
			usedNames[name] = struct{}{}
		}
	}

	name := addonComponentStateKeepPolicyName
	for suffix := 2; ; suffix++ {
		if _, found := usedNames[name]; !found {
			break
		}
		name = fmt.Sprintf("%s-%d", addonComponentStateKeepPolicyName, suffix)
	}
	spec["policies"] = append(policies, map[string]interface{}{
		"name":       name,
		"type":       v1alpha1.ApplyOncePolicyType,
		"properties": map[string]interface{}{"enable": false},
	})
}

// validateSystemRequirements checks the addon's SystemRequirements against the
// running environment. When the rest config is nil (e.g. in unit tests) the
// check is skipped with a logged note, since a discovery client cannot be built.
func (r *rendererImpl) validateSystemRequirements(ctx context.Context, name string, installPkg *pkgaddon.InstallPackage) error {
	cfg := r.restConfig()
	if cfg == nil {
		klog.InfoS("skipping addon system requirement check: no rest config available", "addon", name)
		return nil
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return fmt.Errorf("build discovery client for addon %q version check: %w", name, err)
	}
	if err := pkgaddon.ValidateSystemRequirements(ctx, installPkg.SystemRequirements, r.client(), dc); err != nil {
		return fmt.Errorf("addon %q does not meet system requirements: %w", name, err)
	}
	return nil
}

// fetchExactVersion resolves a specific addon version from the named registry.
func (r *rendererImpl) fetchExactVersion(ctx context.Context, registryName, addonName, version string) (*pkgaddon.InstallPackage, error) {
	return pkgaddon.GetAddonInstallPackageFromRegistry(ctx, r.client(), registryName, addonName, version)
}

// auxComponent is one k8s-objects component to append to the addon Application.
// Each corresponds to a category of auxiliary the dispatcher normally applies
// alongside the addon Application.
type auxComponent struct {
	name    string
	objects []*unstructured.Unstructured
}

// auxComponents renders the addon's definition, config-template, schema, view
// and args-secret objects and returns them grouped by category, in a
// deterministic order.
func (r *rendererImpl) auxComponents(ctx context.Context, installPkg *pkgaddon.InstallPackage, properties map[string]interface{}) ([]auxComponent, error) {
	defs, err := pkgaddon.RenderDefinitions(installPkg, r.restConfig())
	if err != nil {
		return nil, err
	}
	configTemplates, err := pkgaddon.RenderConfigTemplates(ctx, installPkg, r.client())
	if err != nil {
		return nil, err
	}
	schemas, err := pkgaddon.RenderDefinitionSchema(installPkg)
	if err != nil {
		return nil, err
	}
	views, err := pkgaddon.RenderViews(ctx, installPkg)
	if err != nil {
		return nil, err
	}
	var secretObjs []*unstructured.Unstructured
	if secret := pkgaddon.RenderArgsSecret(installPkg, properties); secret != nil {
		secretObjs = []*unstructured.Unstructured{secret}
	}
	return []auxComponent{
		{name: "addon-definitions", objects: defs},
		{name: "addon-config-templates", objects: configTemplates},
		{name: "addon-schemas", objects: schemas},
		{name: "addon-views", objects: views},
		{name: "addon-secret", objects: secretObjs},
	}, nil
}

// appendAuxComponents wraps each non-empty group as a k8s-objects component and
// appends it to the Application's spec.components. Each auxiliary object is
// sanitized (status + creationTimestamp) before nesting so the manifest
// completes cleanly as CUE.
func appendAuxComponents(appMap map[string]interface{}, groups []auxComponent) {
	spec, ok := appMap["spec"].(map[string]interface{})
	if !ok {
		spec = map[string]interface{}{}
		appMap["spec"] = spec
	}
	comps, _ := spec["components"].([]interface{})
	for _, g := range groups {
		objs := make([]interface{}, 0, len(g.objects))
		for _, o := range g.objects {
			if o == nil {
				continue
			}
			sanitizeManifest(o.Object)
			objs = append(objs, o.Object)
		}
		if len(objs) == 0 {
			continue // omit empty categories
		}
		comps = append(comps, map[string]interface{}{
			"name":       g.name,
			"type":       "k8s-objects",
			"properties": map[string]interface{}{"objects": objs},
		})
	}
	spec["components"] = comps
}
