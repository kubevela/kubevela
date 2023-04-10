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

func TestNamespaceView(t *testing.T) {
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

	nsView := new(NamespaceView)

	t.Run("init view", func(t *testing.T) {
		assert.Empty(t, nsView.CommonResourceView)
		nsView.InitView(ctx, app)
		assert.NotEmpty(t, nsView.CommonResourceView)
	})

	t.Run("init", func(t *testing.T) {
		nsView.Init()
		assert.Equal(t, nsView.Table.GetTitle(), "[ Namespace ]")
	})

	t.Run("refresh", func(t *testing.T) {
		keyEvent := nsView.Refresh(nil)
		assert.Empty(t, keyEvent)
	})

	t.Run("hint", func(t *testing.T) {
		assert.Equal(t, len(nsView.Hint()), 5)
	})

	t.Run("start", func(t *testing.T) {
		nsView.Start()
		assert.Equal(t, nsView.GetCell(0, 0).Text, "Name")
	})

	t.Run("stop", func(t *testing.T) {
		nsView.Stop()
		assert.Equal(t, nsView.GetCell(0, 0).Text, "")
	})

	t.Run("app view", func(t *testing.T) {
		testData := []string{"local", "", "", "", ""}
		for j := 0; j < 5; j++ {
			nsView.Table.SetCell(1, j, tview.NewTableCell(testData[j]))
		}
		assert.Equal(t, nsView.GetCell(1, 0).Text, "local")
		nsView.Table.Table = nsView.Table.Select(1, 1)
		assert.Empty(t, nsView.applicationView(nil))
	})
}
