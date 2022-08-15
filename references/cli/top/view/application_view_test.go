/*
Copyright 2022 The KubeVela Authors.

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

package view

import (
	"context"
	"testing"
	"time"

	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

func TestApplicationView(t *testing.T) {
	testEnv := &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.BoolPtr(false),
	}
	cfg, err := testEnv.Start()
	assert.NoError(t, err)
	testClient, err := client.New(cfg, client.Options{Scheme: common.Scheme})
	assert.NoError(t, err)
	app := NewApp(testClient, cfg, "")
	assert.Equal(t, len(app.Components), 4)
	ctx := context.Background()
	ctx = context.WithValue(ctx, &model.CtxKeyNamespace, "")
	view := NewApplicationView(ctx, app)
	appView, ok := (view).(*ApplicationView)
	assert.Equal(t, ok, true)

	t.Run("init", func(t *testing.T) {
		appView.Init()
		assert.Equal(t, appView.Table.GetTitle(), "[ Application (all) ]")
		assert.Equal(t, appView.GetCell(0, 0).Text, "Name")
	})

	t.Run("colorize text", func(t *testing.T) {
		testData := [][]string{{"app", "ns", "running", ""}, {"app", "ns", "workflowSuspending", ""}, {"app", "ns", "workflowTerminated", ""}, {"app", "ns", "rendering", ""}}
		for i := 0; i < len(testData); i++ {
			for j := 0; j < 4; j++ {
				appView.Table.SetCell(1+i, j, tview.NewTableCell(testData[i][j]))
			}
		}
		appView.ColorizeStatusText(4)
		assert.Equal(t, appView.GetCell(1, 2).Text, "[green::]running")
		assert.Equal(t, appView.GetCell(2, 2).Text, "[yellow::]workflowSuspending")
		assert.Equal(t, appView.GetCell(3, 2).Text, "[red::]workflowTerminated")
		assert.Equal(t, appView.GetCell(4, 2).Text, "[blue::]rendering")
	})

	t.Run("hint", func(t *testing.T) {
		assert.Equal(t, len(appView.Hint()), 4)
	})

	t.Run("object view", func(t *testing.T) {
		appView.Table.Table.Table = appView.Table.Select(1, 1)
		assert.Empty(t, appView.k8sObjectView(nil))
	})

}
