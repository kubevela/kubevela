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

package service

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test authentication service functions", func() {
	var (
		authService    *authenticationServiceImpl
		userService    *userServiceImpl
		sysService     *systemInfoServiceImpl
		projectService ProjectService
		ds             datastore.DataStore
	)

	BeforeEach(func() {
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "auth-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		authService = &authenticationServiceImpl{KubeClient: k8sClient, Store: ds}
		sysService = &systemInfoServiceImpl{Store: ds, KubeClient: k8sClient}
		userService = &userServiceImpl{Store: ds, SysService: sysService}
		projectService = NewTestProjectService(ds, k8sClient)
	})
	It("Test Dex login", func() {
		testIDToken := &oidc.IDToken{}
		patch := ApplyMethod(reflect.TypeOf(testIDToken), "Claims", func(_ *oidc.IDToken, v interface{}) error {
			return json.Unmarshal([]byte(`{"email":"test@test.com", "name":"show name", "sub": "testuser"}`), v)
		})
		defer patch.Reset()

		err := sysService.Init(context.TODO())
		Expect(err).Should(BeNil())
		err = userService.Init(context.TODO())
		Expect(err).Should(BeNil())
		err = projectService.Init(context.TODO())
		Expect(err).Should(BeNil())

		_, err = sysService.UpdateSystemInfo(context.TODO(), apisv1.SystemInfoRequest{
			LoginType: "local",
			DexUserDefaultProjects: []model.ProjectRef{{
				Name:  "default",
				Roles: []string{"app-developer"},
			}},
		})
		Expect(err).Should(BeNil())

		dexHandler := dexHandlerImpl{
			idToken:           testIDToken,
			Store:             ds,
			projectService:    projectService,
			systemInfoService: sysService,
		}
		resp, err := dexHandler.login(context.Background())
		Expect(err).Should(BeNil())
		Expect(resp.Email).Should(Equal("test@test.com"))
		Expect(resp.Name).Should(Equal("testuser"))
		Expect(resp.Alias).Should(Equal("show name"))

		projects, err := projectService.ListUserProjects(context.TODO(), "testuser")
		Expect(err).Should(BeNil())
		Expect(len(projects)).Should(Equal(1))

		user := &model.User{
			Name: "testuser",
		}
		err = ds.Get(context.Background(), user)
		Expect(err).Should(BeNil())
		Expect(user.Email).Should(Equal("test@test.com"))

		existUser := &model.User{
			Name: "testuser",
		}
		err = ds.Delete(context.Background(), existUser)
		Expect(err).Should(BeNil())

		existUser = &model.User{
			Name:  "exist-user",
			Email: "test@test.com",
		}
		err = ds.Add(context.Background(), existUser)
		Expect(err).Should(BeNil())
		resp, err = dexHandler.login(context.Background())
		Expect(err).Should(BeNil())
		Expect(resp.Email).Should(Equal("test@test.com"))
		Expect(resp.Name).Should(Equal("exist-user"))

		err = ds.Delete(context.Background(), existUser)
		Expect(err).Should(BeNil())

		existUser = &model.User{
			Name:   "zhangsan",
			Email:  "test2@test.com",
			DexSub: "testuser",
		}
		err = ds.Add(context.Background(), existUser)
		Expect(err).Should(BeNil())
		resp, err = dexHandler.login(context.Background())
		Expect(err).Should(BeNil())
		Expect(resp.Email).Should(Equal("test2@test.com"))
		Expect(resp.Name).Should(Equal("zhangsan"))

	})

	It("Test local login", func() {
		_, err := userService.CreateUser(context.Background(), apisv1.CreateUserRequest{
			Name:     "test-login",
			Email:    "test@example.com",
			Password: "password1",
		})
		Expect(err).Should(BeNil())
		localHandler := localHandlerImpl{
			userService: userService,
			ds:          ds,
			username:    "test-login",
			password:    "password1",
		}
		resp, err := localHandler.login(context.Background())
		Expect(err).Should(BeNil())
		Expect(resp.Name).Should(Equal("test-login"))
	})

	It("Test update dex config", func() {
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
		err = k8sClient.Create(context.Background(), &corev1.Secret{
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
		By("try to update dex config without config secret")
		connectors, err := utils.GetDexConnectors(context.Background(), authService.KubeClient)
		Expect(err).Should(BeNil())
		err = generateDexConfig(context.Background(), authService.KubeClient, &model.UpdateDexConfig{
			Connectors: connectors,
		})
		Expect(err).Should(BeNil())
		dexConfigSecret := &corev1.Secret{}
		err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "dex-config", Namespace: "vela-system"}, dexConfigSecret)
		Expect(err).Should(BeNil())
		config := &model.DexConfig{}
		err = yaml.Unmarshal(dexConfigSecret.Data[secretDexConfigKey], config)
		Expect(err).Should(BeNil())
		Expect(len(config.Connectors)).Should(Equal(1))
		By("try to update dex config with config secret")
		err = generateDexConfig(context.Background(), authService.KubeClient, &model.UpdateDexConfig{})
		Expect(err).Should(BeNil())
	})

	It("Test get dex config", func() {
		err := ds.Add(context.Background(), &model.User{Name: "admin", Email: "test@test.com"})
		Expect(err).Should(BeNil())
		_, err = sysService.UpdateSystemInfo(context.Background(), apisv1.SystemInfoRequest{
			LoginType:   model.LoginTypeDex,
			VelaAddress: "http://velaux.com",
		})
		Expect(err).Should(BeNil())
		config, err := authService.GetDexConfig(context.Background())
		Expect(err).Should(BeNil())
		Expect(config.Issuer).Should(Equal("http://velaux.com/dex"))
		Expect(config.ClientID).Should(Equal("velaux"))
		Expect(config.ClientSecret).Should(Equal("velaux-secret"))
		Expect(config.RedirectURL).Should(Equal("http://velaux.com/callback"))
	})
})
