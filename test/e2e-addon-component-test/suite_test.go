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

// Package controllers_test is a standalone Ginkgo suite for the
// addon-as-a-component feature: registry management, push, install and use,
// reconciler behaviour, uninstall, ownership between the two install paths,
// version movement, and admission refusals.
//
// It is separate from the core E2E packages so a failure anywhere else in those
// larger, longer-running suites cannot prevent this one from running, and
// separate from e2e/addon-component (one spec against the mock OSS registry)
// because this suite needs a registry it can push to.
package controllers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

// suiteState is what synchronized setup on the first worker hands every
// worker: where the shared registry answers.
type suiteState struct {
	RegistryURL string
}

var (
	root        string
	registryURL string
)

func TestAddonComponentE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Addon Component E2E Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	initializeAddonComponentWorker()
	state := suiteState{RegistryURL: setUpSharedRegistry(context.Background())}
	data, err := json.Marshal(state)
	Expect(err).NotTo(HaveOccurred())
	return data
}, func(data []byte) {
	initializeAddonComponentWorker()
	var state suiteState
	Expect(json.Unmarshal(data, &state)).To(Succeed())
	registryURL = state.RegistryURL
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	tearDownSharedRegistry(context.Background())
})

func initializeAddonComponentWorker() {
	if k8sClient != nil {
		return
	}
	cleanupHome, err := prepareAddonComponentWorkerHome()
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(cleanupHome()).To(Succeed()) })
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))

	Expect(clientgoscheme.AddToScheme(scheme)).Should(Succeed())
	Expect(core.AddToScheme(scheme)).Should(Succeed())

	k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	Expect(err).ShouldNot(HaveOccurred())
	root = repoRoot()
}

// CLI subprocesses in different Ginkgo workers must not share a writable
// VELA_HOME cache. The kubeconfig remains unchanged.
func prepareAddonComponentWorkerHome() (func() error, error) {
	home, err := os.MkdirTemp("", fmt.Sprintf("kubevela-addon-component-e2e-p%d-", GinkgoParallelProcess()))
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

// randomNamespaceName generates a random name based on the basic name, so
// each spec that needs its own namespace does not collide with another run's
// leftovers.
func randomNamespaceName(basic string) string {
	return fmt.Sprintf("%s-%d", basic, rand.Int63())
}
