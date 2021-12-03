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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var _ = Describe("Test application usecase function", func() {
	var (
		appUsecase            *applicationUsecaseImpl
		workflowUsecase       *workflowUsecaseImpl
		envBindingUsecase     *envBindingUsecaseImpl
		deliveryTargetUsecase *deliveryTargetUsecaseImpl
		definitionUsecase     *definitionUsecaseImpl
	)
	BeforeEach(func() {
		workflowUsecase = &workflowUsecaseImpl{ds: ds}
		definitionUsecase = &definitionUsecaseImpl{kubeClient: k8sClient}
		envBindingUsecase = &envBindingUsecaseImpl{ds: ds, workflowUsecase: workflowUsecase, kubeClient: k8sClient, definitionUsecase: definitionUsecase}
		deliveryTargetUsecase = &deliveryTargetUsecaseImpl{ds: ds}
		appUsecase = &applicationUsecaseImpl{
			ds:                    ds,
			workflowUsecase:       workflowUsecase,
			apply:                 apply.NewAPIApplicator(k8sClient),
			kubeClient:            k8sClient,
			envBindingUsecase:     envBindingUsecase,
			definitionUsecase:     definitionUsecase,
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
		_, err := appUsecase.ListApplications(context.TODO(), v1.ListApplicatioOptions{})
		Expect(err).Should(BeNil())
	})

	It("Test ListApplications and filter by targetName function", func() {
		list, err := appUsecase.ListApplications(context.TODO(), v1.ListApplicatioOptions{
			Namespace:  "test-app-namespace",
			TargetName: "dev-target"})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(list), 2)).Should(BeEmpty())
	})

	It("Test DetailApplication function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())

		detail, err := appUsecase.DetailApplication(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(detail.ResourceInfo.ComponentNum, int64(2))).Should(BeEmpty())
		Expect(cmp.Diff(len(detail.Policies), 0)).Should(BeEmpty())
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
			Properties:  `{"envs":{ "name": "test", "placement":{"namespaceSelector":{ "name": "TEST_NAMESPACE"}}, "selector":{ "components": ["data-worker"]}}}`,
		})
		Expect(err).Should(BeNil())

		_, err = appUsecase.AddPolicy(context.TODO(), appModel, v1.CreatePolicyRequest{
			Name:        EnvBindingPolicyDefaultName,
			Description: "this is a test2 policy",
			Type:        "env-binding",
			Properties:  ``,
		})
		Expect(cmp.Equal(err, bcode.ErrApplicationPolicyExist, cmpopts.EquateErrors())).Should(BeTrue())
	})

	It("Test ListPolicies function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())

		policies, err := appUsecase.ListPolicies(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(policies), 1)).Should(BeEmpty())
	})

	It("Test DetailPolicy function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		detail, err := appUsecase.DetailPolicy(context.TODO(), appModel, EnvBindingPolicyDefaultName)
		Expect(err).Should(BeNil())
		Expect(detail.Properties).ShouldNot(BeNil())
		Expect((*detail.Properties)["envs"]).ShouldNot(BeEmpty())
	})

	It("Test UpdatePolicy function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		base, err := appUsecase.UpdatePolicy(context.TODO(), appModel, EnvBindingPolicyDefaultName, v1.UpdatePolicyRequest{
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
		err = appUsecase.DeletePolicy(context.TODO(), appModel, EnvBindingPolicyDefaultName)
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

	It("Test ApplicationEnvRecycle function", func() {
		req := v1.CreateApplicationRequest{
			Name:        "app-env-recycle" + "-dev",
			Namespace:   "test-app-namespace",
			Description: "this is a test app with env",
		}
		base, err := appUsecase.CreateApplication(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())

		err = envBindingUsecase.ApplicationEnvRecycle(context.TODO(), &model.Application{
			Name:      "app-env-recycle",
			Namespace: "test-app-namespace",
		}, &model.EnvBinding{Name: "dev"})
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

	It("Test createTargetClusterEnv function", func() {
		var namespace corev1.Namespace
		err := k8sClient.Get(context.TODO(), k8stypes.NamespacedName{Name: types.DefaultKubeVelaNS}, &namespace)
		if apierrors.IsNotFound(err) {
			err := k8sClient.Create(context.TODO(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: types.DefaultKubeVelaNS,
				},
			})
			Expect(err).Should(BeNil())
		} else {
			Expect(err).Should(BeNil())
		}
		definition := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aliyun-rds",
				Namespace: types.DefaultKubeVelaNS,
			},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload: common.WorkloadTypeDescriptor{
					Type: TerraformWorkfloadType,
				},
			},
		}
		err = k8sClient.Create(context.TODO(), definition)
		Expect(err).Should(BeNil())
		envConfig := appUsecase.createTargetClusterEnv(context.TODO(), &model.Application{
			Namespace: "prod",
		}, &model.EnvBinding{
			TargetNames: []string{"prod"},
		}, &model.DeliveryTarget{
			Name: "prod",
			Variable: map[string]interface{}{
				"region":       "hangzhou",
				"providerName": "aliyun",
			},
		}, []*model.ApplicationComponent{
			{
				Name: "component1",
				Type: "aliyun-rds",
			},
		})
		Expect(cmp.Diff(len(envConfig.Patch.Components), 1)).Should(BeEmpty())
		Expect(cmp.Diff(strings.Contains(string(envConfig.Patch.Components[0].Properties.Raw), "aliyun"), true)).Should(BeEmpty())
		err = k8sClient.Delete(context.TODO(), definition)
		Expect(err).Should(BeNil())
	})
})

func createTestSuspendApp(ctx context.Context, appName, envName, revisionVersion, wfName, recordName string, kubeClient client.Client) (*v1beta1.Application, error) {
	testapp := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      convertAppName(appName, envName),
			Namespace: "default",
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
				Type:       "worker",
				Properties: &runtime.RawExtension{Raw: []byte(`{"test":"test"}`)},
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

	if err := kubeClient.Create(ctx, testapp); err != nil {
		return nil, err
	}
	if err := kubeClient.Status().Patch(ctx, testapp, client.Merge); err != nil {
		return nil, err
	}

	return testapp, nil
}
