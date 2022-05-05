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
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
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
		projectUsecase  *projectUsecaseImpl
		envUsecase      *envUsecaseImpl
		testProject     = "workflow-project"
		ds              datastore.DataStore
	)

	BeforeEach(func() {
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "workflow-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		rbacUsecase := &rbacUsecaseImpl{ds: ds}
		projectUsecase = &projectUsecaseImpl{ds: ds, rbacUsecase: rbacUsecase}
		envUsecase = &envUsecaseImpl{ds: ds, kubeClient: k8sClient, projectUsecase: projectUsecase}
		workflowUsecase = &workflowUsecaseImpl{
			ds:         ds,
			kubeClient: k8sClient,
			apply:      apply.NewAPIApplicator(k8sClient),
			envUsecase: envUsecase}
		appUsecase = &applicationUsecaseImpl{ds: ds, kubeClient: k8sClient,
			apply:          apply.NewAPIApplicator(k8sClient),
			projectUsecase: projectUsecase,
			envUsecase:     envUsecase,
			envBindingUsecase: &envBindingUsecaseImpl{
				ds:              ds,
				workflowUsecase: workflowUsecase,
				envUsecase:      envUsecase,
			}}
	})
	It("Test CreateWorkflow function", func() {
		_, err := projectUsecase.CreateProject(context.TODO(), apisv1.CreateProjectRequest{Name: testProject})
		Expect(err).Should(BeNil())
		reqApp := apisv1.CreateApplicationRequest{
			Name:        appName,
			Project:     testProject,
			Description: "this is a test app",
			EnvBinding: []*apisv1.EnvBinding{{
				Name: "dev",
			}},
		}
		_, err = appUsecase.CreateApplication(context.TODO(), reqApp)
		Expect(err).Should(BeNil())
		req := apisv1.CreateWorkflowRequest{
			Name:        "test-workflow-1",
			Description: "this is a workflow",
			EnvName:     "dev",
		}

		base, err := workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		req2 := apisv1.CreateWorkflowRequest{
			Name:        "test-workflow-1",
			Description: "change description",
			EnvName:     "dev2",
		}

		base, err = workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, req2)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req2.Description)).Should(BeEmpty())
		Expect(cmp.Diff(base.EnvName, req2.EnvName)).ShouldNot(BeEmpty())
		var defaultW = true
		req = apisv1.CreateWorkflowRequest{
			Name:        "test-workflow-2",
			Description: "this is test workflow",
			EnvName:     "dev",
			Steps: []apisv1.WorkflowStep{
				{
					Name:  "apply-pvc",
					Alias: "step-alias-1",
				},
				{
					Name:  "apply-server",
					Alias: "step-alias-2",
				},
			},
			Default: &defaultW,
		}
		base, err = workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		By("Test GetApplicationDefaultWorkflow function")
		workflow, err := workflowUsecase.GetApplicationDefaultWorkflow(context.TODO(), &model.Application{
			Name: appName,
		})
		Expect(err).Should(BeNil())
		Expect(workflow).ShouldNot(BeNil())

		By("Test ListWorkflowRecords function")
		By("create some workflow records to test list workflow records")
		raw, err := yaml.YAMLToJSON([]byte(yamlStr))
		Expect(err).Should(BeNil())
		app := &v1beta1.Application{}
		err = json.Unmarshal(raw, app)
		Expect(err).Should(BeNil())
		app.Annotations[oam.AnnotationWorkflowName] = "test-workflow-2"
		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		for i := 0; i < 3; i++ {
			app.Annotations[oam.AnnotationPublishVersion] = fmt.Sprintf("list-workflow-name-%d", i)
			app.Status.Workflow.AppRevision = fmt.Sprintf("list-workflow-name-%d", i)
			err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
				Name: appName,
			}, app, workflow)
			Expect(err).Should(BeNil())
		}

		resp, err := workflowUsecase.ListWorkflowRecords(context.TODO(), workflow, 0, 10)
		Expect(err).Should(BeNil())
		Expect(resp.Total).Should(Equal(int64(3)))

		By("create one workflow record to test detail workflow record")
		raw, err = yaml.YAMLToJSON([]byte(yamlStr))
		Expect(err).Should(BeNil())
		app = &v1beta1.Application{}
		err = json.Unmarshal(raw, app)
		Expect(err).Should(BeNil())
		app.Annotations[oam.AnnotationPublishVersion] = "test-workflow-2-123"
		app.Status.Workflow.AppRevision = "test-workflow-2-123"
		app.Annotations[oam.AnnotationDeployVersion] = "1234"
		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
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

		By("create one workflow record to test sync status from application")
		raw, err = yaml.YAMLToJSON([]byte(yamlStr))
		Expect(err).Should(BeNil())
		app = &v1beta1.Application{}
		err = json.Unmarshal(raw, app)
		Expect(err).Should(BeNil())
		app.Status.Workflow.Finished = false
		app.Annotations[oam.AnnotationWorkflowName] = "test-workflow-2"
		app.Annotations[oam.AnnotationPublishVersion] = "test-workflow-2-233"
		app.Status.Workflow.AppRevision = "test-workflow-2-233"
		app.Annotations[oam.AnnotationDeployVersion] = "4321"
		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
		}, app, workflow)
		Expect(err).Should(BeNil())

		By("create one revision to test sync workflow record")
		revision = &model.ApplicationRevision{
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
		err = workflowUsecase.kubeClient.Create(ctx, app.DeepCopy())
		Expect(err).Should(BeNil())
		err = workflowUsecase.kubeClient.Status().Patch(ctx, app, client.Merge)
		Expect(err).Should(BeNil())
		err = workflowUsecase.SyncWorkflowRecord(ctx)
		Expect(err).Should(BeNil())

		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		By("check the record")
		record, err := workflowUsecase.DetailWorkflowRecord(context.TODO(), workflow, "test-workflow-2-233")
		Expect(err).Should(BeNil())
		Expect(record.Status).Should(Equal(model.RevisionStatusComplete))
		Expect(record.Steps[0].Alias).Should(Equal("step-alias-1"))
		Expect(record.Steps[0].Phase).Should(Equal(common.WorkflowStepPhaseSucceeded))
		Expect(record.Steps[1].Alias).Should(Equal("step-alias-2"))
		Expect(record.Steps[1].Phase).Should(Equal(common.WorkflowStepPhaseSucceeded))

		By("check the application revision")
		err = workflowUsecase.ds.Get(ctx, revision)
		Expect(err).Should(BeNil())
		Expect(revision.Status).Should(Equal(model.RevisionStatusComplete))

		By("create another workflow record to test sync status from controller revision")
		app.Status.Workflow.Finished = false
		app.Annotations[oam.AnnotationPublishVersion] = "test-workflow-2-111"
		app.Status.Workflow.AppRevision = "test-workflow-2-111"
		app.Annotations[oam.AnnotationDeployVersion] = "1111"
		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
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
				Name:      "record-app-workflow-test-workflow-2-111",
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

	It("Test CreateRecord function", func() {
		ctx := context.TODO()
		for i := 0; i < 3; i++ {
			workflowUsecase.ds.Add(ctx, &model.WorkflowRecord{
				AppPrimaryKey: "record-app",
				Name:          fmt.Sprintf("test-record-%d", i),
				WorkflowName:  "test-workflow",
				Finished:      "false",
			})
		}

		app, err := createTestSuspendApp(ctx, "record-app", "default", "revision-123", "test-workflow", "test-record-3", workflowUsecase.kubeClient)
		Expect(err).Should(BeNil())

		err = workflowUsecase.CreateWorkflowRecord(ctx, &model.Application{
			Name: "record-app",
		}, app, &model.Workflow{Name: "test-workflow"})
		Expect(err).Should(BeNil())

		record := &model.WorkflowRecord{
			Name:          "test-record-3",
			AppPrimaryKey: "record-app",
			WorkflowName:  "test-workflow",
		}
		err = workflowUsecase.ds.Get(ctx, record)
		Expect(err).Should(BeNil())
		Expect(record.Status).Should(Equal(model.RevisionStatusRunning))
	})

	It("Test ResumeRecord function", func() {
		ctx := context.TODO()

		_, err := envUsecase.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{Name: "resume"})
		Expect(err).Should(BeNil())
		ResumeWorkflow := "resume-workflow"
		req := apisv1.CreateWorkflowRequest{
			Name:        ResumeWorkflow,
			Description: "this is a workflow",
			EnvName:     "resume",
		}

		base, err := workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		app, err := createTestSuspendApp(ctx, appName, "resume", "revision-resume1", ResumeWorkflow, "workflow-resume-1", workflowUsecase.kubeClient)
		Expect(err).Should(BeNil())

		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
		}, app, &model.Workflow{Name: ResumeWorkflow})
		Expect(err).Should(BeNil())

		err = workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "revision-resume1",
			Status:        model.RevisionStatusRunning,
		})
		Expect(err).Should(BeNil())

		err = workflowUsecase.ResumeRecord(ctx, &model.Application{
			Name: appName,
		}, &model.Workflow{Name: ResumeWorkflow, EnvName: "resume"}, "workflow-resume-1")
		Expect(err).Should(BeNil())

		record, err := workflowUsecase.DetailWorkflowRecord(ctx, &model.Workflow{Name: ResumeWorkflow, AppPrimaryKey: appName}, "workflow-resume-1")
		Expect(err).Should(BeNil())
		Expect(record.Status).Should(Equal(model.RevisionStatusRunning))
	})

	It("Test TerminateRecord function", func() {
		ctx := context.TODO()

		_, err := envUsecase.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{Name: "terminate"})
		Expect(err).Should(BeNil())
		workflowName := "terminate-workflow"
		req := apisv1.CreateWorkflowRequest{
			Name:        workflowName,
			Description: "this is a workflow",
			EnvName:     "terminate",
		}
		workflow := &model.Workflow{Name: workflowName, EnvName: "terminate"}
		base, err := workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		app, err := createTestSuspendApp(ctx, appName, "terminate", "revision-terminate1", workflow.Name, "test-workflow-2-1", workflowUsecase.kubeClient)
		Expect(err).Should(BeNil())

		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
		}, app, workflow)
		Expect(err).Should(BeNil())

		err = workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "revision-terminate1",
			Status:        model.RevisionStatusRunning,
		})
		Expect(err).Should(BeNil())

		err = workflowUsecase.TerminateRecord(ctx, &model.Application{
			Name: appName,
		}, workflow, "test-workflow-2-1")
		Expect(err).Should(BeNil())

		record, err := workflowUsecase.DetailWorkflowRecord(ctx, workflow, "test-workflow-2-1")
		Expect(err).Should(BeNil())
		Expect(record.Status).Should(Equal(model.RevisionStatusTerminated))
	})

	It("Test RollbackRecord function", func() {
		ctx := context.TODO()
		_, err := envUsecase.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{Name: "rollback"})
		Expect(err).Should(BeNil())
		workflowName := "rollback-workflow"
		req := apisv1.CreateWorkflowRequest{
			Name:        workflowName,
			Description: "this is a workflow",
			EnvName:     "rollback",
		}
		workflow := &model.Workflow{Name: workflowName, EnvName: "rollback"}
		base, err := workflowUsecase.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		app, err := createTestSuspendApp(ctx, appName, "rollback", "revision-rollback1", workflow.Name, "test-workflow-2-2", workflowUsecase.kubeClient)
		Expect(err).Should(BeNil())

		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
		}, app, workflow)
		Expect(err).Should(BeNil())

		err = workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "revision-rollback1",
			Status:        model.RevisionStatusRunning,
			WorkflowName:  workflow.Name,
			EnvName:       "rollback",
		})
		Expect(err).Should(BeNil())
		err = workflowUsecase.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey:  appName,
			Version:        "revision-rollback0",
			ApplyAppConfig: `{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"annotations":{"app.oam.dev/workflowName":"test-workflow-2-2","app.oam.dev/deployVersion":"revision-rollback1","vela.io/publish-version":"workflow-rollback1"},"name":"first-vela-app","namespace":"default"},"spec":{"components":[{"name":"express-server","properties":{"image":"crccheck/hello-world","port":8000},"traits":[{"properties":{"domain":"testsvc.example.com","http":{"/":8000}},"type":"ingress-1-20"}],"type":"webservice"}]}}`,
			Status:         model.RevisionStatusComplete,
			WorkflowName:   workflow.Name,
			EnvName:        "rollback",
		})
		Expect(err).Should(BeNil())

		err = workflowUsecase.RollbackRecord(ctx, &model.Application{
			Name: appName,
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
		app.Status.Workflow.AppRevision = "workflow-rollback-2"
		err = workflowUsecase.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
		}, app, workflow)
		Expect(err).Should(BeNil())

		err = workflowUsecase.RollbackRecord(ctx, &model.Application{
			Name: appName,
		}, workflow, "workflow-rollback-2", "")
		Expect(err).Should(BeNil())

		recordsNum, err = workflowUsecase.ds.Count(ctx, &model.WorkflowRecord{
			AppPrimaryKey:      appName,
			WorkflowName:       workflow.Name,
			RevisionPrimaryKey: "revision-rollback0",
		}, nil)
		Expect(err).Should(BeNil())
		Expect(recordsNum).Should(Equal(int64(2)))

		originalRevision := &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "revision-rollback1",
		}
		err = workflowUsecase.ds.Get(ctx, originalRevision)
		Expect(err).Should(BeNil())
		Expect(originalRevision.Status).Should(Equal(model.RevisionStatusRollback))
		Expect(originalRevision.RollbackVersion).Should(Equal("revision-rollback0"))
	})

	It("Test resetRevisionsAndRecords function", func() {
		ctx := context.TODO()

		err := workflowUsecase.ds.Add(ctx, &model.WorkflowRecord{
			AppPrimaryKey: "reset-app",
			WorkflowName:  "reset-workflow",
			Name:          "reset-record",
			Finished:      "false",
			Steps: []model.WorkflowStepStatus{
				{
					Phase: common.WorkflowStepPhaseSucceeded,
				},
				{
					Phase: common.WorkflowStepPhaseRunning,
				},
			},
		})
		Expect(err).Should(BeNil())

		err = resetRevisionsAndRecords(ctx, workflowUsecase.ds, "reset-app", "reset-workflow", "", "")
		Expect(err).Should(BeNil())

		record := &model.WorkflowRecord{
			AppPrimaryKey: "reset-app",
			WorkflowName:  "reset-workflow",
			Name:          "reset-record",
		}
		err = workflowUsecase.ds.Get(ctx, record)
		Expect(err).Should(BeNil())
		Expect(record.Status).Should(Equal(model.RevisionStatusTerminated))
		Expect(record.Finished).Should(Equal("true"))
		Expect(record.Steps[1].Phase).Should(Equal(common.WorkflowStepPhaseStopped))
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
  name: app-workflow
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
    finished: true
    appRevision: "test-workflow-name-111"`

func (w *workflowUsecaseImpl) createTestApplicationRevision(ctx context.Context, revision *model.ApplicationRevision) error {
	if err := w.ds.Add(ctx, revision); err != nil {
		return err
	}
	return nil
}
