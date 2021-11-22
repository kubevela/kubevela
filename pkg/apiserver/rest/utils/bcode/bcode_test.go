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

package bcode

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test bcode package", func() {
	It("Test New bcode funtion", func() {
		bcode := NewBcode(400, 4000, "test")
		Expect(bcode).ShouldNot(BeNil())
		Expect(bcode.Message).ShouldNot(BeNil())
		Expect(bcode.Error()).ShouldNot(BeNil())
	})
})
