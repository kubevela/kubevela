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

package controllers_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	veltypes "github.com/oam-dev/kubevela/apis/types"
)

const (
	// chartMuseumNodePort must match testdata/addon/chartmuseum.yaml. It is
	// distinct from the module suite's 30500 so both suites can run against
	// the same cluster without fighting over a port.
	chartMuseumNodePort = 30800

	// addonE2ERegistryURLEnv overrides the derived registry URL for an
	// environment where the node's InternalIP is not routable from wherever
	// this test runs. CI leaves it unset.
	addonE2ERegistryURLEnv = "ADDON_E2E_REGISTRY_URL"
)

// repoRoot resolves the repository root from this file's own path. The Go
// test binary runs with its working directory set to the package under test,
// so relative paths written against the repo root (matching how CI invokes
// bin/vela and names the addon fixtures) need resolving explicitly.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(file, "../../..")
}

// runVelaCommand runs the e2e job's built vela binary with its working
// directory set to the repository root, so relative arguments such as an
// addon fixture path resolve the same way they would from a shell there.
func runVelaCommand(root string, args ...string) (string, error) {
	return runVelaCommandContext(context.Background(), root, args...)
}

// runVelaCommandContext is runVelaCommand with a caller-supplied context, used
// to bound invocations that wait for readiness inside the CLI process itself.
func runVelaCommandContext(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, filepath.Join(root, "bin", "vela"), args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	GinkgoWriter.Printf("$ bin/vela %v\n%s\n", args, string(out))
	return string(out), err
}

// runVelaCommandSucceed runs bin/vela and fails the spec with the command's
// combined output on error, rather than the bare "exit status 1" a plain
// Expect(err).Should(Succeed()) would print.
func runVelaCommandSucceed(root string, args ...string) string {
	out, err := runVelaCommand(root, args...)
	Expect(err).Should(Succeed(), "vela %v failed\noutput:\n%s", args, out)
	return out
}

// addonE2ERegistryURL returns the ChartMuseum URL used for both the push/
// register CLI calls and the controller's own fetch. One URL has to serve
// both sides: `vela addon push` resolves the registry client-side, and the
// controller pulls the same chart in-cluster, so an address that only works
// in one place breaks the other.
func addonE2ERegistryURL(ctx context.Context) string {
	if v := os.Getenv(addonE2ERegistryURLEnv); v != "" {
		return v
	}
	var nodes corev1.NodeList
	Expect(k8sClient.List(ctx, &nodes)).Should(Succeed())
	Expect(nodes.Items).ShouldNot(BeEmpty(), "no nodes found to derive the registry URL from")
	for _, addr := range nodes.Items[0].Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return fmt.Sprintf("http://%s:%d", addr.Address, chartMuseumNodePort)
		}
	}
	Fail(fmt.Sprintf("node %q has no status.addresses entry of type InternalIP; set %s to work around this",
		nodes.Items[0].Name, addonE2ERegistryURLEnv))
	return ""
}

// waitForChartMuseumAvailable waits for the Deployment applied from
// testdata/addon/chartmuseum.yaml to report Available.
func waitForChartMuseumAvailable(ctx context.Context) {
	Eventually(func() bool {
		d := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: "default", Name: "addon-chartmuseum"}, d); err != nil {
			return false
		}
		for _, c := range d.Status.Conditions {
			if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
				return true
			}
		}
		return false
	}, 180*time.Second, 2*time.Second).Should(BeTrue(), "addon-chartmuseum Deployment did not become Available")
}

// waitForChartMuseumReachable polls the repository index until it answers, so
// a spec refuses to push into a registry that is not serving yet. A Deployment
// reporting Available is not the same as a server accepting requests.
func waitForChartMuseumReachable(baseURL string) {
	client := &http.Client{Timeout: 5 * time.Second}
	Eventually(func() error {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/index.yaml", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s/index.yaml answered %s; expected 200", baseURL, resp.Status)
		}
		return nil
	}, 120*time.Second, 2*time.Second).Should(Succeed())
}

// applyManifestFile reads a multi-document YAML/JSON file and creates each
// object, ignoring AlreadyExists so a re-run that left its fixture behind does
// not fail. The kinds handled are exactly the ones this suite's fixtures use.
func applyManifestFile(ctx context.Context, c client.Client, path string) error {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := yaml.NewYAMLOrJSONDecoder(bufio.NewReader(f), 4096)
	for {
		raw := map[string]any{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if len(raw) == 0 {
			continue
		}
		obj, err := decodeKubeObject(raw)
		if err != nil {
			return err
		}
		if err := c.Create(ctx, obj); err != nil && !apierrIsAlreadyExists(err) {
			return err
		}
	}
}

func decodeKubeObject(raw map[string]any) (client.Object, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	kind, _ := raw["kind"].(string)
	var obj client.Object
	switch kind {
	case "Namespace":
		obj = &corev1.Namespace{}
	case "ConfigMap":
		obj = &corev1.ConfigMap{}
	case "Secret":
		obj = &corev1.Secret{}
	case "Service":
		obj = &corev1.Service{}
	case "Deployment":
		obj = &appsv1.Deployment{}
	case "Application":
		obj = &v1beta1.Application{}
	default:
		return nil, fmt.Errorf("decodeKubeObject: unsupported kind %q", kind)
	}
	if err := json.Unmarshal(b, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// apierrIsAlreadyExists locally inlines the kerrors.IsAlreadyExists check to
// avoid bringing in another import just for one branch.
func apierrIsAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already exists")
}

// addonScope is one group's own copy of the fixtures. An addon's identity is
// cluster-wide (one addon-<name> Application in vela-system, one namespace,
// one ComponentDefinition), so groups that install the same addon cannot run
// at once. Each group installs renamed copies instead and can take its own
// Ginkgo worker; the specs inside a group still share its copy and run in
// order.
type addonScope struct {
	echo            string // addon name, ComponentDefinition and workload name
	echoNamespace   string
	config          string
	configNamespace string
	wrappingApp     string
	bundleApp       string
}

func newAddonScope(id string) addonScope {
	echo, config := echoAddonName+"-"+id, configAddonName+"-"+id
	return addonScope{
		echo:            echo,
		echoNamespace:   echoNamespace + "-" + id,
		config:          config,
		configNamespace: config + "-system",
		wrappingApp:     echo + "-addon",
		bundleApp:       bundleAppName + "-" + id,
	}
}

func (s addonScope) echoOwnedApp() string   { return "addon-" + s.echo }
func (s addonScope) configOwnedApp() string { return "addon-" + s.config }

var (
	installScope   = newAddonScope("install")
	reconcileScope = newAddonScope("reconcile")
	uninstallScope = newAddonScope("uninstall")
	ownershipScope = newAddonScope("owner")
	versionScope   = newAddonScope("version")
	bundleScope    = newAddonScope("bundle")
	admissionScope = newAddonScope("admit")
	addonScopes    = []addonScope{installScope, reconcileScope, uninstallScope, ownershipScope, versionScope, bundleScope, admissionScope}
)

// echoImageRepo contains the addon's name but is the container image, so a
// scoped copy must keep it as it is.
const echoImageRepo = "ealen/echo-server"

// writeScopedFixture copies an addon fixture directory to dst, applying the
// old/new renames to every file so the copy installs as a separate addon.
func writeScopedFixture(src, dst string, renames ...string) error {
	replacer := strings.NewReplacer(renames...)
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		text := strings.ReplaceAll(string(data), echoImageRepo, "\x00")
		text = strings.ReplaceAll(replacer.Replace(text), "\x00", echoImageRepo)
		return os.WriteFile(target, []byte(text), 0o600)
	})
}

// setUpSharedRegistry runs once, before any worker starts: it brings up the
// registry, registers it, and pushes the fixtures and every group's copy.
func setUpSharedRegistry(ctx context.Context) string {
	// ADDON_E2E_REGISTRY_URL points the suite at a registry that is already
	// running, and skips bringing up the in-cluster one. It exists because
	// the NodePort route needs the node's address to be reachable from both
	// the CLI on the host and the controller in the cluster. That holds on a
	// CI runner; on Docker Desktop for macOS a connection to a container IP
	// is accepted and then reset, so a developer there runs ChartMuseum on
	// the host and passes its LAN address here instead.
	var url string
	if external := os.Getenv(addonE2ERegistryURLEnv); external != "" {
		By("using the externally provided registry at " + external)
		url = external
	} else {
		By("bringing up the in-cluster ChartMuseum")
		Expect(applyManifestFile(ctx, k8sClient, "testdata/addon/chartmuseum.yaml")).Should(Succeed())
		waitForChartMuseumAvailable(ctx)
		url = addonE2ERegistryURL(ctx)
	}
	waitForChartMuseumReachable(url)

	By("registering it as an addon registry")
	// A previous run that died before teardown leaves the entry behind, and
	// `registry add` refuses an existing name.
	_, _ = runVelaCommand(root, "addon", "registry", "delete", addonRegistryName)
	runVelaCommandSucceed(root, "addon", "registry", "add", addonRegistryName,
		"--type", "helm", "--endpoint", url)

	By("pushing echo-server 1.0.0 and 1.1.0, and config-store 1.0.0")
	runVelaCommandSucceed(root, "addon", "push", echoFixturePath, addonRegistryName, "--use-http", "-f")
	runVelaCommandSucceed(root, "addon", "push", echoUpgradePath, addonRegistryName, "--use-http", "-f")
	runVelaCommandSucceed(root, "addon", "push", configFixturePath, addonRegistryName, "--use-http", "-f")

	By("pushing each group's renamed copy of those fixtures")
	dir, err := os.MkdirTemp("", "kubevela-addon-component-fixtures-")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(dir)
	for _, s := range addonScopes {
		echoRenames := []string{echoAddonName, s.echo, echoNamespace, s.echoNamespace}
		for _, fixture := range []struct {
			src, name string
			renames   []string
		}{
			{echoFixturePath, s.echo + "-" + echoAddonVersion, echoRenames},
			{echoUpgradePath, s.echo + "-" + echoAddonUpgrade, echoRenames},
			{configFixturePath, s.config, []string{configAddonName, s.config}},
		} {
			dst := filepath.Join(dir, fixture.name)
			Expect(writeScopedFixture(filepath.Join(root, fixture.src), dst, fixture.renames...)).To(Succeed())
			runVelaCommandSucceed(root, "addon", "push", dst, addonRegistryName, "--use-http", "-f")
		}
	}
	return url
}

// tearDownSharedRegistry runs once, after every worker has finished.
func tearDownSharedRegistry(ctx context.Context) {
	for _, s := range addonScopes {
		for _, n := range []string{s.wrappingApp, s.bundleApp} {
			_ = k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: veltypes.DefaultKubeVelaNS}})
		}
		_, _ = runVelaCommand(root, "addon", "disable", s.echo)
		_, _ = runVelaCommand(root, "addon", "disable", s.config)
	}
	Eventually(func(g Gomega) {
		for _, s := range addonScopes {
			for _, n := range []string{s.echoOwnedApp(), s.configOwnedApp()} {
				err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: n, Namespace: veltypes.DefaultKubeVelaNS}, &v1beta1.Application{})
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue(),
					"owned Application %s must be gone before the next run of this suite reuses this cluster", n)
			}
		}
	}, waitTimeout, pollPeriod).Should(Succeed())
	_, _ = runVelaCommand(root, "addon", "registry", "delete", addonRegistryName)
	if os.Getenv(addonE2ERegistryURLEnv) == "" {
		_ = k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "addon-chartmuseum", Namespace: "default"}})
		_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "addon-chartmuseum", Namespace: "default"}})
	}
	for _, s := range addonScopes {
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: s.echoNamespace}})
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: s.configNamespace}})
	}
}
