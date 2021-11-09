/*
Copyright 2021 The KubeVela Authors.

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

package e2e_multicluster_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/cli"
)

const (
	WorkerClusterName           = "cluster-worker"
	WorkerClusterKubeConfigPath = "/tmp/worker.kubeconfig"
)

var (
	k8sClient client.Client
)

func execCommand(args ...string) (string, error) {
	command := cli.NewCommand()
	command.SetArgs(args)
	var buf bytes.Buffer
	command.SetOut(&buf)
	command.SetErr(&buf)
	err := command.Execute()
	return buf.String(), err
}

func TestMulticluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KubeVela MultiCluster Test Suite")
}

var _ = BeforeSuite(func() {
	var err error

	// initialize clients
	options := client.Options{Scheme: common.Scheme}
	config := config.GetConfigOrDie()
	config.Wrap(multicluster.NewSecretModeMultiClusterRoundTripper)
	k8sClient, err = client.New(config, options)
	Expect(err).Should(Succeed())

	// join worker cluster
	_, err = execCommand("cluster", "join", WorkerClusterKubeConfigPath, "--name", WorkerClusterName)
	Expect(err).Should(Succeed())
})

var _ = AfterSuite(func() {
	Eventually(func(g Gomega) {
		apps := &v1beta1.ApplicationList{}
		Expect(k8sClient.List(context.Background(), apps)).Should(Succeed())
		for _, app := range apps.Items {
			Expect(k8sClient.Delete(context.Background(), app.DeepCopy())).Should(Succeed())
		}
		Expect(len(apps.Items)).Should(Equal(0))
	}, 3*time.Minute).Should(Succeed())
	Eventually(func(g Gomega) {
		_, err := execCommand("cluster", "detach", WorkerClusterName)
		Expect(err).Should(Succeed())
	}, time.Minute).Should(Succeed())
})
