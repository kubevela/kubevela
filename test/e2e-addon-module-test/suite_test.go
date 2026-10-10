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

// Package addonmoduletest is the live-cluster e2e suite for addons installed
// as "type: addon" components that import modules through
// modules/_imports.cue. It follows the shape of test/e2e-module-test: a real
// vela-core (both EnableAddonComponent and EnableModuleComponent on, the
// admission webhook enabled, reSyncPeriod=1m as makefiles/e2e.mk installs it),
// the vela CLI built from this source at bin/vela, and in-cluster registries
// brought up from testdata/registry.yaml.
//
// Modules are published to the same in-cluster OCI registry the module suite
// uses (registry:2 on NodePort 30500). Addons go to an in-cluster ChartMuseum
// registered as a Helm addon registry, because the controller reads an OCI
// addon registry over TLS only (see testdata/registry.yaml for the detail).
//
// The scenarios are the ones documented in
// testing/addon-module-cr-based-single-cluster: publish and inspect, the
// three-level install, several modules per addon, type reference forms,
// _imports.cue options, template-declared module components, upgrade and
// line removal, "latest" versus pinned, same-tag republish and caching,
// dropping an import, self-healing, the delete chain, ownership conflicts,
// broken imports, registry outages, the legacy CLI installer and the feature
// gates. publish_validation_test.go holds the CLI-only checks, and
// addon_module_e2e_test.go the ordered cluster scenarios.
package addonmoduletest

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

var k8sClient client.Client
var scheme = runtime.NewScheme()

func TestAddonModuleE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Addon Module Component E2E Suite")
}

func initializeAddonModuleClient() {
	if k8sClient != nil {
		return
	}
	cleanupHome, err := prepareAddonModuleWorkerHome()
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(cleanupHome()).To(Succeed()) })
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))

	Expect(clientgoscheme.AddToScheme(scheme)).Should(Succeed())
	Expect(core.AddToScheme(scheme)).Should(Succeed())
	Expect(apiextensionsv1.AddToScheme(scheme)).Should(Succeed())

	k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	Expect(err).ShouldNot(HaveOccurred())
}

// CLI subprocesses in different Ginkgo workers must not share writable
// registry configuration or caches. The kubeconfig remains unchanged.
func prepareAddonModuleWorkerHome() (func() error, error) {
	home, err := os.MkdirTemp("", fmt.Sprintf("kubevela-addon-module-e2e-p%d-", GinkgoParallelProcess()))
	if err != nil {
		return nil, err
	}
	previous, existed := os.LookupEnv(system.VelaHomeEnv)
	if err := os.Setenv(system.VelaHomeEnv, home); err != nil {
		_ = os.RemoveAll(home)
		return nil, err
	}
	return func() error {
		var restoreErr error
		if existed {
			restoreErr = os.Setenv(system.VelaHomeEnv, previous)
		} else {
			restoreErr = os.Unsetenv(system.VelaHomeEnv)
		}
		return errors.Join(restoreErr, os.RemoveAll(home))
	}, nil
}

type scenarioSuiteState struct {
	RunID string
	Root  string
}

func initializeScenarioScopes(state scenarioSuiteState) {
	for _, id := range scenarioIDs {
		*scenarioScopes[id] = *newScenarioScope(state.RunID, id, state.Root)
	}
}

var _ = SynchronizedBeforeSuite(func() []byte {
	initializeAddonModuleClient()
	var token [8]byte
	_, err := cryptorand.Read(token[:])
	Expect(err).NotTo(HaveOccurred())
	root, err := os.MkdirTemp("", "kubevela-addon-module-fixtures-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(os.RemoveAll(root)).To(Succeed()) })
	state := scenarioSuiteState{RunID: hex.EncodeToString(token[:]), Root: root}
	initializeScenarioScopes(state)
	for _, id := range scenarioIDs {
		Expect(scenarioScopes[id].Materialize(testdataPath())).To(Succeed())
	}
	setupAddonModuleFixtures(context.Background())
	data, err := json.Marshal(state)
	Expect(err).NotTo(HaveOccurred())
	return data
}, func(data []byte) {
	initializeAddonModuleClient()
	var state scenarioSuiteState
	Expect(json.Unmarshal(data, &state)).To(Succeed())
	initializeScenarioScopes(state)
	resolveAddonModuleRegistries(context.Background())
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	cleanupAddonModuleFixtures(context.Background())
})

// randomName appends a random suffix to basic so a spec that needs its own
// object does not collide with another run's leftovers.
func randomName(basic string) string {
	return fmt.Sprintf("%s-%d", basic, rand.Int63())
}
