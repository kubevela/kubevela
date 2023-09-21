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

package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

var workflowSpec = v1beta1.ApplicationSpec{
	Components: []common.ApplicationComponent{{
		Name:       "test-component",
		Type:       "worker",
		Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
	}},
	Workflow: &v1beta1.Workflow{
		Steps: []workflowv1alpha1.WorkflowStep{{
			WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
				Name:       "test-wf1",
				Type:       "foowf",
				Properties: &runtime.RawExtension{Raw: []byte(`{"namespace":"default"}`)},
			},
		}},
	},
}

func TestWorkflowSuspend(t *testing.T) {
	c := initArgs()
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	ctx := context.TODO()

	testCases := map[string]struct {
		app         *v1beta1.Application
		expected    *v1beta1.Application
		step        string
		expectedErr string
	}{
		"no app name specified": {
			expectedErr: "please specify the name of application/workflow",
		},
		"workflow not running": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow-not-running",
					Namespace: "default",
				},
				Spec:   workflowSpec,
				Status: common.AppStatus{},
			},
			expectedErr: "the workflow in application workflow-not-running is not start",
		},
		"suspend successfully": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow",
					Namespace: "test",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend: false,
					},
				},
			},
			expected: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow",
					Namespace: "test",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend: true,
					},
				},
			},
		},
		"step not found": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "step-not-found",
					Namespace: "default",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend: false,
					},
				},
			},
			step:        "not-found",
			expectedErr: "can not find",
		},
		"suspend all": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "suspend-all",
					Namespace: "default",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Steps: []workflowv1alpha1.WorkflowStepStatus{
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Name:  "step1",
									Phase: workflowv1alpha1.WorkflowStepPhaseRunning,
								},
								SubStepsStatus: []workflowv1alpha1.StepStatus{
									{
										Name:  "sub1",
										Phase: workflowv1alpha1.WorkflowStepPhaseRunning,
									},
								},
							},
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Name:  "step2",
									Phase: workflowv1alpha1.WorkflowStepPhaseRunning,
								},
								SubStepsStatus: []workflowv1alpha1.StepStatus{
									{
										Name:  "sub2",
										Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded,
									},
								},
							},
						},
					},
				},
			},
			expected: &v1beta1.Application{
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend: true,
						Steps: []workflowv1alpha1.WorkflowStepStatus{
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Name:  "step1",
									Phase: workflowv1alpha1.WorkflowStepPhaseSuspending,
								},
								SubStepsStatus: []workflowv1alpha1.StepStatus{
									{
										Name:  "sub1",
										Phase: workflowv1alpha1.WorkflowStepPhaseSuspending,
									},
								},
							},
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Name:  "step2",
									Phase: workflowv1alpha1.WorkflowStepPhaseSuspending,
								},
								SubStepsStatus: []workflowv1alpha1.StepStatus{
									{
										Name:  "sub2",
										Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded,
									},
								},
							},
						},
					},
				},
			},
		},
		"suspend specific step": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "suspend-step",
					Namespace: "default",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Steps: []workflowv1alpha1.WorkflowStepStatus{
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Name:  "step1",
									Phase: workflowv1alpha1.WorkflowStepPhaseRunning,
								},
								SubStepsStatus: []workflowv1alpha1.StepStatus{
									{
										Name:  "sub1",
										Phase: workflowv1alpha1.WorkflowStepPhaseRunning,
									},
								},
							},
						},
					},
				},
			},
			expected: &v1beta1.Application{
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend: true,
						Steps: []workflowv1alpha1.WorkflowStepStatus{
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Name:  "step1",
									Phase: workflowv1alpha1.WorkflowStepPhaseSuspending,
								},
								SubStepsStatus: []workflowv1alpha1.StepStatus{
									{
										Name:  "sub1",
										Phase: workflowv1alpha1.WorkflowStepPhaseSuspending,
									},
								},
							},
						},
					},
				},
			},
			step: "step1",
		},
		"suspend specific sub step": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "suspend-sub-step",
					Namespace: "default",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Steps: []workflowv1alpha1.WorkflowStepStatus{
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Name:  "step1",
									Phase: workflowv1alpha1.WorkflowStepPhaseRunning,
								},
								SubStepsStatus: []workflowv1alpha1.StepStatus{
									{
										Name:  "sub1",
										Phase: workflowv1alpha1.WorkflowStepPhaseRunning,
									},
								},
							},
						},
					},
				},
			},
			expected: &v1beta1.Application{
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend: true,
						Steps: []workflowv1alpha1.WorkflowStepStatus{
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Name:  "step1",
									Phase: workflowv1alpha1.WorkflowStepPhaseRunning,
								},
								SubStepsStatus: []workflowv1alpha1.StepStatus{
									{
										Name:  "sub1",
										Phase: workflowv1alpha1.WorkflowStepPhaseSuspending,
									},
								},
							},
						},
					},
				},
			},
			step: "sub1",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cmd := NewWorkflowSuspendCommand(c, ioStream, &WorkflowArgs{Args: c, Writer: ioStream.Out})
			initCommand(cmd)
			// clean up the arguments before start
			cmd.SetArgs([]string{})
			client, err := c.GetClient()
			r.NoError(err)
			if tc.app != nil {
				err := client.Create(ctx, tc.app)
				r.NoError(err)
				cmdArgs := []string{tc.app.Name}
				if tc.app.Namespace != corev1.NamespaceDefault {
					err := client.Create(ctx, &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: tc.app.Namespace,
						},
					})
					r.NoError(err)
					cmdArgs = append(cmdArgs, "-n", tc.app.Namespace)
					cmd.SetArgs([]string{tc.app.Name, "-n", tc.app.Namespace})
				}
				if tc.step != "" {
					cmdArgs = append(cmdArgs, "--step", tc.step)
				}
				cmd.SetArgs(cmdArgs)
			}
			err = cmd.Execute()
			if tc.expectedErr != "" {
				r.Contains(err.Error(), tc.expectedErr)
				return
			}
			r.NoError(err)

			wf := &v1beta1.Application{}
			err = client.Get(ctx, types.NamespacedName{
				Namespace: tc.app.Namespace,
				Name:      tc.app.Name,
			}, wf)
			r.NoError(err)
			r.Equal(true, wf.Status.Workflow.Suspend)
			r.Equal(tc.expected.Status, wf.Status)
		})
	}
}

func TestWorkflowResume(t *testing.T) {
	c := initArgs()
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	ctx := context.TODO()

	testCases := map[string]struct {
		app         *v1beta1.Application
		expectedErr error
	}{
		"no app name specified": {
			expectedErr: fmt.Errorf("please specify the name of application/workflow"),
		},
		"workflow not suspended": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow-not-suspended",
					Namespace: "default",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend: false,
					},
				},
			},
		},
		"workflow not running": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow-not-running",
					Namespace: "default",
				},
				Spec:   workflowSpec,
				Status: common.AppStatus{},
			},
			expectedErr: fmt.Errorf("the workflow in application workflow-not-running is not start"),
		},
		"workflow terminated": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow-terminated",
					Namespace: "default",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Terminated: true,
					},
				},
			},
			expectedErr: fmt.Errorf("can not resume a terminated workflow"),
		},
		"resume successfully": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow",
					Namespace: "test",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend: true,
						Steps: []workflowv1alpha1.WorkflowStepStatus{
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Type:  "suspend",
									Phase: "running",
								},
							},
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Type: "step-group",
								},
								SubStepsStatus: []workflowv1alpha1.StepStatus{
									{
										Type:  "suspend",
										Phase: "running",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cmd := NewWorkflowResumeCommand(c, ioStream, &WorkflowArgs{Args: c, Writer: ioStream.Out})
			initCommand(cmd)
			// clean up the arguments before start
			cmd.SetArgs([]string{})
			client, err := c.GetClient()
			r.NoError(err)
			if tc.app != nil {
				err := client.Create(ctx, tc.app)
				r.NoError(err)

				if tc.app.Namespace != corev1.NamespaceDefault {
					err := client.Create(ctx, &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: tc.app.Namespace,
						},
					})
					r.NoError(err)
					cmd.SetArgs([]string{tc.app.Name, "-n", tc.app.Namespace})
				} else {
					cmd.SetArgs([]string{tc.app.Name})
				}
			}
			err = cmd.Execute()
			if tc.expectedErr != nil {
				r.Equal(tc.expectedErr, err)
				return
			}
			r.NoError(err)

			wf := &v1beta1.Application{}
			err = client.Get(ctx, types.NamespacedName{
				Namespace: tc.app.Namespace,
				Name:      tc.app.Name,
			}, wf)
			r.NoError(err)
			r.Equal(false, wf.Status.Workflow.Suspend)
			for _, step := range wf.Status.Workflow.Steps {
				if step.Type == "suspend" {
					r.Equal(step.Phase, workflowv1alpha1.WorkflowStepPhaseRunning)
				}
				for _, sub := range step.SubStepsStatus {
					if sub.Type == "suspend" {
						r.Equal(sub.Phase, workflowv1alpha1.WorkflowStepPhaseRunning)
					}
				}
			}
		})
	}
}

func TestWorkflowTerminate(t *testing.T) {
	c := initArgs()
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	ctx := context.TODO()

	testCases := map[string]struct {
		app         *v1beta1.Application
		expectedErr error
	}{
		"no app name specified": {
			expectedErr: fmt.Errorf("please specify the name of application/workflow"),
		},
		"workflow not running": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow-not-running",
					Namespace: "default",
				},
				Spec:   workflowSpec,
				Status: common.AppStatus{},
			},
			expectedErr: fmt.Errorf("the workflow in application workflow-not-running is not start"),
		},
		"terminate successfully": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow",
					Namespace: "test",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Terminated: false,
						Steps: []workflowv1alpha1.WorkflowStepStatus{
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Name:  "1",
									Type:  "suspend",
									Phase: "succeeded",
								},
							},
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Name:  "2",
									Type:  "suspend",
									Phase: "running",
								},
							},
							{
								StepStatus: workflowv1alpha1.StepStatus{
									Name:  "3",
									Type:  "step-group",
									Phase: "running",
								},
								SubStepsStatus: []workflowv1alpha1.StepStatus{
									{
										Type:  "suspend",
										Phase: "running",
									},
									{
										Type:  "suspend",
										Phase: "succeeded",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cmd := NewWorkflowTerminateCommand(c, ioStream, &WorkflowArgs{Args: c, Writer: ioStream.Out})
			initCommand(cmd)
			// clean up the arguments before start
			cmd.SetArgs([]string{})
			client, err := c.GetClient()
			r.NoError(err)
			if tc.app != nil {
				err := client.Create(ctx, tc.app)
				r.NoError(err)

				if tc.app.Namespace != corev1.NamespaceDefault {
					err := client.Create(ctx, &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: tc.app.Namespace,
						},
					})
					r.NoError(err)
					cmd.SetArgs([]string{tc.app.Name, "-n", tc.app.Namespace})
				} else {
					cmd.SetArgs([]string{tc.app.Name})
				}
			}
			err = cmd.Execute()
			if tc.expectedErr != nil {
				r.Equal(tc.expectedErr, err)
				return
			}
			r.NoError(err)

			wf := &v1beta1.Application{}
			err = client.Get(ctx, types.NamespacedName{
				Namespace: tc.app.Namespace,
				Name:      tc.app.Name,
			}, wf)
			r.NoError(err)
			r.Equal(true, wf.Status.Workflow.Terminated)
			for _, step := range wf.Status.Workflow.Steps {
				if step.Phase != workflowv1alpha1.WorkflowStepPhaseSucceeded {
					r.Equal(step.Phase, workflowv1alpha1.WorkflowStepPhaseFailed)
					r.Equal(step.Reason, wfTypes.StatusReasonTerminate)
				}
				for _, sub := range step.SubStepsStatus {
					if sub.Phase != workflowv1alpha1.WorkflowStepPhaseSucceeded {
						r.Equal(sub.Phase, workflowv1alpha1.WorkflowStepPhaseFailed)
						r.Equal(sub.Reason, wfTypes.StatusReasonTerminate)
					}
				}
			}
		})
	}
}

func TestWorkflowRestart(t *testing.T) {
	c := initArgs()
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	ctx := context.TODO()

	testCases := map[string]struct {
		app         *v1beta1.Application
		expectedErr error
	}{
		"no app name specified": {
			expectedErr: fmt.Errorf("please specify the name of application/workflow"),
		},
		"workflow not running": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow-not-running",
					Namespace: "default",
				},
				Spec:   workflowSpec,
				Status: common.AppStatus{},
			},
			expectedErr: fmt.Errorf("the workflow in application workflow-not-running is not start"),
		},
		"restart successfully": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow",
					Namespace: "test",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Terminated: true,
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cmd := NewWorkflowRestartCommand(c, ioStream, &WorkflowArgs{Args: c, Writer: ioStream.Out})
			initCommand(cmd)
			// clean up the arguments before start
			cmd.SetArgs([]string{})
			client, err := c.GetClient()
			r.NoError(err)
			if tc.app != nil {
				err := client.Create(ctx, tc.app)
				r.NoError(err)

				if tc.app.Namespace != corev1.NamespaceDefault {
					err := client.Create(ctx, &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: tc.app.Namespace,
						},
					})
					r.NoError(err)
					cmd.SetArgs([]string{tc.app.Name, "-n", tc.app.Namespace})
				} else {
					cmd.SetArgs([]string{tc.app.Name})
				}
			}
			err = cmd.Execute()
			if tc.expectedErr != nil {
				r.Equal(tc.expectedErr, err)
				return
			}
			r.NoError(err)

			wf := &v1beta1.Application{}
			err = client.Get(ctx, types.NamespacedName{
				Namespace: tc.app.Namespace,
				Name:      tc.app.Name,
			}, wf)
			r.NoError(err)
			var nilStatus *common.WorkflowStatus = nil
			r.Equal(nilStatus, wf.Status.Workflow)
		})
	}
}

func TestWorkflowRollback(t *testing.T) {
	c := initArgs()
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	ctx := context.TODO()

	testCases := map[string]struct {
		app         *v1beta1.Application
		revision    *v1beta1.ApplicationRevision
		expectedErr error
	}{
		"no app name specified": {
			expectedErr: fmt.Errorf("please specify the name of application/workflow"),
		},
		"workflow running": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow-running",
					Namespace: "default",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend:    false,
						Terminated: false,
						Finished:   false,
					},
				},
			},
			expectedErr: fmt.Errorf("can not rollback a running workflow"),
		},
		"invalid revision": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-revision",
					Namespace: "default",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend: true,
					},
				},
			},
			expectedErr: fmt.Errorf("the latest revision is not set: invalid-revision"),
		},
		"rollback successfully": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow",
					Namespace: "test",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					LatestRevision: &common.Revision{
						Name: "revision-v1",
					},
					Workflow: &common.WorkflowStatus{
						Terminated: true,
					},
				},
			},
			revision: &v1beta1.ApplicationRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "revision-v1",
					Namespace: "test",
				},
				Spec: v1beta1.ApplicationRevisionSpec{
					ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
						Application: v1beta1.Application{
							Spec: v1beta1.ApplicationSpec{
								Components: []common.ApplicationComponent{{
									Name:       "revision-component",
									Type:       "worker",
									Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
								}},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cmd := NewWorkflowRollbackCommand(c, ioStream, &WorkflowArgs{Args: c, Writer: ioStream.Out})
			initCommand(cmd)
			// clean up the arguments before start
			cmd.SetArgs([]string{})
			client, err := c.GetClient()
			r.NoError(err)
			if tc.app != nil {
				err := client.Create(ctx, tc.app)
				r.NoError(err)

				if tc.app.Namespace != corev1.NamespaceDefault {
					err := client.Create(ctx, &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: tc.app.Namespace,
						},
					})
					r.NoError(err)
					cmd.SetArgs([]string{tc.app.Name, "-n", tc.app.Namespace})
				} else {
					cmd.SetArgs([]string{tc.app.Name})
				}
			}
			if tc.revision != nil {
				err := client.Create(ctx, tc.revision)
				r.NoError(err)
			}
			err = cmd.Execute()
			if tc.expectedErr != nil {
				r.Equal(tc.expectedErr, err)
				return
			}
			r.NoError(err)

			wf := &v1beta1.Application{}
			err = client.Get(ctx, types.NamespacedName{
				Namespace: tc.app.Namespace,
				Name:      tc.app.Name,
			}, wf)
			r.NoError(err)
			r.Equal(wf.Spec.Components[0].Name, "revision-component")
		})
	}
}

func TestWorkflowList(t *testing.T) {
	c := initArgs()
	buf := new(bytes.Buffer)
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: buf, ErrOut: os.Stderr}
	ctx := context.TODO()
	testCases := map[string]struct {
		workflows         []interface{}
		apps              []*v1beta1.Application
		workflowRuns      []*workflowv1alpha1.WorkflowRun
		expectedErr       error
		namespace         string
		expectAppListSize int
	}{
		"specified all namespaces flag": {
			workflows: []interface{}{
				&v1beta1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app-workflow",
						Namespace: "some-ns",
					},
					Spec: workflowSpec,
					Status: common.AppStatus{
						LatestRevision: &common.Revision{
							Name: "revision-v1",
						},
						Workflow: &common.WorkflowStatus{
							Terminated: true,
							Phase:      workflowv1alpha1.WorkflowStateInitializing,
							StartTime:  metav1.NewTime(time.Now().Add(-10 * time.Minute)),
							EndTime:    metav1.NewTime(time.Now()),
						},
					},
				},
				&workflowv1alpha1.WorkflowRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "workflow-run",
						Namespace: "some-ns",
					},
					Status: workflowv1alpha1.WorkflowRunStatus{
						Phase:     workflowv1alpha1.WorkflowStateExecuting,
						StartTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
						EndTime:   metav1.NewTime(time.Now()),
					},
				},
			},
			expectedErr: nil,
		},
		"specified namespace flag": {
			workflows: []interface{}{
				&workflowv1alpha1.WorkflowRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "workflow-run-1",
						Namespace: "test",
					},
					Status: workflowv1alpha1.WorkflowRunStatus{
						Phase:     workflowv1alpha1.WorkflowStateExecuting,
						StartTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
						EndTime:   metav1.NewTime(time.Now()),
					},
				},
			},
			namespace:   "test",
			expectedErr: nil,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			buf.Reset()
			cmd := NewWorkflowListCommand(c, ioStream, &WorkflowArgs{Args: c, Writer: ioStream.Out})
			initCommand(cmd)
			// clean up the arguments before start
			cmd.SetArgs([]string{})
			client, err := c.GetClient()
			r.NoError(err)
			if tc.workflows != nil && len(tc.workflows) > 0 {
				for _, w := range tc.workflows {
					if workflow, ok := w.(*workflowv1alpha1.WorkflowRun); ok {
						err := client.Create(ctx, workflow)
						r.NoError(err)
					} else if app, ok := w.(*v1beta1.Application); ok {
						err := client.Create(ctx, app)
						r.NoError(err)
					}
				}
			}
			args := []string{}
			if tc.namespace == "" {
				args = append(args, "-A")
			} else {
				args = append(args, "-n", tc.namespace)
			}

			cmd.SetArgs(args)
			err = cmd.Execute()

			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

			headerFields := strings.Fields(lines[0])
			if tc.namespace == "" {
				r.Equal(headerFields[0], "NAMESPACE")

			} else {
				r.Equal(headerFields[0], "NAME")
			}
			offset := 0
			for i, line := range lines[1:] {
				fields := strings.Fields(line)
				if workflow, ok := tc.workflows[i].(*workflowv1alpha1.WorkflowRun); ok {
					if tc.namespace == "" {
						r.Equal(fields[0], workflow.Namespace)
						offset = offset + 1
					} else if tc.namespace == workflow.Namespace {
						r.Equal(fields[0+offset], workflow.Name)
						r.Equal(fields[1+offset], "WorkflowRun")
						r.Equal(fields[2+offset], string(workflow.Status.Phase))
						r.Equal(fields[3+offset], workflow.Status.StartTime.Format("2006-01-02"))
						r.Equal(fields[4+offset], workflow.Status.StartTime.Format("15:04:05"))
						//skipping a few fields due to the format including  timezone information
						r.Equal(fields[7+offset], workflow.Status.EndTime.Format("2006-01-02"))
						r.Equal(fields[8+offset], workflow.Status.EndTime.Format("15:04:05"))
					}
				} else if app, ok := tc.workflows[i].(*v1beta1.Application); ok {
					offset = 0
					if tc.namespace == "" {
						r.Equal(fields[0+offset], app.Namespace)
						offset = offset + 1
					} else if tc.namespace == app.Namespace {
						r.Equal(fields[0+offset], app.Name)
						r.Equal(fields[1+offset], "Application")
						r.Equal(fields[2+offset], string(app.Status.Workflow.Phase))
						r.Equal(fields[3+offset], app.Status.Workflow.StartTime.Format("2006-01-02"))
						r.Equal(fields[4+offset], app.Status.Workflow.StartTime.Format("15:04:05"))
						//skipping a few fields due to the format including  timezone information
						r.Equal(fields[7+offset], app.Status.Workflow.EndTime.Format("2006-01-02"))
						r.Equal(fields[8+offset], app.Status.Workflow.EndTime.Format("15:04:05"))
					}
				}

			}

			if tc.expectedErr != nil {
				r.Equal(tc.expectedErr, err)
				return
			}
			r.NoError(err)

		})
	}
}
