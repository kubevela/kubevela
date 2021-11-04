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

package webservice

import (
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

var _ = Describe("Test validate function", func() {
	It("Test check name validate ", func() {
		Expect(cmp.Diff(nameRegexp.MatchString("///Asd asda "), false)).Should(BeEmpty())
		var app0 = apisv1.CreateApplicationPlanRequest{
			Name:      "a",
			Namespace: "namespace",
		}
		err := validate.Struct(&app0)
		Expect(err).ShouldNot(BeNil())
		var app1 = apisv1.CreateApplicationPlanRequest{
			Name:      "Asdasd",
			Namespace: "namespace",
		}
		err = validate.Struct(&app1)
		Expect(err).ShouldNot(BeNil())
		var app2 = apisv1.CreateApplicationPlanRequest{
			Name:      "asdasd asdasd ++",
			Namespace: "namespace",
		}
		err = validate.Struct(&app2)
		Expect(err).ShouldNot(BeNil())

		var app3 = apisv1.CreateApplicationPlanRequest{
			Name:      "asdasd",
			Namespace: "namespace",
		}
		err = validate.Struct(&app3)
		Expect(err).Should(BeNil())

		var app4 = apisv1.CreateApplicationPlanRequest{
			Name:      "asdasd-asdasd",
			Namespace: "namespace",
		}
		err = validate.Struct(&app4)
		Expect(err).Should(BeNil())

		var component = apisv1.CreateComponentPlanRequest{
			Name:          "asdasd-asdasd",
			ComponentType: "alibaba-ack",
		}
		err = validate.Struct(&component)
		Expect(err).Should(BeNil())
	})
})
