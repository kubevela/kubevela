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
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore/mongodb"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var appName = "app-workflow"
var _ = Describe("Test workflow service functions", func() {
	var (
		workflowService *workflowServiceImpl
		appService      *applicationServiceImpl
		projectService  *projectServiceImpl
		envService      *envServiceImpl
		envBinding      *envBindingServiceImpl
		targetService   *targetServiceImpl
		testProject     = "workflow-project"
		ds              datastore.DataStore
	)

	BeforeEach(func() {
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "workflow-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		rbacService := &rbacServiceImpl{Store: ds}
		projectService = &projectServiceImpl{Store: ds, RbacService: rbacService, K8sClient: k8sClient}
		envService = &envServiceImpl{Store: ds, KubeClient: k8sClient, ProjectService: projectService}
		targetService = &targetServiceImpl{Store: ds, K8sClient: k8sClient}
		envBinding = &envBindingServiceImpl{
			Store:           ds,
			WorkflowService: workflowService,
			EnvService:      envService,
		}
		workflowService = &workflowServiceImpl{
			Store:             ds,
			KubeClient:        k8sClient,
			Apply:             apply.NewAPIApplicator(k8sClient),
			EnvService:        envService,
			EnvBindingService: envBinding,
		}
		appService = &applicationServiceImpl{Store: ds, KubeClient: k8sClient,
			Apply:             apply.NewAPIApplicator(k8sClient),
			ProjectService:    projectService,
			EnvService:        envService,
			EnvBindingService: envBinding,
			WorkflowService:   workflowService,
		}
	})
	It("Test CreateWorkflow function", func() {

		_, err := projectService.CreateProject(context.TODO(), apisv1.CreateProjectRequest{Name: testProject})
		Expect(err).Should(BeNil())
		_, err = targetService.CreateTarget(context.TODO(), apisv1.CreateTargetRequest{
			Name: "dev-1", Project: testProject, Cluster: &apisv1.ClusterTarget{ClusterName: "local", Namespace: "dev-1"}})
		Expect(err).Should(BeNil())

		_, err = envService.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{Name: "dev", Namespace: "dev-1", Targets: []string{"dev-1"}, Project: testProject})
		Expect(err).Should(BeNil())

		reqApp := apisv1.CreateApplicationRequest{
			Name:        appName,
			Project:     testProject,
			Description: "this is a test app",
			EnvBinding: []*apisv1.EnvBinding{{
				Name: "dev",
			}},
		}
		_, err = appService.CreateApplication(context.TODO(), reqApp)
		Expect(err).Should(BeNil())
		req := apisv1.CreateWorkflowRequest{
			Name:        "test-workflow-1",
			Description: "this is a workflow",
			EnvName:     "dev",
		}

		base, err := workflowService.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		req2 := apisv1.CreateWorkflowRequest{
			Name:        "test-workflow-1",
			Description: "change description",
			EnvName:     "dev2",
		}

		base, err = workflowService.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
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
					WorkflowStepBase: apisv1.WorkflowStepBase{
						Name:  "apply-server",
						Alias: "step-alias-1",
					},
				},
				{
					WorkflowStepBase: apisv1.WorkflowStepBase{
						Name:  "apply-server2",
						Alias: "step-alias-2",
					},
				},
				{
					WorkflowStepBase: apisv1.WorkflowStepBase{
						Name:  "group",
						Alias: "group-alias",
					},
					SubSteps: []apisv1.WorkflowStepBase{
						{
							Name:  "suspend",
							Alias: "my-suspend",
						},
					},
				},
			},
			Default: &defaultW,
		}
		base, err = workflowService.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		By("Test GetApplicationDefaultWorkflow function")
		workflow, err := workflowService.GetApplicationDefaultWorkflow(context.TODO(), &model.Application{
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
		workflow, err = workflowService.GetWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		for i := 0; i < 3; i++ {
			app.Annotations[oam.AnnotationPublishVersion] = fmt.Sprintf("list-workflow-name-%d", i)
			app.Status.Workflow.AppRevision = fmt.Sprintf("list-workflow-name-%d", i)
			_, err = workflowService.CreateWorkflowRecord(context.TODO(), &model.Application{
				Name: appName,
			}, app, workflow)
			Expect(err).Should(BeNil())
		}

		resp, err := workflowService.ListWorkflowRecords(context.TODO(), workflow, 0, 10)
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
		workflow, err = workflowService.GetWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		_, err = workflowService.CreateWorkflowRecord(context.TODO(), &model.Application{
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

		err = workflowService.createTestApplicationRevision(context.TODO(), revision)
		Expect(err).Should(BeNil())

		detail, err := workflowService.DetailWorkflowRecord(context.TODO(), workflow, "test-workflow-2-123")
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
		workflow, err = workflowService.GetWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		_, err = workflowService.CreateWorkflowRecord(context.TODO(), &model.Application{
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
		err = workflowService.createTestApplicationRevision(context.TODO(), revision)
		Expect(err).Should(BeNil())

		By("create the application to sync")
		ctx := context.Background()
		app.Status.Workflow.Finished = true
		err = workflowService.KubeClient.Create(ctx, app.DeepCopy())
		Expect(err).Should(BeNil())
		app.Status.ObservedGeneration = 1
		err = workflowService.KubeClient.Status().Patch(ctx, app, client.Merge)
		Expect(err).Should(BeNil())
		err = workflowService.SyncWorkflowRecord(ctx)
		Expect(err).Should(BeNil())

		workflow, err = workflowService.GetWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, "test-workflow-2")
		Expect(err).Should(BeNil())
		By("check the record")
		record, err := workflowService.DetailWorkflowRecord(context.TODO(), workflow, "test-workflow-2-233")
		Expect(err).Should(BeNil())
		Expect(record.Status).Should(Equal(string(workflowv1alpha1.WorkflowStateFailed)))
		Expect(record.Steps[0].Alias).Should(Equal("step-alias-1"))
		Expect(record.Steps[0].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseSucceeded))
		Expect(record.Steps[1].Alias).Should(Equal("step-alias-2"))
		Expect(record.Steps[1].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseSucceeded))
		Expect(record.Steps[2].Alias).Should(Equal("group-alias"))
		Expect(record.Steps[2].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseFailed))
		Expect(record.Steps[2].SubStepsStatus[0].Alias).Should(Equal("my-suspend"))
		Expect(record.Steps[2].SubStepsStatus[0].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseFailed))

		By("check the application revision")
		err = workflowService.Store.Get(ctx, revision)
		Expect(err).Should(BeNil())
		Expect(revision.Status).Should(Equal(model.RevisionStatusFail))

		By("create another workflow record to test sync status from controller revision")
		app.Status.Workflow.Finished = false
		app.Annotations[oam.AnnotationPublishVersion] = "test-workflow-2-111"
		app.Status.Workflow.AppRevision = "test-workflow-2-111"
		app.Annotations[oam.AnnotationDeployVersion] = "1111"
		_, err = workflowService.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
		}, app, workflow)
		Expect(err).Should(BeNil())

		By("create another revision to test sync workflow record")
		var anotherRevision = &model.ApplicationRevision{
			AppPrimaryKey:  appName,
			Version:        "1111",
			Status:         model.RevisionStatusInit,
			DeployUser:     "test-user",
			WorkflowName:   "test-workflow-2",
			RevisionCRName: "1111-v1",
		}
		err = workflowService.createTestApplicationRevision(context.TODO(), anotherRevision)
		Expect(err).Should(BeNil())

		By("create one application revision to test sync workflow record")

		appWithRevision := &v1beta1.Application{}
		err = json.Unmarshal(raw, appWithRevision)
		Expect(err).Should(BeNil())
		var appRevision = &v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "1111-v1",
				Namespace: "default",
				Labels:    map[string]string{"vela.io/wf-revision": "test-workflow-2-111"},
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: *appWithRevision,
				},
			},
		}
		err = workflowService.KubeClient.Create(ctx, appRevision)
		Expect(err).Should(BeNil())
		appRevision.Status.Workflow = appWithRevision.Status.Workflow
		appRevision.Status.Workflow.AppRevision = app.Annotations[oam.AnnotationPublishVersion]
		err = workflowService.KubeClient.Status().Update(ctx, appRevision)
		Expect(err).Should(BeNil())
		err = workflowService.SyncWorkflowRecord(ctx)
		Expect(err).Should(BeNil())

		By("check the record")
		anotherRecord, err := workflowService.DetailWorkflowRecord(context.TODO(), workflow, "test-workflow-2-111")
		Expect(err).Should(BeNil())
		Expect(anotherRecord.Status).Should(Equal(string(workflowv1alpha1.WorkflowStepPhaseFailed)))

		By("check the application revision")
		err = workflowService.Store.Get(ctx, anotherRevision)
		Expect(err).Should(BeNil())
		Expect(anotherRevision.Status).Should(Equal(model.RevisionStatusFail))
	})

	It("Test CreateRecord function", func() {
		ctx := context.TODO()
		for i := 0; i < 3; i++ {
			workflowService.Store.Add(ctx, &model.WorkflowRecord{
				AppPrimaryKey: "record-app",
				Name:          fmt.Sprintf("test-record-%d", i),
				WorkflowName:  "test-workflow",
				Finished:      "false",
			})
		}

		app, err := createTestSuspendApp(ctx, "record-app", "default", "revision-123", "test-workflow", "test-record-3", workflowService.KubeClient)
		Expect(err).Should(BeNil())

		_, err = workflowService.CreateWorkflowRecord(ctx, &model.Application{
			Name: "record-app",
		}, app, &model.Workflow{Name: "test-workflow"})
		Expect(err).Should(BeNil())

		record := &model.WorkflowRecord{
			Name:          "test-record-3",
			AppPrimaryKey: "record-app",
			WorkflowName:  "test-workflow",
		}
		err = workflowService.Store.Get(ctx, record)
		Expect(err).Should(BeNil())
		Expect(record.Status).Should(Equal(string(workflowv1alpha1.WorkflowStateInitializing)))
	})

	It("Test ResumeRecord function", func() {
		ctx := context.TODO()

		_, err := envService.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{Name: "resume"})
		Expect(err).Should(BeNil())
		_, err = envBinding.CreateEnvBinding(context.TODO(), &model.Application{Name: appName}, apisv1.CreateApplicationEnvbindingRequest{EnvBinding: apisv1.EnvBinding{Name: "resume"}})
		Expect(err).Should(BeNil())
		ResumeWorkflow := "resume-workflow"
		req := apisv1.CreateWorkflowRequest{
			Name:        ResumeWorkflow,
			Description: "this is a workflow",
			EnvName:     "resume",
		}

		base, err := workflowService.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		app, err := createTestSuspendApp(ctx, appName, "resume", "revision-resume1", ResumeWorkflow, "workflow-resume-1", workflowService.KubeClient)
		Expect(err).Should(BeNil())

		_, err = workflowService.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
		}, app, &model.Workflow{Name: ResumeWorkflow})
		Expect(err).Should(BeNil())

		err = workflowService.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey:  appName,
			Version:        "revision-resume1",
			RevisionCRName: "revision-resume1",
			Status:         model.RevisionStatusRunning,
		})
		Expect(err).Should(BeNil())

		err = workflowService.ResumeRecord(ctx, &model.Application{
			Name: appName,
		}, &model.Workflow{Name: ResumeWorkflow, EnvName: "resume"}, "workflow-resume-1")
		Expect(err).Should(BeNil())

		record, err := workflowService.DetailWorkflowRecord(ctx, &model.Workflow{Name: ResumeWorkflow, AppPrimaryKey: appName}, "workflow-resume-1")
		Expect(err).Should(BeNil())
		Expect(len(record.Steps)).Should(Equal(1))
		Expect(record.Steps[0].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseSucceeded))
	})

	It("Test TerminateRecord function", func() {
		ctx := context.TODO()

		_, err := envService.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{Name: "terminate"})
		Expect(err).Should(BeNil())
		_, err = envBinding.CreateEnvBinding(context.TODO(), &model.Application{Name: appName}, apisv1.CreateApplicationEnvbindingRequest{EnvBinding: apisv1.EnvBinding{Name: "terminate"}})
		Expect(err).Should(BeNil())
		workflowName := "terminate-workflow"
		req := apisv1.CreateWorkflowRequest{
			Name:        workflowName,
			Description: "this is a workflow",
			EnvName:     "terminate",
		}
		workflow := &model.Workflow{Name: workflowName, EnvName: "terminate"}
		base, err := workflowService.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		app, err := createTestSuspendApp(ctx, appName, "terminate", "revision-terminate1", workflow.Name, "test-workflow-2-1", workflowService.KubeClient)
		Expect(err).Should(BeNil())

		_, err = workflowService.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
		}, app, workflow)
		Expect(err).Should(BeNil())

		err = workflowService.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "revision-terminate1",
			Status:        model.RevisionStatusRunning,
		})
		Expect(err).Should(BeNil())

		err = workflowService.TerminateRecord(ctx, &model.Application{
			Name: appName,
		}, workflow, "test-workflow-2-1")
		Expect(err).Should(BeNil())

		record, err := workflowService.DetailWorkflowRecord(ctx, workflow, "test-workflow-2-1")
		Expect(err).Should(BeNil())
		Expect(len(record.Steps)).Should(Equal(1))
		Expect(record.Steps[0].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseFailed))
	})

	It("Test RollbackRecord function", func() {
		ctx := context.TODO()
		_, err := envService.CreateEnv(context.TODO(), apisv1.CreateEnvRequest{Name: "rollback"})
		Expect(err).Should(BeNil())
		_, err = envBinding.CreateEnvBinding(context.TODO(), &model.Application{Name: appName}, apisv1.CreateApplicationEnvbindingRequest{EnvBinding: apisv1.EnvBinding{Name: "rollback"}})
		Expect(err).Should(BeNil())
		workflowName := "rollback-workflow"
		req := apisv1.CreateWorkflowRequest{
			Name:        workflowName,
			Description: "this is a workflow",
			EnvName:     "rollback",
		}
		workflow := &model.Workflow{Name: workflowName, EnvName: "rollback"}
		base, err := workflowService.CreateOrUpdateWorkflow(context.TODO(), &model.Application{
			Name: appName,
		}, req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		app, err := createTestSuspendApp(ctx, appName, "rollback", "revision-rollback1", workflow.Name, "test-workflow-2-2", workflowService.KubeClient)
		Expect(err).Should(BeNil())

		_, err = workflowService.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
		}, app, workflow)
		Expect(err).Should(BeNil())

		err = workflowService.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey: appName,
			Version:       "revision-rollback1",
			Status:        model.RevisionStatusRunning,
			WorkflowName:  workflow.Name,
			EnvName:       "rollback",
		})
		Expect(err).Should(BeNil())
		err = workflowService.createTestApplicationRevision(ctx, &model.ApplicationRevision{
			AppPrimaryKey:  appName,
			Version:        "revision-rollback0",
			ApplyAppConfig: `{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"annotations":{"app.oam.dev/workflowName":"test-workflow-2-2","app.oam.dev/deployVersion":"revision-rollback1","vela.io/publish-version":"workflow-rollback1"},"name":"first-vela-app","namespace":"default"},"spec":{"components":[{"name":"express-server","properties":{"image":"crccheck/hello-world","port":8000},"traits":[{"properties":{"domain":"testsvc.example.com","http":{"/":8000}},"type":"ingress-1-20"}],"type":"webservice"}]}}`,
			Status:         model.RevisionStatusComplete,
			WorkflowName:   workflow.Name,
			EnvName:        "rollback",
		})
		Expect(err).Should(BeNil())

		_, err = workflowService.RollbackRecord(ctx, &model.Application{
			Name: appName,
		}, workflow, "test-workflow-2-2", "revision-rollback0")
		Expect(err).Should(BeNil())

		recordsNum, err := workflowService.Store.Count(ctx, &model.WorkflowRecord{
			AppPrimaryKey:      appName,
			WorkflowName:       workflow.Name,
			RevisionPrimaryKey: "revision-rollback0",
		}, nil)
		Expect(err).Should(BeNil())
		Expect(recordsNum).Should(Equal(int64(1)))

		By("rollback application without revision version")
		app.Annotations[oam.AnnotationPublishVersion] = "workflow-rollback-2"
		app.Status.Workflow.AppRevision = "workflow-rollback-2"
		_, err = workflowService.CreateWorkflowRecord(context.TODO(), &model.Application{
			Name: appName,
		}, app, workflow)
		Expect(err).Should(BeNil())

		_, err = workflowService.RollbackRecord(ctx, &model.Application{
			Name: appName,
		}, workflow, "workflow-rollback-2", "")
		Expect(err).Should(BeNil())

		recordsNum, err = workflowService.Store.Count(ctx, &model.WorkflowRecord{
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
		err = workflowService.Store.Get(ctx, originalRevision)
		Expect(err).Should(BeNil())
		Expect(originalRevision.Status).Should(Equal(model.RevisionStatusRollback))
		Expect(originalRevision.RollbackVersion).Should(Equal("revision-rollback0"))
	})

	It("Test resetRevisionsAndRecords function", func() {
		ctx := context.TODO()

		err := workflowService.Store.Add(ctx, &model.WorkflowRecord{
			AppPrimaryKey: "reset-app",
			WorkflowName:  "reset-workflow",
			Name:          "reset-record",
			Finished:      "false",
			Steps: []model.WorkflowStepStatus{
				{
					StepStatus: model.StepStatus{
						Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded,
					},
				},
				{
					StepStatus: model.StepStatus{
						Phase: workflowv1alpha1.WorkflowStepPhaseRunning,
					},
				},
			},
		})
		Expect(err).Should(BeNil())

		err = resetRevisionsAndRecords(ctx, workflowService.Store, "reset-app", "reset-workflow", "", "")
		Expect(err).Should(BeNil())

		record := &model.WorkflowRecord{
			AppPrimaryKey: "reset-app",
			WorkflowName:  "reset-workflow",
			Name:          "reset-record",
		}
		err = workflowService.Store.Get(ctx, record)
		Expect(err).Should(BeNil())
		Expect(record.Status).Should(Equal(model.RevisionStatusTerminated))
		Expect(record.Finished).Should(Equal("true"))
		Expect(record.Steps[1].Phase).Should(Equal(model.WorkflowStepPhaseStopped))
	})

	It("Test deleting workflow", func() {
		By("Test deleting the workflow from the mongo")
		mongodbDriver, err := mongodb.New(context.TODO(), datastore.Config{
			URL:      "mongodb://localhost:27017",
			Database: "kubevela",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(mongodbDriver).ToNot(BeNil())

		Expect(mongodbDriver.BatchAdd(context.Background(), []datastore.Entity{
			&model.Workflow{
				Name:          "workflow-default",
				AppPrimaryKey: "war-app",
			},
			&model.WorkflowRecord{
				Name:               "workflow-default-20220809081934217",
				WorkflowName:       "workflow-default",
				AppPrimaryKey:      "war-app",
				RevisionPrimaryKey: "20220809081934216",
			},
			&model.WorkflowRecord{
				WorkflowName:       "workflow-default",
				AppPrimaryKey:      "war-app",
				Name:               "workflow-default-20220809082525833",
				RevisionPrimaryKey: "20220809082525832",
			},
		})).ToNot(HaveOccurred())

		var record = model.WorkflowRecord{
			AppPrimaryKey: "war-app",
			WorkflowName:  "workflow-default",
		}
		records, err := mongodbDriver.List(context.TODO(), &record, &datastore.ListOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(len(records)).Should(Equal(2))

		srv := workflowServiceImpl{
			Store: mongodbDriver,
		}
		Expect(srv.DeleteWorkflowByApp(context.TODO(), &model.Application{Name: "war-app"})).ToNot(HaveOccurred())
		wc, err := mongodbDriver.Count(context.TODO(), &model.Workflow{AppPrimaryKey: "war-app"}, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(int(wc)).Should(Equal(0))

		list, err := mongodbDriver.List(context.TODO(), &model.WorkflowRecord{AppPrimaryKey: "war-app"}, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(list)).Should(Equal(0))
	})
})

var yamlStr = `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  annotations:
    app.oam.dev/appName: app-workflow
    app.oam.dev/deployVersion: "1234"
    app.oam.dev/publishVersion: test-workflow-name-111
    app.oam.dev/workflowName: test-workflow-2
  name: app-workflow
  namespace: default
spec:
  components:
  - name: express-server
    properties:
      image: crccheck/hello-world
      port: 8000
    type: webservice
  - name: express-server2
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
    - name: apply-server2
      properties:
        component: express-server
      type: apply-component
    - name: group
      subSteps:
      - name: suspend
        timeout: 1s
        type: suspend
      type: step-group
status:
  status: workflowFailed
  workflow:
    appRevision: test-workflow-name-111
    contextBackend:
      name: workflow-app-workflow-context
      namespace: default
      uid: ef9bcf49-66a7-4c69-b349-150810aa2bac
    endTime: "2022-10-28T06:45:46Z"
    finished: true
    message: The workflow terminates because of the failed steps
    mode: StepByStep-DAG
    startTime: "2022-10-28T06:45:37Z"
    status: failed
    steps:
    - firstExecuteTime: "2022-10-28T06:45:37Z"
      id: fg5uiwroe6
      lastExecuteTime: "2022-10-28T06:45:45Z"
      name: apply-server
      phase: succeeded
      type: apply-component
    - firstExecuteTime: "2022-10-28T06:45:45Z"
      id: prouwp48y7
      lastExecuteTime: "2022-10-28T06:45:45Z"
      name: apply-server2
      phase: succeeded
      type: apply-component
    - firstExecuteTime: "2022-10-28T06:45:45Z"
      id: s6o27xnkzq
      lastExecuteTime: "2022-10-28T06:45:46Z"
      name: group
      phase: failed
      reason: Timeout
      subSteps:
      - firstExecuteTime: "2022-10-28T06:45:45Z"
        id: ctu63esz2m
        lastExecuteTime: "2022-10-28T06:45:46Z"
        name: suspend
        phase: failed
        reason: Timeout
        type: suspend
      type: step-group
    suspend: false
    terminated: true`

func (w *workflowServiceImpl) createTestApplicationRevision(ctx context.Context, revision *model.ApplicationRevision) error {
	if err := w.Store.Add(ctx, revision); err != nil {
		return err
	}
	return nil
}
