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

package service

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test project service functions", func() {
	var (
		projectService   *projectServiceImpl
		envImpl          *envServiceImpl
		userService      *userServiceImpl
		targetImpl       *targetServiceImpl
		defaultNamespace = "project-default-ns1-test"
	)
	BeforeEach(func() {
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: "target-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		userService = &userServiceImpl{Store: ds, K8sClient: k8sClient}
		var ns = corev1.Namespace{}
		ns.Name = defaultNamespace
		err = k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		targetImpl = &targetServiceImpl{K8sClient: k8sClient, Store: ds}
		envImpl = &envServiceImpl{KubeClient: k8sClient, Store: ds}
		rbacService := &rbacServiceImpl{Store: ds}
		userService := &userServiceImpl{Store: ds, RbacService: rbacService, SysService: systemInfoServiceImpl{Store: ds}}
		projectService = &projectServiceImpl{
			K8sClient:     k8sClient,
			Store:         ds,
			RbacService:   rbacService,
			TargetService: targetImpl,
			UserService:   userService,
			EnvService:    envImpl,
		}
		userService.ProjectService = projectService
		envImpl.ProjectService = projectService
		pp, err := projectService.ListProjects(context.TODO(), 0, 0)
		Expect(err).Should(BeNil())
		// reset all projects
		for _, p := range pp.Projects {
			_ = projectService.DeleteProject(context.TODO(), p.Name)
		}
		ctx := context.WithValue(context.TODO(), &apisv1.CtxKeyUser, "admin")
		envs, err := envImpl.ListEnvs(ctx, 0, 0, apisv1.ListEnvOptions{})
		Expect(err).Should(BeNil())
		// reset all projects
		for _, e := range envs.Envs {
			_ = envImpl.DeleteEnv(context.TODO(), e.Name)
		}
		targets, err := targetImpl.ListTargets(context.TODO(), 0, 0, "")
		Expect(err).Should(BeNil())
		// reset all projects
		for _, t := range targets.Targets {
			_ = targetImpl.DeleteTarget(context.TODO(), t.Name)
		}
	})
	It("Test project initialize function", func() {

		// init admin user
		var ns = corev1.Namespace{}
		ns.Name = velatypes.DefaultKubeVelaNS
		err := k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		err = userService.Init(context.TODO())
		Expect(err).Should(BeNil())

		// init default project
		err = projectService.InitDefaultProjectEnvTarget(context.WithValue(context.TODO(), &apisv1.CtxKeyUser, model.DefaultAdminUserName), defaultNamespace)
		Expect(err).Should(BeNil())
		By("test env created")
		var namespace corev1.Namespace
		Eventually(func() error {
			return k8sClient.Get(context.TODO(), types.NamespacedName{Name: defaultNamespace}, &namespace)
		}, time.Second*3, time.Microsecond*300).Should(BeNil())

		Expect(cmp.Diff(namespace.Labels[oam.LabelNamespaceOfEnvName], model.DefaultInitName)).Should(BeEmpty())
		Expect(cmp.Diff(namespace.Labels[oam.LabelNamespaceOfTargetName], model.DefaultInitName)).Should(BeEmpty())
		Expect(cmp.Diff(namespace.Labels[oam.LabelControlPlaneNamespaceUsage], oam.VelaNamespaceUsageEnv)).Should(BeEmpty())
		Expect(cmp.Diff(namespace.Labels[oam.LabelRuntimeNamespaceUsage], oam.VelaNamespaceUsageTarget)).Should(BeEmpty())

		By("check project created")
		dp, err := projectService.GetProject(context.TODO(), model.DefaultInitName)
		Expect(err).Should(BeNil())
		Expect(dp.Alias).Should(BeEquivalentTo("Default"))
		Expect(dp.Description).Should(BeEquivalentTo(model.DefaultProjectDescription))

		By("check env created")

		env, err := envImpl.GetEnv(context.TODO(), model.DefaultInitName)
		Expect(err).Should(BeNil())
		Expect(env.Alias).Should(BeEquivalentTo("Default"))
		Expect(env.Description).Should(BeEquivalentTo(model.DefaultEnvDescription))
		Expect(env.Project).Should(BeEquivalentTo(model.DefaultInitName))
		Expect(env.Targets).Should(BeEquivalentTo([]string{model.DefaultInitName}))
		Expect(env.Namespace).Should(BeEquivalentTo(defaultNamespace))

		By("check target created")

		tg, err := targetImpl.GetTarget(context.TODO(), model.DefaultInitName)
		Expect(err).Should(BeNil())
		Expect(tg.Alias).Should(BeEquivalentTo("Default"))
		Expect(tg.Description).Should(BeEquivalentTo(model.DefaultTargetDescription))
		Expect(tg.Cluster).Should(BeEquivalentTo(&model.ClusterTarget{
			ClusterName: multicluster.ClusterLocalName,
			Namespace:   defaultNamespace,
		}))
		Expect(env.Targets).Should(BeEquivalentTo([]string{model.DefaultInitName}))
		Expect(env.Namespace).Should(BeEquivalentTo(defaultNamespace))

		err = targetImpl.DeleteTarget(context.TODO(), model.DefaultInitName)
		Expect(err).Should(BeNil())
		err = envImpl.DeleteEnv(context.TODO(), model.DefaultInitName)
		Expect(err).Should(BeNil())

	})
	It("Test Create project function", func() {
		req := apisv1.CreateProjectRequest{
			Name:        "test-project",
			Description: "this is a project description",
		}
		base, err := projectService.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())
		_, err = projectService.ListProjects(context.TODO(), 0, 0)
		Expect(err).Should(BeNil())
		projectService.DeleteProject(context.TODO(), "test-project")
	})

	It("Test Update project function", func() {
		req := apisv1.CreateProjectRequest{
			Name:        "test-project",
			Description: "this is a project description",
		}
		app1 := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config-sync-test-project",
				Namespace: "vela-system",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Type: "aaa",
				}},
			},
		}
		Expect(k8sClient.Create(context.TODO(), app1)).Should(BeNil())
		_, err := projectService.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())

		base, err := projectService.UpdateProject(context.TODO(), "test-project", apisv1.UpdateProjectRequest{
			Alias:       "Change alias",
			Description: "Change description",
			Owner:       "admin",
		})
		Expect(err).Should(BeNil())
		Expect(base.Alias).Should(BeEquivalentTo("Change alias"))
		Expect(base.Description).Should(BeEquivalentTo("Change description"))
		Expect(base.Owner.Alias).Should(BeEquivalentTo("Administrator"))

		user := &model.User{
			Name:     "admin-2",
			Alias:    "Administrator2",
			Password: "ddddd",
			Disabled: false,
		}
		err = projectService.Store.Add(context.TODO(), user)
		Expect(err).Should(BeNil())
		base, err = projectService.UpdateProject(context.TODO(), "test-project", apisv1.UpdateProjectRequest{
			Alias:       "Change alias",
			Description: "Change description",
			Owner:       "admin-2",
		})
		Expect(err).Should(BeNil())
		Expect(base.Alias).Should(BeEquivalentTo("Change alias"))
		Expect(base.Description).Should(BeEquivalentTo("Change description"))
		Expect(base.Owner.Alias).Should(BeEquivalentTo("Administrator2"))
		res, err := projectService.ListProjectUser(context.TODO(), "test-project", 0, 0)
		Expect(err).Should(BeNil())
		Expect(res.Total).Should(Equal(int64(2)))

		_, err = projectService.UpdateProject(context.TODO(), "test-project", apisv1.UpdateProjectRequest{
			Alias:       "Change alias",
			Description: "Change description",
			Owner:       "admin-error",
		})
		Expect(err).Should(BeEquivalentTo(bcode.ErrProjectOwnerIsNotExist))
		err = projectService.DeleteProject(context.TODO(), "test-project")
		Expect(err).Should(BeNil())
	})

	It("Test Create project user function", func() {
		req := apisv1.CreateProjectRequest{
			Name:        "test-project",
			Description: "this is a project description",
		}
		_, err := projectService.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())

		_, err = projectService.AddProjectUser(context.TODO(), "test-project", apisv1.AddProjectUserRequest{
			UserName:  "admin",
			UserRoles: []string{"project-admin"},
		})
		Expect(err).Should(BeNil())
	})

	It("Test Update project user function", func() {
		req := apisv1.CreateProjectRequest{
			Name:        "test-project",
			Description: "this is a project description",
		}
		_, err := projectService.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())

		_, err = projectService.AddProjectUser(context.TODO(), "test-project", apisv1.AddProjectUserRequest{
			UserName:  "admin",
			UserRoles: []string{"project-admin"},
		})
		Expect(err).Should(BeNil())

		_, err = projectService.UpdateProjectUser(context.TODO(), "test-project", "admin", apisv1.UpdateProjectUserRequest{
			UserRoles: []string{"project-admin", "app-developer"},
		})
		Expect(err).Should(BeNil())

		_, err = projectService.UpdateProjectUser(context.TODO(), "test-project", "admin", apisv1.UpdateProjectUserRequest{
			UserRoles: []string{"project-admin", "app-developer", "xxx"},
		})
		Expect(err).Should(BeEquivalentTo(bcode.ErrProjectRoleCheckFailure))
	})

	It("Test delete project user and delete project function", func() {
		req := apisv1.CreateProjectRequest{
			Name:        "test-project",
			Description: "this is a project description",
		}
		app1 := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config-sync-test-project",
				Namespace: "vela-system",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Type: "aaa",
				}},
			},
		}
		Expect(k8sClient.Create(context.TODO(), app1)).Should(BeNil())

		_, err := projectService.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())

		_, err = projectService.AddProjectUser(context.TODO(), "test-project", apisv1.AddProjectUserRequest{
			UserName:  "admin",
			UserRoles: []string{"project-admin"},
		})
		Expect(err).Should(BeNil())

		err = projectService.DeleteProjectUser(context.TODO(), "test-project", "admin")
		Expect(err).Should(BeNil())
		err = projectService.DeleteProject(context.TODO(), "test-project")
		Expect(err).Should(BeNil())
		perms, err := projectService.RbacService.ListPermissions(context.TODO(), "test-project")
		Expect(err).Should(BeNil())
		Expect(len(perms)).Should(BeEquivalentTo(0))
		roles, err := projectService.RbacService.ListRole(context.TODO(), "test-project", 0, 0)
		Expect(err).Should(BeNil())
		Expect(roles.Total).Should(BeEquivalentTo(0))
	})
})
