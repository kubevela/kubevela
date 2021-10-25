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

	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

var _ = Describe("Test workflow usecase functions", func() {
	var (
		workflowUsecase *workflowUsecaseImpl
	)
	BeforeEach(func() {
		workflowUsecase = &workflowUsecaseImpl{ds: ds}
	})
	It("Test CreateNamespace function", func() {
		req := apisv1.CreateWorkflowRequest{
			Name:        "test-workflow-1",
			Description: "this is a workflow",
		}
		base, err := workflowUsecase.CreateWorkflow(context.TODO(), &model.Application{
			Name: "test-app",
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		req = apisv1.CreateWorkflowRequest{
			Name:        "test-workflow-2",
			Description: "this is test workflow",
			Default:     true,
		}
		base, err = workflowUsecase.CreateWorkflow(context.TODO(), &model.Application{
			Name: "test-app",
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())
	})

	It("Test GetApplicationDefaultWorkflow function", func() {
		workflow, err := workflowUsecase.GetApplicationDefaultWorkflow(context.TODO(), &model.Application{
			Name: "test-app",
		})
		Expect(err).Should(BeNil())
		Expect(workflow).ShouldNot(BeNil())
		Expect(cmp.Diff(workflow.Name, "test-workflow-2")).Should(BeEmpty())
	})
})
