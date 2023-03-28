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

func TestManagedResourceView(t *testing.T) {
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

	resourceView := new(ManagedResourceView)

	t.Run("init view", func(t *testing.T) {
		assert.Empty(t, resourceView.CommonResourceView)
		resourceView.InitView(ctx, app)
		assert.NotEmpty(t, resourceView.CommonResourceView)
	})

	t.Run("init", func(t *testing.T) {
		resourceView.Init()
		assert.Equal(t, resourceView.Table.GetTitle(), "[ Managed Resource (all/all) ]")
	})

	t.Run("refresh", func(t *testing.T) {
		keyEvent := resourceView.Refresh(nil)
		assert.Empty(t, keyEvent)
	})

	t.Run("start", func(t *testing.T) {
		resourceView.Start()
		assert.Equal(t, resourceView.GetCell(0, 0).Text, "Name")
	})

	t.Run("stop", func(t *testing.T) {
		resourceView.Stop()
		assert.Equal(t, resourceView.GetCell(0, 0).Text, "")
	})

	t.Run("colorize text", func(t *testing.T) {
		testData := [][]string{
			{"app", "ns", "", "", "", "", "Healthy"},
			{"app", "ns", "", "", "", "", "UnHealthy"},
			{"app", "ns", "", "", "", "", "Progressing"},
			{"app", "ns", "", "", "", "", "UnKnown"}}
		for i := 0; i < len(testData); i++ {
			for j := 0; j < len(testData[i]); j++ {
				resourceView.Table.SetCell(1+i, j, tview.NewTableCell(testData[i][j]))
			}
		}
		resourceView.ColorizeStatusText(4)
		assert.Equal(t, resourceView.GetCell(1, 6).Text, fmt.Sprintf("[%s::]%s", resourceView.app.config.Theme.Status.Healthy.String(), "Healthy"))
		assert.Equal(t, resourceView.GetCell(2, 6).Text, fmt.Sprintf("[%s::]%s", resourceView.app.config.Theme.Status.UnHealthy.String(), "UnHealthy"))
		assert.Equal(t, resourceView.GetCell(3, 6).Text, fmt.Sprintf("[%s::]%s", resourceView.app.config.Theme.Status.Waiting.String(), "Progressing"))
		assert.Equal(t, resourceView.GetCell(4, 6).Text, fmt.Sprintf("[%s::]%s", resourceView.app.config.Theme.Status.Unknown.String(), "UnKnown"))
	})

	t.Run("hint", func(t *testing.T) {
		assert.Equal(t, len(resourceView.Hint()), 8)
	})

	t.Run("select cluster", func(t *testing.T) {
		assert.Empty(t, resourceView.clusterView(nil))
	})

	t.Run("select cluster namespace", func(t *testing.T) {
		assert.Empty(t, resourceView.clusterNamespaceView(nil))
	})

	t.Run("pod view", func(t *testing.T) {
		resourceView.Table.Table = resourceView.Table.Select(1, 1)
		assert.Empty(t, resourceView.podView(nil))
	})
}
