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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var _ = Describe("Test application usecase function", func() {
	var (
		webhookUsecase        *webhookUsecaseImpl
		appUsecase            *applicationUsecaseImpl
		workflowUsecase       *workflowUsecaseImpl
		envBindingUsecase     *envBindingUsecaseImpl
		deliveryTargetUsecase *deliveryTargetUsecaseImpl
		definitionUsecase     *definitionUsecaseImpl
		projectUsecase        *projectUsecaseImpl
	)

	BeforeEach(func() {
		workflowUsecase = &workflowUsecaseImpl{ds: ds}
		definitionUsecase = &definitionUsecaseImpl{kubeClient: k8sClient}
		envBindingUsecase = &envBindingUsecaseImpl{ds: ds, workflowUsecase: workflowUsecase, kubeClient: k8sClient, definitionUsecase: definitionUsecase}
		projectUsecase = &projectUsecaseImpl{ds: ds, kubeClient: k8sClient}
		deliveryTargetUsecase = &deliveryTargetUsecaseImpl{ds: ds, projectUsecase: projectUsecase}
		appUsecase = &applicationUsecaseImpl{
			ds:                    ds,
			workflowUsecase:       workflowUsecase,
			apply:                 apply.NewAPIApplicator(k8sClient),
			kubeClient:            k8sClient,
			envBindingUsecase:     envBindingUsecase,
			definitionUsecase:     definitionUsecase,
			deliveryTargetUsecase: deliveryTargetUsecase,
			projectUsecase:        projectUsecase,
		}
		webhookUsecase = &webhookUsecaseImpl{
			ds:                 ds,
			applicationUsecase: appUsecase,
		}
	})

	It("Test HandleApplicationWebhook function", func() {
		_, err := projectUsecase.CreateProject(context.TODO(), apisv1.CreateProjectRequest{Name: "project-webhook"})
		Expect(err).Should(BeNil())
		req := apisv1.CreateApplicationRequest{
			Name:        "test-app-webhook",
			Project:     "project-webhook",
			Description: "this is a test app",
			EnvBinding: []*apisv1.EnvBinding{{
				Name:        "dev-webhook",
				Description: "dev env",
				TargetNames: []string{"dev-target-webhook"},
			}},
			Component: &apisv1.CreateComponentRequest{
				Name:          "component-name-webhook",
				ComponentType: "webservice",
			},
		}
		_, err = deliveryTargetUsecase.CreateDeliveryTarget(context.TODO(), apisv1.CreateDeliveryTargetRequest{
			Name:    "dev-target-webhook",
			Project: "project-webhook",
		})
		Expect(err).Should(BeNil())
		_, err = appUsecase.CreateApplication(context.TODO(), req)
		Expect(err).Should(BeNil())

		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-webhook")
		Expect(err).Should(BeNil())

		_, err = webhookUsecase.HandleApplicationWebhook(context.TODO(), "invalid-token", apisv1.HandleApplicationWebhookRequest{})
		Expect(err).Should(Equal(bcode.ErrInvalidWebhookToken))

		res, err := webhookUsecase.HandleApplicationWebhook(context.TODO(), appModel.WebhookToken, apisv1.HandleApplicationWebhookRequest{
			ComponentProperties: map[string]*model.JSONStruct{
				"component-name-webhook": {
					"image": "test-image",
				},
			},
			GitInfo: &model.GitInfo{
				Commit: "test-commit",
				Branch: "test-branch",
				User:   "test-user",
			},
		})
		Expect(err).Should(BeNil())
		comp, err := appUsecase.GetApplicationComponent(context.TODO(), appModel, "component-name-webhook")
		Expect(err).Should(BeNil())
		Expect((*comp.Properties)["image"]).Should(Equal("test-image"))

		revision := &model.ApplicationRevision{
			AppPrimaryKey: "test-app-webhook",
			Version:       res.Version,
		}
		err = webhookUsecase.ds.Get(context.TODO(), revision)
		Expect(err).Should(BeNil())
		Expect(revision.GitInfo.Commit).Should(Equal("test-commit"))
		Expect(revision.GitInfo.Branch).Should(Equal("test-branch"))
		Expect(revision.GitInfo.User).Should(Equal("test-user"))
	})
})
