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
	"io/ioutil"
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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test authentication usecase functions", func() {
	var (
		authUsecase *authenticationUsecaseImpl
		userUsecase *userUsecaseImpl
		sysUsecase  *systemInfoUsecaseImpl
		ds          datastore.DataStore
	)

	BeforeEach(func() {
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "auth-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		authUsecase = &authenticationUsecaseImpl{kubeClient: k8sClient, ds: ds}
		sysUsecase = &systemInfoUsecaseImpl{ds: ds, kubeClient: k8sClient}
		userUsecase = &userUsecaseImpl{ds: ds, sysUsecase: sysUsecase}
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
		Expect(resp.Email).Should(Equal("test@test.com"))
		Expect(resp.Name).Should(Equal("test"))

		user := &model.User{
			Name: "test",
		}
		err = ds.Get(context.Background(), user)
		Expect(err).Should(BeNil())
		Expect(user.Email).Should(Equal("test@test.com"))
	})

	It("Test local login", func() {
		_, err := userUsecase.CreateUser(context.Background(), apisv1.CreateUserRequest{
			Name:     "test-login",
			Email:    "test@example.com",
			Password: "password1",
		})
		Expect(err).Should(BeNil())
		localHandler := localHandlerImpl{
			userUsecase: userUsecase,
			ds:          ds,
			username:    "test-login",
			password:    "password1",
		}
		resp, err := localHandler.login(context.Background())
		Expect(err).Should(BeNil())
		Expect(resp.Name).Should(Equal("test-login"))
	})

	It("Test get dex config", func() {
		err := k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-system",
			},
		})
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		webserver, err := ioutil.ReadFile("./testdata/dex-config-def.yaml")
		Expect(err).Should(Succeed())
		var cd v1beta1.ComponentDefinition
		err = yaml.Unmarshal(webserver, &cd)
		Expect(err).Should(Succeed())
		err = k8sClient.Create(context.Background(), &cd)
		Expect(err).Should(Succeed())
		err = k8sClient.Create(context.Background(), &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "addon-dex",
				Namespace: "vela-system",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "dex",
						// only for test here
						Type:       "dex-config",
						Properties: &runtime.RawExtension{Raw: []byte(`{"values":{"test":"test"}}`)},
						Traits:     []common.ApplicationTrait{},
						Scopes:     map[string]string{},
					},
				},
			},
		})
		Expect(err).Should(BeNil())
		_, err = sysUsecase.UpdateSystemInfo(context.Background(), apisv1.SystemInfoRequest{
			LoginType:   model.LoginTypeDex,
			VelaAddress: "http://velaux.com",
		})
		Expect(err).Should(BeNil())

		config, err := authUsecase.GetDexConfig(context.Background())
		Expect(err).Should(BeNil())
		Expect(config.Issuer).Should(Equal("http://velaux.com/dex"))
		Expect(config.ClientID).Should(Equal("velaux"))
		Expect(config.ClientSecret).Should(Equal("velaux-secret"))
		Expect(config.RedirectURL).Should(Equal("http://velaux.com/callback"))
	})

	It("Test update dex config", func() {
		err := k8sClient.Create(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "a",
				Namespace: "vela-system",
				Labels: map[string]string{
					"app.oam.dev/source-of-truth": "from-inner-system",
					"config.oam.dev/catalog":      "velacore-config",
					"config.oam.dev/type":         "config-dex-connector",
					"config.oam.dev/sub-type":     "ldap",
					"project":                     "abc",
				},
			},
			StringData: map[string]string{
				"ldap": `{"clientID":"clientID","clientSecret":"clientSecret"}`,
			},
			Type: corev1.SecretTypeOpaque,
		})
		Expect(err).Should(BeNil())
		err = authUsecase.UpdateDexConfig(context.Background())
		Expect(err).Should(BeNil())
		dexConfigSecret := &corev1.Secret{}
		err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "dex-config", Namespace: "vela-system"}, dexConfigSecret)
		Expect(err).Should(BeNil())
		config := &dexConfig{}
		err = yaml.Unmarshal(dexConfigSecret.Data[secretDexConfigKey], config)
		Expect(err).Should(BeNil())
		Expect(len(config.Connectors)).Should(Equal(1))
	})
})
