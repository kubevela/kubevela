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
	"golang.org/x/oauth2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
)

var _ = Describe("Test authentication usecase functions", func() {
	var (
		authUsecase *authenticationUsecaseImpl
		ds          datastore.DataStore
	)

	BeforeEach(func() {
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "auth-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		authUsecase = &authenticationUsecaseImpl{kubeClient: k8sClient, ds: ds}
	})
	It("Test Dex login", func() {
		testIDToken := &oidc.IDToken{}
		patch := ApplyMethod(reflect.TypeOf(testIDToken), "Claims", func(_ *oidc.IDToken, v interface{}) error {
			return json.Unmarshal([]byte(`{"email":"test@test.com","name":"test"}`), v)
		})
		defer patch.Reset()
		dexHandler := dexHandlerImpl{
			token: &oauth2.Token{
				AccessToken:  "access-token",
				RefreshToken: "refresh-token",
			},
			idToken: testIDToken,
			ds:      ds,
		}
		resp, err := dexHandler.login(context.Background())
		Expect(err).Should(BeNil())
		Expect(resp.UserInfo.Email).Should(Equal("test@test.com"))
		Expect(resp.UserInfo.Name).Should(Equal("test"))
		Expect(resp.AccessToken).Should(Equal("access-token"))
		Expect(resp.RefreshToken).Should(Equal("refresh-token"))

		user := &model.User{
			Name: "test",
		}
		err = ds.Get(context.Background(), user)
		Expect(err).Should(BeNil())
		Expect(user.Email).Should(Equal("test@test.com"))
	})

	It("Test get dex config", func() {
		err := k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-system",
			},
		})
		Expect(err).Should(BeNil())
		err = k8sClient.Create(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretDexConfig,
				Namespace: "vela-system",
			},
			StringData: map[string]string{
				secretDexConfig: `{"issuer":"https://dex.oam.dev","staticClients":[{"id":"client-id","secret":"client-secret","redirectURIs":["http://localhost:8080/auth/callback"]}]}`,
			},
		})
		Expect(err).Should(BeNil())

		config, err := authUsecase.GetDexConfig(context.Background())
		Expect(err).Should(BeNil())
		Expect(config.Issuer).Should(Equal("https://dex.oam.dev"))
		Expect(config.ClientID).Should(Equal("client-id"))
		Expect(config.ClientSecret).Should(Equal("client-secret"))
		Expect(config.RedirectURL).Should(Equal("http://localhost:8080/auth/callback"))
	})

})
