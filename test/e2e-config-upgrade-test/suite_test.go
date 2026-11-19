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
package upgrade_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// The upgrade path of #7410's verification plan, as ordered phases against one cluster. Each
// phase is one run of this suite, selected with -ginkgo.label-filter, because between phases the
// job changes what runs on the cluster:
//
//	chart 1.11.0 installed     seed
//	branch chart upgraded      after-upgrade, migrate, gate, nacos
//	helm rollback to 1.11.0    after-rollback
//
// The environment says where things are:
//
//	VELA_OLD_CLI    the 1.11.0 vela binary
//	VELA_NEW_CLI    the branch's vela binary
//	NACOS_ADDR      the Nacos server, from here
//	NACOS_CLI_HOST  the Nacos host the endpoint config names: reached by the 1.11 CLI, which
//	                publishes from this machine, and by the controller's pods
//
// Without VELA_OLD_CLI the suite skips.
const (
	seedNS      = "up-seed"
	targetNS    = "up-target"
	stateName   = "up-state"
	waitTimeout = 3 * time.Minute
	waitPoll    = 3 * time.Second
)

var (
	k8sClient client.Client
	ctx       = context.Background()
)

func TestConfigUpgrade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config upgrade E2E suite")
}

var _ = BeforeSuite(func() {
	if os.Getenv("VELA_OLD_CLI") == "" {
		Skip("VELA_OLD_CLI is not set: this suite runs only in the config upgrade job")
	}
	var err error
	k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: common.Scheme})
	Expect(err).ShouldNot(HaveOccurred())
})

// run executes a command from an empty directory (the 1.11 CLI's Nacos client writes its log and
// cache under the working directory) and returns its output.
func run(name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = GinkgoT().TempDir()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func mustRun(name string, args ...string) string {
	out, err := run(name, args...)
	ExpectWithOffset(1, err).ShouldNot(HaveOccurred(), "%s %s\n%s", name, strings.Join(args, " "), out)
	return out
}

func oldCLI(args ...string) string { return mustRun(os.Getenv("VELA_OLD_CLI"), args...) }
func newCLI(args ...string) string { return mustRun(os.Getenv("VELA_NEW_CLI"), args...) }

func testdata(name string) string {
	path, err := filepath.Abs(filepath.Join("testdata", name))
	ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
	return path
}

func getSecret(ns, name string) *corev1.Secret {
	var secret corev1.Secret
	ExpectWithOffset(1, k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &secret)).Should(Succeed())
	return &secret
}

// loadState reads what the seed phase recorded.
func loadState() map[string]string {
	var cm corev1.ConfigMap
	ExpectWithOffset(1, k8sClient.Get(ctx, client.ObjectKey{Namespace: seedNS, Name: stateName}, &cm)).Should(Succeed(), "the seed phase records its state")
	return cm.Data
}

func saveState(data map[string]string) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: seedNS, Name: stateName}, Data: data}
	ExpectWithOffset(1, k8sClient.Create(ctx, cm)).Should(Succeed())
}
