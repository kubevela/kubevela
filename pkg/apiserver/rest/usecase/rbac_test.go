/*
Copyright 2022 The KubeVela Authors.

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

package usecase

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test rbac service", func() {
	It("Test check resource", func() {
		path, err := checkResourcePath("project")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("project:{projectName}"))

		path, err = checkResourcePath("application")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("project:{projectName}/application:{appName}"))

		_, err = checkResourcePath("applications")
		Expect(err).ShouldNot(BeNil())

		_, err = checkResourcePath("project/component")
		Expect(err).ShouldNot(BeNil())

		_, err = checkResourcePath("workflow")
		Expect(err).ShouldNot(BeNil())

		path, err = checkResourcePath("project/application/workflow")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("project:{projectName}/application:{appName}/workflow:{workflowName}"))

		path, err = checkResourcePath("project/workflow")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("project:{projectName}/workflow:{workflowName}"))
	})
})
