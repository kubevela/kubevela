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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func (s *scenarioScope) prepare(ctx context.Context) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: s.Namespace, Labels: map[string]string{"e2e.oam.dev/addon-module-scope": s.Namespace},
	}}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed(), "scenario namespace must be fresh")
	// Registered before scenario-specific cleanup, so it runs last and also
	// handles failures in a scenario's installation BeforeAll.
	DeferCleanup(func() { s.cleanup(ctx) })
}

func (s *scenarioScope) cleanup(ctx context.Context) {
	namespace := &corev1.Namespace{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: s.Namespace}, namespace)
	if apierrors.IsNotFound(err) {
		return
	}
	Expect(err).To(Succeed())
	if namespace.Labels["e2e.oam.dev/addon-module-scope"] != s.Namespace {
		return // Never clean up a namespace whose creation collided.
	}
	var apps v1beta1.ApplicationList
	err = k8sClient.List(ctx, &apps, client.InNamespace(s.Namespace))
	Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue(), "list scenario Applications: %v", err)
	for i := range apps.Items {
		deleteApp(ctx, s.Namespace, apps.Items[i].Name)
	}
	for _, addon := range fixtureAddons {
		waitAppGone(ctx, systemNS, "addon-"+s.Text(addon), reconcileWait)
	}
	for _, module := range fixtureModules {
		waitAppGone(ctx, systemNS, "module-"+s.Text(module), reconcileWait)
	}
	Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, namespace))).To(Succeed())
	waitGone(ctx, namespace, reconcileWait)
	for _, ns := range []string{s.Text("widget-platform-system"), s.Text("kit-tenant")} {
		// Their owning addon Applications remove these namespaces. Do not
		// forcibly delete an unrelated namespace if addon creation collided.
		waitGone(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, reconcileWait)
	}
}

func (s *scenarioScope) addonApplication(name, addon, version string, props map[string]interface{}) *v1beta1.Application {
	return addonApplication(s.Namespace, name, addon, version, props)
}

func (s *scenarioScope) setAddonVersion(ctx context.Context, name, version string) {
	setAddonVersion(ctx, s.Namespace, name, version)
}

func (s *scenarioScope) expectConsumerOfRemovedDefinition(ctx context.Context, name, definition, widget, publishVersion string) {
	expectConsumerOfRemovedDefinition(ctx, s.Namespace, s.GVK("Widget"), name, definition, widget, publishVersion)
}

func (s *scenarioScope) publishModuleFixture(dir string, extra ...string) string {
	args := append([]string{"module", "publish", s.Path("modules", dir), moduleRegistry.host, "--force"}, extra...)
	return runVelaSucceed(args...)
}

func (s *scenarioScope) pushAddonFixture(dir string) string {
	tmp, err := os.MkdirTemp("", "addon-push-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = os.RemoveAll(tmp) })
	target := filepath.Join(tmp, dir)
	Expect(copyDir(s.Path("addons", dir), target)).To(Succeed())
	return runVelaSucceed("addon", "push", target, addonRegistry.host, "-f")
}

func (s *scenarioScope) uninstall(ctx context.Context, appName, addonAppName string, moduleApps ...string) {
	deleteApp(ctx, s.Namespace, appName)
	waitAppGone(ctx, s.Namespace, appName, shortWait)
	waitAppGone(ctx, systemNS, addonAppName, reconcileWait)
	for _, module := range moduleApps {
		waitAppGone(ctx, systemNS, module, reconcileWait)
		name := ""
		switch module {
		case s.Text("module-widget-kit"):
			name = s.Text("widgets.kit.example.com")
		case s.Text("module-gadget-kit"):
			name = s.Text("gadgets.kit.example.com")
		}
		if name != "" {
			waitGone(ctx, &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: name}}, shortWait)
		}
	}
}

// Publish only what each scenario needs. All addon versions are available
// before rendering starts; module upgrades remain actions in the 08/07 chain.
func (s *scenarioScope) publishInitialFixtures() {
	modules := []string{"widget-kit-1.0.0"}
	addons := []string{"widget-platform-1.0.0"}
	switch s.ID {
	case "01":
		modules = append(modules, "gadget-kit-1.0.0", "probe-kit-1.0.0-a")
		addons = []string{"widget-platform-1.0.0", "widget-platform-1.1.0", "widget-platform-1.2.0", "kit-suite-1.0.0", "kit-suite-2.0.0", "widget-latest-1.0.0", "import-options-1.0.0", "tenant-widgets-1.0.0", "cache-probe-1.0.0-a"}
		for _, version := range []string{"1.0.1", "1.0.2", "1.0.3", "1.0.4", "1.0.5", "1.0.6", "1.0.7"} {
			addons = append(addons, "broken-imports-"+version)
		}
	case "03", "10":
		modules = append(modules, "gadget-kit-1.0.0")
		addons = []string{"kit-suite-1.0.0", "kit-suite-2.0.0"}
	case "05":
		addons = []string{"import-options-1.0.0"}
	case "06":
		addons = []string{"tenant-widgets-1.0.0"}
	case "08":
		addons = []string{"widget-latest-1.0.0", "widget-platform-1.0.0", "widget-platform-1.1.0", "widget-platform-1.2.0"}
	case "09":
		modules, addons = []string{"probe-kit-1.0.0-a"}, []string{"cache-probe-1.0.0-a"}
	case "13", "18":
		modules = append(modules, "gadget-kit-1.0.0")
		if s.ID == "13" {
			addons = append(addons, "kit-suite-1.0.0")
		}
	case "14":
		for _, version := range []string{"1.0.1", "1.0.2", "1.0.3", "1.0.4", "1.0.5", "1.0.6", "1.0.7"} {
			addons = append(addons, "broken-imports-"+version)
		}
	}
	for _, dir := range modules {
		s.publishModuleFixture(dir)
	}
	for _, dir := range addons {
		s.pushAddonFixture(dir)
	}
}
