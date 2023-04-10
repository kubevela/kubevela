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
	"fmt"
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
		UseExistingCluster:       pointer.Bool(false),
	}
	cfg, err := testEnv.Start()
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, testEnv.Stop())
	}()

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

	podView := new(PodView)

	t.Run("init view", func(t *testing.T) {
		assert.Empty(t, podView.CommonResourceView)
		podView.InitView(ctx, app)
		assert.NotEmpty(t, podView.CommonResourceView)
	})

	t.Run("init", func(t *testing.T) {
		podView.Init()
		assert.Equal(t, podView.Table.GetTitle(), "[ Pod ]")
	})

	t.Run("refresh", func(t *testing.T) {
		keyEvent := podView.Refresh(nil)
		assert.Empty(t, keyEvent)
	})

	t.Run("start", func(t *testing.T) {
		podView.Start()
		assert.Equal(t, podView.GetCell(0, 0).Text, "Name")
	})

	t.Run("stop", func(t *testing.T) {
		podView.Stop()
		assert.Equal(t, podView.GetCell(0, 0).Text, "")
	})

	t.Run("colorize text", func(t *testing.T) {
		testData := [][]string{
			{"app", "ns", "", "1/1", "Running", "", "", "", "", "", "", "", "", ""},
			{"app", "ns", "", "1/1", "Pending", "", "", "", "", "", "", "", "", ""},
			{"app", "ns", "", "1/1", "Succeeded", "", "", "", "", "", "", "", "", ""},
			{"app", "ns", "", "1/1", "Failed", "", "", "", "", "", "", "", "", ""},
		}
		for i := 0; i < len(testData); i++ {
			for j := 0; j < len(testData[i]); j++ {
				podView.Table.SetCell(1+i, j, tview.NewTableCell(testData[i][j]))
			}
		}
		podView.ColorizePhaseText(5)
		assert.Equal(t, podView.GetCell(1, 4).Text, fmt.Sprintf("[%s::]%s", podView.app.config.Theme.Status.Healthy.String(), "Running"))
		assert.Equal(t, podView.GetCell(2, 4).Text, fmt.Sprintf("[%s::]%s", podView.app.config.Theme.Status.Waiting.String(), "Pending"))
		assert.Equal(t, podView.GetCell(3, 4).Text, fmt.Sprintf("[%s::]%s", podView.app.config.Theme.Status.Succeeded.String(), "Succeeded"))
		assert.Equal(t, podView.GetCell(4, 4).Text, fmt.Sprintf("[%s::]%s", podView.app.config.Theme.Status.UnHealthy.String(), "Failed"))
	})

	t.Run("hint", func(t *testing.T) {
		assert.Equal(t, len(podView.Hint()), 7)
	})
}
