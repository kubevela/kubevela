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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

func TestUp(t *testing.T) {

	app := &v1beta1.Application{}
	app.Name = "app-up"
	msg := common.Info(app)
	assert.Contains(t, msg, "App has been deployed")
	// This test case can not run in the TERM with the color.
	assert.Contains(t, msg, fmt.Sprintf("App status: vela status %s", app.Name))
}

func TestUpOverrideNamespace(t *testing.T) {
	cases := map[string]struct {
		application       string
		applicationName   string
		namespace         string
		expectedNamespace string
	}{
		"use default namespace if not set": {
			application: `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: first-vela-app
spec:
  components: []
`,
			applicationName:   "first-vela-app",
			namespace:         "",
			expectedNamespace: types.DefaultAppNamespace,
		},
		"override namespace": {
			application: `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: first-vela-app
spec:
  components: []
`,
			applicationName:   "first-vela-app",
			namespace:         "overridden-namespace",
			expectedNamespace: "overridden-namespace",
		},
		"use application namespace": {
			application: `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: first-vela-app
  namespace: vela-apps
spec:
  components: []
`,
			applicationName:   "first-vela-app",
			namespace:         "",
			expectedNamespace: "vela-apps",
		},
		"override application namespace": {
			application: `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: first-vela-app
  namespace: vela-apps
spec:
  components: []
`,
			applicationName:   "first-vela-app",
			namespace:         "overridden-namespace",
			expectedNamespace: "overridden-namespace",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			args := initArgs()
			kc, err := args.GetClient()
			require.NoError(t, err)

			af, err := os.CreateTemp(os.TempDir(), "up-override-namespace-*.yaml")
			require.NoError(t, err)
			defer func() {
				_ = af.Close()
				_ = os.Remove(af.Name())
			}()
			_, err = af.WriteString(c.application)
			require.NoError(t, err)

			// Ensure namespace
			require.NoError(t, kc.Create(context.TODO(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: c.expectedNamespace},
			}))

			var buf bytes.Buffer
			cmd := NewUpCommand(velacmd.NewDelegateFactory(args.GetClient, args.GetConfig), "", args, util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
			cmd.SetArgs([]string{})
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			if c.namespace != "" {
				require.NoError(t, cmd.Flags().Set(FlagNamespace, c.namespace))
			}
			require.NoError(t, cmd.Flags().Set("file", af.Name()))
			require.NoError(t, cmd.Execute())

			var app v1beta1.Application
			require.NoError(t, kc.Get(context.TODO(), client.ObjectKey{
				Name:      c.applicationName,
				Namespace: c.expectedNamespace,
			}, &app))
			require.Equal(t, c.expectedNamespace, app.Namespace)
			require.Equal(t, c.applicationName, app.Name)
		})
	}
}
