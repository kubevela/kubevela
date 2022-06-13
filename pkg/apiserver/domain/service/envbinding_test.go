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

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/repository"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

var _ = Describe("Test envBindingService functions", func() {
	var (
		envService        *envServiceImpl
		envBindingService *envBindingServiceImpl
		workflowService   *workflowServiceImpl
		definitionService DefinitionService
		envBindingDemo1   apisv1.EnvBinding
		envBindingDemo2   apisv1.EnvBinding
		testApp           *model.Application
		ds                datastore.DataStore
	)
	BeforeEach(func() {
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "env-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		testApp = &model.Application{
			Name:    "test-app-env",
			Project: "default",
		}
		rbacService := &rbacServiceImpl{Store: ds}
		projectService := &projectServiceImpl{Store: ds, K8sClient: k8sClient, RbacService: rbacService}
		envService = &envServiceImpl{Store: ds, KubeClient: k8sClient, ProjectService: projectService}
		workflowService = &workflowServiceImpl{Store: ds, KubeClient: k8sClient, EnvService: envService}
		definitionService = &definitionServiceImpl{KubeClient: k8sClient}
		envBindingService = &envBindingServiceImpl{Store: ds, WorkflowService: workflowService, DefinitionService: definitionService, KubeClient: k8sClient, EnvService: envService}
		envBindingDemo1 = apisv1.EnvBinding{
			Name: "envbinding-dev",
		}
		envBindingDemo2 = apisv1.EnvBinding{
			Name: "envbinding-prod",
		}
	})

	It("Test Create Application Env function", func() {

		// create target
		err := ds.Add(context.TODO(), &model.Target{
			Name:    "dev-target",
			Project: "default",
			Cluster: &model.ClusterTarget{ClusterName: "local", Namespace: "dev-target"}})
		Expect(err).Should(BeNil())

		err = ds.Add(context.TODO(), &model.Target{
			Name:    "prod-target",
			Project: "default",
			Cluster: &model.ClusterTarget{ClusterName: "local", Namespace: "prod-target"}})
		Expect(err).Should(BeNil())

		_, err = envService.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{
			Project: "default",
			Name:    "envbinding-dev", Targets: []string{"dev-target"}})
		Expect(err).Should(BeNil())
		_, err = envService.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{
			Project: "default",
			Name:    "envbinding-prod", Targets: []string{"prod-target"}})
		Expect(err).Should(BeNil())

		By("create two envbinding")
		req := apisv1.CreateApplicationEnvbindingRequest{EnvBinding: envBindingDemo1}
		base, err := envBindingService.CreateEnvBinding(context.TODO(), testApp, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		req = apisv1.CreateApplicationEnvbindingRequest{EnvBinding: envBindingDemo2}
		base, err = envBindingService.CreateEnvBinding(context.TODO(), testApp, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		By("test the auto created workflow")
		workflow, err := workflowService.GetWorkflow(context.TODO(), testApp, repository.ConvertWorkflowName("envbinding-dev"))
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(workflow.Steps[0].Name, "dev-target")).Should(BeEmpty())

		workflow, err = workflowService.GetWorkflow(context.TODO(), testApp, repository.ConvertWorkflowName("envbinding-prod"))
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(workflow.Steps[0].Name, "prod-target")).Should(BeEmpty())
	})

	It("Test GetApplication Envs function", func() {
		envBindings, err := envBindingService.GetEnvBindings(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(envBindings).ShouldNot(BeNil())
		Expect(cmp.Diff(len(envBindings), 2)).Should(BeEmpty())
	})

	It("Test GetApplication Env function", func() {
		envBinding, err := envBindingService.GetEnvBinding(context.TODO(), testApp, "envbinding-dev")
		Expect(err).Should(BeNil())
		Expect(envBinding).ShouldNot(BeNil())
		Expect(cmp.Diff(envBinding.Name, "envbinding-dev")).Should(BeEmpty())
	})

	It("Test Application UpdateEnv function", func() {
		envBinding, err := envBindingService.UpdateEnvBinding(context.TODO(), testApp, "envbinding-prod", apisv1.PutApplicationEnvBindingRequest{})
		Expect(err).Should(BeNil())
		Expect(envBinding).ShouldNot(BeNil())
		Expect(cmp.Diff(envBinding.TargetNames[0], "prod-target")).Should(BeEmpty())
		workflow, err := workflowService.GetWorkflow(context.TODO(), testApp, "workflow-envbinding-prod")
		Expect(err).Should(BeNil())
		Expect(len(workflow.Steps)).Should(Equal(1))
		Expect(cmp.Diff(workflow.Steps[0].Name, "prod-target")).Should(BeEmpty())
	})

	It("Test Application DeleteEnv function", func() {
		err := envBindingService.DeleteEnvBinding(context.TODO(), testApp, "envbinding-dev")
		Expect(err).Should(BeNil())
		_, err = workflowService.GetWorkflow(context.TODO(), testApp, repository.ConvertWorkflowName("envbinding-dev"))
		Expect(err).ShouldNot(BeNil())

		err = envBindingService.DeleteEnvBinding(context.TODO(), testApp, "envbinding-prod")
		Expect(err).Should(BeNil())
		_, err = workflowService.GetWorkflow(context.TODO(), testApp, repository.ConvertWorkflowName("envbinding-prod"))
		Expect(err).ShouldNot(BeNil())
		policies, err := repository.ListApplicationPolicies(context.TODO(), ds, testApp)
		Expect(err).Should(BeNil())
		Expect(len(policies)).Should(Equal(0))
	})

	It("Test Application BatchCreateEnv function", func() {
		testBatchApp := &model.Application{
			Name: "test-batch-created",
		}
		err := envBindingService.BatchCreateEnvBinding(context.TODO(), testBatchApp, apisv1.EnvBindingList{&envBindingDemo1, &envBindingDemo2})
		Expect(err).Should(BeNil())
		envBindings, err := envBindingService.GetEnvBindings(context.TODO(), testBatchApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(envBindings), 2)).Should(BeEmpty())
		workflows, err := workflowService.ListApplicationWorkflow(context.TODO(), testBatchApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(workflows), 2)).Should(BeEmpty())
	})

	It("Test BatchDeleteEnvBinding function", func() {
		err := envBindingService.BatchDeleteEnvBinding(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		envBindings, err := envBindingService.GetEnvBindings(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(envBindings), 0)).Should(BeEmpty())
	})

})
