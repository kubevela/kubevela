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
		path, err := checkResourcePath("Project")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("Project"))

		path, err = checkResourcePath("Application")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("Project/Application"))

		_, err = checkResourcePath("Applications")
		Expect(err).ShouldNot(BeNil())

		_, err = checkResourcePath("Project/Component")
		Expect(err).ShouldNot(BeNil())

		_, err = checkResourcePath("Workflow")
		Expect(err).ShouldNot(BeNil())

		path, err = checkResourcePath("Project/Application/Workflow")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("Project/Application/Workflow"))

		path, err = checkResourcePath("Project/Workflow")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("Project/Workflow"))
	})
})
