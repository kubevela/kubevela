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
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/custom"
)

var workflowSpec = v1beta1.ApplicationSpec{
	Components: []common.ApplicationComponent{{
		Name:       "test-component",
		Type:       "worker",
		Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
	}},
	Workflow: &v1beta1.Workflow{
		Steps: []v1beta1.WorkflowStep{{
			Name:       "test-wf1",
			Type:       "foowf",
			Properties: &runtime.RawExtension{Raw: []byte(`{"namespace":"default"}`)},
		}},
	},
}

func TestWorkflowSuspend(t *testing.T) {
	c := initArgs()
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	ctx := context.TODO()

	testCases := map[string]struct {
		app         *v1beta1.Application
		expectedErr error
	}{
		"no app name specified": {
			expectedErr: fmt.Errorf("must specify application name"),
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
			expectedErr: fmt.Errorf("the workflow in application is not running"),
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
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cmd := NewWorkflowSuspendCommand(c, ioStream)
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
			r.Equal(true, wf.Status.Workflow.Suspend)
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
			expectedErr: fmt.Errorf("must specify application name"),
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
			expectedErr: fmt.Errorf("the workflow in application is not running"),
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
						Steps: []common.WorkflowStepStatus{
							{
								StepStatus: common.StepStatus{
									Type:  "suspend",
									Phase: "running",
								},
							},
							{
								StepStatus: common.StepStatus{
									Type: "step-group",
								},
								SubStepsStatus: []common.WorkflowSubStepStatus{
									{
										StepStatus: common.StepStatus{
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
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cmd := NewWorkflowResumeCommand(c, ioStream)
			initCommand(cmd)
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
					r.Equal(step.Phase, common.WorkflowStepPhaseSucceeded)
				}
				for _, sub := range step.SubStepsStatus {
					if sub.Type == "suspend" {
						r.Equal(sub.Phase, common.WorkflowStepPhaseSucceeded)
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
			expectedErr: fmt.Errorf("must specify application name"),
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
			expectedErr: fmt.Errorf("the workflow in application is not running"),
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
						Steps: []common.WorkflowStepStatus{
							{
								StepStatus: common.StepStatus{
									Name:  "1",
									Type:  "suspend",
									Phase: "succeeded",
								},
							},
							{
								StepStatus: common.StepStatus{
									Name:  "2",
									Type:  "suspend",
									Phase: "running",
								},
							},
							{
								StepStatus: common.StepStatus{
									Name:  "3",
									Type:  "step-group",
									Phase: "running",
								},
								SubStepsStatus: []common.WorkflowSubStepStatus{
									{
										StepStatus: common.StepStatus{
											Type:  "suspend",
											Phase: "running",
										},
									},
									{
										StepStatus: common.StepStatus{
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
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cmd := NewWorkflowTerminateCommand(c, ioStream)
			initCommand(cmd)
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
				if step.Phase != common.WorkflowStepPhaseSucceeded {
					fmt.Println("======", step.Name)
					r.Equal(step.Phase, common.WorkflowStepPhaseFailed)
					r.Equal(step.Reason, custom.StatusReasonTerminate)
				}
				for _, sub := range step.SubStepsStatus {
					if sub.Phase != common.WorkflowStepPhaseSucceeded {
						r.Equal(sub.Phase, common.WorkflowStepPhaseFailed)
						r.Equal(sub.Reason, custom.StatusReasonTerminate)
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
			expectedErr: fmt.Errorf("must specify application name"),
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
			expectedErr: fmt.Errorf("the workflow in application is not running"),
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
			cmd := NewWorkflowRestartCommand(c, ioStream)
			initCommand(cmd)
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
			expectedErr: fmt.Errorf("must specify application name"),
		},
		"workflow running": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow-not-running",
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
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cmd := NewWorkflowRollbackCommand(c, ioStream)
			initCommand(cmd)
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
