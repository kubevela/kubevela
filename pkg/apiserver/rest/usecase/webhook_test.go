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
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/emicklei/go-restful/v3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var _ = Describe("Test application usecase function", func() {
	var (
		appUsecase        *applicationUsecaseImpl
		workflowUsecase   *workflowUsecaseImpl
		envUsecase        *envUsecaseImpl
		envBindingUsecase *envBindingUsecaseImpl
		targetUsecase     *targetUsecaseImpl
		definitionUsecase *definitionUsecaseImpl
		projectUsecase    *projectUsecaseImpl
		webhookUsecase    *webhookUsecaseImpl
	)

	BeforeEach(func() {
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: "app-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		envUsecase = &envUsecaseImpl{ds: ds, kubeClient: k8sClient}
		workflowUsecase = &workflowUsecaseImpl{ds: ds, envUsecase: envUsecase}
		definitionUsecase = &definitionUsecaseImpl{kubeClient: k8sClient}
		envBindingUsecase = &envBindingUsecaseImpl{ds: ds, envUsecase: envUsecase, workflowUsecase: workflowUsecase, kubeClient: k8sClient, definitionUsecase: definitionUsecase}
		targetUsecase = &targetUsecaseImpl{ds: ds, k8sClient: k8sClient}
		projectUsecase = &projectUsecaseImpl{ds: ds, k8sClient: k8sClient}
		appUsecase = &applicationUsecaseImpl{
			ds:                ds,
			workflowUsecase:   workflowUsecase,
			apply:             apply.NewAPIApplicator(k8sClient),
			kubeClient:        k8sClient,
			envBindingUsecase: envBindingUsecase,
			envUsecase:        envUsecase,
			definitionUsecase: definitionUsecase,
			targetUsecase:     targetUsecase,
			projectUsecase:    projectUsecase,
		}
		webhookUsecase = &webhookUsecaseImpl{
			ds:                 ds,
			applicationUsecase: appUsecase,
		}
	})

	It("Test HandleApplicationWebhook function", func() {
		_, err := targetUsecase.CreateTarget(context.TODO(), apisv1.CreateTargetRequest{Name: "dev-target-webhook"})
		Expect(err).Should(BeNil())

		_, err = projectUsecase.CreateProject(context.TODO(), apisv1.CreateProjectRequest{Name: "project-webhook"})
		Expect(err).Should(BeNil())

		_, err = envUsecase.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{Name: "webhook-dev", Namespace: "webhook-dev", Targets: []string{"dev-target-webhook"}, Project: "project-webhook"})
		Expect(err).Should(BeNil())

		Expect(err).Should(BeNil())
		req := apisv1.CreateApplicationRequest{
			Name:        "test-app-webhook",
			Project:     "project-webhook",
			Description: "this is a test app",
			EnvBinding: []*apisv1.EnvBinding{{
				Name: "webhook-dev",
			}},
			Component: &apisv1.CreateComponentRequest{
				Name:          "component-name-webhook",
				ComponentType: "webservice",
			},
		}
		_, err = appUsecase.CreateApplication(context.TODO(), req)
		Expect(err).Should(BeNil())

		appModel, err := appUsecase.GetApplication(context.TODO(), "test-app-webhook")
		Expect(err).Should(BeNil())

		_, err = webhookUsecase.HandleApplicationWebhook(context.TODO(), "invalid-token", nil)
		Expect(err).Should(Equal(bcode.ErrInvalidWebhookToken))

		triggers, err := appUsecase.ListApplicationTriggers(context.TODO(), appModel)
		Expect(err).Should(BeNil())

		invalidReq, err := http.NewRequest("post", "/", bytes.NewBuffer([]byte(`{"upgrade": "test"}`)))
		invalidReq.Header.Add(restful.HEADER_ContentType, "application/json")
		Expect(err).Should(BeNil())
		_, err = webhookUsecase.HandleApplicationWebhook(context.TODO(), triggers[0].Token, restful.NewRequest(invalidReq))
		Expect(err).Should(Equal(bcode.ErrInvalidWebhookPayloadBody))

		By("Test HandleApplicationWebhook function with custom payload")
		reqBody := apisv1.HandleApplicationTriggerWebhookRequest{
			Upgrade: map[string]*model.JSONStruct{
				"component-name-webhook": {
					"image": "test-image",
					"test1": map[string]string{
						"test2": "test3",
					},
				},
			},
			CodeInfo: &model.CodeInfo{
				Commit: "test-commit",
				Branch: "test-branch",
				User:   "test-user",
			},
		}
		body, err := json.Marshal(reqBody)
		Expect(err).Should(BeNil())
		httpreq, err := http.NewRequest("post", "/", bytes.NewBuffer(body))
		httpreq.Header.Add(restful.HEADER_ContentType, "application/json")
		Expect(err).Should(BeNil())
		res, err := webhookUsecase.HandleApplicationWebhook(context.TODO(), triggers[0].Token, restful.NewRequest(httpreq))
		Expect(err).Should(BeNil())
		appDeployRes := res.(*apisv1.ApplicationDeployResponse)
		comp, err := appUsecase.GetApplicationComponent(context.TODO(), appModel, "component-name-webhook")
		Expect(err).Should(BeNil())
		Expect((*comp.Properties)["image"]).Should(Equal("test-image"))
		Expect((*comp.Properties)["test1"]).Should(Equal(map[string]interface{}{
			"test2": "test3",
		}))

		revision := &model.ApplicationRevision{
			AppPrimaryKey: "test-app-webhook",
			Version:       appDeployRes.Version,
		}
		err = webhookUsecase.ds.Get(context.TODO(), revision)
		Expect(err).Should(BeNil())
		Expect(revision.CodeInfo.Commit).Should(Equal("test-commit"))
		Expect(revision.CodeInfo.Branch).Should(Equal("test-branch"))
		Expect(revision.CodeInfo.User).Should(Equal("test-user"))

		By("Test HandleApplicationWebhook function with ACR payload")
		acrTrigger, err := appUsecase.CreateApplicationTrigger(context.TODO(), appModel, apisv1.CreateApplicationTriggerRequest{
			Name:          "test-acr",
			PayloadType:   "acr",
			Type:          "webhook",
			ComponentName: "component-name-webhook",
		})
		Expect(err).Should(BeNil())

		acrBody := apisv1.HandleApplicationTriggerACRRequest{
			PushData: apisv1.ACRPushData{
				Digest: "test-digest",
				Tag:    "test-tag",
			},
			Repository: apisv1.ACRRepository{
				Name:         "test-repo",
				Namespace:    "test-namespace",
				Region:       "test-region",
				RepoFullName: "test-namespace/test-repo",
				RepoType:     "public",
			},
		}
		body, err = json.Marshal(acrBody)
		Expect(err).Should(BeNil())
		httpreq, err = http.NewRequest("post", "/", bytes.NewBuffer(body))
		httpreq.Header.Add(restful.HEADER_ContentType, "application/json")
		Expect(err).Should(BeNil())
		_, err = webhookUsecase.HandleApplicationWebhook(context.TODO(), acrTrigger.Token, restful.NewRequest(httpreq))
		Expect(err).Should(BeNil())
		comp, err = appUsecase.GetApplicationComponent(context.TODO(), appModel, "component-name-webhook")
		Expect(err).Should(BeNil())
		Expect((*comp.Properties)["image"]).Should(Equal("registry.test-region.aliyuncs.com/test-namespace/test-repo:test-tag"))

		By("Test HandleApplicationWebhook function with harbor payload")
		harborTrigger, err := appUsecase.CreateApplicationTrigger(context.TODO(), appModel, apisv1.CreateApplicationTriggerRequest{
			Name:          "test-harbor",
			PayloadType:   "harbor",
			Type:          "webhook",
			ComponentName: "component-name-webhook",
		})
		Expect(err).Should(BeNil())

		harborBody := apisv1.HandleApplicationHarborReq{
			Type: model.HarborEventTypePushArtifact,
			EventData: apisv1.EventData{
				Resources: []apisv1.Resources{
					{
						Digest:      "test-digest",
						Tag:         "test-tag",
						ResourceURL: "harbor.server/test-pro/test-repo:test-tag",
					},
				},
				Repository: apisv1.Repository{
					Name:         "test-repo",
					Namespace:    "test-namespace",
					RepoFullName: "test-pro/test-repo",
					RepoType:     "public",
				},
			},
		}
		body, err = json.Marshal(harborBody)
		Expect(err).Should(BeNil())
		httpreq, err = http.NewRequest("post", "/", bytes.NewBuffer(body))
		httpreq.Header.Add(restful.HEADER_ContentType, "application/json")
		Expect(err).Should(BeNil())
		_, err = webhookUsecase.HandleApplicationWebhook(context.TODO(), harborTrigger.Token, restful.NewRequest(httpreq))
		Expect(err).Should(BeNil())
		comp, err = appUsecase.GetApplicationComponent(context.TODO(), appModel, "component-name-webhook")
		Expect(err).Should(BeNil())
		Expect((*comp.Properties)["image"]).Should(Equal("harbor.server/test-pro/test-repo:test-tag"))

		By("Test HandleApplicationWebhook function with dockerhub payload")
		dockerhubTrigger, err := appUsecase.CreateApplicationTrigger(context.TODO(), appModel, apisv1.CreateApplicationTriggerRequest{
			Name:          "test-dockerhub",
			PayloadType:   "dockerhub",
			Type:          "webhook",
			ComponentName: "component-name-webhook",
		})
		Expect(err).Should(BeNil())

		dockerhubBody := apisv1.HandleApplicationTriggerDockerHubRequest{
			PushData: apisv1.DockerHubData{
				Tag: "test-tag",
			},
			Repository: apisv1.DockerHubRepository{
				IsPrivate: true,
				Name:      "test-repo",
				Namespace: "test-namespace",
				RepoName:  "test-namespace/test-repo",
				Status:    "Active",
			},
		}
		body, err = json.Marshal(dockerhubBody)
		Expect(err).Should(BeNil())
		httpreq, err = http.NewRequest("post", "/", bytes.NewBuffer(body))
		httpreq.Header.Add(restful.HEADER_ContentType, "application/json")
		Expect(err).Should(BeNil())
		_, err = webhookUsecase.HandleApplicationWebhook(context.TODO(), dockerhubTrigger.Token, restful.NewRequest(httpreq))
		Expect(err).Should(BeNil())
		comp, err = appUsecase.GetApplicationComponent(context.TODO(), appModel, "component-name-webhook")
		Expect(err).Should(BeNil())
		Expect((*comp.Properties)["image"]).Should(Equal("docker.io/test-namespace/test-repo:test-tag"))

		By("Test HandleApplicationWebhook function with jfrog payload without header of X-JFrogURL")
		jfrogTrigger, err := appUsecase.CreateApplicationTrigger(context.TODO(), appModel, apisv1.CreateApplicationTriggerRequest{
			Name:          "test-jfrog",
			PayloadType:   "jfrog",
			Type:          "webhook",
			ComponentName: "component-name-webhook",
		})
		Expect(err).Should(BeNil())
		jfrogBody := apisv1.HandleApplicationTriggerJFrogRequest{
			Domain:    "docker",
			EventType: "pushed",
			Data: apisv1.JFrogWebhookData{
				ImageName: "test-image",
				RepoKey:   "test-repo",
				Digest:    "test-digest",
				Tag:       "test-tag",
			},
		}
		body, err = json.Marshal(jfrogBody)
		Expect(err).Should(BeNil())
		httpreq, err = http.NewRequest("post", "/", bytes.NewBuffer(body))
		httpreq.Header.Add(restful.HEADER_ContentType, "application/json")
		Expect(err).Should(BeNil())
		_, err = webhookUsecase.HandleApplicationWebhook(context.TODO(), jfrogTrigger.Token, restful.NewRequest(httpreq))
		Expect(err).Should(BeNil())
		comp, err = appUsecase.GetApplicationComponent(context.TODO(), appModel, "component-name-webhook")
		Expect(err).Should(BeNil())
		Expect((*comp.Properties)["image"]).Should(Equal("test-repo/test-image:test-tag"))

		By("Test HandleApplicationWebhook function with jfrog payload with header of X-JFrogURL")
		httpreq, err = http.NewRequest("post", "/", bytes.NewBuffer(body))
		Expect(err).Should(BeNil())
		httpreq.Header.Add(restful.HEADER_ContentType, "application/json")
		httpreq.Header.Add("X-JFrogURL", "test-addr")
		_, err = webhookUsecase.HandleApplicationWebhook(context.TODO(), jfrogTrigger.Token, restful.NewRequest(httpreq))
		Expect(err).Should(BeNil())
		comp, err = appUsecase.GetApplicationComponent(context.TODO(), appModel, "component-name-webhook")
		Expect(err).Should(BeNil())
		Expect((*comp.Properties)["image"]).Should(Equal("test-addr/test-repo/test-image:test-tag"))
	})
})
