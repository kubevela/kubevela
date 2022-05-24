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

package e2e_apiserver_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

const (
	username = "my-user"
)

var _ = Describe("Test user rest api", func() {
	It("Test create user", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateUserRequest{
			Name:     username,
			Alias:    "alias",
			Email:    "test@example.com",
			Password: "password1",
		}
		res := post("/users", req)
		userBase := &apisv1.UserBase{}
		Expect(decodeResponseBody(res, userBase)).Should(Succeed())
		Expect(userBase.Name).Should(Equal(username))
		Expect(userBase.Alias).Should(Equal("alias"))
		Expect(userBase.Email).Should(Equal("test@example.com"))
	})

	It("Test list users", func() {
		defer GinkgoRecover()
		res := get("/users")
		users := &apisv1.ListUserResponse{}
		Expect(decodeResponseBody(res, users)).Should(Succeed())
		// two users in total, one is admin user
		Expect(users.Total).Should(Equal(int64(2)))
	})

	It("Test detail user", func() {
		defer GinkgoRecover()
		res := get(fmt.Sprintf("/users/%s", username))
		detail := &apisv1.DetailUserResponse{}
		Expect(decodeResponseBody(res, detail)).Should(Succeed())
		Expect(detail.Name).Should(Equal(username))
		Expect(detail.Alias).Should(Equal("alias"))
		Expect(detail.Email).Should(Equal("test@example.com"))
		Expect(len(detail.Projects)).Should(Equal(0))
	})

	It("Test update user", func() {
		defer GinkgoRecover()
		var req = apisv1.UpdateUserRequest{
			Alias: "updated-alias",
		}
		res := put(fmt.Sprintf("/users/%s", username), req)
		userBase := &apisv1.UserBase{}
		Expect(decodeResponseBody(res, userBase)).Should(Succeed())
		Expect(userBase.Alias).Should(Equal("updated-alias"))
	})

	It("Test delete user", func() {
		defer GinkgoRecover()
		res := delete(fmt.Sprintf("/users/%s", username))
		Expect(decodeResponseBody(res, nil)).Should(Succeed())
	})

})
