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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

const (
	username  = "my-user"
	urlPrefix = "http://127.0.0.1:8000/api/v1/users"
)

var _ = Describe("Test user rest api", func() {
	It("Test create user", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateUserRequest{
			Name:     username,
			Alias:    "alias",
			Email:    "test@example.com",
			Password: "password",
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).Should(BeNil())
		res, err := http.Post(urlPrefix, "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).Should(BeNil())
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		userBase := &apisv1.UserBase{}
		err = json.NewDecoder(res.Body).Decode(&userBase)
		Expect(err).Should(BeNil())
		Expect(userBase.Name).Should(Equal(username))
		Expect(userBase.Alias).Should(Equal("alias"))
		Expect(userBase.Email).Should(Equal("test@example.com"))
	})

	It("Test list users", func() {
		defer GinkgoRecover()
		res, err := http.Get(urlPrefix)
		Expect(err).Should(BeNil())
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		users := &apisv1.ListUserResponse{}
		err = json.NewDecoder(res.Body).Decode(users)
		Expect(err).Should(BeNil())
		Expect(users.Total).Should(Equal(int64(1)))
	})

	It("Test detail user", func() {
		defer GinkgoRecover()
		res, err := http.Get(fmt.Sprintf("%s/%s", urlPrefix, username))
		Expect(err).Should(BeNil())
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var detail apisv1.DetailUserResponse
		err = json.NewDecoder(res.Body).Decode(&detail)
		Expect(err).Should(BeNil())
		Expect(detail.Name).Should(Equal(username))
		Expect(detail.Alias).Should(Equal("alias"))
		Expect(detail.Email).Should(Equal("test@example.com"))
		Expect(len(detail.Projects)).Should(Equal(0))
	})

	It("Test update user", func() {
		defer GinkgoRecover()
		var updateReq = apisv1.UpdateUserRequest{
			Alias: "updated-alias",
		}
		bodyByte, err := json.Marshal(updateReq)
		Expect(err).Should(BeNil())
		req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", urlPrefix, username), bytes.NewBuffer(bodyByte))
		Expect(err).Should(BeNil())
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		Expect(err).Should(BeNil())
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		userBase := &apisv1.UserBase{}
		err = json.NewDecoder(res.Body).Decode(&userBase)
		Expect(err).Should(BeNil())
		Expect(userBase.Alias).Should(Equal("updated-alias"))
	})

	It("Test delete user", func() {
		defer GinkgoRecover()
		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%s", urlPrefix, username), nil)
		Expect(err).Should(BeNil())
		res, err := http.DefaultClient.Do(req)
		Expect(err).Should(BeNil())
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
	})

})
