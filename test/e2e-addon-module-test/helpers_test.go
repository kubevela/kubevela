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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	veltypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	regcomponent "github.com/oam-dev/kubevela/pkg/registry/component"
)

// systemNS is where every addon-<name> and module-<name> Application, and
// every module definition installed without a namespace, lands.
var systemNS = veltypes.DefaultKubeVelaNS

const (
	// testNS is where the suite's user Applications and consumers live, the
	// same namespace the manual scenarios use.
	testNS = "default"

	moduleRegistryName = "e2e-modules"
	addonRegistryName  = "e2e-addons"

	// The NodePorts testdata/registry.yaml exposes the two registries on. A
	// NodePort plus a node's InternalIP resolves from both this test process
	// and the vela-core pod. That property is required, not a convenience:
	// "vela module publish/deploy" and "vela addon push" talk to the registry
	// from the CLI process, while the controller fetches from its pod, and a
	// registry entry has exactly one URL.
	moduleRegistryNodePort = 30500
	addonRegistryNodePort  = 30501

	// Overrides for the URL stored in each registry record (what the
	// controller pod fetches from). The module suite honours the first one;
	// this suite adds the second for ChartMuseum.
	moduleRegistryURLEnv = "MODULE_E2E_REGISTRY_URL"
	addonRegistryURLEnv  = "ADDON_E2E_REGISTRY_URL"
	// Overrides for the URL this process uses (the CLI's pushes and the HTTP
	// checks). Only needed when neither the stored URL nor the k3d port
	// mapping on 127.0.0.1 is reachable from where the tests run.
	moduleRegistryHostURLEnv = "MODULE_E2E_REGISTRY_HOST_URL"
	addonRegistryHostURLEnv  = "ADDON_E2E_REGISTRY_HOST_URL"

	// velaCoreDeployment is the controller Deployment makefiles/e2e.mk installs
	// (Helm release "kubevela" of charts/vela-core). Two scenarios restart it
	// and one toggles its feature gates.
	velaCoreDeployment = "kubevela-vela-core"

	pollInterval = 3 * time.Second
	// shortWait covers admission plus one workflow run of a small Application.
	shortWait = 2 * time.Minute
	// reconcileWait covers one reSyncPeriod (1m in e2e.mk) plus the health
	// check render it triggers, with margin.
	reconcileWait = 4 * time.Minute
	// installWait covers the full three-level install, including a line tier
	// that fails once with "no matches for kind" before the CRD is served.
	installWait = 6 * time.Minute
	// retryWait covers a failing workflow converging through its back-off once
	// the object it waited for is gone.
	retryWait = 8 * time.Minute
)

// The two registries, resolved once in the suite's BeforeAll.
var (
	moduleRegistry registryEndpoints
	addonRegistry  registryEndpoints
)

var (
	widgetGVK      = schema.GroupVersionKind{Group: "kit.example.com", Version: "v1alpha1", Kind: "Widget"}
	widgetClassGVK = schema.GroupVersionKind{Group: "kit.example.com", Version: "v1alpha1", Kind: "WidgetClass"}
)

// --- paths and the CLI ---

// repoRoot is computed from this file's own path rather than the process
// working directory: ginkgo runs the test binary from the package directory,
// while bin/vela and the fixture paths are spelled against the repository
// root.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(file, "../../..")
}

// testdataPath resolves a path below test/e2e-addon-module-test/testdata.
func testdataPath(rel ...string) string {
	return filepath.Join(append([]string{repoRoot(), "test", "e2e-addon-module-test", "testdata"}, rel...)...)
}

// runVela runs bin/vela (built from this source, so "vela module" and the
// addon component wiring exist) with its working directory at the repo root.
func runVela(args ...string) (string, error) {
	return runVelaContext(context.Background(), args...)
}

func runVelaContext(ctx context.Context, args ...string) (string, error) {
	root := repoRoot()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "bin", "vela"), args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	GinkgoWriter.Printf("$ bin/vela %v\n%s\n", args, string(out))
	return string(out), err
}

// runVelaSucceed fails the spec with the command's combined output on error,
// rather than the bare "exit status 1".
func runVelaSucceed(args ...string) string {
	out, err := runVela(args...)
	Expect(err).Should(Succeed(), "vela %v failed\noutput:\n%s", args, out)
	return out
}

// publishModuleFixture publishes testdata/modules/<dir> to the module
// registry through its host-side URL (the positional reference form, which
// does not read the stored registry record). --force makes a re-run against
// a registry that still holds the tag succeed; the content of a version is
// the same wherever it is published.
func publishModuleFixture(dir string, extra ...string) string {
	args := append([]string{"module", "publish", testdataPath("modules", dir), moduleRegistry.host, "--force"}, extra...)
	return runVelaSucceed(args...)
}

// pushAddonFixture pushes testdata/addons/<dir> to ChartMuseum through its
// host-side URL. "vela addon push" writes a Chart.yaml into the folder it
// packages, so the fixture is copied to a temporary directory first and
// testdata stays as committed. -f lets a version that is already there be
// replaced, which the same-tag republish scenario needs and every other push
// tolerates.
func pushAddonFixture(dir string) string {
	tmp, err := os.MkdirTemp("", "addon-push-")
	Expect(err).ShouldNot(HaveOccurred())
	DeferCleanup(func() { _ = os.RemoveAll(tmp) })
	target := filepath.Join(tmp, dir)
	Expect(copyDir(testdataPath("addons", dir), target)).Should(Succeed())
	return runVelaSucceed("addon", "push", target, addonRegistry.host, "-f")
}

// addAddonRegistry writes the addon registry record (a Helm repository at the
// cluster-side ChartMuseum URL) straight into the vela-addon-registry
// ConfigMap. "vela addon registry add" would do the same, but it first lists
// the repository from this process, which cannot reach a node-IP URL on a Mac
// running k3d. The controller and the webhook read the record the same way
// either way. Adding an existing name overwrites it.
func addAddonRegistry(ctx context.Context) {
	store := regcomponent.NewRegistryDataStore(k8sClient)
	Expect(store.AddRegistry(ctx, regcomponent.Registry{
		Name: addonRegistryName,
		Helm: &regcomponent.HelmSource{URL: addonRegistry.cluster},
	})).Should(Succeed())
	Expect(runVelaSucceed("addon", "registry", "list")).Should(ContainSubstring(addonRegistryName))
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// --- registries ---

// waitForDeploymentAvailable waits for an in-cluster registry Deployment to
// report Available.
func waitForDeploymentAvailable(ctx context.Context, ns, name string) {
	Eventually(func() bool {
		d := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: ns, Name: name}, d); err != nil {
			return false
		}
		for _, c := range d.Status.Conditions {
			if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
				return true
			}
		}
		return false
	}, 3*time.Minute, 2*time.Second).Should(BeTrue(), "Deployment %s/%s did not become Available", ns, name)
}

// registryEndpoints is the pair of URLs one in-cluster registry is reached
// at. A registry record holds exactly one URL and two parties read it: the
// controller pod, which fetches modules and addons from it, and the vela CLI
// on the machine running this suite, which fetches through it for "vela
// module deploy" and "vela addon enable". On a Linux runner (kind, k3d) a
// node's InternalIP is routable from both, so one URL serves. On a Mac the
// k3d node sits on a docker network the host cannot route to; the node
// stays reachable from every pod, and the host reaches the registry only
// through k3d's port mapping on 127.0.0.1. So the record keeps the cluster
// URL, this process publishes and pushes through the host URL, and the few
// specs that need the CLI to read through the record are skipped when the
// host cannot reach it.
type registryEndpoints struct {
	// cluster is stored in the registry record; pods reach it.
	cluster string
	// host is what this process uses for publishing, pushing and HTTP checks.
	host string
	// hostBase is host without its path ("scheme://host:port").
	hostBase string
	// hostReachesCluster reports whether cluster is also reachable from here,
	// which is what the CLI needs to read through the stored record.
	hostReachesCluster bool
}

// resolveRegistryEndpoints derives both URLs of a registry exposed on
// nodePort. The cluster URL is the env override or http://<node
// InternalIP>:<nodePort><path>. The host URL is the cluster URL when this
// process can reach it, else the host override, else the k3d port mapping
// http://127.0.0.1:<nodePort><path>. ping is the path that must answer 200
// for the registry to count as reachable.
func resolveRegistryEndpoints(ctx context.Context, clusterEnv, hostEnv string, nodePort int, path, ping string) registryEndpoints {
	cluster := os.Getenv(clusterEnv)
	if cluster == "" {
		cluster = fmt.Sprintf("http://%s:%d%s", nodeInternalIP(ctx, clusterEnv), nodePort, path)
	}
	ep := registryEndpoints{cluster: cluster}
	if reachable(registryHTTPBase(cluster)+ping, 30*time.Second) {
		ep.host, ep.hostReachesCluster = cluster, true
	} else {
		ep.host = os.Getenv(hostEnv)
		if ep.host == "" {
			ep.host = fmt.Sprintf("http://127.0.0.1:%d%s", nodePort, path)
		}
		GinkgoWriter.Printf("registry %s is not reachable from this process; using %s here (the cluster keeps %s)\n", cluster, ep.host, cluster)
		Expect(reachable(registryHTTPBase(ep.host)+ping, 30*time.Second)).Should(BeTrue(),
			"neither %s nor %s answers %s from this process; set %s (and, if the k3d port mapping is missing, recreate the cluster with -p %d:%d@server:0)",
			cluster, ep.host, ping, hostEnv, nodePort, nodePort)
	}
	ep.hostBase = registryHTTPBase(ep.host)
	return ep
}

// nodeInternalIP returns the first node's InternalIP.
func nodeInternalIP(ctx context.Context, env string) string {
	var nodes corev1.NodeList
	Expect(k8sClient.List(ctx, &nodes)).Should(Succeed())
	Expect(nodes.Items).ShouldNot(BeEmpty(), "no nodes found to derive the registry URL from")
	for _, addr := range nodes.Items[0].Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	Fail(fmt.Sprintf("node %q has no InternalIP address; set %s to work around this", nodes.Items[0].Name, env))
	return ""
}

// probeClient dials with a short timeout: an unroutable node IP fails by
// timing out, and a 30-second default per attempt would hide the fallback.
var probeClient = &http.Client{Timeout: 3 * time.Second}

// reachable reports whether pingURL answers 200 within timeout.
func reachable(pingURL string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		resp, err := probeClient.Get(pingURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Second)
	}
}

// skipUnlessHostReachesCluster skips a spec whose CLI step reads through the
// stored registry record from this process.
func skipUnlessHostReachesCluster(ep registryEndpoints, what string) {
	if !ep.hostReachesCluster {
		Skip(fmt.Sprintf("%s needs the vela CLI on this machine to reach %s, which it cannot (a Mac running k3d); it runs where a node IP is routable, such as a Linux CI runner", what, ep.cluster))
	}
}

// registryHTTPBase returns "scheme://host:port" of a registry URL, dropping
// any path prefix such as "/modules".
func registryHTTPBase(rawURL string) string {
	u, err := url.Parse(rawURL)
	Expect(err).ShouldNot(HaveOccurred(), "registry URL %q", rawURL)
	Expect(u.Scheme).ShouldNot(BeEmpty(), "registry URL %q has no scheme", rawURL)
	Expect(u.Host).ShouldNot(BeEmpty(), "registry URL %q has no host", rawURL)
	return u.Scheme + "://" + u.Host
}

// httpGet returns the body of url as a string, failing on a non-200 status.
func httpGet(rawURL, accept string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return string(body), fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	return string(body), nil
}

// ociTags lists the tags of <repository> in the module registry over its
// plain HTTP distribution API.
func ociTags(base, repository string) []string {
	body, err := httpGet(base+"/v2/"+repository+"/tags/list", "")
	Expect(err).ShouldNot(HaveOccurred(), body)
	var resp struct {
		Tags []string `json:"tags"`
	}
	Expect(json.Unmarshal([]byte(body), &resp)).Should(Succeed(), body)
	return resp.Tags
}

// --- manifests ---

// loadManifestObjects decodes a multi-document YAML file into unstructured
// objects, so a fixture may hold any kind.
func loadManifestObjects(path string) ([]*unstructured.Unstructured, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	decoder := yaml.NewYAMLOrJSONDecoder(bufio.NewReader(f), 4096)
	var objs []*unstructured.Unstructured
	for {
		raw := map[string]interface{}{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				return objs, nil
			}
			return nil, err
		}
		if len(raw) == 0 {
			continue
		}
		objs = append(objs, &unstructured.Unstructured{Object: raw})
	}
}

// applyManifestFile creates every object in the file, ignoring AlreadyExists
// so a re-run does not trip over a fixture a previous run left behind.
func applyManifestFile(ctx context.Context, path string) error {
	objs, err := loadManifestObjects(path)
	if err != nil {
		return err
	}
	for _, obj := range objs {
		if err := k8sClient.Create(ctx, obj); err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create %s %s/%s: %w", obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
		}
	}
	return nil
}

// createEachFromFile creates every object on its own and returns the error
// each one got, keyed by name, for fixtures whose documents are expected to
// be refused individually at admission.
func createEachFromFile(ctx context.Context, path string) map[string]error {
	objs, err := loadManifestObjects(path)
	Expect(err).ShouldNot(HaveOccurred())
	results := map[string]error{}
	for _, obj := range objs {
		results[obj.GetName()] = k8sClient.Create(ctx, obj)
	}
	return results
}

// deleteManifestFile deletes every object in the file, ignoring NotFound.
func deleteManifestFile(ctx context.Context, path string) {
	objs, err := loadManifestObjects(path)
	Expect(err).ShouldNot(HaveOccurred())
	for _, obj := range objs {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).Should(Succeed())
	}
}

// --- Applications ---

// rawExtension wraps a JSON-marshallable value as component properties.
func rawExtension(v interface{}) *k8sruntime.RawExtension {
	raw, err := json.Marshal(v)
	Expect(err).ShouldNot(HaveOccurred())
	return &k8sruntime.RawExtension{Raw: raw}
}

// propertiesOf decodes a component's properties into a map.
func propertiesOf(raw *k8sruntime.RawExtension) map[string]interface{} {
	out := map[string]interface{}{}
	if raw == nil || len(raw.Raw) == 0 {
		return out
	}
	Expect(json.Unmarshal(raw.Raw, &out)).Should(Succeed(), string(raw.Raw))
	return out
}

// addonApplication is the one thing a platform user writes: an Application in
// testNS with a single "type: addon" component named after the addon (the
// addon name defaults to the component name), pinned to version, from the
// suite's addon registry. props, when non-nil, becomes the addon's own
// parameters (its parameter.cue).
func addonApplication(name, addon, version string, props map[string]interface{}) *v1beta1.Application {
	properties := map[string]interface{}{
		"registry": addonRegistryName,
		"version":  version,
	}
	if props != nil {
		properties["properties"] = props
	}
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name:       addon,
				Type:       "addon",
				Properties: rawExtension(properties),
			}},
		},
	}
}

func getApp(ctx context.Context, ns, name string) (*v1beta1.Application, error) {
	app := &v1beta1.Application{}
	err := k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: ns, Name: name}, app)
	return app, err
}

func mustGetApp(ctx context.Context, ns, name string) *v1beta1.Application {
	app, err := getApp(ctx, ns, name)
	Expect(err).ShouldNot(HaveOccurred(), "Application %s/%s", ns, name)
	return app
}

// getAppG is mustGetApp for use inside a polled function: it asserts through
// the Gomega that Eventually or Consistently passes in, so a NotFound (for
// example an Application the controller has not created yet) is retried
// instead of aborting the spec. Outside a polled function, use mustGetApp.
func getAppG(g Gomega, ctx context.Context, ns, name string) *v1beta1.Application {
	app, err := getApp(ctx, ns, name)
	g.Expect(err).ShouldNot(HaveOccurred(), "Application %s/%s", ns, name)
	return app
}

// deleteApp deletes an Application, ignoring NotFound.
func deleteApp(ctx context.Context, ns, name string) {
	Expect(client.IgnoreNotFound(k8sClient.Delete(ctx,
		&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}))).Should(Succeed())
}

// appStatusText joins everything an Application reports about itself: the
// phase, the workflow message, every step and sub-step message, and every
// service's message. A render failure surfaces on a step; a health-check
// failure surfaces on a service; a reader of this string sees both.
func appStatusText(app *v1beta1.Application) string {
	parts := []string{"phase=" + string(app.Status.Phase)}
	if app.Status.Workflow != nil {
		parts = append(parts, app.Status.Workflow.Message)
		for _, step := range app.Status.Workflow.Steps {
			parts = append(parts, step.Message)
			for _, sub := range step.SubStepsStatus {
				parts = append(parts, sub.Message)
			}
		}
	}
	for _, svc := range app.Status.Services {
		parts = append(parts, fmt.Sprintf("service %s healthy=%t %s", svc.Name, svc.Healthy, svc.Message))
	}
	// A reconcile that fails before the workflow (a parse error) only leaves a
	// condition behind.
	for _, cond := range app.Status.Conditions {
		if cond.Message != "" {
			parts = append(parts, fmt.Sprintf("condition %s=%s %s", cond.Type, cond.Status, cond.Message))
		}
	}
	return strings.Join(parts, "\n")
}

// waitAppRunning waits for the Application to reach the running phase.
func waitAppRunning(ctx context.Context, ns, name string, timeout time.Duration) {
	Eventually(func(g Gomega) {
		app, err := getApp(ctx, ns, name)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning), appStatusText(app))
	}, timeout, pollInterval).Should(Succeed(), "Application %s/%s did not reach running", ns, name)
}

// waitAppStatusContains waits until the Application's status text mentions
// substr, which is how render and health-check failures are observed.
func waitAppStatusContains(ctx context.Context, ns, name, substr string, timeout time.Duration) {
	Eventually(func(g Gomega) string {
		app, err := getApp(ctx, ns, name)
		g.Expect(err).ShouldNot(HaveOccurred())
		return appStatusText(app)
	}, timeout, pollInterval).Should(ContainSubstring(substr), "Application %s/%s never reported %q", ns, name, substr)
}

// waitGone waits until the object is NotFound.
func waitGone(ctx context.Context, obj client.Object, timeout time.Duration) {
	key := client.ObjectKeyFromObject(obj)
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, obj.DeepCopyObject().(client.Object))
		return k8serrors.IsNotFound(err)
	}, timeout, pollInterval).Should(BeTrue(), "%T %s still exists", obj, key)
}

func waitAppGone(ctx context.Context, ns, name string, timeout time.Duration) {
	waitGone(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}, timeout)
}

// isNotFound reports whether getting the object answers NotFound right now.
func isNotFound(ctx context.Context, obj client.Object) bool {
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	return k8serrors.IsNotFound(err)
}

// updateApp applies mutate to the Application and writes it back, retrying on
// conflicts with the controller's own status writes.
func updateApp(ctx context.Context, ns, name string, mutate func(app *v1beta1.Application)) {
	Eventually(func() error {
		app, err := getApp(ctx, ns, name)
		if err != nil {
			return err
		}
		mutate(app)
		return k8sClient.Update(ctx, app)
	}, 30*time.Second, time.Second).Should(Succeed(), "update Application %s/%s", ns, name)
}

// setAddonVersion changes the pinned version of the first component of an
// addon Application, which is how a user upgrades or rolls back.
func setAddonVersion(ctx context.Context, name, version string) {
	updateApp(ctx, testNS, name, func(app *v1beta1.Application) {
		props := propertiesOf(app.Spec.Components[0].Properties)
		props["version"] = version
		app.Spec.Components[0].Properties = rawExtension(props)
	})
}

// annotateApp sets (or, with an empty value, removes) one annotation.
func annotateApp(ctx context.Context, ns, name, key, value string) {
	updateApp(ctx, ns, name, func(app *v1beta1.Application) {
		annotations := app.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		if value == "" {
			delete(annotations, key)
		} else {
			annotations[key] = value
		}
		app.SetAnnotations(annotations)
	})
}

// restartWorkflow asks an Application to run its workflow again. "true" is
// one-shot (the annotation is removed once used); a duration recurs.
func restartWorkflow(ctx context.Context, ns, name, value string) {
	annotateApp(ctx, ns, name, oam.AnnotationWorkflowRestart, value)
}

// expectPublishVersionRefused tries to bump publishVersion on an Application
// whose definition is gone and returns admission's refusal. The webhook
// renders every update, annotation-only ones included, so the update never
// reaches the controller. Conflicts with the controller's status writes are
// retried; any other outcome, including success, fails the spec.
func expectPublishVersionRefused(ctx context.Context, ns, name, value string) error {
	var refusal error
	Eventually(func(g Gomega) {
		app, err := getApp(ctx, ns, name)
		g.Expect(err).ShouldNot(HaveOccurred())
		annotations := app.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[oam.AnnotationPublishVersion] = value
		app.SetAnnotations(annotations)
		err = k8sClient.Update(ctx, app)
		g.Expect(k8serrors.IsConflict(err)).Should(BeFalse(), "retry on conflict")
		g.Expect(err).Should(HaveOccurred(), "admission must refuse the update of %s/%s", ns, name)
		refusal = err
	}, 30*time.Second, time.Second).Should(Succeed())
	return refusal
}

// expectConsumerOfRemovedDefinition checks what happens to an Application in
// testNS whose definition a module upgrade or rollback removed:
//   - admission refuses any update of it, here a publishVersion bump;
//   - the controller re-parses it from the live definitions on its next
//     reconcile, fails, and reports the definition with phase rendering;
//   - no new revision is created, and its applied workload (if widget names
//     one) is left in place.
func expectConsumerOfRemovedDefinition(ctx context.Context, name, definition, widget, publishVersion string) {
	before := mustGetApp(ctx, testNS, name)
	err := expectPublishVersionRefused(ctx, testNS, name, publishVersion)
	Expect(err.Error()).Should(SatisfyAll(ContainSubstring(`"`+definition+`"`), ContainSubstring("not found")), name)

	waitAppStatusContains(ctx, testNS, name, definition, reconcileWait)
	after := mustGetApp(ctx, testNS, name)
	Expect(after.Status.Phase).Should(Equal(common.ApplicationRendering), name)
	Expect(after.Annotations).ShouldNot(HaveKeyWithValue(oam.AnnotationPublishVersion, publishVersion), name)
	Expect(after.Status.LatestRevision).Should(Equal(before.Status.LatestRevision), "%s: no new revision", name)
	if widget != "" {
		_, err := getUnstructured(ctx, widgetGVK, testNS, widget)
		Expect(err).ShouldNot(HaveOccurred(), "nothing deletes a consumer's already-applied objects")
	}
}

// bumpPublishVersion forces a new revision of an Application without changing
// its spec, so its components are rendered again.
func bumpPublishVersion(ctx context.Context, ns, name, value string) {
	annotateApp(ctx, ns, name, oam.AnnotationPublishVersion, value)
}

// componentNames lists an Application's component names in spec order.
// policyNames lists an Application's policy names in spec order.
func policyNames(app *v1beta1.Application) []string {
	names := make([]string, 0, len(app.Spec.Policies))
	for _, p := range app.Spec.Policies {
		names = append(names, p.Name)
	}
	return names
}

func componentNames(app *v1beta1.Application) []string {
	names := make([]string, 0, len(app.Spec.Components))
	for _, c := range app.Spec.Components {
		names = append(names, c.Name)
	}
	return names
}

func findComponent(app *v1beta1.Application, name string) *common.ApplicationComponent {
	for i := range app.Spec.Components {
		if app.Spec.Components[i].Name == name {
			return &app.Spec.Components[i]
		}
	}
	return nil
}

func findService(app *v1beta1.Application, name string) *common.ApplicationComponentStatus {
	for i := range app.Status.Services {
		if app.Status.Services[i].Name == name {
			return &app.Status.Services[i]
		}
	}
	return nil
}

// --- definitions and objects ---

// moduleDefinitionNames lists the ComponentDefinitions and TraitDefinitions in
// ns that carry the module label of module.
func moduleDefinitionNames(ctx context.Context, ns, module string) []string {
	var names []string
	var cds v1beta1.ComponentDefinitionList
	Expect(k8sClient.List(ctx, &cds, client.InNamespace(ns), client.MatchingLabels{veltypes.LabelDefinitionModule: module})).Should(Succeed())
	for _, cd := range cds.Items {
		names = append(names, cd.Name)
	}
	var tds v1beta1.TraitDefinitionList
	Expect(k8sClient.List(ctx, &tds, client.InNamespace(ns), client.MatchingLabels{veltypes.LabelDefinitionModule: module})).Should(Succeed())
	for _, td := range tds.Items {
		names = append(names, td.Name)
	}
	return names
}

func getComponentDefinition(ctx context.Context, ns, name string) (*v1beta1.ComponentDefinition, error) {
	cd := &v1beta1.ComponentDefinition{}
	return cd, k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: ns, Name: name}, cd)
}

func getTraitDefinition(ctx context.Context, ns, name string) (*v1beta1.TraitDefinition, error) {
	td := &v1beta1.TraitDefinition{}
	return td, k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: ns, Name: name}, td)
}

func getConfigMap(ctx context.Context, ns, name string) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	return cm, k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: ns, Name: name}, cm)
}

func componentDefinitionObj(ns, name string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
}

func traitDefinitionObj(ns, name string) *v1beta1.TraitDefinition {
	return &v1beta1.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
}

func configMapObj(ns, name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
}

// getUnstructured fetches a custom resource of the module CRDs (Widget,
// WidgetClass, Gadget), which have no Go types.
func getUnstructured(ctx context.Context, gvk schema.GroupVersionKind, ns, name string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	return obj, k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: ns, Name: name}, obj)
}

func unstructuredObj(gvk schema.GroupVersionKind, ns, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(ns)
	obj.SetName(name)
	return obj
}

// nestedString reads a dotted path of an unstructured object as a string
// (numbers are formatted), "" when absent.
func nestedString(obj *unstructured.Unstructured, fields ...string) string {
	v, found, err := unstructured.NestedFieldNoCopy(obj.Object, fields...)
	if err != nil || !found || v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// --- vela-core ---

func getVelaCore(ctx context.Context) *appsv1.Deployment {
	d := &appsv1.Deployment{}
	Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: systemNS, Name: velaCoreDeployment}, d)).Should(Succeed(),
		"the vela-core Deployment %s/%s must exist (installed by makefiles/e2e.mk as Helm release kubevela)", systemNS, velaCoreDeployment)
	return d
}

// waitVelaCoreRolledOut waits for the controller Deployment to finish a
// rollout and then for its admission webhook to answer again, since the
// webhook runs in the same pod and Application writes fail while it is down.
func waitVelaCoreRolledOut(ctx context.Context) {
	Eventually(func(g Gomega) {
		d := getVelaCore(ctx)
		replicas := int32(1)
		if d.Spec.Replicas != nil {
			replicas = *d.Spec.Replicas
		}
		g.Expect(d.Status.ObservedGeneration).Should(BeNumerically(">=", d.Generation))
		g.Expect(d.Status.UpdatedReplicas).Should(Equal(replicas))
		g.Expect(d.Status.Replicas).Should(Equal(replicas))
		g.Expect(d.Status.AvailableReplicas).Should(Equal(replicas))
		g.Expect(d.Status.UnavailableReplicas).Should(BeZero())
	}, 5*time.Minute, pollInterval).Should(Succeed(), "vela-core did not finish rolling out")

	probe := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: randomName("webhook-probe"), Namespace: testNS},
		Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{
			Name:       "probe",
			Type:       "k8s-objects",
			Properties: rawExtension(map[string]interface{}{"objects": []interface{}{}}),
		}}},
	}
	Eventually(func() error {
		return k8sClient.Create(ctx, probe.DeepCopy(), client.DryRunAll)
	}, 3*time.Minute, pollInterval).Should(Succeed(), "the admission webhook did not come back after the vela-core rollout")
}

// restartVelaCore restarts the controller the way "kubectl rollout restart"
// does, which empties every in-process cache (the pinned addon render cache
// among them), and waits for the rollout.
func restartVelaCore(ctx context.Context) {
	Eventually(func() error {
		d := getVelaCore(ctx)
		if d.Spec.Template.Annotations == nil {
			d.Spec.Template.Annotations = map[string]string{}
		}
		d.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
		return k8sClient.Update(ctx, d)
	}, 30*time.Second, time.Second).Should(Succeed())
	waitVelaCoreRolledOut(ctx)
}

// setFeatureGates rewrites the two component feature-gate arguments of the
// controller container (the chart renders one --feature-gates=<Gate>=<bool>
// argument per gate) and waits for the rollout that follows.
func setFeatureGates(ctx context.Context, addon, module bool) {
	Eventually(func() error {
		d := getVelaCore(ctx)
		container := &d.Spec.Template.Spec.Containers[0]
		want := map[string]string{
			"EnableAddonComponent":  fmt.Sprintf("%t", addon),
			"EnableModuleComponent": fmt.Sprintf("%t", module),
		}
		seen := map[string]bool{}
		for i, arg := range container.Args {
			for gate, value := range want {
				prefix := "--feature-gates=" + gate + "="
				if strings.HasPrefix(arg, prefix) {
					container.Args[i] = prefix + value
					seen[gate] = true
				}
			}
		}
		for gate, value := range want {
			if !seen[gate] {
				container.Args = append(container.Args, "--feature-gates="+gate+"="+value)
			}
		}
		return k8sClient.Update(ctx, d)
	}, 30*time.Second, time.Second).Should(Succeed())
	waitVelaCoreRolledOut(ctx)
}

// featureGateArgs returns the controller's EnableAddonComponent and
// EnableModuleComponent argument values as "true"/"false".
func featureGateArgs(ctx context.Context) (addon, module string) {
	for _, arg := range getVelaCore(ctx).Spec.Template.Spec.Containers[0].Args {
		if strings.HasPrefix(arg, "--feature-gates=EnableAddonComponent=") {
			addon = strings.TrimPrefix(arg, "--feature-gates=EnableAddonComponent=")
		}
		if strings.HasPrefix(arg, "--feature-gates=EnableModuleComponent=") {
			module = strings.TrimPrefix(arg, "--feature-gates=EnableModuleComponent=")
		}
	}
	return addon, module
}
