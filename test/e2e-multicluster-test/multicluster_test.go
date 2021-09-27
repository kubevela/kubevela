/*
 Copyright 2021. The KubeVela Authors.

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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/references/cli"
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

var _ = Describe("multicluster related e2e-test.", func() {

	Context("Test vela cluster command", func() {

		It("Test adding cluster by kubeconfig", func() {
			const oldClusterName = "worker-cluster"
			const newClusterName = "cluster-worker"
			_, err := execCommand("cluster", "list")
			Expect(err).Should(Succeed())
			_, err = execCommand("cluster", "join", "/tmp/worker.kubeconfig", "--name", oldClusterName)
			Expect(err).Should(Succeed())
			out, err := execCommand("cluster", "list")
			Expect(err).Should(Succeed())
			Expect(out).Should(ContainSubstring(oldClusterName))
			_, err = execCommand("cluster", "rename", oldClusterName, newClusterName)
			Expect(err).Should(Succeed())
			out, err = execCommand("cluster", "list")
			Expect(err).Should(Succeed())
			Expect(out).Should(ContainSubstring(newClusterName))
			_, err = execCommand("cluster", "detach", newClusterName)
			Expect(err).Should(Succeed())
			out, err = execCommand("cluster", "list")
			Expect(err).Should(Succeed())
			Expect(out).ShouldNot(ContainSubstring(newClusterName))
		})

	})

})
