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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	terraformtypes "github.com/oam-dev/terraform-controller/api/types"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

		projectService = &projectServiceImpl{K8sClient: k8sClient, Store: ds, RbacService: &rbacServiceImpl{Store: ds}}
		pp, err := projectService.ListProjects(context.TODO(), 0, 0)
		Expect(err).Should(BeNil())
		// reset all projects
		for _, p := range pp.Projects {
			_ = projectService.DeleteProject(context.TODO(), p.Name)
		}

		envImpl = &envServiceImpl{KubeClient: k8sClient, Store: ds, ProjectService: projectService}
		ctx := context.WithValue(context.TODO(), &apisv1.CtxKeyUser, "admin")
		envs, err := envImpl.ListEnvs(ctx, 0, 0, apisv1.ListEnvOptions{})
		Expect(err).Should(BeNil())
		// reset all projects
		for _, e := range envs.Envs {
			_ = envImpl.DeleteEnv(context.TODO(), e.Name)
		}
		targetImpl = &targetServiceImpl{K8sClient: k8sClient, Store: ds}
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

func TestProjectGetConfigs(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	corev1.AddToScheme(s)
	terraformapi.AddToScheme(s)

	createdTime, _ := time.Parse(time.UnixDate, "Wed Apr 7 11:06:39 PST 2022")

	app1 := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "a1",
			Namespace: velatypes.DefaultKubeVelaNS,
			Labels: map[string]string{
				model.LabelSourceOfTruth:     model.FromInner,
				velatypes.LabelConfigCatalog: velatypes.VelaCoreConfig,
				velatypes.LabelConfigType:    "terraform-provider",
				"config.oam.dev/project":     "p1",
			},
			CreationTimestamp: metav1.NewTime(createdTime),
		},
		Status: common.AppStatus{Phase: common.ApplicationRunning},
	}

	app2 := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "a2",
			Namespace: velatypes.DefaultKubeVelaNS,
			Labels: map[string]string{
				model.LabelSourceOfTruth:     model.FromInner,
				velatypes.LabelConfigCatalog: velatypes.VelaCoreConfig,
				velatypes.LabelConfigType:    "terraform-provider",
			},
			CreationTimestamp: metav1.NewTime(createdTime),
		},
		Status: common.AppStatus{Phase: common.ApplicationRunning},
	}

	app3 := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "a3",
			Namespace: velatypes.DefaultKubeVelaNS,
			Labels: map[string]string{
				model.LabelSourceOfTruth:     model.FromInner,
				velatypes.LabelConfigCatalog: velatypes.VelaCoreConfig,
				velatypes.LabelConfigType:    "dex-connector",
				"config.oam.dev/project":     "p3",
			},
			CreationTimestamp: metav1.NewTime(createdTime),
		},
		Status: common.AppStatus{Phase: common.ApplicationRunning},
	}

	provider1 := &terraformapi.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "provider1",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(createdTime),
		},
		Status: terraformapi.ProviderStatus{
			State: terraformtypes.ProviderIsReady,
		},
	}

	provider2 := &terraformapi.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provider2",
			Namespace: "default",
			Labels: map[string]string{
				velatypes.LabelConfigCatalog: velatypes.VelaCoreConfig,
			},
		},
		Status: terraformapi.ProviderStatus{
			State: terraformtypes.ProviderIsNotReady,
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(app1, app2, app3, provider1, provider2).Build()

	h := &projectServiceImpl{K8sClient: k8sClient}

	type args struct {
		projectName string
		configType  string
		h           ProjectService
	}

	type want struct {
		configs []*apisv1.Config
		errMsg  string
	}

	ctx := context.Background()

	testcases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "project is matched",
			args: args{
				projectName: "p1",
				configType:  "terraform-provider",
				h:           h,
			},
			want: want{
				configs: []*apisv1.Config{{
					ConfigType:        "terraform-provider",
					Name:              "a1",
					Project:           "p1",
					CreatedTime:       &createdTime,
					ApplicationStatus: "running",
					Status:            "Ready",
				}, {
					ConfigType:        "terraform-provider",
					Name:              "a2",
					Project:           "",
					CreatedTime:       &createdTime,
					ApplicationStatus: "running",
					Status:            "Ready",
				}, {
					Name:        "provider1",
					CreatedTime: &createdTime,
					Status:      "Ready",
				}},
			},
		},
		{
			name: "project is not matched",
			args: args{
				projectName: "p999",
				configType:  "terraform-provider",
				h:           h,
			},
			want: want{
				configs: []*apisv1.Config{{
					ConfigType:        "terraform-provider",
					Name:              "a2",
					Project:           "",
					CreatedTime:       &createdTime,
					ApplicationStatus: "running",
					Status:            "Ready",
				}, {
					Name:        "provider1",
					CreatedTime: &createdTime,
					Status:      "Ready",
				}},
			},
		},
		{
			name: "config type is empty",
			args: args{
				projectName: "p3",
				configType:  "",
				h:           h,
			},
			want: want{
				configs: []*apisv1.Config{{
					ConfigType:        "terraform-provider",
					Name:              "a2",
					Project:           "",
					CreatedTime:       &createdTime,
					ApplicationStatus: "running",
					Status:            "Ready",
				}, {
					ConfigType:        "dex-connector",
					Name:              "a3",
					Project:           "p3",
					CreatedTime:       &createdTime,
					ApplicationStatus: "running",
					Status:            "Ready",
				}, {
					Name:        "provider1",
					CreatedTime: &createdTime,
					Status:      "Ready",
				}},
			},
		},
		{
			name: "config type is dex",
			args: args{
				projectName: "p3",
				configType:  "config-dex-connector",
				h:           h,
			},
			want: want{
				configs: []*apisv1.Config{{
					ConfigType:        "dex-connector",
					Name:              "a3",
					Project:           "p3",
					CreatedTime:       &createdTime,
					ApplicationStatus: "running",
					Status:            "Ready",
				}},
			},
		},
		{
			name: "config type is invalid",
			args: args{
				configType: "xxx",
				h:          h,
			},
			want: want{
				errMsg: "unsupported config type",
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.args.h.GetConfigs(ctx, tc.args.projectName, tc.args.configType)
			if tc.want.errMsg != "" || err != nil {
				assert.ErrorContains(t, err, tc.want.errMsg)
			}
			assert.DeepEqual(t, got, tc.want.configs)
		})
	}
}

func TestValidateImage(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	corev1.AddToScheme(s)
	terraformapi.AddToScheme(s)

	s1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s1",
			Namespace: velatypes.DefaultKubeVelaNS,
			Labels: map[string]string{
				velatypes.LabelConfigCatalog:    velatypes.VelaCoreConfig,
				velatypes.LabelConfigType:       velatypes.ImageRegistry,
				velatypes.LabelConfigProject:    "",
				velatypes.LabelConfigIdentifier: "abce34289jwerojwerofaf77.com789",
			},
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"abce34289jwerojwerofaf77.com789":{"auth":"aHlicmlkY2xvdWRAcHJvZC5YTEyMw==","username":"xxx","password":"yyy"}}}`),
		},
	}
	k8sClient1 := fake.NewClientBuilder().WithScheme(s).WithObjects(s1).Build()
	h1 := &projectServiceImpl{K8sClient: k8sClient1}

	s2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s2",
			Namespace: velatypes.DefaultKubeVelaNS,
			Labels: map[string]string{
				velatypes.LabelConfigCatalog:    velatypes.VelaCoreConfig,
				velatypes.LabelConfigType:       velatypes.ImageRegistry,
				velatypes.LabelConfigProject:    "",
				velatypes.LabelConfigIdentifier: "index.docker.io",
			},
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"index.docker.io":{"auth":"aHlicmlkY2xvdWRAcHJvZC5YTEyMw==","username":"xxx","password":"yyy"}}}`),
		},
	}

	k8sClient2 := fake.NewClientBuilder().WithScheme(s).WithObjects(s2).Build()
	h2 := &projectServiceImpl{K8sClient: k8sClient2}

	type args struct {
		project   string
		imageName string
		h         ProjectService
	}

	type want struct {
		resp   *apisv1.ImageResponse
		errMsg string
	}

	ctx := context.Background()

	testcases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "validate image",
			args: args{
				project:   "p1",
				imageName: "nginx",
				h:         h1,
			},
			want: want{
				resp: &apisv1.ImageResponse{
					Existed: true,
				},
			},
		},
		{
			name: "invalid image",
			args: args{
				project:   "p1",
				imageName: "abce34289jwerojwerofaf77.com789/d/e:v1",
				h:         h1,
			},
			want: want{
				errMsg: "Get",
			},
		},
		{
			name: "private docker image",
			args: args{
				project:   "p1",
				imageName: "nginx424ru823-should-not-existed",
				h:         h2,
			},
			want: want{
				errMsg: "incorrect username or password",
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.args.h.ValidateImage(ctx, tc.args.project, tc.args.imageName)
			if tc.want.errMsg != "" || err != nil {
				assert.ErrorContains(t, err, tc.want.errMsg)
			}
			assert.DeepEqual(t, got, tc.want.resp)
		})
	}
}
