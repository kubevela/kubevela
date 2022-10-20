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

package model

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("test container list", func() {
	It("convert container list to table", func() {
		containers := ContainerList{
			Container{
				name:               "test-container",
				image:              "test-image",
				ready:              "Yes",
				state:              "Running",
				CPU:                "N/A",
				Mem:                "N/A",
				CPUR:               "N/A",
				CPUL:               "N/A",
				MemR:               "N/A",
				MemL:               "N/A",
				lastTerminationMsg: "",
				restartCount:       "0",
			},
		}
		info := containers.ToTableBody()[0]

		Expect(info[0]).To(Equal("test-container"))
		Expect(info[1]).To(Equal("test-image"))
		Expect(info[2]).To(Equal("Yes"))
		Expect(info[3]).To(Equal("Running"))
		Expect(info[4]).To(Equal("N/A"))
		Expect(info[5]).To(Equal("N/A"))
		Expect(info[6]).To(Equal("N/A"))
		Expect(info[7]).To(Equal("N/A"))
		Expect(info[8]).To(Equal("N/A"))
		Expect(info[9]).To(Equal("N/A"))
		Expect(info[10]).To(Equal(""))
		Expect(info[11]).To(Equal("0"))
	})
})
