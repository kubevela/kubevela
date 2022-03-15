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

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
)

var _ = Describe("Test envBindingUsecase functions", func() {
	var (
		envUsecase        *envUsecaseImpl
		envBindingUsecase *envBindingUsecaseImpl
		workflowUsecase   *workflowUsecaseImpl
		definitionUsecase DefinitionUsecase
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
			Name: "test-app-env",
		}
		envUsecase = &envUsecaseImpl{ds: ds, kubeClient: k8sClient}
		workflowUsecase = &workflowUsecaseImpl{ds: ds, kubeClient: k8sClient, envUsecase: envUsecase}
		definitionUsecase = &definitionUsecaseImpl{kubeClient: k8sClient, caches: utils.NewMemoryCacheStore(context.TODO())}
		envBindingUsecase = &envBindingUsecaseImpl{ds: ds, workflowUsecase: workflowUsecase, definitionUsecase: definitionUsecase, kubeClient: k8sClient, envUsecase: envUsecase}
		envBindingDemo1 = apisv1.EnvBinding{
			Name: "envbinding-dev",
		}
		envBindingDemo2 = apisv1.EnvBinding{
			Name: "envbinding-prod",
		}
	})

	It("Test Create Application Env function", func() {

		// create target
		err := ds.Add(context.TODO(), &model.Target{Name: "dev-target"})
		Expect(err).Should(BeNil())

		err = ds.Add(context.TODO(), &model.Target{Name: "prod-target"})
		Expect(err).Should(BeNil())

		_, err = envUsecase.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{Name: "envbinding-dev", Targets: []string{"dev-target"}})
		Expect(err).Should(BeNil())
		_, err = envUsecase.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{Name: "envbinding-prod", Targets: []string{"prod-target"}})
		Expect(err).Should(BeNil())

		By("create two envbinding")
		req := apisv1.CreateApplicationEnvbindingRequest{EnvBinding: envBindingDemo1}
		base, err := envBindingUsecase.CreateEnvBinding(context.TODO(), testApp, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		req = apisv1.CreateApplicationEnvbindingRequest{EnvBinding: envBindingDemo2}
		base, err = envBindingUsecase.CreateEnvBinding(context.TODO(), testApp, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		By("auto create two workflow")
		workflow, err := workflowUsecase.GetWorkflow(context.TODO(), testApp, convertWorkflowName("envbinding-dev"))
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(workflow.Steps[0].Name, "dev-target")).Should(BeEmpty())

		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), testApp, convertWorkflowName("envbinding-prod"))
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(workflow.Steps[0].Name, "prod-target")).Should(BeEmpty())
	})

	It("Test GetApplication Envs function", func() {
		envBindings, err := envBindingUsecase.GetEnvBindings(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(envBindings).ShouldNot(BeNil())
		Expect(cmp.Diff(len(envBindings), 2)).Should(BeEmpty())
	})

	It("Test GetApplication Env function", func() {
		envBinding, err := envBindingUsecase.GetEnvBinding(context.TODO(), testApp, "envbinding-dev")
		Expect(err).Should(BeNil())
		Expect(envBinding).ShouldNot(BeNil())
		Expect(cmp.Diff(envBinding.Name, "envbinding-dev")).Should(BeEmpty())
	})

	It("Test Application UpdateEnv function", func() {

		envBinding, err := envBindingUsecase.UpdateEnvBinding(context.TODO(), testApp, "envbinding-prod", apisv1.PutApplicationEnvBindingRequest{})
		Expect(err).Should(BeNil())
		Expect(envBinding).ShouldNot(BeNil())
		Expect(cmp.Diff(envBinding.TargetNames[0], "prod-target")).Should(BeEmpty())
		workflow, err := workflowUsecase.GetWorkflow(context.TODO(), testApp, "workflow-envbinding-prod")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(workflow.Steps[0].Name, "prod-target")).Should(BeEmpty())
	})

	It("Test Application DeleteEnv function", func() {
		err := envBindingUsecase.DeleteEnvBinding(context.TODO(), testApp, "envbinding-dev")
		Expect(err).Should(BeNil())
		_, err = workflowUsecase.GetWorkflow(context.TODO(), testApp, convertWorkflowName("envbinding-dev"))
		Expect(err).ShouldNot(BeNil())
		err = envBindingUsecase.DeleteEnvBinding(context.TODO(), testApp, "envbinding-prod")
		Expect(err).Should(BeNil())
		_, err = workflowUsecase.GetWorkflow(context.TODO(), testApp, convertWorkflowName("envbinding-prod"))
		Expect(err).ShouldNot(BeNil())
	})

	It("Test Application BatchCreateEnv function", func() {
		testBatchApp := &model.Application{
			Name: "test-batch-createt",
		}
		err := envBindingUsecase.BatchCreateEnvBinding(context.TODO(), testBatchApp, apisv1.EnvBindingList{&envBindingDemo1, &envBindingDemo2})
		Expect(err).Should(BeNil())
		envBindings, err := envBindingUsecase.GetEnvBindings(context.TODO(), testBatchApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(envBindings), 2)).Should(BeEmpty())
		workflows, err := workflowUsecase.ListApplicationWorkflow(context.TODO(), testBatchApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(workflows), 2)).Should(BeEmpty())
	})

	It("Test BatchDeleteEnvBinding function", func() {
		err := envBindingUsecase.BatchDeleteEnvBinding(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		envBindings, err := envBindingUsecase.GetEnvBindings(context.TODO(), testApp)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(envBindings), 0)).Should(BeEmpty())
	})

})
