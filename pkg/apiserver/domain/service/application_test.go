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
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/repository"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var _ = Describe("Test application service function", func() {
	var (
		rbacService        *rbacServiceImpl
		appService         *applicationServiceImpl
		workflowService    *workflowServiceImpl
		envService         *envServiceImpl
		envBindingService  *envBindingServiceImpl
		targetService      *targetServiceImpl
		definitionService  *definitionServiceImpl
		projectService     *projectServiceImpl
		userService        *userServiceImpl
		testProject        = "app-project"
		testApp            = "test-app"
		defaultTarget      = "default"
		defaultTarget2     = "default2"
		namespace1         = "app-test1"
		namespace2         = "app-test2"
		envnsdev           = "envnsdev"
		envnstest          = "envnstest"
		overridePolicyName = "test-override"
	)

	BeforeEach(func() {
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: "app-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		rbacService = &rbacServiceImpl{Store: ds}
		userService = &userServiceImpl{Store: ds, K8sClient: k8sClient}
		projectService = &projectServiceImpl{Store: ds, K8sClient: k8sClient, RbacService: rbacService}
		envService = &envServiceImpl{Store: ds, KubeClient: k8sClient, ProjectService: projectService}
		workflowService = &workflowServiceImpl{Store: ds, EnvService: envService}
		definitionService = &definitionServiceImpl{KubeClient: k8sClient}
		envBindingService = &envBindingServiceImpl{Store: ds, EnvService: envService, WorkflowService: workflowService, KubeClient: k8sClient, DefinitionService: definitionService}
		targetService = &targetServiceImpl{Store: ds, K8sClient: k8sClient}
		appService = &applicationServiceImpl{
			Store:             ds,
			WorkflowService:   workflowService,
			Apply:             apply.NewAPIApplicator(k8sClient),
			KubeClient:        k8sClient,
			KubeConfig:        cfg,
			EnvBindingService: envBindingService,
			EnvService:        envService,
			DefinitionService: definitionService,
			TargetService:     targetService,
			ProjectService:    projectService,
			UserService:       userService,
		}
	})

	It("Test CreateApplication function", func() {

		By("init default admin user")
		var ns = corev1.Namespace{}
		ns.Name = types.DefaultKubeVelaNS
		err := k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		err = userService.Init(context.TODO())
		Expect(err).Should(BeNil())

		By("prepare test project")
		_, err = projectService.CreateProject(context.TODO(), v1.CreateProjectRequest{Name: testProject, Owner: model.DefaultAdminUserName})
		Expect(err).Should(BeNil())

		_, err = targetService.CreateTarget(context.TODO(), v1.CreateTargetRequest{
			Name: defaultTarget, Project: testProject, Cluster: &v1.ClusterTarget{ClusterName: "local", Namespace: namespace1}})
		Expect(err).Should(BeNil())

		_, err = targetService.CreateTarget(context.TODO(), v1.CreateTargetRequest{
			Name: defaultTarget2, Project: testProject, Cluster: &v1.ClusterTarget{ClusterName: "local", Namespace: namespace2}})
		Expect(err).Should(BeNil())

		_, err = envService.CreateEnv(context.TODO(), v1.CreateEnvRequest{Name: "app-dev", Namespace: envnsdev, Targets: []string{defaultTarget}, Project: testProject})
		Expect(err).Should(BeNil())

		_, err = envService.CreateEnv(context.TODO(), v1.CreateEnvRequest{Name: "app-test", Namespace: envnstest, Targets: []string{defaultTarget2}, Project: testProject})
		Expect(err).Should(BeNil())
		req := v1.CreateApplicationRequest{
			Name:        testApp,
			Project:     testProject,
			Description: "this is a test app",
			EnvBinding: []*v1.EnvBinding{{
				Name: "app-dev",
			}, {
				Name: "app-test",
			}},
			Component: &v1.CreateComponentRequest{
				Name:          "component-name",
				ComponentType: "webservice",
				Properties:    "{\"image\":\"nginx\"}",
			},
		}
		By("test create application")
		base, err := appService.CreateApplication(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())

		triggers, err := appService.ListApplicationTriggers(context.TODO(), &model.Application{Name: testApp})
		Expect(err).Should(BeNil())
		Expect(len(triggers)).Should(Equal(1))

		By("test creating a cloud service application")

		rds, err := os.ReadFile("./testdata/terraform-alibaba-rds.yaml")
		Expect(err).Should(BeNil())
		var cd v1beta1.ComponentDefinition
		err = yaml.Unmarshal(rds, &cd)
		Expect(err).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &cd))

		req2 := v1.CreateApplicationRequest{
			Name:        "test-cloud-application",
			Project:     testProject,
			Description: "this is a cloud service app",
			EnvBinding: []*v1.EnvBinding{{
				Name: "app-dev",
			}},
			Component: &v1.CreateComponentRequest{
				Name:          "rds",
				ComponentType: "alibaba-rds",
				Properties:    "{\"password\":\"test\"}",
			},
		}
		_, err = appService.CreateApplication(context.TODO(), req2)
		Expect(err).Should(BeNil())
		err = appService.DeleteApplication(context.TODO(), &model.Application{Project: testProject, Name: "test-cloud-application"})
		Expect(err).Should(BeNil())
		err = k8sClient.Delete(context.TODO(), &cd)
		Expect(err).Should(BeNil())
	})

	It("Test ListApplications function", func() {
		_, err := appService.ListApplications(context.WithValue(context.TODO(), &v1.CtxKeyUser, model.DefaultAdminUserName), v1.ListApplicationOptions{})
		Expect(err).Should(BeNil())
	})

	It("Test ListApplications and filter by targetName function", func() {
		list, err := appService.ListApplications(context.WithValue(context.TODO(), &v1.CtxKeyUser, model.DefaultAdminUserName), v1.ListApplicationOptions{
			Projects:   []string{testProject},
			TargetName: defaultTarget})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(list), 1)).Should(BeEmpty())
	})

	It("Test DetailApplication function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())

		detail, err := appService.DetailApplication(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(detail.ResourceInfo.ComponentNum, int64(1))).Should(BeEmpty())
		Expect(cmp.Diff(len(detail.Policies), 2)).Should(BeEmpty())
	})

	It("Test CreateTrigger function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		_, err = appService.CreateApplicationTrigger(context.TODO(), appModel, v1.CreateApplicationTriggerRequest{
			Name: "trigger-name",
		})
		Expect(err).Should(BeNil())
		base, err := appService.CreateApplicationTrigger(context.TODO(), appModel, v1.CreateApplicationTriggerRequest{
			Name:          "trigger-name-2",
			ComponentName: "trigger-component",
		})
		Expect(err).Should(BeNil())
		Expect(base.ComponentName).Should(Equal("trigger-component"))
	})

	It("Test ListTriggers function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		triggers, err := appService.ListApplicationTriggers(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(len(triggers)).Should(Equal(3))
	})

	It("Test DeleteTrigger function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		triggers, err := appService.ListApplicationTriggers(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(len(triggers)).Should(Equal(3))
		var trigger *v1.ApplicationTriggerBase
		for _, t := range triggers {
			if t.Name == "trigger-name" {
				trigger = t
				break
			}
		}
		Expect(trigger).ShouldNot(BeNil())
		Expect(appService.DeleteApplicationTrigger(context.TODO(), appModel, trigger.Token)).Should(BeNil())
		triggers, err = appService.ListApplicationTriggers(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(len(triggers)).Should(Equal(2))
		trigger = nil
		for _, t := range triggers {
			if t.Name == "trigger-name" {
				trigger = t
				break
			}
		}
		Expect(trigger).Should(BeNil())
	})

	It("Test ListComponents function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())

		components, err := appService.ListComponents(context.TODO(), appModel, v1.ListApplicationComponentOptions{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 1)).Should(BeEmpty())
		Expect(cmp.Diff(components[0].ComponentType, "webservice")).Should(BeEmpty())
	})

	It("Test CreateComponent function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())

		base, err := appService.CreateComponent(context.TODO(), appModel, v1.CreateComponentRequest{
			Name:          "test2",
			Description:   "this is a test2 component",
			Labels:        map[string]string{},
			ComponentType: "webservice",
			Properties:    `{"image": "busybox","cmd":["sleep", "1000"],"lives": "3","enemies": "alien"}`,
			DependsOn:     []string{"component-name"},
			Traits: []*v1.CreateApplicationTraitRequest{
				{
					Type:       "scaler",
					Alias:      "Scaler",
					Properties: `{"replicas": 2}`,
				},
				{
					Type:        "labels",
					Alias:       "Labels",
					Description: "This is a trait to set labels",
					Properties:  `{"key1": "value1"}`,
				},
			},
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.ComponentType, "webservice")).Should(BeEmpty())

		detailResponse, err := appService.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(detailResponse.Traits), 2)).Should(BeEmpty())
	})

	It("Test DetailComponent function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		detailResponse, err := appService.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(detailResponse.DependsOn[0], "component-name")).Should(BeEmpty())
		Expect(detailResponse.Properties).ShouldNot(BeNil())
		Expect(cmp.Diff((*detailResponse.Properties)["image"], "busybox")).Should(BeEmpty())
	})

	It("Test AddPolicy function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		_, err = appService.CreatePolicy(context.TODO(), appModel, v1.CreatePolicyRequest{
			Name:        overridePolicyName,
			Description: "this is a test2 policy",
			Type:        "override",
			Properties:  `{"components":[{"name":"component-name"}]}`,
		})
		Expect(err).Should(BeNil())

		_, err = appService.CreatePolicy(context.TODO(), appModel, v1.CreatePolicyRequest{
			Name:        overridePolicyName,
			Description: "this is a test2 policy",
			Type:        "override",
			Properties:  ``,
		})
		Expect(cmp.Equal(err, bcode.ErrApplicationPolicyExist, cmpopts.EquateErrors())).Should(BeTrue())
	})

	It("Test ListPolicies function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())

		policies, err := appService.ListPolicies(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		var count int
		for _, p := range policies {
			if p.Type == "override" {
				count++
			}
		}
		Expect(cmp.Diff(count, 1)).Should(BeEmpty())
	})

	It("Test DetailPolicy function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		detail, err := appService.DetailPolicy(context.TODO(), appModel, overridePolicyName)
		Expect(err).Should(BeNil())
		Expect(detail.Properties).ShouldNot(BeNil())
		Expect((*detail.Properties)["components"]).ShouldNot(BeEmpty())
	})

	It("Test UpdatePolicy function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		base, err := appService.UpdatePolicy(context.TODO(), appModel, overridePolicyName, v1.UpdatePolicyRequest{
			Type:       "override",
			Properties: `{"components":{}}`,
		})
		Expect(err).Should(BeNil())
		Expect(base.Properties).ShouldNot(BeNil())
		Expect((*base.Properties)["components"]).Should(BeEmpty())
	})
	It("Test DeletePolicy function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		err = appService.DeletePolicy(context.TODO(), appModel, overridePolicyName, false)
		Expect(err).Should(BeNil())
	})

	It("Test DeleteComponent function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		component, err := appService.GetApplicationComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		err = appService.DeleteComponent(context.TODO(), appModel, component)
		Expect(err).Should(BeNil())
	})

	It("Test ListRevisions function", func() {
		for i := 0; i < 3; i++ {
			appModel := &model.ApplicationRevision{
				AppPrimaryKey: "test-app-sadasd",
				Version:       fmt.Sprintf("%d", i),
				EnvName:       fmt.Sprintf("env-%d", i),
				Status:        model.RevisionStatusRunning,
				DeployUser:    model.DefaultAdminUserName,
			}
			if i == 0 {
				appModel.Status = model.RevisionStatusTerminated
			}
			err := workflowService.createTestApplicationRevision(context.TODO(), appModel)
			Expect(err).Should(BeNil())
		}
		revisions, err := appService.ListRevisions(context.TODO(), "test-app-sadasd", "", "", 0, 10)
		Expect(err).Should(BeNil())
		Expect(revisions.Total).Should(Equal(int64(3)))

		revisions, err = appService.ListRevisions(context.TODO(), "test-app-sadasd", "env-0", "", 0, 10)
		Expect(err).Should(BeNil())
		Expect(revisions.Total).Should(Equal(int64(1)))
		Expect(revisions.Revisions[0].DeployUser.Name).Should(Equal(model.DefaultAdminUserName))
		Expect(revisions.Revisions[0].DeployUser.Alias).Should(Equal(model.DefaultAdminUserAlias))

		revisions, err = appService.ListRevisions(context.TODO(), "test-app-sadasd", "", "terminated", 0, 10)
		Expect(err).Should(BeNil())
		Expect(revisions.Total).Should(Equal(int64(1)))

		revisions, err = appService.ListRevisions(context.TODO(), "test-app", "env-1", "terminated", 0, 10)
		Expect(err).Should(BeNil())
		Expect(revisions.Total).Should(Equal(int64(0)))
	})

	It("Test DetailRevision function", func() {
		err := workflowService.createTestApplicationRevision(context.TODO(), &model.ApplicationRevision{
			AppPrimaryKey: "test-app",
			Version:       "123",
			DeployUser:    model.DefaultAdminUserName,
		})
		Expect(err).Should(BeNil())
		revision, err := appService.DetailRevision(context.TODO(), "test-app", "123")
		Expect(err).Should(BeNil())
		Expect(revision.Version).Should(Equal("123"))
		Expect(revision.DeployUser.Name).Should(Equal(model.DefaultAdminUserName))
		Expect(revision.DeployUser.Alias).Should(Equal(model.DefaultAdminUserAlias))
	})

	It("Test ApplicationEnvRecycle function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		revision, err := appService.Deploy(
			context.WithValue(context.TODO(), &v1.CtxKeyUser, model.DefaultAdminUserName),
			appModel, v1.ApplicationDeployRequest{WorkflowName: repository.ConvertWorkflowName("app-dev")})
		Expect(err).Should(BeNil())
		Expect(revision.DeployUser.Name).Should(Equal(model.DefaultAdminUserName))
		Expect(revision.DeployUser.Alias).Should(Equal(model.DefaultAdminUserAlias))
		err = envBindingService.ApplicationEnvRecycle(context.TODO(), &model.Application{
			Name: testApp,
		}, &model.EnvBinding{Name: "app-dev"})
		Expect(err).Should(BeNil())
	})

	It("Test ListRecords function", func() {
		By("no running records in application")
		ctx := context.TODO()
		for i := 0; i < 2; i++ {
			appService.Store.Add(ctx, &model.WorkflowRecord{
				AppPrimaryKey: "app-records",
				Name:          fmt.Sprintf("list-%d", i),
				Finished:      "true",
				Status:        model.RevisionStatusComplete,
			})
		}

		resp, err := appService.ListRecords(context.TODO(), "app-records")
		Expect(err).Should(BeNil())
		Expect(resp.Total).Should(Equal(int64(1)))

		By("3 running records in application")
		for i := 0; i < 3; i++ {
			appService.Store.Add(ctx, &model.WorkflowRecord{
				AppPrimaryKey: "app-records",
				Name:          fmt.Sprintf("list-running-%d", i),
				Finished:      "false",
				Status:        model.RevisionStatusRunning,
			})
		}

		resp, err = appService.ListRecords(context.TODO(), "app-records")
		Expect(err).Should(BeNil())
		Expect(resp.Total).Should(Equal(int64(3)))
	})

	It("Test CompareApp function", func() {
		check := func(compareResponse *v1.AppCompareResponse, isDiff bool) {
			Expect(cmp.Diff(compareResponse.BaseAppYAML, "")).ShouldNot(BeEmpty())
			Expect(cmp.Diff(compareResponse.TargetAppYAML, "")).ShouldNot(BeEmpty())
			Expect(cmp.Diff(compareResponse.IsDiff, isDiff)).Should(BeEmpty())
		}
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		_, err = appService.Deploy(context.TODO(), appModel, v1.ApplicationDeployRequest{WorkflowName: repository.ConvertWorkflowName("app-dev")})
		Expect(err).Should(BeNil())
		component, err := appService.GetApplicationComponent(context.TODO(), appModel, "component-name")
		Expect(err).Should(BeNil())

		By("compare when app not change, should return false")
		compareResponse, err := appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareRevisionWithRunning: &v1.CompareRevisionWithRunningOption{},
		})
		Expect(err).Should(BeNil())
		check(compareResponse, false)

		compareResponse, err = appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareRevisionWithLatest: &v1.CompareRevisionWithLatestOption{},
		})
		Expect(err).Should(BeNil())
		check(compareResponse, false)

		compareResponse, err = appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareLatestWithRunning: &v1.CompareLatestWithRunningOption{
				Env: "app-dev",
			},
		})
		Expect(err).Should(BeNil())
		check(compareResponse, false)

		By("compare when app add env, not change, should return false")
		_, err = envService.CreateEnv(context.TODO(), v1.CreateEnvRequest{Name: "app-prod", Namespace: "envnsprod", Targets: []string{defaultTarget}, Project: "app-prod"})
		Expect(err).Should(BeNil())
		_, err = envBindingService.CreateEnvBinding(context.TODO(), appModel, v1.CreateApplicationEnvbindingRequest{EnvBinding: v1.EnvBinding{Name: "app-prod"}})
		Expect(err).Should(BeNil())
		compareResponse, err = appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareLatestWithRunning: &v1.CompareLatestWithRunningOption{
				Env: "app-prod",
			},
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(compareResponse.IsDiff, true)).Should(BeEmpty())
		Expect(cmp.Diff(compareResponse.TargetAppYAML, "")).Should(BeEmpty())
		Expect(cmp.Diff(compareResponse.BaseAppYAML, "")).ShouldNot(BeEmpty())

		By("compare when app's env add target, should return true")
		_, err = targetService.CreateTarget(context.TODO(), v1.CreateTargetRequest{Name: "dev-target1", Project: appModel.Project, Cluster: &v1.ClusterTarget{ClusterName: "local", Namespace: "dev-target1"}})
		Expect(err).Should(BeNil())
		_, err = envService.UpdateEnv(context.TODO(), "app-dev",
			v1.UpdateEnvRequest{
				Description: "this is a env description update",
				Targets:     []string{defaultTarget, "dev-target1"},
			})
		Expect(err).Should(BeNil())
		compareResponse, err = appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareLatestWithRunning: &v1.CompareLatestWithRunningOption{
				Env: "app-dev",
			},
		})
		Expect(err).Should(BeNil())
		check(compareResponse, true)

		By("compare when update app's trait, should return true")
		// reset app config
		_, err = appService.ResetAppToLatestRevision(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		_, err = appService.UpdateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "component-name"}, "scaler", v1.UpdateApplicationTraitRequest{
			Properties:  `{"replicas":2}`,
			Alias:       "alias",
			Description: "description",
		})
		Expect(err).Should(BeNil())
		compareResponse, err = appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareRevisionWithLatest: &v1.CompareRevisionWithLatestOption{},
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(compareResponse.IsDiff, true)).Should(BeEmpty())

		compareResponse, err = appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareLatestWithRunning: &v1.CompareLatestWithRunningOption{
				Env: "app-dev",
			},
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(compareResponse.IsDiff, true)).Should(BeEmpty())

		By("compare when update component's target after app deployed ,should return true")
		// reset app config
		_, err = appService.ResetAppToLatestRevision(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		newProperties := "{\"exposeType\":\"NodePort\",\"image\":\"nginx\",\"imagePullPolicy\":\"Always\"}"
		_, err = appService.UpdateComponent(context.TODO(),
			appModel,
			component,
			v1.UpdateApplicationComponentRequest{
				Properties: &newProperties,
			})
		Expect(err).Should(BeNil())
		compareResponse, err = appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareRevisionWithLatest: &v1.CompareRevisionWithLatestOption{},
		})
		Expect(err).Should(BeNil())
		check(compareResponse, true)

		compareResponse, err = appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareLatestWithRunning: &v1.CompareLatestWithRunningOption{
				Env: "app-dev",
			},
		})
		Expect(err).Should(BeNil())
		check(compareResponse, true)

		compareResponse, err = appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareRevisionWithRunning: &v1.CompareRevisionWithRunningOption{},
		})

		Expect(err).Should(BeNil())
		check(compareResponse, false)

		By("compare when changed the application CR, should return true")

		appCR, err := appService.GetApplicationCRInEnv(context.TODO(), appModel, "app-dev")
		Expect(err).Should(BeNil())
		appCR.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte("{\"exposeType\":\"NodePort\",\"image\":\"nginx:222\",\"imagePullPolicy\":\"Always\"}")}
		err = k8sClient.Update(context.TODO(), appCR)
		Expect(err).Should(BeNil())

		compareResponse, err = appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareRevisionWithRunning: &v1.CompareRevisionWithRunningOption{},
		})
		Expect(err).Should(BeNil())
		check(compareResponse, true)

		compareResponse, err = appService.CompareApp(context.TODO(), appModel, v1.AppCompareReq{
			CompareLatestWithRunning: &v1.CompareLatestWithRunningOption{
				Env: "app-dev",
			},
		})
		Expect(err).Should(BeNil())
		check(compareResponse, true)

		err = envBindingService.ApplicationEnvRecycle(context.TODO(), &model.Application{Name: testApp}, &model.EnvBinding{Name: "app-dev"})
		Expect(err).Should(BeNil())
	})

	It("Test ResetAppToLatestRevision function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		resetResponse, err := appService.ResetAppToLatestRevision(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(resetResponse.IsReset, true)).Should(BeEmpty())
		component, err := appService.GetApplicationComponent(context.TODO(), appModel, "component-name")
		Expect(err).Should(BeNil())
		expectProperties := "{\"image\":\"nginx\"}"
		Expect(cmp.Diff(component.Properties.JSON(), expectProperties)).Should(BeEmpty())
	})

	It("Test DryRun with app function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		resetResponse, err := appService.DryRunAppOrRevision(context.TODO(), appModel, v1.AppDryRunReq{DryRunType: "APP"})
		Expect(err).Should(BeNil())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app)")).Should(BeTrue())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app) -- Component(component-name)")).Should(BeTrue())
	})

	It("Test DryRun with env revision function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		resetResponse, err := appService.DryRunAppOrRevision(context.TODO(), appModel, v1.AppDryRunReq{DryRunType: "REVISION", Env: "app-dev"})
		Expect(err).Should(BeNil())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app)")).Should(BeTrue())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app) -- Component(component-name)")).Should(BeTrue())
	})

	It("Test DryRun with last revision function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		resetResponse, err := appService.DryRunAppOrRevision(context.TODO(), appModel, v1.AppDryRunReq{DryRunType: "REVISION"})
		Expect(err).Should(BeNil())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app)")).Should(BeTrue())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app) -- Component(component-name)")).Should(BeTrue())
	})

	It("Test DeleteApplication function", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		time.Sleep(time.Second * 3)
		err = appService.DeleteApplication(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		components, err := appService.ListComponents(context.TODO(), appModel, v1.ListApplicationComponentOptions{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 0)).Should(BeEmpty())
		policies, err := appService.ListPolicies(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(policies), 0)).Should(BeEmpty())
	})
})

var _ = Describe("Test application component service function", func() {

	var (
		appService     *applicationServiceImpl
		projectService *projectServiceImpl
		envService     *envServiceImpl
		testApp        string
		testProject    string
	)

	BeforeEach(func() {
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: "app-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		rbacService := &rbacServiceImpl{Store: ds}
		projectService = &projectServiceImpl{Store: ds, K8sClient: k8sClient, RbacService: rbacService}
		envService = &envServiceImpl{Store: ds, KubeClient: k8sClient, ProjectService: projectService}
		workflowService := &workflowServiceImpl{Store: ds, EnvService: envService}
		envBindingService := &envBindingServiceImpl{Store: ds, EnvService: envService, WorkflowService: workflowService, KubeClient: k8sClient}

		appService = &applicationServiceImpl{
			Store:             ds,
			Apply:             apply.NewAPIApplicator(k8sClient),
			KubeClient:        k8sClient,
			ProjectService:    projectService,
			WorkflowService:   workflowService,
			EnvBindingService: envBindingService,
			EnvService:        envService,
		}
		testApp = "test-trait-app"
		testProject = "test-trait-project"

	})

	It("Test add application trait", func() {
		_, err := projectService.CreateProject(context.TODO(), v1.CreateProjectRequest{Name: testProject})
		Expect(err).Should(BeNil())
		_, err = appService.CreateApplication(context.TODO(), v1.CreateApplicationRequest{Name: testApp, Project: testProject})
		Expect(err).Should(BeNil())
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		_, err = appService.CreateComponent(context.TODO(), appModel, v1.CreateComponentRequest{Name: "test2", ComponentType: "webservice"})
		Expect(err).Should(BeNil())
		alias := "alias"
		description := "description"
		res, err := appService.CreateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, v1.CreateApplicationTraitRequest{
			Type:        "Ingress",
			Properties:  `{"domain":"www.test.com"}`,
			Alias:       alias,
			Description: description,
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(res.Type, "Ingress")).Should(BeEmpty())
		comp, err := appService.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(comp).ShouldNot(BeNil())
		// A scaler trait is automatically generated for the webservice component.
		Expect(len(comp.Traits)).Should(BeEquivalentTo(2))
		Expect(comp.Traits[1].Properties.JSON()).Should(BeEquivalentTo(`{"domain":"www.test.com"}`))
		Expect(comp.Traits[1].Alias).Should(BeEquivalentTo(alias))
		Expect(comp.Traits[1].Description).Should(BeEquivalentTo(description))

		Expect(err).Should(BeNil())
		_, err = appService.CreateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, v1.CreateApplicationTraitRequest{
			Type:       "Ingress",
			Properties: `{"domain":"www.dup.com"}`,
		})
		Expect(err).ShouldNot(BeNil())
	})

	It("Test update application trait", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		alias := "newAlias"
		description := "newDescription"
		res, err := appService.UpdateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, "Ingress", v1.UpdateApplicationTraitRequest{
			Properties:  `{"domain":"www.test1.com"}`,
			Alias:       alias,
			Description: description,
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(res.Type, "Ingress")).Should(BeEmpty())
		comp, err := appService.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(comp).ShouldNot(BeNil())
		Expect(len(comp.Traits)).Should(BeEquivalentTo(2))
		Expect(comp.Traits[1].Properties.JSON()).Should(BeEquivalentTo(`{"domain":"www.test1.com"}`))
		Expect(comp.Traits[1].Alias).Should(BeEquivalentTo(alias))
		Expect(comp.Traits[1].Description).Should(BeEquivalentTo(description))
	})

	It("Test update a not exist", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		_, err = appService.UpdateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, "Ingress-1-20", v1.UpdateApplicationTraitRequest{
			Properties: `{"domain":"www.test1.com"}`,
		})
		Expect(err).ShouldNot(BeNil())
	})

	It("Test delete an exist trait", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		err = appService.DeleteApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, "Ingress")
		Expect(err).Should(BeNil())
		app, err := appService.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(app).ShouldNot(BeNil())
		Expect(len(app.Traits)).Should(BeEquivalentTo(1))
	})
})

var _ = Describe("Test apiserver policy rest api", func() {
	var (
		appService     *applicationServiceImpl
		projectService *projectServiceImpl
		envService     *envServiceImpl
		testApp        string
		testProject    string
		ctx            context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: "app-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		rbacService := &rbacServiceImpl{Store: ds}
		projectService = &projectServiceImpl{Store: ds, K8sClient: k8sClient, RbacService: rbacService}
		envService = &envServiceImpl{Store: ds, KubeClient: k8sClient, ProjectService: projectService}
		workflowService := &workflowServiceImpl{Store: ds, EnvService: envService}
		envBindingService := &envBindingServiceImpl{Store: ds, EnvService: envService, WorkflowService: workflowService, KubeClient: k8sClient}

		appService = &applicationServiceImpl{
			Store:             ds,
			Apply:             apply.NewAPIApplicator(k8sClient),
			KubeClient:        k8sClient,
			ProjectService:    projectService,
			WorkflowService:   workflowService,
			EnvBindingService: envBindingService,
			EnvService:        envService,
		}
		testApp = "app-policy-workflow-binding"
		testProject = "project-policy-workflow-binding"
	})

	It("Test add policy", func() {
		_, err := projectService.CreateProject(context.TODO(), v1.CreateProjectRequest{Name: testProject})
		Expect(err).Should(BeNil())
		_, err = appService.CreateApplication(context.TODO(), v1.CreateApplicationRequest{Name: testApp, Project: testProject})
		Expect(err).Should(BeNil())
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())

		workflow := v1.CreateWorkflowRequest{
			Name:    "default",
			EnvName: "default",
			Steps: []v1.WorkflowStep{
				{
					Name:       "default",
					Type:       "deploy",
					Properties: `{"policies":["local"]}`,
				},
				{
					Name:       "suspend",
					Type:       "suspend",
					Properties: `{"duration": "10m"}`,
				},
				{
					Name:       "second",
					Type:       "deploy",
					Properties: `{"policies":["cluster1"]}`,
				},
			},
		}
		_, err = appService.WorkflowService.CreateOrUpdateWorkflow(ctx, appModel, workflow)
		Expect(err).Should(BeNil())

		workflow2 := v1.CreateWorkflowRequest{
			Name:    "second",
			EnvName: "default",
			Steps: []v1.WorkflowStep{
				{
					Name:       "second",
					Type:       "deploy",
					Properties: `{"policies":["cluster3"]}`,
				},
			},
		}
		_, err = appService.WorkflowService.CreateOrUpdateWorkflow(ctx, appModel, workflow2)
		Expect(err).Should(BeNil())

		policyReq := v1.CreatePolicyRequest{
			Name:       "override1",
			Type:       "override",
			Properties: `{"components": [{"image": "busybox","cmd":["sleep", "1000"],"lives": "3","enemies": "alien"}]}`,
			WorkflowPolicyBindings: []v1.WorkflowPolicyBinding{
				{
					Name:  "default",
					Steps: []string{"default"},
				},
			},
		}
		_, err = appService.CreatePolicy(ctx, appModel, policyReq)
		Expect(err).Should(BeNil())

		checkWorkflow, err := appService.WorkflowService.GetWorkflow(ctx, appModel, "default")
		Expect(err).Should(BeNil())
		checkRes, err := json.Marshal(checkWorkflow.Steps[0].Properties)
		Expect(err).Should(BeNil())
		Expect(string(checkRes)).Should(BeEquivalentTo(`{"policies":["local","override1"]}`))

		// guarantee the suspend workflow step shouldn't be changed
		suspendStep := checkWorkflow.Steps[1]
		Expect(suspendStep.Name).Should(BeEquivalentTo("suspend"))
		Expect(suspendStep.Type).Should(BeEquivalentTo("suspend"))
		suspendPropertyStr, err := json.Marshal(suspendStep.Properties)
		Expect(err).Should(BeNil())
		Expect(string(suspendPropertyStr)).Should(BeEquivalentTo(`{"duration":"10m"}`))
	})

	It("Update policy to more workflow Step", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		policyName := "override1"
		policyRes, err := appService.DetailPolicy(ctx, appModel, policyName)
		Expect(err).Should(BeNil())
		propertyStr, err := json.Marshal(policyRes.Properties)
		Expect(err).Should(BeNil())
		updatePolicyReq := v1.UpdatePolicyRequest{
			Description: policyRes.Description,
			Type:        policyRes.Type,
			Properties:  string(propertyStr),
			WorkflowPolicyBindings: []v1.WorkflowPolicyBinding{
				{
					Name:  "second",
					Steps: []string{"second"},
				},
			},
		}
		_, err = appService.UpdatePolicy(ctx, appModel, policyName, updatePolicyReq)
		Expect(err).Should(BeNil())

		checkWorkflow, err := appService.WorkflowService.GetWorkflow(ctx, appModel, "default")
		Expect(err).Should(BeNil())
		checkRes, err := json.Marshal(checkWorkflow.Steps[0].Properties)
		Expect(err).Should(BeNil())
		Expect(string(checkRes)).Should(BeEquivalentTo(`{"policies":["local"]}`))

		checkWorkflow, err = appService.WorkflowService.GetWorkflow(ctx, appModel, "second")
		Expect(err).Should(BeNil())
		checkRes, err = json.Marshal(checkWorkflow.Steps[0].Properties)
		Expect(err).Should(BeNil())
		Expect(string(checkRes)).Should(BeEquivalentTo(`{"policies":["cluster3","override1"]}`))

		policyRes, err = appService.DetailPolicy(ctx, appModel, policyName)
		Expect(err).Should(BeNil())
		Expect(policyRes.WorkflowPolicyBindings).Should(BeEquivalentTo([]v1.WorkflowPolicyBinding{
			{
				Name:  "second",
				Steps: []string{"second"},
			},
		}))
	})

	It("Exist binding will block policy delete operation", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		policyName := "override1"
		_, err = appService.DetailPolicy(ctx, appModel, policyName)
		Expect(err).Should(BeNil())
		err = appService.DeletePolicy(ctx, appModel, policyName, false)
		Expect(err).ShouldNot(BeNil())
	})

	It("Force delete policy will delete policy workflow binding", func() {
		appModel, err := appService.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		policyName := "override1"

		// try delete again
		err = appService.DeletePolicy(ctx, appModel, policyName, true)
		Expect(err).Should(BeNil())

		// check workflow bindings and policy has been removed
		checkWorkflow, err := appService.WorkflowService.GetWorkflow(ctx, appModel, "default")
		Expect(err).Should(BeNil())
		checkRes, err := json.Marshal(checkWorkflow.Steps[0].Properties)
		Expect(err).Should(BeNil())
		Expect(string(checkRes)).Should(BeEquivalentTo(`{"policies":["local"]}`))

		checkWorkflow, err = appService.WorkflowService.GetWorkflow(ctx, appModel, "second")
		Expect(err).Should(BeNil())
		checkRes, err = json.Marshal(checkWorkflow.Steps[0].Properties)
		Expect(err).Should(BeNil())
		Expect(string(checkRes)).Should(BeEquivalentTo(`{"policies":["cluster3"]}`))
	})
})

func createTestSuspendApp(ctx context.Context, appName, envName, revisionVersion, wfName, recordName string, kubeClient client.Client) (*v1beta1.Application, error) {
	testapp := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: envName,
			Annotations: map[string]string{
				oam.AnnotationDeployVersion:  revisionVersion,
				oam.AnnotationWorkflowName:   wfName,
				oam.AnnotationPublishVersion: recordName,
				oam.AnnotationAppName:        appName,
			},
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name:       "test-component",
				Type:       "webservice",
				Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx"}`)},
				Traits:     []common.ApplicationTrait{},
				Scopes:     map[string]string{},
			}},
		},
		Status: common.AppStatus{
			Workflow: &common.WorkflowStatus{
				Suspend:     true,
				AppRevision: recordName,
			},
		},
	}

	if err := kubeClient.Create(ctx, testapp.DeepCopy()); err != nil {
		return nil, err
	}
	if err := kubeClient.Status().Patch(ctx, testapp, client.Merge); err != nil {
		return nil, err
	}

	return testapp, nil
}
