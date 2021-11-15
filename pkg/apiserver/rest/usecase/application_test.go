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
	"io/ioutil"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var _ = Describe("Test application usecase function", func() {
	var (
		appUsecase      *applicationUsecaseImpl
		workflowUsecase *workflowUsecaseImpl
	)
	BeforeEach(func() {
		workflowUsecase = &workflowUsecaseImpl{ds: ds}
		appUsecase = &applicationUsecaseImpl{
			ds:              ds,
			workflowUsecase: workflowUsecase,
			apply:           apply.NewAPIApplicator(k8sClient),
			kubeClient:      k8sClient,
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
		Expect(cmp.Diff(len(appBase.EnvBinding), 2)).Should(BeEmpty())

		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd4")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())

	})

	It("Test GetApplicationEnvBindingingPolicy", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd4")
		Expect(err).Should(BeNil())
		envBinding, err := appUsecase.GetApplicationEnvBindingPolicy(context.TODO(), appModel)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(envBinding.Envs), 2)).Should(BeEmpty())
	})

	It("Test UpdateApplicationEnvBindingingDiff", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		_, err = appUsecase.UpdateApplicationEnvBinding(context.TODO(), appModel, "staging", v1.PutApplicationEnvRequest{
			ComponentSelector: &v1.ComponentSelector{Components: []string{"hello-world-server"}},
		})
		Expect(err).Should(BeNil())
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
		Expect(cmp.Diff(len(components), 1)).Should(BeEmpty())
		Expect(cmp.Diff(components[0].Name, "data-worker")).Should(BeEmpty())

		components, err = appUsecase.ListComponents(context.TODO(), appModel, v1.ListApplicationComponentOptions{
			EnvName: "staging",
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 1)).Should(BeEmpty())
		Expect(cmp.Diff(components[0].Name, "hello-world-server")).Should(BeEmpty())
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
	It("Test DeleteComponent function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		err = appUsecase.DeleteComponent(context.TODO(), appModel, "test2")
		Expect(err).Should(BeNil())
	})

	It("Test CreateApplicationEnvBinding function", func() {
		req := v1.CreateApplicationRequest{
			Name:        "not-have-env-bind",
			Namespace:   "test-app-namespace",
			Description: "this is a test app",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
		}
		_, err := appUsecase.CreateApplication(context.TODO(), req)
		Expect(err).Should(BeNil())
		appModel4, err := appUsecase.GetApplication(context.TODO(), "not-have-env-bind")
		Expect(err).Should(BeNil())
		By("test create first env")
		env4, err := appUsecase.CreateApplicationEnvBinding(context.TODO(), appModel4, v1.CreateApplicationEnvRequest{
			EnvBinding: v1.EnvBinding{
				Name:        "prod2",
				Alias:       "生产环境",
				Description: "这是一个用户某客户的生产环境",
				TargetNames: []string{"prod-target"},
			},
		})
		Expect(err).Should(BeNil())
		Expect(env4).ShouldNot(BeNil())

		appModelNew, err := appUsecase.GetApplication(context.TODO(), "not-have-env-bind")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(appModelNew.EnvBinding), 1)).Should(BeEmpty())

		By("test create not first env")
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		env, err := appUsecase.CreateApplicationEnvBinding(context.TODO(), appModel, v1.CreateApplicationEnvRequest{
			EnvBinding: v1.EnvBinding{
				Name:        "prod2",
				Alias:       "生产环境",
				Description: "这是一个用户某客户的生产环境",
				TargetNames: []string{"prod-target"},
			},
		})
		Expect(err).Should(BeNil())
		Expect(env).ShouldNot(BeNil())

		appModelNew, err = appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(appModelNew.EnvBinding), 4)).Should(BeEmpty())

		spec, err := appUsecase.GetApplicationEnvBindingPolicy(context.TODO(), appModelNew)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(spec.Envs), 4)).Should(BeEmpty())
	})

	It("Test UpdateApplicationEnvBinding function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		env, err := appUsecase.UpdateApplicationEnvBinding(context.TODO(), appModel, "prod2", v1.PutApplicationEnvRequest{
			ComponentSelector: &v1.ComponentSelector{
				Components: []string{},
			},
		})
		Expect(err).Should(BeNil())
		Expect(env).ShouldNot(BeNil())

		components, err := appUsecase.ListComponents(context.TODO(), appModel, v1.ListApplicationComponentOptions{
			EnvName: "prod2",
		})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 0)).Should(BeEmpty())
	})

	It("Test DeleteApplicationEnvBindinging function", func() {
		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-sadasd")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(appModel.Namespace, "test-app-namespace")).Should(BeEmpty())
		err = appUsecase.DeleteApplicationEnvBinding(context.TODO(), appModel, "prod2")
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
		Expect(cmp.Diff(res.Status, model.DeployEventRunning)).Should(BeEmpty())

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
})
