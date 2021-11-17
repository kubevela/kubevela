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
	"encoding/json"
	"fmt"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

var _ = Describe("Test workflow usecase functions", func() {
	var (
		workflowUsecase *workflowUsecaseImpl
	)
	BeforeEach(func() {
		workflowUsecase = &workflowUsecaseImpl{ds: ds, kubeClient: k8sClient}
	})
	It("Test CreateWorkflow function", func() {
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

	It("Test ListWorkflowRecords function", func() {
		By("create some controller revisions to test list workflow records")
		raw, err := yaml.YAMLToJSON([]byte(yamlStr))
		Expect(err).Should(BeNil())
		app := &v1beta1.Application{}
		err = json.Unmarshal(raw, app)
		Expect(err).Should(BeNil())
		for i := 0; i < 3; i++ {
			err := workflowUsecase.createWorkflowRecord(context.TODO(), app, fmt.Sprintf("record-test-%v", i))
			Expect(err).Should(BeNil())
		}

		resp, err := workflowUsecase.ListWorkflowRecords(context.TODO(), "test-workflow-name", 0, 10)
		Expect(err).Should(BeNil())
		Expect(resp.Total).Should(Equal(int64(3)))
	})

	It("Test DetailWorkflowRecord function", func() {
		By("create one controller revision to test detail workflow record")
		raw, err := yaml.YAMLToJSON([]byte(yamlStr))
		Expect(err).Should(BeNil())
		app := &v1beta1.Application{}
		err = json.Unmarshal(raw, app)
		Expect(err).Should(BeNil())
		err = workflowUsecase.createWorkflowRecord(context.TODO(), app, "record-test-123")
		Expect(err).Should(BeNil())

		var deployEvent = &model.ApplicationRevision{
			AppPrimaryKey: "test",
			Version:       "123",
			Status:        model.RevisionStatusInit,
			DeployUser:    "test-user",
			Note:          "test-commit",
			TriggerType:   "API",
			WorkflowName:  "test-workflow-name",
		}

		err = workflowUsecase.createTestApplicationRevision(context.TODO(), deployEvent)
		Expect(err).Should(BeNil())

		detail, err := workflowUsecase.DetailWorkflowRecord(context.TODO(), "test-workflow-name", "test-123")
		Expect(err).Should(BeNil())
		Expect(detail.WorkflowRecord.Name).Should(Equal("test-123"))
		Expect(detail.DeployUser).Should(Equal("test-user"))
	})

	It("Test SyncWorkflowRecord function", func() {
		By("create one controller revision to test sync workflow record")
		ctx := context.Background()
		raw, err := yaml.YAMLToJSON([]byte(yamlStr))
		Expect(err).Should(BeNil())
		cr := &appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "record-test-1234",
				Namespace: "default",
				Labels:    map[string]string{"vela.io/wf-revision": "1234"},
			},
			Data: runtime.RawExtension{Raw: raw},
		}
		err = workflowUsecase.kubeClient.Create(ctx, cr)
		Expect(err).Should(BeNil())

		By("create one deploy event to test sync workflow record")
		var deployEvent = &model.ApplicationRevision{
			AppPrimaryKey: "test",
			Version:       "1234",
			Status:        model.RevisionStatusInit,
			DeployUser:    "test-user",
			WorkflowName:  "test-workflow-name",
		}

		err = workflowUsecase.createTestApplicationRevision(context.TODO(), deployEvent)
		Expect(err).Should(BeNil())

		err = workflowUsecase.SyncWorkflowRecord(ctx)
		Expect(err).Should(BeNil())

		By("check the record")
		app := &v1beta1.Application{}
		err = json.Unmarshal(raw, app)
		Expect(err).Should(BeNil())
		err = workflowUsecase.createWorkflowRecord(context.TODO(), app, "test-1234")
		Expect(err).Should(Equal(datastore.ErrRecordExist))

		By("check the deploy event")
		err = workflowUsecase.ds.Get(ctx, deployEvent)
		Expect(err).Should(BeNil())
		Expect(deployEvent.Status).Should(Equal(model.RevisionStatusComplete))
	})
})

var yamlStr = `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  annotations:
    app.oam.dev/workflowName: test-workflow-name
    app.oam.dev/deployVersion: "1234"
  name: test
  namespace: default
spec:
  components:
  - name: express-server
    properties:
      image: crccheck/hello-world
      port: 8000
    type: webservice
  workflow:
    steps:
    - name: apply-server
      properties:
        component: express-server
      type: apply-component
status:
  workflow:
    steps:
    - firstExecuteTime: "2021-10-26T11:19:33Z"
      id: t8bpvi88d1
      lastExecuteTime: "2021-10-26T11:19:33Z"
      name: apply-pvc
      phase: succeeded
      type: apply-object
    - firstExecuteTime: "2021-10-26T11:19:33Z"
      id: 9fou7rbq9r
      lastExecuteTime: "2021-10-26T11:19:33Z"
      name: apply-server
      phase: succeeded
      type: apply-component
    suspend: false
    terminated: false
    finished: true`

func (w *workflowUsecaseImpl) createTestApplicationRevision(ctx context.Context, deployEvent *model.ApplicationRevision) error {
	if err := w.ds.Add(ctx, deployEvent); err != nil {
		return err
	}
	return nil
}
