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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestWorkflowSuspend(t *testing.T) {
	c := initArgs()
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	cmd := NewWorkflowSuspendCommand(c, ioStream)
	initCommand(cmd)
	ctx := context.TODO()

	testCases := map[string]struct {
		app         *v1beta1.Application
		expectedErr error
	}{
		"no app name specified": {
			expectedErr: fmt.Errorf("must specify application name"),
		},
		"no workflow in app": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-workflow",
					Namespace: "default",
				},
			},
			expectedErr: fmt.Errorf("the application must have workflow"),
		},
		"suspend successfully": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "test-component",
						Type:       "worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					}},
					Workflow: &v1beta1.Workflow{
						Steps: []v1beta1.WorkflowStep{{
							Name:       "test-wf1",
							Type:       "foowf",
							Properties: runtime.RawExtension{Raw: []byte(`{"namespace":"default"}`)},
						}},
					},
				},
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

			if tc.app != nil {
				err := c.Client.Create(ctx, tc.app)
				r.NoError(err)

				cmd.SetArgs([]string{tc.app.Name})
			}
			err := cmd.Execute()
			if tc.expectedErr != nil {
				r.Equal(tc.expectedErr, err)
				return
			}
			r.NoError(err)

			wf := &v1beta1.Application{}
			err = c.Client.Get(ctx, types.NamespacedName{
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
	cmd := NewWorkflowResumeCommand(c, ioStream)
	initCommand(cmd)
	ctx := context.TODO()

	testCases := map[string]struct {
		app         *v1beta1.Application
		expectedErr error
	}{
		"no app name specified": {
			expectedErr: fmt.Errorf("must specify application name"),
		},
		"no workflow in app": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-workflow",
					Namespace: "default",
				},
			},
			expectedErr: fmt.Errorf("the application must have workflow"),
		},
		"workflow not suspended": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow-not-suspended",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "test-component",
						Type:       "worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					}},
					Workflow: &v1beta1.Workflow{
						Steps: []v1beta1.WorkflowStep{{
							Name:       "test-wf1",
							Type:       "foowf",
							Properties: runtime.RawExtension{Raw: []byte(`{"namespace":"default"}`)},
						}},
					},
				},
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend: false,
					},
				},
			},
		},
		"resume successfully": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "test-component",
						Type:       "worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					}},
					Workflow: &v1beta1.Workflow{
						Steps: []v1beta1.WorkflowStep{{
							Name:       "test-wf1",
							Type:       "foowf",
							Properties: runtime.RawExtension{Raw: []byte(`{"namespace":"default"}`)},
						}},
					},
				},
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						Suspend: true,
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			if tc.app != nil {
				err := c.Client.Create(ctx, tc.app)
				r.NoError(err)

				cmd.SetArgs([]string{tc.app.Name})
			}
			err := cmd.Execute()
			if tc.expectedErr != nil {
				r.Equal(tc.expectedErr, err)
				return
			}
			r.NoError(err)

			wf := &v1beta1.Application{}
			err = c.Client.Get(ctx, types.NamespacedName{
				Namespace: tc.app.Namespace,
				Name:      tc.app.Name,
			}, wf)
			r.NoError(err)
			r.Equal(false, wf.Status.Workflow.Suspend)
		})
	}
}
