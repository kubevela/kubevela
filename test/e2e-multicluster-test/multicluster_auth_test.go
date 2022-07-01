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
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/pkg/multicluster"
)

var _ = Describe("Test multicluster Auth commands", func() {

	Context("Test vela auth commands", func() {

		It("Test vela create kubeconfig for given user", func() {
			outputs, err := execCommand("auth", "gen-kubeconfig", "--user", "kubevela", "--group", "kubevela:dev", "--group", "kubevela:test")
			Expect(err).Should(Succeed())
			Expect(outputs).Should(ContainSubstring("Certificate signing request kubevela-csr-kubevela approved"))
		})

		It("Test vela create kubeconfig for serviceaccount", func() {
			outputs, err := execCommand("auth", "gen-kubeconfig", "--serviceaccount", "default", "-n", "vela-system")
			Expect(err).Should(Succeed())
			Expect(outputs).Should(ContainSubstring("ServiceAccount vela-system/default found."))
		})

		It("Test vela list-privileges for user", func() {
			outputs, err := execCommand("auth", "list-privileges", "--user", "example", "--group", "kubevela:dev-team", "--group", "kubevela:test-team")
			Expect(err).Should(Succeed())
			Expect(outputs).Should(ContainSubstring("local"))
		})

		It("Test vela list-privileges for ServiceAccount", func() {
			outputs, err := execCommand("auth", "list-privileges", "--serviceaccount", "node-controller", "-n", "kube-system", "--cluster", "local", "--cluster", WorkerClusterName)
			Expect(err).Should(Succeed())
			Expect(outputs).Should(SatisfyAny(
				ContainSubstring(WorkerClusterName),
				ContainSubstring("nodes/status"),
			))
		})

		It("Test vela list-privileges for kubeconfig", func() {
			outputs, err := execCommand("auth", "list-privileges", "--kubeconfig", WorkerClusterKubeConfigPath, "--cluster", "local")
			Expect(err).Should(Succeed())
			Expect(outputs).Should(ContainSubstring("cluster-admin"))
		})

		It("Test vela grant-privileges for user and create namespace", func() {
			_, err := execCommand("auth", "grant-privileges", "--user", "alice", "--for-namespace", "alice", "--create-namespace", "--for-cluster", "local", "--for-cluster", WorkerClusterName)
			Expect(err).Should(Succeed())
			Expect(k8sClient.Get(multicluster.ContextWithClusterName(context.Background(), "local"), apitypes.NamespacedName{Name: "alice"}, &metav1.Namespace{})).Should(Succeed())
			Expect(k8sClient.Get(multicluster.ContextWithClusterName(context.Background(), WorkerClusterName), apitypes.NamespacedName{Name: "alice"}, &metav1.Namespace{})).Should(Succeed())
		})

		It("Test vela grant-privileges for groups and readonly", func() {
			_, err := execCommand("auth", "grant-privileges", "--group", "kubevela:dev-team", "--group", "kubevela:test-team", "--readonly")
			Expect(err).Should(Succeed())
		})

		It("Test vela grant-privileges for serviceaccount", func() {
			_, err := execCommand("auth", "grant-privileges", "--serviceaccount", "default", "-n", "default", "--for-namespace", "default")
			Expect(err).Should(Succeed())
		})

		It("Test vela grant-privileges for kubeconfig with cluster-scoped privileges", func() {
			_, err := execCommand("auth", "grant-privileges", "--kubeconfig", WorkerClusterKubeConfigPath, "--for-cluster", WorkerClusterName)
			Expect(err).Should(Succeed())
		})

	})

})
