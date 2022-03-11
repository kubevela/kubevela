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
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"time"

	. "github.com/agiledragon/gomonkey/v2"
	"github.com/coreos/go-oidc"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
)

var _ = Describe("Test authentication usecase functions", func() {
	var (
		ds datastore.DataStore
	)

	BeforeEach(func() {
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "auth-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
	})
	It("Test Dex login", func() {
		testIDToken := &oidc.IDToken{}
		patch := ApplyMethod(reflect.TypeOf(testIDToken), "Claims", func(_ *oidc.IDToken, v interface{}) error {
			return json.Unmarshal([]byte(`{"email":"test@test.com","name":"test"}`), v)
		})
		defer patch.Reset()
		dexHandler := dexHandlerImpl{
			idToken: testIDToken,
			ds:      ds,
		}
		resp, err := dexHandler.login(context.Background())
		Expect(err).Should(BeNil())
		Expect(resp.UserInfo.Email).Should(Equal("test@test.com"))
		Expect(resp.UserInfo.Name).Should(Equal("test"))

		user := &model.User{
			Email: "test@test.com",
		}
		err = ds.Get(context.Background(), user)
		Expect(err).Should(BeNil())
		Expect(user.Name).Should(Equal("test"))
	})
})
