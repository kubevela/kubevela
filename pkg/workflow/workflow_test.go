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
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		done, pause, err := wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(pause).Should(BeFalse())
		Expect(done).Should(BeFalse())
		workflowStatus := app.Status.Workflow
		Expect(workflowStatus.ContextBackend.Name).Should(BeEquivalentTo("workflow-" + revision.Name))
		workflowStatus.ContextBackend = nil
		Expect(cmp.Diff(*workflowStatus, common.WorkflowStatus{
			AppRevision: workflowStatus.AppRevision,
			StepIndex:   1,
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
		done, pause, err = wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(pause).Should(BeFalse())
		Expect(done).Should(BeTrue())
		app.Status.Workflow.ContextBackend = nil
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			StepIndex:   3,
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
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		done, pause, err := wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(done).Should(BeFalse())
		Expect(pause).Should(BeTrue())
		wfStatus := *app.Status.Workflow
		wfStatus.ContextBackend = nil
		Expect(cmp.Diff(wfStatus, common.WorkflowStatus{
			AppRevision: wfStatus.AppRevision,
			StepIndex:   2,
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
		done, pause, err = wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(pause).Should(BeTrue())
		Expect(done).Should(BeFalse())

		// check resume
		app.Status.Workflow.Suspend = false
		// check app meta changed
		app.Name = "changed"
		done, pause, err = wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(pause).Should(BeFalse())
		Expect(done).Should(BeTrue())
		app.Status.Workflow.ContextBackend = nil
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			StepIndex:   3,
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

		done, pause, err = wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(pause).Should(BeFalse())
		Expect(done).Should(BeTrue())
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
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		done, pause, err := wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(pause).Should(BeFalse())
		Expect(done).Should(BeTrue())
		app.Status.Workflow.ContextBackend = nil
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			StepIndex:   2,
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

		done, pause, err = wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(pause).Should(BeFalse())
		Expect(done).Should(BeTrue())
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
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		done, pause, err := wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).To(HaveOccurred())
		Expect(done).Should(BeFalse())
		Expect(pause).Should(BeFalse())
		app.Status.Workflow.ContextBackend = nil
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			StepIndex:   1,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))
	})

	It("skip workflow", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{})
		wf := NewWorkflow(app, k8sClient, common.WorkflowModeStep)
		done, pause, err := wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(done).Should(BeTrue())
		Expect(pause).Should(BeFalse())
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
		done, pause, err := wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(pause).Should(BeFalse())
		Expect(done).Should(BeFalse())
		app.Status.Workflow.ContextBackend = nil
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			StepIndex:   2,
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

		done, pause, err = wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(pause).Should(BeFalse())
		Expect(done).Should(BeFalse())

		pending = false
		done, pause, err = wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(pause).Should(BeFalse())
		Expect(done).Should(BeTrue())
		app.Status.Workflow.ContextBackend = nil
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: app.Status.Workflow.AppRevision,
			StepIndex:   3,
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
})

func makeTestCase(steps []oamcore.WorkflowStep) (*oamcore.Application, []wfTypes.TaskRunner) {
	app := &oamcore.Application{
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
