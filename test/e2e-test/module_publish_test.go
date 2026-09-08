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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	veltypes "github.com/oam-dev/kubevela/apis/types"
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
)

// The module name, version, and API line are fixed by the fixture at
// testdata/module/e2e-widget (module.name = "e2e-widget", version = "1.0.0",
// one enabled line "v1"). They are used verbatim below rather than derived,
// mirroring how the CLI itself names things: the deploy Application is
// "module-e2e-widget-deploy" and the controller's owned Application is
// "module-e2e-widget" (see references/cli/module-deploy.go).
const (
	modulePublishFixtureRelPath = "test/e2e-test/testdata/module/e2e-widget"
	modulePublishModuleName     = "e2e-widget"
	modulePublishModuleVersion  = "1.0.0"
	modulePublishRegistryName   = "e2e-oci"

	// ociRegistryNodePort is the fixed NodePort testdata/module/registry.yaml
	// exposes the in-cluster registry on. A NodePort plus a node's own
	// InternalIP resolves from both this test process and the controller's
	// pod with no DNS tricks and no privileges. That "resolves from both"
	// property is required, not a convenience: "vela module deploy" fetches
	// the module client-side, in the CLI process itself, before it applies
	// anything to the cluster (references/cli/module-deploy.go calls
	// FetchModule before apply.NewAPIApplicator(...).Apply), so a registry
	// URL that only resolves inside the cluster -- such as the Service DNS
	// name oci-registry.default.svc.cluster.local -- makes the CLI itself
	// fail before the controller is ever involved. One URL, reachable from
	// both sides, is the only shape that works.
	ociRegistryNodePort = 30500

	// moduleE2ERegistryURLEnv is an escape hatch for environments where a kind
	// node's IP is not routable from wherever this test runs (for example, a
	// devcontainer on a different docker network than the kind node: the node
	// sits at an address like 172.18.0.2 on the host's docker network, which
	// is unreachable from inside such a container). CI leaves this unset and
	// derives the URL from a node's InternalIP and ociRegistryNodePort; where
	// that derivation would not resolve, the operator sets this to a URL that
	// resolves for both the CLI and the controller.
	moduleE2ERegistryURLEnv = "MODULE_E2E_REGISTRY_URL"

	moduleDeployAppNameE2E = "module-" + modulePublishModuleName + "-deploy"
	moduleOwnedAppNameE2E  = "module-" + modulePublishModuleName
)

var _ = Describe("Module publish and deploy", func() {
	It("packages a module, publishes it to an in-cluster OCI registry, and deploys it through the controller", func() {
		ctx := context.Background()
		repoRoot := modulePublishRepoRoot()
		testNamespace := randomNamespaceName("module-e2e")

		By("Creating the test namespace: " + testNamespace)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
		})

		By("Applying the in-cluster OCI registry manifest and waiting for it to be Available")
		Expect(applyManifestFile(ctx, k8sClient, "testdata/module/registry.yaml")).Should(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "oci-registry", Namespace: "default"}})
			_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "oci-registry", Namespace: "default"}})
		})
		waitForOCIRegistryDeploymentAvailable(ctx)

		By("Deriving one registry URL that resolves for both the CLI and the controller")
		registryURL := moduleE2ERegistryURL(ctx)
		registryBase, err := registryHTTPBase(registryURL)
		Expect(err).ShouldNot(HaveOccurred(), "registry URL: %s", registryURL)
		waitForOCIRegistryReachable(registryBase + "/v2/")

		By("Publishing the module with the positional-reference form (no registry entry needed)")
		out := runVelaCommandSucceed(repoRoot, "module", "publish", modulePublishFixtureRelPath, registryURL)
		Expect(out).Should(ContainSubstring("modules/e2e-widget:1.0.0"))

		By("Asserting the artifact is really in the registry over HTTP")
		verifyPublishedArtifact(registryBase)

		By("Asserting immutability: republishing without --force fails, naming the version bump")
		out, err = runVelaCommand(repoRoot, "module", "publish", modulePublishFixtureRelPath, registryURL)
		Expect(err).Should(HaveOccurred(), "expected republish without --force to fail\noutput:\n%s", out)
		Expect(out).Should(ContainSubstring("bump version"), "expected the error to name the version bump\noutput:\n%s", out)

		By("Republishing with --force succeeds")
		out = runVelaCommandSucceed(repoRoot, "module", "publish", modulePublishFixtureRelPath, registryURL, "--force")
		Expect(out).Should(ContainSubstring("modules/e2e-widget:1.0.0"))

		By("Registering the registry URL, which resolves for both the CLI and the controller")
		out = runVelaCommandSucceed(repoRoot, "module", "registry", "add", modulePublishRegistryName, registryURL, "--type", "oci")
		DeferCleanup(func() {
			_, _ = runVelaCommand(repoRoot, "module", "registry", "delete", modulePublishRegistryName)
		})
		Expect(out).Should(ContainSubstring(modulePublishRegistryName))

		By("Deploying through the controller: fetching the published artifact and rendering its tiers")
		deployCtx, cancel := context.WithTimeout(ctx, 6*time.Minute)
		defer cancel()
		out, err = runVelaCommandContext(deployCtx, repoRoot, "module", "deploy", modulePublishModuleName,
			"--registry", modulePublishRegistryName, "-n", testNamespace)
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: moduleDeployAppNameE2E, Namespace: testNamespace}})
		})
		Expect(err).Should(Succeed(), "vela module deploy failed\noutput:\n%s", out)

		By("Asserting the module's ComponentDefinition exists in the test namespace")
		Eventually(func(g Gomega) {
			var cdList v1beta1.ComponentDefinitionList
			g.Expect(k8sClient.List(ctx, &cdList, client.InNamespace(testNamespace), client.MatchingLabels{
				veltypes.LabelDefinitionModule: modulePublishModuleName,
			})).Should(Succeed())
			g.Expect(cdList.Items).ShouldNot(BeEmpty(), "expected a ComponentDefinition from module %q in namespace %q", modulePublishModuleName, testNamespace)
		}, 30*time.Second, 2*time.Second).Should(Succeed())

		By("Asserting the owned Application reports every tier healthy")
		Eventually(func(g Gomega) {
			var owned v1beta1.Application
			g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: moduleOwnedAppNameE2E, Namespace: testNamespace}, &owned)).Should(Succeed())
			g.Expect(owned.Status.Services).ShouldNot(BeEmpty())
			for _, svc := range owned.Status.Services {
				g.Expect(svc.Healthy).Should(BeTrue(), "tier %q is not healthy: %s", svc.Name, svc.Message)
			}
		}, 30*time.Second, 2*time.Second).Should(Succeed())
	})
})

// modulePublishRepoRoot returns the repository root, computed from this
// file's own path rather than the process working directory: ginkgo runs the
// test binary with its working directory set to the package under test
// (test/e2e-test), so relative paths written against the repo root (matching
// how the CI job invokes "bin/vela" and names the module fixture) need to be
// resolved explicitly.
func modulePublishRepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(file, "../../..")
}

// runVelaCommand runs the e2e job's built vela binary (bin/vela, built from
// this source so the module render provider is present) with its working
// directory set to the repository root, so relative arguments such as the
// module fixture path resolve the same way they would from a shell at the
// repo root.
func runVelaCommand(repoRoot string, args ...string) (string, error) {
	return runVelaCommandContext(context.Background(), repoRoot, args...)
}

// runVelaCommandContext is runVelaCommand with a caller-supplied context, used
// to bound the "module deploy" invocation, which waits for the module to
// become healthy inside the CLI process itself.
func runVelaCommandContext(ctx context.Context, repoRoot string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, filepath.Join(repoRoot, "bin", "vela"), args...)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	GinkgoWriter.Printf("$ bin/vela %v\n%s\n", args, string(out))
	return string(out), err
}

// runVelaCommandSucceed runs bin/vela and fails the spec with the command's
// combined output on error, rather than the bare "exit status 1" that a plain
// Expect(err).Should(Succeed()) would print.
func runVelaCommandSucceed(repoRoot string, args ...string) string {
	out, err := runVelaCommand(repoRoot, args...)
	Expect(err).Should(Succeed(), "vela %v failed\noutput:\n%s", args, out)
	return out
}

// waitForOCIRegistryDeploymentAvailable waits for the oci-registry Deployment
// applied from testdata/module/registry.yaml to report Available, mirroring
// waitForAuthDeploymentsReady in auth_registry_helpers_test.go.
func waitForOCIRegistryDeploymentAvailable(ctx context.Context) {
	Eventually(func() bool {
		d := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: "default", Name: "oci-registry"}, d); err != nil {
			return false
		}
		for _, c := range d.Status.Conditions {
			if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
				return true
			}
		}
		return false
	}, 120*time.Second, 2*time.Second).Should(BeTrue(), "oci-registry Deployment did not become Available")
}

// moduleE2ERegistryURL returns the one registry URL used for both the
// publish/register CLI calls and (indirectly, once registered) the
// controller's fetch. See the comment on ociRegistryNodePort for why one URL
// covering both sides is required rather than a convenience: "vela module
// deploy" validates and fetches the module client-side before applying
// anything, so a registry entry that only resolves inside the cluster breaks
// the CLI itself, not just the controller.
//
// MODULE_E2E_REGISTRY_URL, when set, is used verbatim and skips the node-IP
// derivation entirely -- for an environment (like this devcontainer) where
// the kind node's IP is not routable from wherever this test runs. CI leaves
// it unset and derives the URL from testdata/module/registry.yaml's NodePort
// (ociRegistryNodePort) and a node's own InternalIP, which is routable from
// both the CI runner and every pod.
func moduleE2ERegistryURL(ctx context.Context) string {
	if v := os.Getenv(moduleE2ERegistryURLEnv); v != "" {
		return v
	}
	var nodes corev1.NodeList
	Expect(k8sClient.List(ctx, &nodes)).Should(Succeed())
	Expect(nodes.Items).ShouldNot(BeEmpty(), "no nodes found to derive the registry URL from")
	for _, addr := range nodes.Items[0].Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return fmt.Sprintf("http://%s:%d/modules", addr.Address, ociRegistryNodePort)
		}
	}
	Fail(fmt.Sprintf("node %q has no status.addresses entry of type InternalIP; "+
		"set %s to work around this", nodes.Items[0].Name, moduleE2ERegistryURLEnv))
	return ""
}

// registryHTTPBase returns the scheme and host ("http://host:port") of
// registryURL, dropping any path such as the "/modules" OCI push/pull
// prefix, so callers can build the registry's plain HTTP API endpoints
// (/v2/...) regardless of that prefix.
func registryHTTPBase(registryURL string) (string, error) {
	u, err := url.Parse(registryURL)
	if err != nil {
		return "", fmt.Errorf("parse registry URL %q: %w", registryURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("registry URL %q has no scheme or host", registryURL)
	}
	return u.Scheme + "://" + u.Host, nil
}

// waitForOCIRegistryReachable polls pingURL (the registry's own /v2/
// endpoint) until it answers 200, so the spec refuses to publish into a
// registry that is not up yet.
func waitForOCIRegistryReachable(pingURL string) {
	Eventually(func() error {
		resp, err := http.Get(pingURL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status %s", resp.Status)
		}
		return nil
	}, 30*time.Second, time.Second).Should(Succeed(), "registry at %s is not answering", pingURL)
}

// fetchRegistryJSON GETs url from the registry, optionally setting an Accept
// header, and decodes the JSON response body. It returns the raw body
// alongside any decode error so a failing assertion can print exactly what the
// registry sent.
func fetchRegistryJSON(url, accept string) (map[string]interface{}, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, string(body), fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, string(body), fmt.Errorf("decode %s: %w", url, err)
	}
	return out, string(body), nil
}

// verifyPublishedArtifact asserts the published module is really in the
// registry: its tag is listed, and the OCI manifest carries the module and
// lines annotations PackageModule stamped on it (pkg/module/publish.go). base
// is the registry's scheme+host, from registryHTTPBase.
func verifyPublishedArtifact(base string) {
	tagsResp, tagsBody, err := fetchRegistryJSON(
		base+"/v2/modules/e2e-widget/tags/list", "")
	Expect(err).ShouldNot(HaveOccurred(), "tags/list body:\n%s", tagsBody)
	rawTags, _ := tagsResp["tags"].([]interface{})
	tags := make([]string, 0, len(rawTags))
	for _, t := range rawTags {
		tags = append(tags, fmt.Sprintf("%v", t))
	}
	Expect(tags).Should(ContainElement(modulePublishModuleVersion), "tags/list body:\n%s", tagsBody)

	manifestResp, manifestBody, err := fetchRegistryJSON(
		base+"/v2/modules/e2e-widget/manifests/1.0.0",
		"application/vnd.oci.image.manifest.v1+json")
	Expect(err).ShouldNot(HaveOccurred(), "manifest body:\n%s", manifestBody)
	annotations, _ := manifestResp["annotations"].(map[string]interface{})
	Expect(annotations).Should(HaveKeyWithValue(pkgmodule.AnnotationModule, modulePublishModuleName), "manifest body:\n%s", manifestBody)
	Expect(annotations).Should(HaveKeyWithValue(pkgmodule.AnnotationLines, "v1"), "manifest body:\n%s", manifestBody)
}
