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

func TestPodView(t *testing.T) {
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
	assert.Equal(t, len(app.Components()), 4)
	ctx := context.Background()
	ctx = context.WithValue(ctx, &model.CtxKeyAppName, "")
	ctx = context.WithValue(ctx, &model.CtxKeyNamespace, "")
	ctx = context.WithValue(ctx, &model.CtxKeyCluster, "")
	ctx = context.WithValue(ctx, &model.CtxKeyClusterNamespace, "")
	ctx = context.WithValue(ctx, &model.CtxKeyComponentName, "")

	view, ok := NewPodView(ctx, app).(*PodView)
	assert.Equal(t, ok, true)

	t.Run("init", func(t *testing.T) {
		view.Init()
		assert.Equal(t, view.Table.GetTitle(), "[ Pod ]")
		assert.Equal(t, view.GetCell(0, 0).Text, "Name")
	})

	t.Run("colorize text", func(t *testing.T) {
		testData := [][]string{
			{"app", "ns", "1/1", "Running", "", "", "", "", "", "", "", "", ""},
			{"app", "ns", "1/1", "Pending", "", "", "", "", "", "", "", "", ""},
			{"app", "ns", "1/1", "Succeeded", "", "", "", "", "", "", "", "", ""},
			{"app", "ns", "1/1", "Failed", "", "", "", "", "", "", "", "", ""},
		}
		for i := 0; i < len(testData); i++ {
			for j := 0; j < len(testData[i]); j++ {
				view.Table.SetCell(1+i, j, tview.NewTableCell(testData[i][j]))
			}
		}
		view.ColorizePhaseText(5)
		assert.Equal(t, view.GetCell(1, 3).Text, "[green::]Running")
		assert.Equal(t, view.GetCell(2, 3).Text, "[yellow::]Pending")
		assert.Equal(t, view.GetCell(3, 3).Text, "[purple::]Succeeded")
		assert.Equal(t, view.GetCell(4, 3).Text, "[red::]Failed")
	})

	t.Run("hint", func(t *testing.T) {
		assert.Equal(t, len(view.Hint()), 2)
	})
}
