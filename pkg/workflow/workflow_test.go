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

package workflow

import (
	"context"
	"encoding/json"

	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

var _ = Describe("Test Workflow", func() {

	BeforeEach(func() {
		cm := &corev1.ConfigMap{}
		revJson, err := yaml.YAMLToJSON([]byte(revYaml))
		Expect(err).ToNot(HaveOccurred())
		err = json.Unmarshal(revJson, cm)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Create(context.Background(), cm)
		if err != nil && !kerrors.IsAlreadyExists(err) {
			Expect(err).ToNot(HaveOccurred())
		}

	})
	It("Workflow test for failed", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "success",
			},
			{
				Name: "s2",
				Type: "failed",
			},
			{
				Name: "s3",
				Type: "success",
			},
		})
		ctx := monitorContext.NewTraceContext(context.Background(), "test-app")
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		state, err := wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateExecuting))
		workflowStatus := app.Status.Workflow
		Expect(workflowStatus.ContextBackend.Name).Should(BeEquivalentTo("workflow-" + app.Name + "-context"))
		workflowStatus.ContextBackend = nil
		cleanStepTimeStamp(workflowStatus)
		Expect(cmp.Diff(*workflowStatus, common.WorkflowStatus{
			AppRevision: workflowStatus.AppRevision,
			Mode:        common.WorkflowModeStep,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s2",
				Type:  "failed",
				Phase: common.WorkflowStepPhaseFailed,
			}},
		})).Should(BeEquivalentTo(""))

		app, runners = makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "success",
			},
			{
				Name: "s2",
				Type: "success",
			},
			{
				Name: "s3",
				Type: "success",
			},
		})

		app.Status.Workflow = workflowStatus
		wf = NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		state, err = wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateTerminated))
		app.Status.Workflow.Finished = true
		state, err = wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateSucceeded))
		app.Status.Workflow.ContextBackend = nil
		cleanStepTimeStamp(app.Status.Workflow)
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			Mode:        common.WorkflowModeStep,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s2",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s3",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))

	})

	It("test for suspend", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "success",
			},
			{
				Name: "s2",
				Type: "suspend",
			},
			{
				Name: "s3",
				Type: "success",
			},
		})
		ctx := monitorContext.NewTraceContext(context.Background(), "test-app")
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		state, err := wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateSuspended))
		wfStatus := *app.Status.Workflow
		wfStatus.ContextBackend = nil
		cleanStepTimeStamp(&wfStatus)
		Expect(cmp.Diff(wfStatus, common.WorkflowStatus{
			AppRevision: wfStatus.AppRevision,
			Mode:        common.WorkflowModeStep,
			Suspend:     true,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s2",
				Type:  "suspend",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))

		// check suspend...
		state, err = wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateSuspended))

		// check resume
		app.Status.Workflow.Suspend = false
		// check app meta changed
		app.Labels = map[string]string{"for-test": "changed"}
		state, err = wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateSucceeded))
		app.Status.Workflow.ContextBackend = nil
		cleanStepTimeStamp(app.Status.Workflow)
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			Mode:        common.WorkflowModeStep,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s2",
				Type:  "suspend",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s3",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))

		state, err = wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateSucceeded))
	})

	It("test for terminate", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "success",
			},
			{
				Name: "s2",
				Type: "terminate",
			},
		})
		ctx := monitorContext.NewTraceContext(context.Background(), "test-app")
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		state, err := wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateTerminated))
		app.Status.Workflow.ContextBackend = nil
		cleanStepTimeStamp(app.Status.Workflow)
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			Mode:        common.WorkflowModeStep,
			Terminated:  true,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s2",
				Type:  "terminate",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))

		state, err = wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateTerminated))
	})

	It("test for error", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "success",
			},
			{
				Name: "s2",
				Type: "error",
			},
		})
		ctx := monitorContext.NewTraceContext(context.Background(), "test-app")
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		state, err := wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).To(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateExecuting))
		app.Status.Workflow.ContextBackend = nil
		cleanStepTimeStamp(app.Status.Workflow)
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			Mode:        common.WorkflowModeStep,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))
	})

	It("skip workflow", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{})
		ctx := monitorContext.NewTraceContext(context.Background(), "test-app")
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		state, err := wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateFinished))
	})

	It("test for DAG", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "success",
			},
			{
				Name: "s2",
				Type: "pending",
			},
			{
				Name: "s3",
				Type: "success",
			},
		})
		pending = true
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeDAG)
		ctx := monitorContext.NewTraceContext(context.Background(), "test-app")
		state, err := wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateExecuting))
		app.Status.Workflow.ContextBackend = nil
		cleanStepTimeStamp(app.Status.Workflow)
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			Mode:        common.WorkflowModeDAG,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s3",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))

		state, err = wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateExecuting))

		pending = false
		state, err = wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateSucceeded))
		app.Status.Workflow.ContextBackend = nil
		cleanStepTimeStamp(app.Status.Workflow)
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			Mode:        common.WorkflowModeDAG,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s3",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s2",
				Type:  "pending",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))
	})

	It("step commit data without success", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "wait-with-set-var",
			},
			{
				Name: "s2",
				Type: "success",
			},
		})
		ctx := monitorContext.NewTraceContext(context.Background(), "test-app")
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		state, err := wf.ExecuteSteps(ctx, revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).Should(BeEquivalentTo(common.WorkflowStateExecuting))
		Expect(app.Status.Workflow.Steps[0].Phase).Should(BeEquivalentTo(common.WorkflowStepPhaseRunning))
		wfCtx, err := wfContext.LoadContext(k8sClient, app.Namespace, app.Name)
		Expect(err).ToNot(HaveOccurred())
		v, err := wfCtx.GetVar("saved")
		Expect(err).ToNot(HaveOccurred())
		saved, err := v.CueValue().Bool()
		Expect(err).ToNot(HaveOccurred())
		Expect(saved).Should(BeEquivalentTo(true))
	})
})

func makeTestCase(steps []oamcore.WorkflowStep) (*oamcore.Application, []wfTypes.TaskRunner) {
	app := &oamcore.Application{
		ObjectMeta: metav1.ObjectMeta{UID: "test-uid"},
		Spec: oamcore.ApplicationSpec{
			Workflow: &oamcore.Workflow{
				Steps: steps,
			},
		},
		Status: common.AppStatus{},
	}
	app.Namespace = "default"
	app.Name = "app"
	runners := []wfTypes.TaskRunner{}
	for _, step := range steps {
		runners = append(runners, makeRunner(step.Name, step.Type))
	}
	return app, runners
}

var pending bool

func makeRunner(name string, tpy string) wfTypes.TaskRunner {
	var run func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.WorkflowStepStatus, *wfTypes.Operation, error)
	switch tpy {
	case "suspend":
		run = func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			return common.WorkflowStepStatus{
					Name:  name,
					Type:  "suspend",
					Phase: common.WorkflowStepPhaseSucceeded,
				}, &wfTypes.Operation{
					Suspend: true,
				}, nil
		}
	case "terminate":
		run = func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			return common.WorkflowStepStatus{
					Name:  name,
					Type:  "terminate",
					Phase: common.WorkflowStepPhaseSucceeded,
				}, &wfTypes.Operation{
					Terminated: true,
				}, nil
		}
	case "success":
		run = func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			return common.WorkflowStepStatus{
				Name:  name,
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, &wfTypes.Operation{}, nil
		}
	case "failed":
		run = func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			return common.WorkflowStepStatus{
				Name:  name,
				Type:  "failed",
				Phase: common.WorkflowStepPhaseFailed,
			}, &wfTypes.Operation{}, nil
		}
	case "error":
		run = func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			return common.WorkflowStepStatus{
				Name:  name,
				Type:  "error",
				Phase: common.WorkflowStepPhaseRunning,
			}, &wfTypes.Operation{}, errors.New("error for test")
		}
	case "wait-with-set-var":
		run = func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			v, _ := value.NewValue(`saved: true`, nil, "")
			err := ctx.SetVar(v)
			return common.WorkflowStepStatus{
				Name:  name,
				Type:  "wait-with-set-var",
				Phase: common.WorkflowStepPhaseRunning,
			}, &wfTypes.Operation{}, err
		}

	default:
		run = func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			return common.WorkflowStepStatus{
				Name:  name,
				Type:  tpy,
				Phase: common.WorkflowStepPhaseSucceeded,
			}, &wfTypes.Operation{}, nil
		}

	}
	return &testTaskRunner{
		name: name,
		run:  run,
		checkPending: func(ctx wfContext.Context) bool {
			if tpy != "pending" {
				return false
			}
			if pending == true {
				return true
			}
			return false
		},
	}
}

var (
	revYaml = `apiVersion: v1
data:
  components: '{"server":"{\"Scopes\":null,\"StandardWorkload\":\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"kind\\\":\\\"Pod\\\",\\\"metadata\\\":{\\\"labels\\\":{\\\"app\\\":\\\"nginx\\\"}},\\\"spec\\\":{\\\"containers\\\":[{\\\"env\\\":[{\\\"name\\\":\\\"APP\\\",\\\"value\\\":\\\"nginx\\\"}],\\\"image\\\":\\\"nginx:1.14.2\\\",\\\"imagePullPolicy\\\":\\\"IfNotPresent\\\",\\\"name\\\":\\\"main\\\",\\\"ports\\\":[{\\\"containerPort\\\":8080,\\\"protocol\\\":\\\"TCP\\\"}]}]}}\",\"Traits\":[\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"kind\\\":\\\"Service\\\",\\\"metadata\\\":{\\\"name\\\":\\\"my-service\\\"},\\\"spec\\\":{\\\"ports\\\":[{\\\"port\\\":80,\\\"protocol\\\":\\\"TCP\\\",\\\"targetPort\\\":8080}],\\\"selector\\\":{\\\"app\\\":\\\"nginx\\\"}}}\"]}"}'
kind: ConfigMap
metadata:
  name: app-v1
  namespace: default
`
	revision = &oamcore.ApplicationRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-v1",
		},
	}
)

type testTaskRunner struct {
	name         string
	run          func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.WorkflowStepStatus, *wfTypes.Operation, error)
	checkPending func(ctx wfContext.Context) bool
}

// Name return step name.
func (tr *testTaskRunner) Name() string {
	return tr.name
}

// Run execute task.
func (tr *testTaskRunner) Run(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
	return tr.run(ctx, nil)
}

// Pending check task should be executed or not.
func (tr *testTaskRunner) Pending(ctx wfContext.Context) bool {
	return tr.checkPending(ctx)
}

func cleanStepTimeStamp(wfStatus *common.WorkflowStatus) {
	wfStatus.StartTime = metav1.Time{}
	for index := range wfStatus.Steps {
		wfStatus.Steps[index].FirstExecuteTime = metav1.Time{}
		wfStatus.Steps[index].LastExecuteTime = metav1.Time{}
	}
}
