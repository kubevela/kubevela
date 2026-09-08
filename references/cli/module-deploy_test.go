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

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/yaml"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// moduleDeployClient returns a fake client seeded with a module registry
// ConfigMap holding the named registries, so ResolveRegistry can run without a
// cluster.
func moduleDeployClient(t *testing.T, registries ...string) client.Client {
	t.Helper()
	data := "{}"
	if len(registries) > 0 {
		entries := make(map[string]pkgaddon.Registry, len(registries))
		for _, name := range registries {
			entries[name] = pkgaddon.Registry{
				Name: name,
				Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/kubevela/catalog", Path: "module"},
			}
		}
		raw, err := json.Marshal(entries)
		require.NoError(t, err)
		data = string(raw)
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pkgmodule.ModuleRegistryConfigMap,
			Namespace: velatypes.DefaultKubeVelaNS,
		},
		Data: map[string]string{"registries": data},
	}
	return fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(cm).Build()
}

// stubModule is the module the fetch seam returns in tests: a module-level
// auxiliary XRD plus one enabled line with an auxiliary Composition and one
// definition.
func stubModule() *pkgmodule.Module {
	return &pkgmodule.Module{
		Name:      "s3",
		Version:   "1.0.0",
		Auxiliary: []map[string]interface{}{{"kind": "CompositeResourceDefinition"}},
		Lines: map[string]pkgmodule.Line{
			"v1": {
				APIVersion:  "v1",
				Enabled:     true,
				Auxiliary:   []map[string]interface{}{{"kind": "Composition"}},
				Definitions: []map[string]interface{}{{"kind": "ComponentDefinition"}},
			},
		},
	}
}

// countApplications returns how many Applications exist on a client.
func countApplications(t *testing.T, cli client.Client) int {
	t.Helper()
	var apps v1beta1.ApplicationList
	require.NoError(t, cli.List(context.Background(), &apps))
	return len(apps.Items)
}

func TestModuleDeployFailsBeforeApply(t *testing.T) {
	testCases := map[string]struct {
		registry    string
		registries  []string
		fetch       func(ctx context.Context, registry, moduleName, version string) (*pkgmodule.Module, error)
		wantErrPart string
	}{
		"unknown registry": {
			registry:    "missing",
			registries:  []string{"catalog"},
			fetch:       func(_ context.Context, _, _, _ string) (*pkgmodule.Module, error) { return stubModule(), nil },
			wantErrPart: `module registry "missing" not found`,
		},
		"no registry configured": {
			registry:    "",
			registries:  nil,
			fetch:       func(_ context.Context, _, _, _ string) (*pkgmodule.Module, error) { return stubModule(), nil },
			wantErrPart: "no module registry is configured",
		},
		"module not in registry": {
			registry:   "catalog",
			registries: []string{"catalog"},
			fetch: func(_ context.Context, _, _, _ string) (*pkgmodule.Module, error) {
				return nil, fmt.Errorf(`module "s4" not found in registry "catalog"`)
			},
			wantErrPart: `module "s4" not found`,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			cli := moduleDeployClient(t, tc.registries...)
			o := &moduleDeployOptions{
				module:    "s3",
				registry:  tc.registry,
				namespace: velatypes.DefaultKubeVelaNS,
				fetch:     tc.fetch,
			}

			err := o.run(context.Background(), cli, &bytes.Buffer{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrPart)
			assert.Equal(t, 0, countApplications(t, cli), "nothing may be applied when validation fails")
		})
	}
}

func TestModuleDeployResolvesDefaultRegistry(t *testing.T) {
	cli := moduleDeployClient(t, "catalog")
	var out bytes.Buffer
	o := &moduleDeployOptions{
		module:    "s3",
		registry:  "",
		namespace: velatypes.DefaultKubeVelaNS,
		dryRun:    true,
		fetch: func(_ context.Context, registry, _, _ string) (*pkgmodule.Module, error) {
			assert.Equal(t, "catalog", registry, "the resolved registry is passed to fetch")
			return stubModule(), nil
		},
	}

	require.NoError(t, o.run(context.Background(), cli, &out))

	var printed v1beta1.Application
	require.NoError(t, yaml.Unmarshal(out.Bytes(), &printed))
	var props map[string]string
	require.NoError(t, json.Unmarshal(printed.Spec.Components[0].Properties.Raw, &props))
	assert.Equal(t, "catalog", props["registry"], "the manifest pins the resolved registry")
}

// moduleApps returns the deploy Application in the given phase and the owned
// module Application with the given tiers (as spec components, the way the
// render service actually creates them) and their reported tier services.
func moduleApps(phase oamcommon.ApplicationPhase, tiers []string, services []oamcommon.ApplicationComponentStatus) []client.Object {
	deployApp := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "module-s3-deploy", Namespace: velatypes.DefaultKubeVelaNS},
		Status:     oamcommon.AppStatus{Phase: phase},
	}
	comps := make([]oamcommon.ApplicationComponent, 0, len(tiers))
	for _, tier := range tiers {
		comps = append(comps, oamcommon.ApplicationComponent{Name: tier, Type: "k8s-objects"})
	}
	ownedApp := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "module-s3", Namespace: velatypes.DefaultKubeVelaNS},
		Spec:       v1beta1.ApplicationSpec{Components: comps},
		Status:     oamcommon.AppStatus{Services: services},
	}
	return []client.Object{deployApp, ownedApp}
}

// moduleTiers is the s3 stub module's install tier names, in render order.
var moduleTiers = []string{"s3-xrd", "s3-v1-comp", "s3-v1-defs"}

func healthyTierServices() []oamcommon.ApplicationComponentStatus {
	return []oamcommon.ApplicationComponentStatus{
		{Name: "s3-xrd", Healthy: true, Message: "Established"},
		{Name: "s3-v1-comp", Healthy: true},
		{Name: "s3-v1-defs", Healthy: true},
	}
}

func TestFirstUnhealthyTier(t *testing.T) {
	testCases := map[string]struct {
		services    []oamcommon.ApplicationComponentStatus
		wantTier    string
		wantMessage string
	}{
		"first tier not reported yet": {
			services:    nil,
			wantTier:    "s3-xrd",
			wantMessage: "not reported yet",
		},
		"second tier unhealthy": {
			services: []oamcommon.ApplicationComponentStatus{
				{Name: "s3-xrd", Healthy: true},
				{Name: "s3-v1-comp", Healthy: false, Message: "composition not ready"},
			},
			wantTier:    "s3-v1-comp",
			wantMessage: "composition not ready",
		},
		"all healthy": {
			services: healthyTierServices(),
			wantTier: "",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tier, message := firstUnhealthyTier([]string{"s3-xrd", "s3-v1-comp", "s3-v1-defs"}, tc.services)
			assert.Equal(t, tc.wantTier, tier)
			if tc.wantMessage != "" {
				assert.Contains(t, message, tc.wantMessage)
			}
		})
	}
}

// TestWaitForModuleReportsResolvedVersion asserts the success message reports
// the concrete version recorded on the owned app's annotation -- including
// when a "latest" request (no --version) resolved to a specific tag, since
// the annotation always carries the resolved tag regardless of the request.
func TestWaitForModuleReportsResolvedVersion(t *testing.T) {
	deployApp := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "module-s3-deploy", Namespace: velatypes.DefaultKubeVelaNS},
		Status:     oamcommon.AppStatus{Phase: oamcommon.ApplicationRunning},
	}
	comps := make([]oamcommon.ApplicationComponent, 0, len(moduleTiers))
	for _, tier := range moduleTiers {
		comps = append(comps, oamcommon.ApplicationComponent{Name: tier, Type: "k8s-objects"})
	}
	ownedApp := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "module-s3",
			Namespace:   velatypes.DefaultKubeVelaNS,
			Annotations: map[string]string{velatypes.AnnoDefinitionModuleVersion: "1.2.0"},
		},
		Spec:   v1beta1.ApplicationSpec{Components: comps},
		Status: oamcommon.AppStatus{Services: healthyTierServices()},
	}
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(deployApp, ownedApp).Build()
	var out bytes.Buffer
	o := &moduleDeployOptions{module: "s3", namespace: velatypes.DefaultKubeVelaNS, timeout: time.Second, pollInterval: time.Millisecond}

	err := o.waitForModule(context.Background(), cli, &out)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "1.2.0")
}

func TestWaitForModuleBecomesHealthy(t *testing.T) {
	pending := []oamcommon.ApplicationComponentStatus{
		{Name: "s3-xrd", Healthy: true, Message: "Established"},
		{Name: "s3-v1-comp", Healthy: false, Message: "waiting for s3-xrd"},
	}
	gets := 0
	watched := fake.NewClientBuilder().WithScheme(common.Scheme).
		WithObjects(moduleApps(oamcommon.ApplicationRunning, moduleTiers, pending)...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if err := c.Get(ctx, key, obj, opts...); err != nil {
					return err
				}
				app, ok := obj.(*v1beta1.Application)
				if !ok || app.Name != "module-s3" {
					return nil
				}
				gets++
				if gets > 2 {
					app.Status.Services = healthyTierServices()
				}
				return nil
			},
		}).Build()
	var out bytes.Buffer
	o := &moduleDeployOptions{module: "s3", namespace: velatypes.DefaultKubeVelaNS, timeout: 2 * time.Second, pollInterval: time.Millisecond}

	err := o.waitForModule(context.Background(), watched, &out)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "waiting for s3-xrd", "the intermediate state is reported")
}

func TestWaitForModuleTimesOut(t *testing.T) {
	stuck := []oamcommon.ApplicationComponentStatus{
		{Name: "s3-xrd", Healthy: true, Message: "Established"},
		{Name: "s3-v1-comp", Healthy: false, Message: "composition not ready"},
	}
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).
		WithObjects(moduleApps(oamcommon.ApplicationRunning, moduleTiers, stuck)...).Build()
	var out bytes.Buffer
	o := &moduleDeployOptions{module: "s3", namespace: velatypes.DefaultKubeVelaNS, timeout: 30 * time.Millisecond, pollInterval: time.Millisecond}

	err := o.waitForModule(context.Background(), cli, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "s3-v1-comp")
	assert.Contains(t, err.Error(), "composition not ready")
}

func TestWaitForModuleStopsOnFailedWorkflow(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).
		WithObjects(moduleApps(oamcommon.ApplicationWorkflowFailed, []string{"s3-xrd"}, nil)...).Build()
	var out bytes.Buffer
	o := &moduleDeployOptions{module: "s3", namespace: velatypes.DefaultKubeVelaNS, timeout: time.Minute, pollInterval: time.Millisecond}

	start := time.Now()
	err := o.waitForModule(context.Background(), cli, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), string(oamcommon.ApplicationWorkflowFailed))
	assert.Less(t, time.Since(start), 5*time.Second, "a terminal phase must not wait out the timeout")
}

// TestWaitForModuleWaitsForOwnedApp confirms the deploy CLI does not assume a
// tier shape before the owned Application exists: with no owned Application
// yet, it reports a rendering state and keeps polling instead of failing or
// treating "no tiers" as "install complete".
func TestWaitForModuleWaitsForOwnedApp(t *testing.T) {
	deployApp := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "module-s3-deploy", Namespace: velatypes.DefaultKubeVelaNS},
		Status:     oamcommon.AppStatus{Phase: oamcommon.ApplicationRendering},
	}
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(deployApp).Build()
	var out bytes.Buffer
	o := &moduleDeployOptions{module: "s3", namespace: velatypes.DefaultKubeVelaNS, timeout: 30 * time.Millisecond, pollInterval: time.Millisecond}

	err := o.waitForModule(context.Background(), cli, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "was not created")
	assert.Contains(t, out.String(), "Rendering module")
}
