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

package app

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("resolveControllerNamespace",
	func(podNamespace, leaderElectionNamespace, want string) {
		previous, wasSet := os.LookupEnv("POD_NAMESPACE")
		DeferCleanup(func() {
			if wasSet {
				Expect(os.Setenv("POD_NAMESPACE", previous)).To(Succeed())
			} else {
				Expect(os.Unsetenv("POD_NAMESPACE")).To(Succeed())
			}
		})
		Expect(os.Setenv("POD_NAMESPACE", podNamespace)).To(Succeed())

		Expect(resolveControllerNamespace(leaderElectionNamespace)).To(Equal(want))
	},
	Entry("prefers the pod namespace over the lease namespace", "kubevela", "kube-system", "kubevela"),
	Entry("falls back to the lease namespace", "", " kube-system ", "kube-system"),
	Entry("defaults to vela-system", " ", "", "vela-system"),
)
