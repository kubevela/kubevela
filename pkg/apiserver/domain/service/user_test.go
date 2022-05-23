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
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test authentication service functions", func() {
	var (
		userService *userServiceImpl
		ds          datastore.DataStore
		db          string
	)

	BeforeEach(func() {
		var err error
		db = "user-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: db})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		rbacService := &rbacServiceImpl{Store: ds}
		projectService := &projectServiceImpl{K8sClient: k8sClient, Store: ds, RbacService: rbacService}
		sysService := &systemInfoServiceImpl{Store: ds}
		userService = &userServiceImpl{Store: ds, K8sClient: k8sClient, ProjectService: projectService, SysService: sysService, RbacService: rbacService}
	})
	AfterEach(func() {
		err := k8sClient.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: db}})
		Expect(err).Should(BeNil())
	})
	It("Test create user", func() {
		user, err := userService.CreateUser(context.Background(), apisv1.CreateUserRequest{
			Name:     "name",
			Alias:    "alias",
			Email:    "email@example.com",
			Password: "password",
		})
		Expect(err).Should(BeNil())
		Expect(user.Name).Should(Equal("name"))
		Expect(user.Alias).Should(Equal("alias"))
		Expect(user.Email).Should(Equal("email@example.com"))

		u := &model.User{
			Name: "name",
		}
		err = ds.Get(context.Background(), u)
		Expect(err).Should(BeNil())
		Expect(u.Name).Should(Equal("name"))
		Expect(u.Alias).Should(Equal("alias"))
		Expect(u.Email).Should(Equal("email@example.com"))
		Expect(u.Disabled).Should(Equal(false))
		Expect(compareHashWithPassword(u.Password, "password")).Should(BeNil())
	})

	It("Test detail user", func() {
		ctx := context.Background()
		err := ds.Add(ctx, &model.User{
			Name:     "name",
			Alias:    "alias",
			Email:    "email@example.com",
			Password: "password",
		})
		Expect(err).Should(BeNil())
		for i := 0; i < 2; i++ {
			err = ds.Add(ctx, &model.ProjectUser{
				Username:    "name",
				ProjectName: fmt.Sprintf("project-%d", i),
				UserRoles:   []string{fmt.Sprintf("user-role-%d", i)},
			})
			Expect(err).Should(BeNil())
			err = ds.Add(ctx, &model.Project{
				Name:  fmt.Sprintf("project-%d", i),
				Alias: fmt.Sprintf("project-alias-%d", i),
			})
			Expect(err).Should(BeNil())
		}

		userModel := &model.User{
			Name: "name",
		}
		err = ds.Get(ctx, userModel)
		Expect(err).Should(BeNil())
		user, err := userService.DetailUser(ctx, userModel)
		Expect(err).Should(BeNil())
		Expect(user.Name).Should(Equal("name"))
		Expect(user.Alias).Should(Equal("alias"))
		Expect(user.Email).Should(Equal("email@example.com"))
		Expect(len(user.Projects)).Should(Equal(2))
	})

	It("Test list users", func() {
		ctx := context.Background()
		for i := 0; i < 2; i++ {
			err := ds.Add(ctx, &model.User{
				Name: fmt.Sprintf("name-%d", i),
			})
			Expect(err).Should(BeNil())
		}
		users, err := userService.ListUsers(ctx, 0, 10, apisv1.ListUserOptions{Name: "1"})
		Expect(err).Should(BeNil())
		Expect(users.Total).Should(Equal(int64(1)))

		users, err = userService.ListUsers(ctx, 0, 10, apisv1.ListUserOptions{})
		Expect(err).Should(BeNil())
		Expect(users.Total).Should(Equal(int64(2)))
	})

	It("Test delete user", func() {
		ctx := context.Background()
		err := ds.Add(ctx, &model.User{
			Name:     "name",
			Alias:    "alias",
			Email:    "email@example.com",
			Password: "password",
		})
		Expect(err).Should(BeNil())
		users, err := userService.ListUsers(ctx, 0, 10, apisv1.ListUserOptions{})
		Expect(err).Should(BeNil())
		Expect(users.Total).Should(Equal(int64(1)))

		err = userService.DeleteUser(ctx, "name")
		Expect(err).Should(BeNil())
		users, err = userService.ListUsers(ctx, 0, 10, apisv1.ListUserOptions{})
		Expect(err).Should(BeNil())
		Expect(users.Total).Should(Equal(int64(0)))
	})

	It("Test update user", func() {
		ctx := context.Background()
		userModel := &model.User{
			Name:     "admin",
			Alias:    "alias",
			Email:    "email@example.com",
			Password: "password",
		}
		err := ds.Add(ctx, userModel)
		Expect(err).Should(BeNil())

		err = k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-system",
			},
		})
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		_, err = userService.UpdateUser(ctx, userModel, apisv1.UpdateUserRequest{
			Alias:    "new-alias",
			Password: "new-password",
		})
		Expect(err).Should(BeNil())
		newUser := &model.User{
			Name: "admin",
		}
		err = ds.Get(ctx, newUser)
		Expect(err).Should(BeNil())
		Expect(newUser.Alias).Should(Equal("new-alias"))
		Expect(compareHashWithPassword(newUser.Password, "new-password")).Should(BeNil())

		dexConfigSecret := &corev1.Secret{}
		err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "dex-config", Namespace: "vela-system"}, dexConfigSecret)
		Expect(err).Should(BeNil())
		config := &model.DexConfig{}
		err = yaml.Unmarshal(dexConfigSecret.Data[secretDexConfigKey], config)
		Expect(err).Should(BeNil())
		Expect(len(config.StaticPasswords)).Should(Equal(1))
		Expect(config.StaticPasswords[0].Username).Should(Equal("admin"))
		Expect(config.StaticPasswords[0].Hash).Should(Equal(newUser.Password))
	})

	It("Test disable user", func() {
		ctx := context.Background()
		userModel := &model.User{
			Name:     "name",
			Disabled: true,
		}
		err := ds.Add(ctx, userModel)
		Expect(err).Should(BeNil())

		err = userService.DisableUser(ctx, userModel)
		Expect(err).Should(Equal(bcode.ErrUserAlreadyDisabled))
		userModel.Disabled = false
		err = ds.Put(ctx, userModel)
		Expect(err).Should(BeNil())

		err = userService.DisableUser(ctx, userModel)
		Expect(err).Should(BeNil())

		newUser := &model.User{
			Name: "name",
		}
		err = ds.Get(ctx, newUser)
		Expect(err).Should(BeNil())
		Expect(newUser.Disabled).Should(Equal(true))
	})

	It("Test enable user", func() {
		ctx := context.Background()
		userModel := &model.User{
			Name:     "name",
			Disabled: false,
		}
		err := ds.Add(ctx, userModel)
		Expect(err).Should(BeNil())

		err = userService.EnableUser(ctx, userModel)
		Expect(err).Should(Equal(bcode.ErrUserAlreadyEnabled))
		userModel.Disabled = true
		err = ds.Put(ctx, userModel)
		Expect(err).Should(BeNil())

		err = userService.EnableUser(ctx, userModel)
		Expect(err).Should(BeNil())

		newUser := &model.User{
			Name: "name",
		}
		err = ds.Get(ctx, newUser)
		Expect(err).Should(BeNil())
		Expect(newUser.Disabled).Should(Equal(false))
	})
})
