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

// Package controllers_test is a standalone Ginkgo suite for the module
// component: "vela module" CLI plumbing (module_publish_test.go) and the full
// scenario suite covering registry management, publish, install, reconciler
// behaviour, uninstall, namespace isolation, and error paths
// (module_e2e_test.go). It is separate from the core E2E packages so a failure
// anywhere else in those larger, longer-running suites cannot prevent this one
// from running.
package controllers_test

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
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

func TestModuleE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Module Component E2E Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	cleanupHome, err := prepareModuleWorkerHome()
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(cleanupHome()).To(Succeed()) })

	Expect(clientgoscheme.AddToScheme(scheme)).Should(Succeed())
	Expect(core.AddToScheme(scheme)).Should(Succeed())

	k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	Expect(err).ShouldNot(HaveOccurred())
})

// Each Ginkgo process launches its own vela CLI subprocesses. Keep the CLI's
// writable configuration and cache out of the user's VELA_HOME and other
// workers' directories while leaving KUBECONFIG/current-context untouched.
func prepareModuleWorkerHome() (func() error, error) {
	home, err := os.MkdirTemp("", fmt.Sprintf("kubevela-module-e2e-p%d-", GinkgoParallelProcess()))
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

// randomNamespaceName preserves the readable prefix while adding the Ginkgo
// process number and a cryptographically random suffix.  This avoids reusing
// names across worker processes or independent test invocations.
func randomNamespaceName(basic string) string {
	const randomBytes = 12
	var token [randomBytes]byte
	if _, err := rand.Read(token[:]); err != nil {
		panic(fmt.Errorf("generate test namespace name: %w", err))
	}

	var prefix strings.Builder
	lastWasSeparator := false
	for _, char := range strings.ToLower(basic) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			prefix.WriteRune(char)
			lastWasSeparator = false
		} else if prefix.Len() > 0 && !lastWasSeparator {
			prefix.WriteByte('-')
			lastWasSeparator = true
		}
	}
	worker := fmt.Sprintf("%d", GinkgoParallelProcess())
	suffix := "-" + worker + "-" + hex.EncodeToString(token[:])
	readable := strings.Trim(prefix.String(), "-")
	if readable == "" {
		readable = "e2e"
	}
	if len(readable) > 63-len(suffix) {
		readable = strings.TrimRight(readable[:63-len(suffix)], "-")
	}
	return readable + suffix
}
