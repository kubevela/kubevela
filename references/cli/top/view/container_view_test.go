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

func TestContainerView(t *testing.T) {
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
	ctx = context.WithValue(ctx, &model.CtxKeyPod, "pod1")
	ctx = context.WithValue(ctx, &model.CtxKeyNamespace, "default")
	ctx = context.WithValue(ctx, &model.CtxKeyCluster, "local")

	containerView := new(ContainerView)

	t.Run("init view", func(t *testing.T) {
		assert.Empty(t, containerView.CommonResourceView)
		containerView.InitView(ctx, app)
		assert.NotEmpty(t, containerView.CommonResourceView)
	})

	t.Run("init", func(t *testing.T) {
		containerView.Init()
		assert.Equal(t, containerView.Table.GetTitle(), "[ Container ]")
	})

	t.Run("refresh", func(t *testing.T) {
		keyEvent := containerView.Refresh(nil)
		assert.Empty(t, keyEvent)
	})

	t.Run("start", func(t *testing.T) {
		containerView.Start()
		assert.Equal(t, containerView.GetCell(0, 0).Text, "Name")
		assert.Equal(t, containerView.GetCell(0, 1).Text, "Image")
		assert.Equal(t, containerView.GetCell(0, 2).Text, "Ready")
		assert.Equal(t, containerView.GetCell(0, 11).Text, "RestartCount")
	})

	t.Run("stop", func(t *testing.T) {
		containerView.Stop()
		assert.Equal(t, containerView.GetCell(0, 0).Text, "")
	})

	t.Run("colorize text", func(t *testing.T) {
		testData := [][]string{
			{"test-container1", "test-image", "Yes", "Running", "N/A", "N/A", "N/A", "N/A", "N/A", "N/A", "", "0"},
			{"test-container2", "test-image", "No", "Waiting", "N/A", "N/A", "N/A", "N/A", "N/A", "N/A", "", "0"},
			{"test-container3", "test-image", "No", "Terminated", "N/A", "N/A", "N/A", "N/A", "N/A", "N/A", "", "0"},
		}
		for i := 0; i < len(testData); i++ {
			for j := 0; j < len(testData[i]); j++ {
				containerView.Table.SetCell(1+i, j, tview.NewTableCell(testData[i][j]))
			}
		}
		containerView.ColorizePhaseText(3)
		assert.Equal(t, containerView.GetCell(1, 3).Text, fmt.Sprintf("[%s::]%s", containerView.app.config.Theme.Status.Healthy.String(), "Running"))
		assert.Equal(t, containerView.GetCell(2, 3).Text, fmt.Sprintf("[%s::]%s", containerView.app.config.Theme.Status.Waiting.String(), "Waiting"))
		assert.Equal(t, containerView.GetCell(3, 3).Text, fmt.Sprintf("[%s::]%s", containerView.app.config.Theme.Status.UnHealthy.String(), "Terminated"))
	})

	t.Run("hint", func(t *testing.T) {
		assert.Equal(t, len(containerView.Hint()), 4)
	})
}
