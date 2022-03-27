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

package usecase

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test project usecase functions", func() {
	var (
		projectUsecase   *projectUsecaseImpl
		envImpl          *envUsecaseImpl
		userUsecase      *userUsecaseImpl
		targetImpl       *targetUsecaseImpl
		defaultNamespace = "project-default-ns1-test"
	)
	BeforeEach(func() {
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: "target-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		userUsecase = &userUsecaseImpl{ds: ds, k8sClient: k8sClient}
		var ns = corev1.Namespace{}
		ns.Name = defaultNamespace
		err = k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		projectUsecase = &projectUsecaseImpl{k8sClient: k8sClient, ds: ds, rbacUsecase: &rbacUsecaseImpl{ds: ds}}
		pp, err := projectUsecase.ListProjects(context.TODO(), 0, 0)
		Expect(err).Should(BeNil())
		// reset all projects
		for _, p := range pp.Projects {
			_ = projectUsecase.DeleteProject(context.TODO(), p.Name)
		}

		envImpl = &envUsecaseImpl{kubeClient: k8sClient, ds: ds, projectUsecase: projectUsecase}
		ctx := context.WithValue(context.TODO(), &apisv1.CtxKeyUser, "admin")
		envs, err := envImpl.ListEnvs(ctx, 0, 0, apisv1.ListEnvOptions{})
		Expect(err).Should(BeNil())
		// reset all projects
		for _, e := range envs.Envs {
			_ = envImpl.DeleteEnv(context.TODO(), e.Name)
		}
		targetImpl = &targetUsecaseImpl{k8sClient: k8sClient, ds: ds}
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
		err = userUsecase.Init(context.TODO())
		Expect(err).Should(BeNil())

		// init default project
		err = projectUsecase.InitDefaultProjectEnvTarget(context.WithValue(context.TODO(), &apisv1.CtxKeyUser, model.DefaultAdminUserName), defaultNamespace)
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
		dp, err := projectUsecase.GetProject(context.TODO(), model.DefaultInitName)
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
		base, err := projectUsecase.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())
		_, err = projectUsecase.ListProjects(context.TODO(), 0, 0)
		Expect(err).Should(BeNil())
		projectUsecase.DeleteProject(context.TODO(), "test-project")
	})

	It("Test Update project function", func() {
		req := apisv1.CreateProjectRequest{
			Name:        "test-project",
			Description: "this is a project description",
		}
		_, err := projectUsecase.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())

		base, err := projectUsecase.UpdateProject(context.TODO(), "test-project", apisv1.UpdateProjectRequest{
			Alias:       "Change alias",
			Description: "Change description",
			Owner:       "admin",
		})
		Expect(err).Should(BeNil())
		Expect(base.Alias).Should(BeEquivalentTo("Change alias"))
		Expect(base.Description).Should(BeEquivalentTo("Change description"))
		Expect(base.Owner.Alias).Should(BeEquivalentTo("Administrator"))

		_, err = projectUsecase.UpdateProject(context.TODO(), "test-project", apisv1.UpdateProjectRequest{
			Alias:       "Change alias",
			Description: "Change description",
			Owner:       "admin-error",
		})
		Expect(err).Should(BeEquivalentTo(bcode.ErrProjectOwnerIsNotExist))
		err = projectUsecase.DeleteProject(context.TODO(), "test-project")
		Expect(err).Should(BeNil())
	})

	It("Test Create project user function", func() {
		req := apisv1.CreateProjectRequest{
			Name:        "test-project",
			Description: "this is a project description",
		}
		_, err := projectUsecase.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())

		_, err = projectUsecase.AddProjectUser(context.TODO(), "test-project", apisv1.AddProjectUserRequest{
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
		_, err := projectUsecase.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())

		_, err = projectUsecase.AddProjectUser(context.TODO(), "test-project", apisv1.AddProjectUserRequest{
			UserName:  "admin",
			UserRoles: []string{"project-admin"},
		})
		Expect(err).Should(BeNil())

		_, err = projectUsecase.UpdateProjectUser(context.TODO(), "test-project", "admin", apisv1.UpdateProjectUserRequest{
			UserRoles: []string{"project-admin", "app-developer"},
		})
		Expect(err).Should(BeNil())

		_, err = projectUsecase.UpdateProjectUser(context.TODO(), "test-project", "admin", apisv1.UpdateProjectUserRequest{
			UserRoles: []string{"project-admin", "app-developer", "xxx"},
		})
		Expect(err).Should(BeEquivalentTo(bcode.ErrProjectRoleCheckFailure))
	})

	It("Test delete project user and delete project function", func() {
		req := apisv1.CreateProjectRequest{
			Name:        "test-project",
			Description: "this is a project description",
		}
		_, err := projectUsecase.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())

		_, err = projectUsecase.AddProjectUser(context.TODO(), "test-project", apisv1.AddProjectUserRequest{
			UserName:  "admin",
			UserRoles: []string{"project-admin"},
		})
		Expect(err).Should(BeNil())

		err = projectUsecase.DeleteProjectUser(context.TODO(), "test-project", "admin")
		Expect(err).Should(BeNil())
		err = projectUsecase.DeleteProject(context.TODO(), "test-project")
		Expect(err).Should(BeNil())
		perms, err := projectUsecase.rbacUsecase.ListPermissions(context.TODO(), "test-project")
		Expect(err).Should(BeNil())
		Expect(len(perms)).Should(BeEquivalentTo(0))
		roles, err := projectUsecase.rbacUsecase.ListRole(context.TODO(), "test-project", 0, 0)
		Expect(err).Should(BeNil())
		Expect(roles.Total).Should(BeEquivalentTo(0))
	})
})
