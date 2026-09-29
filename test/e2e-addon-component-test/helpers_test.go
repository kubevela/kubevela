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
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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
