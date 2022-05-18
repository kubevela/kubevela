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
	"fmt"
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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var _ = Describe("Test application usecase function", func() {
	var (
		rbacUsecase        *rbacUsecaseImpl
		appUsecase         *applicationUsecaseImpl
		workflowUsecase    *workflowUsecaseImpl
		envUsecase         *envUsecaseImpl
		envBindingUsecase  *envBindingUsecaseImpl
		targetUsecase      *targetUsecaseImpl
		definitionUsecase  *definitionUsecaseImpl
		projectUsecase     *projectUsecaseImpl
		userUsecase        *userUsecaseImpl
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
		rbacUsecase = &rbacUsecaseImpl{ds: ds}
		userUsecase = &userUsecaseImpl{ds: ds, k8sClient: k8sClient}
		projectUsecase = &projectUsecaseImpl{ds: ds, k8sClient: k8sClient, rbacUsecase: rbacUsecase}
		envUsecase = &envUsecaseImpl{ds: ds, kubeClient: k8sClient, projectUsecase: projectUsecase}
		workflowUsecase = &workflowUsecaseImpl{ds: ds, envUsecase: envUsecase}
		definitionUsecase = &definitionUsecaseImpl{kubeClient: k8sClient, caches: utils.NewMemoryCacheStore(context.Background())}
		envBindingUsecase = &envBindingUsecaseImpl{ds: ds, envUsecase: envUsecase, workflowUsecase: workflowUsecase, kubeClient: k8sClient, definitionUsecase: definitionUsecase}
		targetUsecase = &targetUsecaseImpl{ds: ds, k8sClient: k8sClient}
		appUsecase = &applicationUsecaseImpl{
			ds:                ds,
			workflowUsecase:   workflowUsecase,
			apply:             apply.NewAPIApplicator(k8sClient),
			kubeClient:        k8sClient,
			kubeConfig:        cfg,
			envBindingUsecase: envBindingUsecase,
			envUsecase:        envUsecase,
			definitionUsecase: definitionUsecase,
			targetUsecase:     targetUsecase,
			projectUsecase:    projectUsecase,
			userUsecase:       userUsecase,
		}
	})

	It("Test CreateApplication function", func() {

		By("init default admin user")
		var ns = corev1.Namespace{}
		ns.Name = types.DefaultKubeVelaNS
		err := k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		err = userUsecase.Init(context.TODO())
		Expect(err).Should(BeNil())

		By("prepare test project")
		_, err = projectUsecase.CreateProject(context.TODO(), v1.CreateProjectRequest{Name: testProject, Owner: model.DefaultAdminUserName})
		Expect(err).Should(BeNil())

		_, err = targetUsecase.CreateTarget(context.TODO(), v1.CreateTargetRequest{
			Name: defaultTarget, Project: testProject, Cluster: &v1.ClusterTarget{ClusterName: "local", Namespace: namespace1}})
		Expect(err).Should(BeNil())

		_, err = targetUsecase.CreateTarget(context.TODO(), v1.CreateTargetRequest{
			Name: defaultTarget2, Project: testProject, Cluster: &v1.ClusterTarget{ClusterName: "local", Namespace: namespace2}})
		Expect(err).Should(BeNil())

		_, err = envUsecase.CreateEnv(context.TODO(), v1.CreateEnvRequest{Name: "app-dev", Namespace: envnsdev, Targets: []string{defaultTarget}, Project: testProject})
		Expect(err).Should(BeNil())

		_, err = envUsecase.CreateEnv(context.TODO(), v1.CreateEnvRequest{Name: "app-test", Namespace: envnstest, Targets: []string{defaultTarget2}, Project: testProject})
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
		base, err := appUsecase.CreateApplication(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())

		triggers, err := appUsecase.ListApplicationTriggers(context.TODO(), &model.Application{Name: testApp})
		Expect(err).Should(BeNil())
		Expect(len(triggers)).Should(Equal(1))
	})

	It("Test ListApplications function", func() {
		_, err := appUsecase.ListApplications(context.WithValue(context.TODO(), &v1.CtxKeyUser, model.DefaultAdminUserName), v1.ListApplicationOptions{})
		Expect(err).Should(BeNil())
	})

	It("Test ListApplications and filter by targetName function", func() {
		list, err := appUsecase.ListApplications(context.WithValue(context.TODO(), &v1.CtxKeyUser, model.DefaultAdminUserName), v1.ListApplicationOptions{
			Projects:   []string{testProject},
			TargetName: defaultTarget})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(list), 1)).Should(BeEmpty())
	})

	It("Test DetailApplication function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())

		detail, err := appUsecase.DetailApplication(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(detail.ResourceInfo.ComponentNum, int64(1))).Should(BeEmpty())
		Expect(cmp.Diff(len(detail.Policies), 2)).Should(BeEmpty())
	})

	It("Test CreateTrigger function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		_, err = appUsecase.CreateApplicationTrigger(context.TODO(), appModel, v1.CreateApplicationTriggerRequest{
			Name: "trigger-name",
		})
		Expect(err).Should(BeNil())
	})

	It("Test ListTriggers function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		triggers, err := appUsecase.ListApplicationTriggers(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(len(triggers)).Should(Equal(2))
	})

	It("Test DeleteTrigger function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		triggers, err := appUsecase.ListApplicationTriggers(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(len(triggers)).Should(Equal(2))
		var trigger *v1.ApplicationTriggerBase
		for _, t := range triggers {
			if t.Name == "trigger-name" {
				trigger = t
				break
			}
		}
		Expect(trigger).ShouldNot(BeNil())
		Expect(appUsecase.DeleteApplicationTrigger(context.TODO(), appModel, trigger.Token)).Should(BeNil())
		triggers, err = appUsecase.ListApplicationTriggers(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(len(triggers)).Should(Equal(1))
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
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())

		components, err := appUsecase.ListComponents(context.TODO(), appModel, v1.ListApplicationComponentOptions{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 1)).Should(BeEmpty())
		Expect(cmp.Diff(components[0].ComponentType, "webservice")).Should(BeEmpty())
	})

	It("Test CreateComponent function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())

		base, err := appUsecase.CreateComponent(context.TODO(), appModel, v1.CreateComponentRequest{
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

		detailResponse, err := appUsecase.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(detailResponse.Traits), 2)).Should(BeEmpty())
	})

	It("Test DetailComponent function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		detailResponse, err := appUsecase.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(detailResponse.DependsOn[0], "component-name")).Should(BeEmpty())
		Expect(detailResponse.Properties).ShouldNot(BeNil())
		Expect(cmp.Diff((*detailResponse.Properties)["image"], "busybox")).Should(BeEmpty())
	})

	It("Test AddPolicy function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		_, err = appUsecase.CreatePolicy(context.TODO(), appModel, v1.CreatePolicyRequest{
			Name:        overridePolicyName,
			Description: "this is a test2 policy",
			Type:        "override",
			Properties:  `{"components":[{"name":"component-name"}]}`,
		})
		Expect(err).Should(BeNil())

		_, err = appUsecase.CreatePolicy(context.TODO(), appModel, v1.CreatePolicyRequest{
			Name:        overridePolicyName,
			Description: "this is a test2 policy",
			Type:        "override",
			Properties:  ``,
		})
		Expect(cmp.Equal(err, bcode.ErrApplicationPolicyExist, cmpopts.EquateErrors())).Should(BeTrue())
	})

	It("Test ListPolicies function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())

		policies, err := appUsecase.ListPolicies(context.TODO(), appModel)
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
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		detail, err := appUsecase.DetailPolicy(context.TODO(), appModel, overridePolicyName)
		Expect(err).Should(BeNil())
		Expect(detail.Properties).ShouldNot(BeNil())
		Expect((*detail.Properties)["components"]).ShouldNot(BeEmpty())
	})

	It("Test UpdatePolicy function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		base, err := appUsecase.UpdatePolicy(context.TODO(), appModel, overridePolicyName, v1.UpdatePolicyRequest{
			Type:       "override",
			Properties: `{"components":{}}`,
		})
		Expect(err).Should(BeNil())
		Expect(base.Properties).ShouldNot(BeNil())
		Expect((*base.Properties)["components"]).Should(BeEmpty())
	})
	It("Test DeletePolicy function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		err = appUsecase.DeletePolicy(context.TODO(), appModel, overridePolicyName)
		Expect(err).Should(BeNil())
	})

	It("Test DeleteComponent function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Project, testProject)).Should(BeEmpty())
		component, err := appUsecase.GetApplicationComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		err = appUsecase.DeleteComponent(context.TODO(), appModel, component)
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
			err := workflowUsecase.createTestApplicationRevision(context.TODO(), appModel)
			Expect(err).Should(BeNil())
		}
		revisions, err := appUsecase.ListRevisions(context.TODO(), "test-app-sadasd", "", "", 0, 10)
		Expect(err).Should(BeNil())
		Expect(revisions.Total).Should(Equal(int64(3)))

		revisions, err = appUsecase.ListRevisions(context.TODO(), "test-app-sadasd", "env-0", "", 0, 10)
		Expect(err).Should(BeNil())
		Expect(revisions.Total).Should(Equal(int64(1)))
		Expect(revisions.Revisions[0].DeployUser.Name).Should(Equal(model.DefaultAdminUserName))
		Expect(revisions.Revisions[0].DeployUser.Alias).Should(Equal(model.DefaultAdminUserAlias))

		revisions, err = appUsecase.ListRevisions(context.TODO(), "test-app-sadasd", "", "terminated", 0, 10)
		Expect(err).Should(BeNil())
		Expect(revisions.Total).Should(Equal(int64(1)))

		revisions, err = appUsecase.ListRevisions(context.TODO(), "test-app", "env-1", "terminated", 0, 10)
		Expect(err).Should(BeNil())
		Expect(revisions.Total).Should(Equal(int64(0)))
	})

	It("Test DetailRevision function", func() {
		err := workflowUsecase.createTestApplicationRevision(context.TODO(), &model.ApplicationRevision{
			AppPrimaryKey: "test-app",
			Version:       "123",
			DeployUser:    model.DefaultAdminUserName,
		})
		Expect(err).Should(BeNil())
		revision, err := appUsecase.DetailRevision(context.TODO(), "test-app", "123")
		Expect(err).Should(BeNil())
		Expect(revision.Version).Should(Equal("123"))
		Expect(revision.DeployUser.Name).Should(Equal(model.DefaultAdminUserName))
		Expect(revision.DeployUser.Alias).Should(Equal(model.DefaultAdminUserAlias))
	})

	It("Test ApplicationEnvRecycle function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		revision, err := appUsecase.Deploy(
			context.WithValue(context.TODO(), &v1.CtxKeyUser, model.DefaultAdminUserName),
			appModel, v1.ApplicationDeployRequest{WorkflowName: convertWorkflowName("app-dev")})
		Expect(err).Should(BeNil())
		Expect(revision.DeployUser.Name).Should(Equal(model.DefaultAdminUserName))
		Expect(revision.DeployUser.Alias).Should(Equal(model.DefaultAdminUserAlias))
		err = envBindingUsecase.ApplicationEnvRecycle(context.TODO(), &model.Application{
			Name: testApp,
		}, &model.EnvBinding{Name: "app-dev"})
		Expect(err).Should(BeNil())
	})

	It("Test ListRecords function", func() {
		By("no running records in application")
		ctx := context.TODO()
		for i := 0; i < 2; i++ {
			appUsecase.ds.Add(ctx, &model.WorkflowRecord{
				AppPrimaryKey: "app-records",
				Name:          fmt.Sprintf("list-%d", i),
				Finished:      "true",
				Status:        model.RevisionStatusComplete,
			})
		}

		resp, err := appUsecase.ListRecords(context.TODO(), "app-records")
		Expect(err).Should(BeNil())
		Expect(resp.Total).Should(Equal(int64(1)))

		By("3 running records in application")
		for i := 0; i < 3; i++ {
			appUsecase.ds.Add(ctx, &model.WorkflowRecord{
				AppPrimaryKey: "app-records",
				Name:          fmt.Sprintf("list-running-%d", i),
				Finished:      "false",
				Status:        model.RevisionStatusRunning,
			})
		}

		resp, err = appUsecase.ListRecords(context.TODO(), "app-records")
		Expect(err).Should(BeNil())
		Expect(resp.Total).Should(Equal(int64(3)))
	})

	It("Test CompareAppWithLatestRevision function", func() {

		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		_, err = appUsecase.Deploy(context.TODO(), appModel, v1.ApplicationDeployRequest{WorkflowName: convertWorkflowName("app-dev")})
		Expect(err).Should(BeNil())
		component, err := appUsecase.GetApplicationComponent(context.TODO(), appModel, "component-name")
		Expect(err).Should(BeNil())

		By("compare when app not change, should return false")
		compareResponse, err := appUsecase.CompareAppWithLatestRevision(context.TODO(), appModel, v1.AppCompareReq{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(compareResponse.IsDiff, false)).Should(BeEmpty())

		By("compare when app not change and env not empty, should return false")
		compareResponse, err = appUsecase.CompareAppWithLatestRevision(context.TODO(), appModel, v1.AppCompareReq{Env: "app-dev"})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(compareResponse.IsDiff, false)).Should(BeEmpty())

		By("compare when app add env, not change, should return false")
		_, err = envUsecase.CreateEnv(context.TODO(), v1.CreateEnvRequest{Name: "app-prod", Namespace: "envnsprod", Targets: []string{defaultTarget}, Project: "app-prod"})
		Expect(err).Should(BeNil())
		_, err = envBindingUsecase.CreateEnvBinding(context.TODO(), appModel, v1.CreateApplicationEnvbindingRequest{EnvBinding: v1.EnvBinding{Name: "app-prod"}})
		Expect(err).Should(BeNil())
		compareResponse, err = appUsecase.CompareAppWithLatestRevision(context.TODO(), appModel, v1.AppCompareReq{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(compareResponse.IsDiff, false)).Should(BeEmpty())

		By("compare when app's env add target, should return true")
		_, err = targetUsecase.CreateTarget(context.TODO(), v1.CreateTargetRequest{Name: "dev-target1", Project: appModel.Project, Cluster: &v1.ClusterTarget{ClusterName: "local", Namespace: "dev-target1"}})
		Expect(err).Should(BeNil())
		_, err = envUsecase.UpdateEnv(context.TODO(), "app-dev",
			v1.UpdateEnvRequest{
				Description: "this is a env description update",
				Targets:     []string{defaultTarget, "dev-target1"},
			})
		Expect(err).Should(BeNil())
		compareResponse, err = appUsecase.CompareAppWithLatestRevision(context.TODO(), appModel, v1.AppCompareReq{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(compareResponse.IsDiff, true)).Should(BeEmpty())

		By("compare when update app's trait, should return true")
		// reset app config
		_, err = appUsecase.ResetAppToLatestRevision(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		_, err = appUsecase.UpdateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "component-name"}, "scaler", v1.UpdateApplicationTraitRequest{
			Properties:  `{"replicas":2}`,
			Alias:       "alias",
			Description: "description",
		})
		Expect(err).Should(BeNil())
		compareResponse, err = appUsecase.CompareAppWithLatestRevision(context.TODO(), appModel, v1.AppCompareReq{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(compareResponse.IsDiff, true)).Should(BeEmpty())

		By("compare when update component's target after app deployed ,should return ture")
		// reset app config
		_, err = appUsecase.ResetAppToLatestRevision(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		newProperties := "{\"exposeType\":\"NodePort\",\"image\":\"nginx\",\"imagePullPolicy\":\"Always\"}"
		_, err = appUsecase.UpdateComponent(context.TODO(),
			appModel,
			component,
			v1.UpdateApplicationComponentRequest{
				Properties: &newProperties,
			})
		Expect(err).Should(BeNil())
		compareResponse, err = appUsecase.CompareAppWithLatestRevision(context.TODO(), appModel, v1.AppCompareReq{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(compareResponse.IsDiff, true)).Should(BeEmpty())
		err = envBindingUsecase.ApplicationEnvRecycle(context.TODO(), &model.Application{Name: testApp}, &model.EnvBinding{Name: "app-dev"})
		Expect(err).Should(BeNil())
	})

	It("Test ResetAppToLatestRevision function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		resetResponse, err := appUsecase.ResetAppToLatestRevision(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(resetResponse.IsReset, true)).Should(BeEmpty())
		component, err := appUsecase.GetApplicationComponent(context.TODO(), appModel, "component-name")
		Expect(err).Should(BeNil())
		expectProperties := "{\"image\":\"nginx\"}"
		Expect(cmp.Diff(component.Properties.JSON(), expectProperties)).Should(BeEmpty())
	})

	It("Test DryRun with app function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		resetResponse, err := appUsecase.DryRunAppOrRevision(context.TODO(), appModel, v1.AppDryRunReq{DryRunType: "APP"})
		Expect(err).Should(BeNil())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app)")).Should(BeTrue())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app) -- Component(component-name)")).Should(BeTrue())
	})

	It("Test DryRun with env revision function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		resetResponse, err := appUsecase.DryRunAppOrRevision(context.TODO(), appModel, v1.AppDryRunReq{DryRunType: "Revision", Env: "app-dev"})
		Expect(err).Should(BeNil())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app)")).Should(BeTrue())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app) -- Component(component-name)")).Should(BeTrue())
	})

	It("Test DryRun with last revision function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		resetResponse, err := appUsecase.DryRunAppOrRevision(context.TODO(), appModel, v1.AppDryRunReq{DryRunType: "Revision"})
		Expect(err).Should(BeNil())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app)")).Should(BeTrue())
		Expect(strings.Contains(resetResponse.YAML, "# Application(test-app) -- Component(component-name)")).Should(BeTrue())
	})

	It("Test DeleteApplication function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		time.Sleep(time.Second * 3)
		err = appUsecase.DeleteApplication(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		components, err := appUsecase.ListComponents(context.TODO(), appModel, v1.ListApplicationComponentOptions{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 0)).Should(BeEmpty())
		policies, err := appUsecase.ListPolicies(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(policies), 0)).Should(BeEmpty())
	})
})

var _ = Describe("Test application component usecase function", func() {

	var (
		appUsecase     *applicationUsecaseImpl
		projectUsecase *projectUsecaseImpl
		envUsecase     *envUsecaseImpl
		testApp        string
		testProject    string
	)

	BeforeEach(func() {
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: "app-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		rbacUsecase := &rbacUsecaseImpl{ds: ds}
		projectUsecase = &projectUsecaseImpl{ds: ds, k8sClient: k8sClient, rbacUsecase: rbacUsecase}
		envUsecase = &envUsecaseImpl{ds: ds, kubeClient: k8sClient, projectUsecase: projectUsecase}
		workflowUsecase := &workflowUsecaseImpl{ds: ds, envUsecase: envUsecase}
		envBindingUsecase := &envBindingUsecaseImpl{ds: ds, envUsecase: envUsecase, workflowUsecase: workflowUsecase, kubeClient: k8sClient}

		appUsecase = &applicationUsecaseImpl{
			ds:                ds,
			apply:             apply.NewAPIApplicator(k8sClient),
			kubeClient:        k8sClient,
			projectUsecase:    projectUsecase,
			workflowUsecase:   workflowUsecase,
			envBindingUsecase: envBindingUsecase,
			envUsecase:        envUsecase,
		}
		testApp = "test-trait-app"
		testProject = "test-trait-project"

	})

	It("Test add application trait", func() {
		_, err := projectUsecase.CreateProject(context.TODO(), v1.CreateProjectRequest{Name: testProject})
		Expect(err).Should(BeNil())
		_, err = appUsecase.CreateApplication(context.TODO(), v1.CreateApplicationRequest{Name: testApp, Project: testProject})
		Expect(err).Should(BeNil())
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		_, err = appUsecase.CreateComponent(context.TODO(), appModel, v1.CreateComponentRequest{Name: "test2", ComponentType: "webservice"})
		Expect(err).Should(BeNil())
		alias := "alias"
		description := "description"
		res, err := appUsecase.CreateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, v1.CreateApplicationTraitRequest{
			Type:        "Ingress",
			Properties:  `{"domain":"www.test.com"}`,
			Alias:       alias,
			Description: description,
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(res.Type, "Ingress")).Should(BeEmpty())
		comp, err := appUsecase.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(comp).ShouldNot(BeNil())
		// A scaler trait is automatically generated for the webservice component.
		Expect(len(comp.Traits)).Should(BeEquivalentTo(2))
		Expect(comp.Traits[1].Properties.JSON()).Should(BeEquivalentTo(`{"domain":"www.test.com"}`))
		Expect(comp.Traits[1].Alias).Should(BeEquivalentTo(alias))
		Expect(comp.Traits[1].Description).Should(BeEquivalentTo(description))

		Expect(err).Should(BeNil())
		_, err = appUsecase.CreateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, v1.CreateApplicationTraitRequest{
			Type:       "Ingress",
			Properties: `{"domain":"www.dup.com"}`,
		})
		Expect(err).ShouldNot(BeNil())
	})

	It("Test update application trait", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		alias := "newAlias"
		description := "newDescription"
		res, err := appUsecase.UpdateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, "Ingress", v1.UpdateApplicationTraitRequest{
			Properties:  `{"domain":"www.test1.com"}`,
			Alias:       alias,
			Description: description,
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(res.Type, "Ingress")).Should(BeEmpty())
		comp, err := appUsecase.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(comp).ShouldNot(BeNil())
		Expect(len(comp.Traits)).Should(BeEquivalentTo(2))
		Expect(comp.Traits[1].Properties.JSON()).Should(BeEquivalentTo(`{"domain":"www.test1.com"}`))
		Expect(comp.Traits[1].Alias).Should(BeEquivalentTo(alias))
		Expect(comp.Traits[1].Description).Should(BeEquivalentTo(description))
	})

	It("Test update a not exist", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		_, err = appUsecase.UpdateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, "Ingress-1-20", v1.UpdateApplicationTraitRequest{
			Properties: `{"domain":"www.test1.com"}`,
		})
		Expect(err).ShouldNot(BeNil())
	})

	It("Test delete an exist trait", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		err = appUsecase.DeleteApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, "Ingress")
		Expect(err).Should(BeNil())
		app, err := appUsecase.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(app).ShouldNot(BeNil())
		Expect(len(app.Traits)).Should(BeEquivalentTo(1))
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
