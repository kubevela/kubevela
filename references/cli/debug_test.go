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
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestDebugApplicationWithWorkflow(t *testing.T) {
	c := initArgs()
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	ctx := context.TODO()

	testCases := map[string]struct {
		app         *v1beta1.Application
		cm          *corev1.ConfigMap
		step        string
		focus       string
		expectedErr string
	}{
		"no debug config map": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-debug-config-map",
					Namespace: "default",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{},
				},
			},
			step:        "test-wf1",
			focus:       "test",
			expectedErr: "failed to get debug configmap",
		},
		"config map no data": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-map-no-data",
					Namespace: "default",
					UID:       "12345",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{},
				},
			},
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-map-no-data-test-wf1-debug-12345",
					Namespace: "default",
				},
			},
			step:        "test-wf1",
			focus:       "test",
			expectedErr: "debug configmap is empty",
		},
		"config map error data": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-map-error-data",
					Namespace: "default",
					UID:       "12345",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{},
				},
			},
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-map-error-data-test-wf1-debug-12345",
					Namespace: "default",
				},
				Data: map[string]string{
					"debug": "error",
				},
			},
			step: "test-wf1",
		},
		"success": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "success",
					Namespace: "default",
					UID:       "12345",
				},
				Spec: workflowSpec,
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{},
				},
			},
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "success-test-wf1-debug-12345",
					Namespace: "default",
				},
				Data: map[string]string{
					"debug": `
test: test
`,
				},
			},
			step:  "test-wf1",
			focus: "test",
		},
		"success-component": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "success",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "test-component",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					}},
				},
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{},
				},
			},
			step: "test-component",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			d := &debugOpts{
				step:  tc.step,
				focus: tc.focus,
			}
			client, err := c.GetClient()
			r.NoError(err)
			if tc.cm != nil {
				err := client.Create(ctx, tc.cm)
				r.NoError(err)
			}
			wargs := &WorkflowArgs{
				Args: c,
				Type: instanceTypeApplication,
				App:  tc.app,
			}
			err = wargs.generateWorkflowInstance(ctx, client)
			r.NoError(err)
			err = d.debugApplication(ctx, wargs, c, ioStream)
			if tc.expectedErr != "" {
				r.Contains(err.Error(), tc.expectedErr)
				return
			}
			r.NoError(err)
		})
	}
}
