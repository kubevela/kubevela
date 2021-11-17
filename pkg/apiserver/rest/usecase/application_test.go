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
	"io/ioutil"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var _ = Describe("Test application usecase function", func() {
	var (
		appUsecase            *applicationUsecaseImpl
		workflowUsecase       *workflowUsecaseImpl
		envBindingUsecase     *envBindingUsecaseImpl
		deliveryTargetUsecase *deliveryTargetUsecaseImpl
	)
	BeforeEach(func() {
		workflowUsecase = &workflowUsecaseImpl{ds: ds}
		envBindingUsecase = &envBindingUsecaseImpl{ds: ds, workflowUsecase: workflowUsecase}
		deliveryTargetUsecase = &deliveryTargetUsecaseImpl{ds: ds}
		appUsecase = &applicationUsecaseImpl{
			ds:                    ds,
			workflowUsecase:       workflowUsecase,
			apply:                 apply.NewAPIApplicator(k8sClient),
			kubeClient:            k8sClient,
			envBindingUsecase:     envBindingUsecase,
			deliveryTargetUsecase: deliveryTargetUsecase,
		}
	})
	It("Test CreateApplication function", func() {
		By("test sample create")
		req := v1.CreateApplicationRequest{
			Name:        "test-app",
			Namespace:   "test-app-namespace",
			Description: "this is a test app",
			EnvBinding: []*v1.EnvBinding{{
				Name:        "dev",
				Description: "dev env",
				TargetNames: []string{"dev-target"},
			}, {
				Name:        "test",
				Description: "test env",
				TargetNames: []string{"test-target"},
			}},
		}
		base, err := appUsecase.CreateApplication(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())

		_, err = appUsecase.CreateApplication(context.TODO(), req)
		equal := cmp.Equal(err, bcode.ErrApplicationExist, cmpopts.EquateErrors())
		Expect(equal).Should(BeTrue())

		By("test with oam yaml config create")
		bs, err := ioutil.ReadFile("./testdata/example-app.yaml")
		Expect(err).Should(Succeed())
		req = v1.CreateApplicationRequest{
			Name:        "test-app-sadasd",
			Namespace:   "test-app-namespace",
			Description: "this is a test app",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
			YamlConfig:  string(bs),
		}
		base, err = appUsecase.CreateApplication(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())

		req = v1.CreateApplicationRequest{
			Name:        "test-app-sadasd2",
			Namespace:   "test-app-namespace",
			Description: "this is a test app",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
			YamlConfig:  "asdasdasdasd",
		}
		base, err = appUsecase.CreateApplication(context.TODO(), req)
		equal = cmp.Equal(err, bcode.ErrApplicationConfig, cmpopts.EquateErrors())
		Expect(equal).Should(BeTrue())
		Expect(base).Should(BeNil())

		bs, err = ioutil.ReadFile("./testdata/example-app-error.yaml")
		Expect(err).Should(Succeed())
		req = v1.CreateApplicationRequest{
			Name:        "test-app-sadasd3",
			Namespace:   "test-app-namespace",
			Description: "this is a test app",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
			YamlConfig:  string(bs),
		}
		_, err = appUsecase.CreateApplication(context.TODO(), req)
		equal = cmp.Equal(err, bcode.ErrInvalidProperties, cmpopts.EquateErrors())
		Expect(equal).Should(BeTrue())

		By("Test create app with env binding")
		req = v1.CreateApplicationRequest{
			Name:        "test-app-sadasd4",
			Namespace:   "test-app-namespace",
			Description: "this is a test app",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
			EnvBinding: []*v1.EnvBinding{
				{
					Name:        "dev",
					Alias:       "Chinese Word",
					Description: "This is a dev env",
					TargetNames: []string{"dev-target"},
				},
				{
					Name:        "prod",
					Description: "This is a prod env",
					TargetNames: []string{"prod-target"},
				},
			},
		}
		appBase, err := appUsecase.CreateApplication(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appBase.Name, "test-app-sadasd4")).Should(BeEmpty())

		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd4")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())

	})

	It("Test ListApplications function", func() {
		apps, err := appUsecase.ListApplications(context.TODO(), v1.ListApplicatioOptions{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(apps), 3)).Should(BeEmpty())
	})

	It("Test DetailApplication function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())

		detail, err := appUsecase.DetailApplication(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(detail.ResourceInfo.ComponentNum, 2)).Should(BeEmpty())
		Expect(cmp.Diff(len(detail.Policies), 1)).Should(BeEmpty())
	})

	It("Test GetWorkflow function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())

		_, err = workflowUsecase.GetWorkflow(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
	})

	It("Test ListPolicies function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())

		policies, err := appUsecase.ListPolicies(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(policies), 1)).Should(BeEmpty())
		Expect(cmp.Diff(policies[0].Type, "env-binding")).Should(BeEmpty())
		Expect((*policies[0].Properties)["envs"]).ShouldNot(BeEmpty())
	})

	It("Test ListComponents function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())

		components, err := appUsecase.ListComponents(context.TODO(), appModel, v1.ListApplicationComponentOptions{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 2)).Should(BeEmpty())
		Expect(cmp.Diff(components[0].ComponentType, "worker")).Should(BeEmpty())
		Expect(components[1].UpdateTime).ShouldNot(BeNil())

		components, err = appUsecase.ListComponents(context.TODO(), appModel, v1.ListApplicationComponentOptions{
			EnvName: "test",
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 2)).Should(BeEmpty())
		Expect(cmp.Diff(components[0].Name, "data-worker")).Should(BeEmpty())

		components, err = appUsecase.ListComponents(context.TODO(), appModel, v1.ListApplicationComponentOptions{
			EnvName: "staging",
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 2)).Should(BeEmpty())
		Expect(cmp.Diff(components[0].Name, "data-worker")).Should(BeEmpty())
	})

	It("Test DetailComponent function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())

		detail, err := appUsecase.DetailComponent(context.TODO(), appModel, "hello-world-server")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(detail.Traits), 1)).Should(BeEmpty())
		Expect(cmp.Diff(detail.Type, "webservice")).Should(BeEmpty())
		Expect(cmp.Diff(strings.Contains((*detail.Properties)["image"].(string), "crccheck/hello-world"), true)).Should(BeEmpty())
	})

	It("Test DetailPolicy function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())

		detail, err := appUsecase.DetailPolicy(context.TODO(), appModel, EnvBindingPolicyDefaultName)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(detail.Type, "env-binding")).Should(BeEmpty())
		Expect((*detail.Properties)["envs"]).ShouldNot(BeEmpty())
	})

	It("Test AddComponent function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		base, err := appUsecase.AddComponent(context.TODO(), appModel, v1.CreateComponentRequest{
			Name:          "test2",
			Description:   "this is a test2 component",
			Labels:        map[string]string{},
			ComponentType: "worker",
			Properties:    `{"image": "busybox","cmd":["sleep", "1000"],"lives": "3","enemies": "alien"}`,
			DependsOn:     []string{"data-worker"},
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.ComponentType, "worker")).Should(BeEmpty())
	})

	It("Test DetailComponent function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		detailResponse, err := appUsecase.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(detailResponse.DependsOn[0], "data-worker")).Should(BeEmpty())
		Expect(detailResponse.Properties).ShouldNot(BeNil())
		Expect(cmp.Diff((*detailResponse.Properties)["image"], "busybox")).Should(BeEmpty())
	})

	It("Test AddPolicy function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		_, err = appUsecase.AddPolicy(context.TODO(), appModel, v1.CreatePolicyRequest{
			Name:        EnvBindingPolicyDefaultName,
			Description: "this is a test2 policy",
			Type:        "env-binding",
			Properties:  ``,
		})
		Expect(cmp.Equal(err, bcode.ErrApplicationPolicyExist, cmpopts.EquateErrors())).Should(BeTrue())
		_, err = appUsecase.AddPolicy(context.TODO(), appModel, v1.CreatePolicyRequest{
			Name:        "env-binding-2",
			Description: "this is a test2 policy",
			Type:        "env-binding",
			Properties:  `{"envs":{ "name": "test", "placement":{"namespaceSelector":{ "name": "TEST_NAMESPACE"}}, "selector":{ "components": ["data-worker"]}}}`,
		})
		Expect(err).Should(BeNil())
	})

	It("Test DetailPolicy function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		detail, err := appUsecase.DetailPolicy(context.TODO(), appModel, "env-binding-2")
		Expect(err).Should(BeNil())
		Expect(detail.Properties).ShouldNot(BeNil())
		Expect((*detail.Properties)["envs"]).ShouldNot(BeEmpty())
	})

	It("Test UpdatePolicy function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		base, err := appUsecase.UpdatePolicy(context.TODO(), appModel, "env-binding-2", v1.UpdatePolicyRequest{
			Type:       "env-binding",
			Properties: `{"envs":{}}`,
		})
		Expect(err).Should(BeNil())
		Expect(base.Properties).ShouldNot(BeNil())
		Expect((*base.Properties)["envs"]).Should(BeEmpty())
	})
	It("Test DeletePolicy function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		err = appUsecase.DeletePolicy(context.TODO(), appModel, "env-binding-2")
		Expect(err).Should(BeNil())
	})

	It("Test add application trait", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
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
		Expect(len(comp.Traits)).Should(BeEquivalentTo(1))
		Expect(comp.Traits[0].Properties.JSON()).Should(BeEquivalentTo(`{"domain":"www.test.com"}`))
		Expect(comp.Traits[0].Alias).Should(BeEquivalentTo(alias))
		Expect(comp.Traits[0].Description).Should(BeEquivalentTo(description))
	})

	It("Test add application a dup trait", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		_, err = appUsecase.CreateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, v1.CreateApplicationTraitRequest{
			Type:       "Ingress",
			Properties: `{"domain":"www.dup.com"}`,
		})
		Expect(err).ShouldNot(BeNil())
	})

	It("Test update application trait", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
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
		Expect(len(comp.Traits)).Should(BeEquivalentTo(1))
		Expect(comp.Traits[0].Properties.JSON()).Should(BeEquivalentTo(`{"domain":"www.test1.com"}`))
		Expect(comp.Traits[0].Alias).Should(BeEquivalentTo(alias))
		Expect(comp.Traits[0].Description).Should(BeEquivalentTo(description))
	})

	It("Test update a not exist", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		_, err = appUsecase.UpdateApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, "Ingress-1-20", v1.UpdateApplicationTraitRequest{
			Properties: `{"domain":"www.test1.com"}`,
		})
		Expect(err).ShouldNot(BeNil())
	})

	It("Test delete an exist trait", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		err = appUsecase.DeleteApplicationTrait(context.TODO(), appModel, &model.ApplicationComponent{Name: "test2"}, "Ingress")
		Expect(err).Should(BeNil())
		app, err := appUsecase.DetailComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
		Expect(app).ShouldNot(BeNil())
		Expect(len(app.Traits)).Should(BeEquivalentTo(0))
	})

	It("Test DeleteComponent function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		err = appUsecase.DeleteComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
	})

	It("Test Deploy Application function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		res, err := appUsecase.Deploy(context.TODO(), appModel, v1.ApplicationDeployRequest{
			Note:        "unit test deploy",
			TriggerType: "api",
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(res.Status, model.RevisionStatusRunning)).Should(BeEmpty())

		var oam v1beta1.Application
		err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: appModel.Name, Namespace: appModel.Namespace}, &oam)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(oam.Spec.Components), 2)).Should(BeEmpty())
		Expect(cmp.Diff(len(oam.Spec.Policies), 1)).Should(BeEmpty())
	})

	It("Test DeleteApplication function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		err = appUsecase.DeleteApplication(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		components, err := appUsecase.ListComponents(context.TODO(), appModel, v1.ListApplicationComponentOptions{})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 0)).Should(BeEmpty())
		policies, err := appUsecase.ListPolicies(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(policies), 0)).Should(BeEmpty())
	})

	It("Test ListRevisions function", func() {
		for i := 0; i < 3; i++ {
			appModel := &model.ApplicationRevision{
				AppPrimaryKey: "test-app",
				Version:       fmt.Sprintf("%d", i),
				EnvName:       fmt.Sprintf("env-%d", i),
				Status:        model.RevisionStatusRunning,
			}
			if i == 0 {
				appModel.Status = model.RevisionStatusTerminated
			}
			err := workflowUsecase.createTestApplicationRevision(context.TODO(), appModel)
			Expect(err).Should(BeNil())
		}
		revisions, err := appUsecase.ListRevisions(context.TODO(), "test-app", "", "", 0, 10)
		Expect(err).Should(BeNil())
		Expect(revisions.Total).Should(Equal(int64(3)))

		revisions, err = appUsecase.ListRevisions(context.TODO(), "test-app", "env-0", "", 0, 10)
		Expect(err).Should(BeNil())
		Expect(revisions.Total).Should(Equal(int64(1)))

		revisions, err = appUsecase.ListRevisions(context.TODO(), "test-app", "", "terminated", 0, 10)
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
			DeployUser:    "test-user",
		})
		Expect(err).Should(BeNil())
		revision, err := appUsecase.DetailRevision(context.TODO(), "test-app", "123")
		Expect(err).Should(BeNil())
		Expect(revision.Version).Should(Equal("123"))
		Expect(revision.DeployUser).Should(Equal("test-user"))
	})

	It("Test ResumeRevision function", func() {
		ctx := context.TODO()
		err := workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey: "resume-app",
			Version:       "resume-1",
			Status:        model.RevisionStatusSuspend,
		})
		Expect(err).Should(BeNil())

		err = createTestSuspendApp(ctx, "resume-app", appUsecase.kubeClient)
		Expect(err).Should(BeNil())

		revision, err := appUsecase.ResumeRevision(ctx, &model.Application{
			Name:      "resume-app",
			Namespace: "default",
		}, "resume-1")
		Expect(err).Should(BeNil())
		Expect(revision.Version).Should(Equal("resume-1"))
		Expect(revision.Status).Should(Equal(model.RevisionStatusRunning))
	})

	It("Test TerminateRevision function", func() {
		ctx := context.TODO()
		err := workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey: "terminate-app",
			Version:       "terminate-1",
			Status:        model.RevisionStatusSuspend,
		})
		Expect(err).Should(BeNil())

		err = createTestSuspendApp(ctx, "terminate-app", appUsecase.kubeClient)
		Expect(err).Should(BeNil())

		revision, err := appUsecase.TerminateRevision(ctx, &model.Application{
			Name:      "terminate-app",
			Namespace: "default",
		}, "terminate-1")
		Expect(err).Should(BeNil())
		Expect(revision.Version).Should(Equal("terminate-1"))
		Expect(revision.Status).Should(Equal(model.RevisionStatusTerminated))
	})

	It("Test RollbackRevision function", func() {
		ctx := context.TODO()
		err := workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey: "rollback-app",
			Version:       "rollback-1",
			Status:        model.RevisionStatusSuspend,
		})
		Expect(err).Should(BeNil())
		err = workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey:  "rollback-app",
			Version:        "rollback-0",
			ApplyAppConfig: `{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"annotations":{"oam.dev/kubevela-version":"UNKNOWN"},"name":"first-vela-app","namespace":"default"},"spec":{"components":[{"name":"express-server","properties":{"image":"crccheck/hello-world","port":8000},"traits":[{"properties":{"domain":"testsvc.example.com","http":{"/":8000}},"type":"ingress-1-20"}],"type":"webservice"}]}}`,
			Status:         model.RevisionStatusComplete,
		})
		Expect(err).Should(BeNil())

		err = createTestSuspendApp(ctx, "rollback-app", appUsecase.kubeClient)
		Expect(err).Should(BeNil())

		revision, err := appUsecase.RollbackRevision(ctx, &model.Application{
			Name:      "rollback-app",
			Namespace: "default",
		}, "rollback-1", "rollback-0")
		Expect(err).Should(BeNil())
		Expect(revision.Status).Should(Equal(model.RevisionStatusInit))
	})
})

func createTestSuspendApp(ctx context.Context, appName string, kubeClient client.Client) error {
	testapp := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: "default",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name:       "test-component",
				Type:       "worker",
				Properties: &runtime.RawExtension{},
				Traits:     []common.ApplicationTrait{},
				Scopes:     map[string]string{},
			}},
		},
		Status: common.AppStatus{
			Workflow: &common.WorkflowStatus{
				Suspend: true,
			},
		},
	}

	if err := kubeClient.Create(ctx, testapp); err != nil {
		return err
	}
	if err := kubeClient.Status().Patch(ctx, testapp, client.Merge); err != nil {
		return err
	}

	return nil
}
