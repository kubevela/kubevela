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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var appName = "app-workflow"
var _ = Describe("Test workflow usecase functions", func() {
	var (
		workflowUsecase *workflowUsecaseImpl
		appUsecase      *applicationUsecaseImpl
	)
	BeforeEach(func() {
		workflowUsecase = &workflowUsecaseImpl{ds: ds, kubeClient: k8sClient, apply: apply.NewAPIApplicator(k8sClient)}
		appUsecase = &applicationUsecaseImpl{ds: ds, kubeClient: k8sClient, apply: apply.NewAPIApplicator(k8sClient), envBindingUsecase: &envBindingUsecaseImpl{
			ds:              ds,
			workflowUsecase: workflowUsecase,
		}}
	})
	It("Test CreateWorkflow function", func() {
		reqApp := apisv1.CreateApplicationRequest{
			Name:        appName,
			Namespace:   "default",
			Description: "this is a test app",
			EnvBinding: []*apisv1.EnvBinding{{
				Name:        "dev",
				Description: "dev env",
				TargetNames: []string{"dev-target"},
			}},
		}
		_, err := appUsecase.CreateApplication(context.TODO(), reqApp)
		Expect(err).Should(BeNil())

		req := apisv1.CreateWorkflowRequest{
			Name:        "test-workflow-1",
			Description: "this is a workflow",
			EnvName:     "dev",
		}

		base, err := workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		req2 := apisv1.CreateWorkflowRequest{
			Name:        "test-workflow-1",
			Description: "change description",
			EnvName:     "dev2",
		}

		base, err = workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, req2)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req2.Description)).Should(BeEmpty())
		Expect(cmp.Diff(base.EnvName, req2.EnvName)).ShouldNot(BeEmpty())

		req = apisv1.CreateWorkflowRequest{
			Name:        "test-workflow-2",
			Description: "this is test workflow",
			EnvName:     "dev",
			Default:     true,
		}
		base, err = workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())
	})

	It("Test GetApplicationDefaultWorkflow function", func() {
		workflow, err := workflowUsecase.GetApplicationDefaultWorkflow(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		})
		Expect(err).Should(BeNil())
		Expect(workflow).ShouldNot(BeNil())
		Expect(cmp.Diff(workflow.Name, "test-workflow-2")).Should(BeEmpty())
	})

	It("Test ListWorkflowRecords function", func() {
		By("create some workflow records to test list workflow records")
		raw, err := yaml.YAMLToJSON([]byte(yamlStr))
		Expect(err).Should(BeNil())
		app := &v1beta1.Application{}
		err = json.Unmarshal(raw, app)
		Expect(err).Should(BeNil())
		app.Annotations[oam.AnnotationWorkflowName] = "test-workflow-2"
		workflow, err := workflowUsecase.GetWorkflow(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		for i := 0; i < 3; i++ {
			app.Annotations[oam.AnnotationPublishVersion] = fmt.Sprintf("list-workflow-name-%d", i)
			err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
				Name:      appName,
				Namespace: "default",
			}, app, workflow)
			Expect(err).Should(BeNil())
		}

		resp, err := workflowUsecase.ListWorkflowRecords(context.TODO(), workflow, 0, 10)
		Expect(err).Should(BeNil())
		Expect(resp.Total).Should(Equal(int64(3)))
	})

	It("Test DetailWorkflowRecord function", func() {
		By("create one workflow record to test detail workflow record")
		raw, err := yaml.YAMLToJSON([]byte(yamlStr))
		Expect(err).Should(BeNil())
		app := &v1beta1.Application{}
		err = json.Unmarshal(raw, app)
		Expect(err).Should(BeNil())
		app.Annotations[oam.AnnotationPublishVersion] = "test-workflow-2-123"
		app.Annotations[oam.AnnotationDeployVersion] = "1234"
		workflow, err := workflowUsecase.GetWorkflow(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, app, workflow)
		Expect(err).Should(BeNil())

		var revision = &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "1234",
			Status:        model.RevisionStatusInit,
			DeployUser:    "test-user",
			Note:          "test-commit",
			TriggerType:   "API",
			WorkflowName:  "test-workflow-2",
		}

		err = workflowUsecase.createTestApplicationRevision(context.TODO(), revision)
		Expect(err).Should(BeNil())

		detail, err := workflowUsecase.DetailWorkflowRecord(context.TODO(), workflow, "test-workflow-2-123")
		Expect(err).Should(BeNil())
		Expect(detail.WorkflowRecord.Name).Should(Equal("test-workflow-2-123"))
		Expect(detail.DeployUser).Should(Equal("test-user"))
	})

	It("Test SyncWorkflowRecord function", func() {
		By("create one workflow record to test sync status from application")
		raw, err := yaml.YAMLToJSON([]byte(yamlStr))
		Expect(err).Should(BeNil())
		app := &v1beta1.Application{}
		err = json.Unmarshal(raw, app)
		Expect(err).Should(BeNil())
		app.Status.Workflow.Finished = false
		app.Annotations[oam.AnnotationWorkflowName] = "test-workflow-2"
		app.Annotations[oam.AnnotationPublishVersion] = "test-workflow-2-233"
		app.Annotations[oam.AnnotationDeployVersion] = "4321"
		workflow, err := workflowUsecase.GetWorkflow(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, app, workflow)
		Expect(err).Should(BeNil())

		By("create one revision to test sync workflow record")
		var revision = &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "4321",
			Status:        model.RevisionStatusInit,
			DeployUser:    "test-user",
			WorkflowName:  "test-workflow-2",
		}
		err = workflowUsecase.createTestApplicationRevision(context.TODO(), revision)
		Expect(err).Should(BeNil())

		By("create the application to sync")
		ctx := context.Background()
		app.Status.Workflow.Finished = true
		err = workflowUsecase.kubeClient.Create(ctx, app)
		Expect(err).Should(BeNil())
		err = workflowUsecase.kubeClient.Status().Patch(ctx, app, client.Merge)
		Expect(err).Should(BeNil())
		err = workflowUsecase.SyncWorkflowRecord(ctx)
		Expect(err).Should(BeNil())

		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		By("check the record")
		record, err := workflowUsecase.DetailWorkflowRecord(context.TODO(), workflow, "test-workflow-2-233")
		Expect(err).Should(BeNil())
		Expect(record.Status).Should(Equal(model.RevisionStatusComplete))

		By("check the application revision")
		err = workflowUsecase.ds.Get(ctx, revision)
		Expect(err).Should(BeNil())
		Expect(revision.Status).Should(Equal(model.RevisionStatusComplete))

		By("create another workflow record to test sync status from controller revision")
		app.Status.Workflow.Finished = false
		app.Annotations[oam.AnnotationPublishVersion] = "test-workflow-2-111"
		app.Annotations[oam.AnnotationDeployVersion] = "1111"
		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, app, workflow)
		Expect(err).Should(BeNil())

		By("create another revision to test sync workflow record")
		var anotherRevision = &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "1111",
			Status:        model.RevisionStatusInit,
			DeployUser:    "test-user",
			WorkflowName:  "test-workflow-2",
		}
		err = workflowUsecase.createTestApplicationRevision(context.TODO(), anotherRevision)
		Expect(err).Should(BeNil())

		By("create one controller revision to test sync workflow record")
		Expect(err).Should(BeNil())
		cr := &appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "record-" + appName + "-test-workflow-2-111",
				Namespace: "default",
				Labels:    map[string]string{"vela.io/wf-revision": "test-workflow-2-111"},
			},
			Data: runtime.RawExtension{Raw: raw},
		}
		err = workflowUsecase.kubeClient.Create(ctx, cr)
		Expect(err).Should(BeNil())

		err = workflowUsecase.SyncWorkflowRecord(ctx)
		Expect(err).Should(BeNil())

		By("check the record")
		anotherRecord, err := workflowUsecase.DetailWorkflowRecord(context.TODO(), workflow, "test-workflow-2-111")
		Expect(err).Should(BeNil())
		Expect(anotherRecord.Status).Should(Equal(model.RevisionStatusComplete))

		By("check the application revision")
		err = workflowUsecase.ds.Get(ctx, anotherRevision)
		Expect(err).Should(BeNil())
		Expect(anotherRevision.Status).Should(Equal(model.RevisionStatusComplete))
	})

	It("Test ResumeRecord function", func() {
		ctx := context.TODO()

		ResumeWorkflow := "resume-workflow"
		req := apisv1.CreateWorkflowRequest{
			Name:        ResumeWorkflow,
			Description: "this is a workflow",
			EnvName:     "resume",
		}

		base, err := workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		app, err := createTestSuspendApp(ctx, appName, "resume", "revision-resume1", ResumeWorkflow, "workflow-resume-1", workflowUsecase.kubeClient)
		Expect(err).Should(BeNil())

		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, app, &model.Workflow{Name: ResumeWorkflow})
		Expect(err).Should(BeNil())

		err = workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "revision-resume1",
			Status:        model.RevisionStatusRunning,
		})
		Expect(err).Should(BeNil())

		err = workflowUsecase.ResumeRecord(ctx, &model.Application{
			Name:      appName,
			Namespace: "default",
		}, &model.Workflow{Name: ResumeWorkflow, EnvName: "resume"}, "workflow-resume-1")
		Expect(err).Should(BeNil())

		record, err := workflowUsecase.DetailWorkflowRecord(ctx, &model.Workflow{Name: ResumeWorkflow, AppPrimaryKey: appName}, "workflow-resume-1")
		Expect(err).Should(BeNil())
		Expect(record.Status).Should(Equal(model.RevisionStatusRunning))
	})

	It("Test TerminateRecord function", func() {
		ctx := context.TODO()

		workflowName := "terminate-workflow"
		req := apisv1.CreateWorkflowRequest{
			Name:        workflowName,
			Description: "this is a workflow",
			EnvName:     "terminate",
		}
		workflow := &model.Workflow{Name: workflowName, EnvName: "terminate"}
		base, err := workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		app, err := createTestSuspendApp(ctx, appName, "terminate", "revision-terminate1", workflow.Name, "test-workflow-2-1", workflowUsecase.kubeClient)
		Expect(err).Should(BeNil())

		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, app, workflow)
		Expect(err).Should(BeNil())

		err = workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "revision-terminate1",
			Status:        model.RevisionStatusRunning,
		})
		Expect(err).Should(BeNil())

		err = workflowUsecase.TerminateRecord(ctx, &model.Application{
			Name:      appName,
			Namespace: "default",
		}, workflow, "test-workflow-2-1")
		Expect(err).Should(BeNil())

		record, err := workflowUsecase.DetailWorkflowRecord(ctx, workflow, "test-workflow-2-1")
		Expect(err).Should(BeNil())
		Expect(record.Status).Should(Equal(model.RevisionStatusTerminated))
	})

	It("Test RollbackRecord function", func() {
		ctx := context.TODO()

		workflowName := "rollback-workflow"
		req := apisv1.CreateWorkflowRequest{
			Name:        workflowName,
			Description: "this is a workflow",
			EnvName:     "rollback",
		}
		workflow := &model.Workflow{Name: workflowName, EnvName: "rollback"}
		base, err := workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		app, err := createTestSuspendApp(ctx, appName, "rollback", "revision-rollback1", workflow.Name, "test-workflow-2-2", workflowUsecase.kubeClient)
		Expect(err).Should(BeNil())

		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, app, workflow)
		Expect(err).Should(BeNil())

		err = workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "revision-rollback1",
			Status:        model.RevisionStatusRunning,
		})
		Expect(err).Should(BeNil())
		err = workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey:  appName,
			Version:        "revision-rollback0",
			ApplyAppConfig: `{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"annotations":{"app.oam.dev/workflowName":"test-workflow-2-2","app.oam.dev/deployVersion":"revision-rollback1","vela.io/publish-version":"workflow-rollback1"},"name":"first-vela-app","namespace":"default"},"spec":{"components":[{"name":"express-server","properties":{"image":"crccheck/hello-world","port":8000},"traits":[{"properties":{"domain":"testsvc.example.com","http":{"/":8000}},"type":"ingress-1-20"}],"type":"webservice"}]}}`,
			Status:         model.RevisionStatusComplete,
		})
		Expect(err).Should(BeNil())

		err = workflowUsecase.RollbackRecord(ctx, &model.Application{
			Name:      appName,
			Namespace: "default",
		}, workflow, "test-workflow-2-2", "revision-rollback0")
		Expect(err).Should(BeNil())

		recordsNum, err := workflowUsecase.ds.Count(ctx, &model.WorkflowRecord{
			AppPrimaryKey:      appName,
			WorkflowName:       workflow.Name,
			RevisionPrimaryKey: "revision-rollback0",
		}, nil)
		Expect(err).Should(BeNil())
		Expect(recordsNum).Should(Equal(int64(1)))

		By("rollback application without revision version")
		app.Annotations[oam.AnnotationPublishVersion] = "workflow-rollback-2"
		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name:      appName,
			Namespace: "default",
		}, app, workflow)
		Expect(err).Should(BeNil())

		err = workflowUsecase.RollbackRecord(ctx, &model.Application{
			Name:      appName,
			Namespace: "default",
		}, workflow, "workflow-rollback-2", "")
		Expect(err).Should(BeNil())

		recordsNum, err = workflowUsecase.ds.Count(ctx, &model.WorkflowRecord{
			AppPrimaryKey:      appName,
			WorkflowName:       workflow.Name,
			RevisionPrimaryKey: "revision-rollback0",
		}, nil)
		Expect(err).Should(BeNil())
		Expect(recordsNum).Should(Equal(int64(2)))
	})
})

var yamlStr = `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  annotations:
    app.oam.dev/workflowName: test-workflow-2
    app.oam.dev/deployVersion: "1234"
    app.oam.dev/publishVersion: "test-workflow-name-111"
    app.oam.dev/appName: "app-workflow"
  name: app-workflow-dev
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

func (w *workflowUsecaseImpl) createTestApplicationRevision(ctx context.Context, revision *model.ApplicationRevision) error {
	if err := w.ds.Add(ctx, revision); err != nil {
		return err
	}
	return nil
}
