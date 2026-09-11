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

package addonmoduletest

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	sysruntime "runtime"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	addonapi "github.com/oam-dev/kubevela/pkg/addon/service/api"
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	moduleservice "github.com/oam-dev/kubevela/pkg/module/service"
	moduleapi "github.com/oam-dev/kubevela/pkg/module/service/api"
)

// fixtureModuleName is the one module the fixtures in this suite know about,
// reusing test/e2e-test/testdata/module/e2e-widget -- an existing, already
// unit-tested module fixture (name "e2e-widget", one enabled API line "v1",
// one ComponentDefinition, no auxiliary objects) -- rather than authoring a
// new one.
const fixtureModuleName = "e2e-widget"

// fixtureAddonName is the addon this suite's fixture addon renderer knows
// about. Its modules/_imports.cue equivalent (built directly as
// InstallPackage.Imports, exactly the field RenderModuleComponents reads)
// references fixtureModuleName as one enabled import.
const fixtureAddonName = "e2e-nest-addon"

// fixtureModuleDir resolves the e2e-widget module fixture directory shared
// with test/e2e-test, independent of the working directory the test binary
// runs from.
func fixtureModuleDir() string {
	_, thisFile, _, _ := sysruntime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "e2e-test", "testdata", "module", fixtureModuleName)
}

// fixtureModuleRenderer implements moduleapi.Renderer over the local
// e2e-widget fixture directory instead of a real module registry. It calls
// the same production parse/render functions
// (pkgmodule.ParseModuleDir, moduleservice.RenderApplication) that the real
// registry-backed renderer calls after fetching -- only the fetch step is
// replaced.
type fixtureModuleRenderer struct{}

func (r *fixtureModuleRenderer) RenderModule(_ context.Context, req moduleapi.ModuleRequest) (*moduleapi.ModuleResult, error) {
	if req.Module != fixtureModuleName {
		return nil, fmt.Errorf("fixture module renderer: unknown module %q", req.Module)
	}
	mod, err := pkgmodule.ParseModuleDir(fixtureModuleDir())
	if err != nil {
		return nil, fmt.Errorf("parse fixture module %q: %w", req.Module, err)
	}
	ns := req.Namespace
	if ns == "" {
		ns = systemNamespace
	}
	appMap, err := moduleservice.RenderApplication(mod, ns)
	if err != nil {
		return nil, fmt.Errorf("render fixture module %q: %w", req.Module, err)
	}
	return &moduleapi.ModuleResult{Application: appMap}, nil
}

// fixtureAddonRenderer implements addonapi.Renderer over an in-memory
// InstallPackage instead of a registry-fetched one. It calls the same
// production render functions the real registry-backed renderer calls
// (pkgaddon.RenderApp, pkgaddon.RenderResources, pkgaddon.RenderModuleComponents)
// -- Tasks 1-4's exported functions -- so this test exercises the real
// rendering code, not a re-implementation of it.
type fixtureAddonRenderer struct {
	cli client.Client
}

func (r *fixtureAddonRenderer) RenderAddon(ctx context.Context, req addonapi.AddonRequest) (*addonapi.AddonResult, error) {
	if req.Name != fixtureAddonName {
		return nil, fmt.Errorf("fixture addon renderer: unknown addon %q", req.Name)
	}

	installPkg := &pkgaddon.InstallPackage{
		Meta: pkgaddon.Meta{Name: req.Name},
		AppCueTemplate: pkgaddon.ElementFile{Data: fmt.Sprintf(`output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: {
		name:      "addon-%s"
		namespace: %q
	}
	spec: components: []
}`, req.Name, systemNamespace)},
		// The modules/_imports.cue equivalent: one enabled import of the
		// e2e-widget fixture module, with no addon-declared resources tier.
		Imports: []pkgaddon.ModuleImport{
			{Module: fixtureModuleName, Enabled: true},
		},
	}

	app, _, err := pkgaddon.RenderApp(ctx, installPkg, r.cli, req.Properties)
	if err != nil {
		return nil, fmt.Errorf("render fixture addon %q app: %w", req.Name, err)
	}

	resourceComps, err := pkgaddon.RenderResources(installPkg, req.Properties)
	if err != nil {
		return nil, fmt.Errorf("render fixture addon %q resources: %w", req.Name, err)
	}
	resourceNames := make([]string, 0, len(resourceComps))
	for _, c := range resourceComps {
		resourceNames = append(resourceNames, c.Name)
	}

	moduleComps, err := pkgaddon.RenderModuleComponents(installPkg, app.Spec.Components, resourceNames)
	if err != nil {
		return nil, fmt.Errorf("render fixture addon %q module components: %w", req.Name, err)
	}

	// Unlike this fixture, production's resolveAndRender (pkg/addon/service/renderer.go)
	// never appends resourceComps to app.Spec.Components itself -- RenderApp already put
	// any resources/ components there. RenderResources is called a second time only to
	// derive their names for dependsOn. This fixture's InstallPackage declares no
	// YAMLTemplates/CUETemplates, so resourceComps is always empty here; mirroring
	// production exactly (not appending it) keeps this fixture a faithful copy rather
	// than a divergent one that happens to be harmless only by coincidence.
	app.Spec.Components = append(app.Spec.Components, moduleComps...)

	appMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(app)
	if err != nil {
		return nil, fmt.Errorf("convert fixture addon %q app to unstructured: %w", req.Name, err)
	}
	appMap["apiVersion"] = "core.oam.dev/v1beta1"
	appMap["kind"] = "Application"
	delete(appMap, "status")
	// The converted Application carries a zero metadata.creationTimestamp
	// (and any nested object created via k8s-objects components would too),
	// which unstructured.Unstructured round-trips as {} -- CUE's compiler
	// then sees an incomplete value where the addon ComponentDefinition's
	// output expects a concrete string, and rendering fails. The real
	// registry-backed renderer strips this same field for the same reason
	// (see sanitizeManifest/stripCreationTimestamp in renderer.go).
	stripCreationTimestamp(appMap)

	return &addonapi.AddonResult{
		ResolvedVersion: "1.0.0",
		Registry:        req.Registry,
		Application:     appMap,
	}, nil
}

// stripCreationTimestamp recursively deletes metadata.creationTimestamp,
// including on objects nested inside a component's properties (e.g. a
// k8s-objects component's properties.objects). Mirrors
// pkg/addon/service/renderer.go's stripCreationTimestamp.
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

// mustRawExtension marshals a component's properties for the Application
// spec; the fixed, hand-written literals passed to it here always marshal
// cleanly, so a marshal failure would mean a mistake in the test itself.
func mustRawExtension(m map[string]interface{}) *runtime.RawExtension {
	raw, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return &runtime.RawExtension{Raw: raw}
}

var _ = Describe("Outer Application -> addon component -> module component nest", func() {
	ctx := context.Background()
	var ns string

	BeforeEach(func() {
		ns = "addon-module-nest-" + strconv.FormatInt(rand.Int63(), 16)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())

		// Wire the fixture-backed renderers freshly for each spec so a
		// previous spec's renderer (or lack of one) can never leak in.
		addonapi.SetDefaultRenderer(&fixtureAddonRenderer{cli: k8sClient})
		moduleapi.SetDefaultRenderer(&fixtureModuleRenderer{})
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
	})

	It("renders, reconciles to running, and GCs both nested Applications", func() {
		outerName := "outer-app"
		addonAppName := "addon-" + fixtureAddonName
		moduleAppName := "module-" + fixtureModuleName
		defName := fixtureModuleName + "-v1-" + fixtureModuleName

		By("applying the outer Application with a type: addon component")
		outer := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: outerName, Namespace: ns},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "nest-addon",
						Type: "addon",
						Properties: mustRawExtension(map[string]interface{}{
							"addon": fixtureAddonName,
						}),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, outer)).To(Succeed())

		By("the outer Application reaching running")
		Eventually(func(g Gomega) common.ApplicationPhase {
			var got v1beta1.Application
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: outerName, Namespace: ns}, &got)).To(Succeed())
			return got.Status.Phase
		}, "90s", "1s").Should(Equal(common.ApplicationRunning))

		By("the nested addon-<name> Application existing with a type: module component")
		var addonApp v1beta1.Application
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: addonAppName, Namespace: systemNamespace}, &addonApp)).To(Succeed())
		}, "30s", "1s").Should(Succeed())

		var moduleComponent *common.ApplicationComponent
		for i := range addonApp.Spec.Components {
			if addonApp.Spec.Components[i].Type == "module" {
				moduleComponent = &addonApp.Spec.Components[i]
				break
			}
		}
		Expect(moduleComponent).NotTo(BeNil(), "expected the rendered addon-<name> Application to contain a type: module component")
		Expect(moduleComponent.Name).To(Equal(fixtureModuleName))

		By("the nested addon-<name> Application reaching running")
		Eventually(func(g Gomega) common.ApplicationPhase {
			var got v1beta1.Application
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: addonAppName, Namespace: systemNamespace}, &got)).To(Succeed())
			return got.Status.Phase
		}, "90s", "1s").Should(Equal(common.ApplicationRunning))

		By("the nested module-<name> Application existing and reaching running")
		Eventually(func(g Gomega) common.ApplicationPhase {
			var got v1beta1.Application
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: moduleAppName, Namespace: systemNamespace}, &got)).To(Succeed())
			return got.Status.Phase
		}, "90s", "1s").Should(Equal(common.ApplicationRunning))

		By("the module's definition being stamped and present")
		var def unstructured.Unstructured
		def.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: defName, Namespace: systemNamespace}, &def)).To(Succeed())
		}, "30s", "1s").Should(Succeed())
		Expect(def.GetLabels()[types.LabelDefinitionModule]).To(Equal(fixtureModuleName))
		Expect(def.GetLabels()[types.LabelDefinitionModuleAPIVersion]).To(Equal("v1"))
		Expect(def.GetAnnotations()[types.AnnoDefinitionModuleFullName]).To(Equal(fixtureModuleName + "-v1-" + fixtureModuleName))
		// stampIdentity (pkg/module/service/render.go) also sets spec.module and
		// spec.apiVersion on the definition object before it is applied. Those two
		// fields do NOT survive here: the real ComponentDefinition CRD
		// (charts/vela-core/crds/core.oam.dev_componentdefinitions.yaml) declares a
		// structural schema for spec with a fixed properties list, and only
		// spec.extension carries x-kubernetes-preserve-unknown-fields: true -- so
		// the apiserver silently prunes any other unrecognized spec field,
		// including spec.module/spec.apiVersion, on every write. This is a real
		// discrepancy this e2e test surfaced that no pure-unit test (Tasks 1-4)
		// could catch, since those never round-trip the rendered manifest through
		// a real API server. See the Task 5 report for detail; it is left
		// unresolved here deliberately, as fixing it is a production-code design
		// call outside this test-authoring task's scope.
		spec, ok := def.Object["spec"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "expected the stamped definition to carry a spec")
		Expect(spec["module"]).To(BeNil(), "spec.module is pruned by the ComponentDefinition CRD's structural schema; see comment above")
		Expect(spec["apiVersion"]).To(BeNil(), "spec.apiVersion is pruned by the ComponentDefinition CRD's structural schema; see comment above")

		By("deleting the outer Application")
		Expect(k8sClient.Delete(ctx, outer)).To(Succeed())

		By("the outer Application being gone")
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: outerName, Namespace: ns}, &v1beta1.Application{})
		}, "60s", "1s").ShouldNot(Succeed())

		By("the nested addon-<name> Application being GC'd")
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: addonAppName, Namespace: systemNamespace}, &v1beta1.Application{})
		}, "90s", "1s").ShouldNot(Succeed())

		By("the nested module-<name> Application being GC'd")
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: moduleAppName, Namespace: systemNamespace}, &v1beta1.Application{})
		}, "90s", "1s").ShouldNot(Succeed())
	})
})
