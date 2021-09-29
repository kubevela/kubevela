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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

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
	_, err := execCommand("cluster", "detach", WorkerClusterName)
	Expect(err).Should(Succeed())
})
