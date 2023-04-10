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
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/cli/top/component"
)

func TestApp(t *testing.T) {
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
	assert.NotEmpty(t, app.config.Theme)
	assert.Equal(t, len(app.Components()), 4)

	t.Run("init", func(t *testing.T) {
		app.Init()
		assert.Equal(t, app.Main.HasPage("main"), true)
		app.Main.SwitchToPage("main")
		name, page := app.Main.GetFrontPage()
		assert.Equal(t, name, "main")
		assert.NotEmpty(t, page)
		flex, ok := page.(*tview.Flex)
		assert.Equal(t, ok, true)
		assert.Equal(t, flex.GetBorderColor(), app.config.Theme.Border.App.Color())
		_, ok = app.HasAction(component.KeyQ)
		assert.Equal(t, ok, true)
		app.content.Stack.RemoveListener(app.Crumbs())
		assert.NotEmpty(t, app.content.Stack.TopView())
		assert.Equal(t, app.content.Stack.Empty(), false)
		assert.Equal(t, app.content.Stack.IsLastView(), true)
	})
	t.Run("keyboard", func(t *testing.T) {
		evt1 := tcell.NewEventKey(component.KeyQ, '/', 0)
		assert.Empty(t, app.keyboard(evt1))
		evt2 := tcell.NewEventKey(tcell.KeyTAB, '/', 0)
		assert.NotEmpty(t, app.keyboard(evt2))
		assert.Equal(t, app.keyboard(evt2), evt2)
	})
	t.Run("help view", func(t *testing.T) {
		assert.Empty(t, app.helpView(nil))
		assert.Equal(t, app.content.IsLastView(), false)
		assert.Empty(t, app.helpView(nil))
		assert.Equal(t, app.content.IsLastView(), true)
	})
	t.Run("back", func(t *testing.T) {
		assert.Empty(t, app.helpView(nil))
		app.Back(nil)
		assert.Equal(t, app.content.IsLastView(), true)
	})
	t.Run("theme switch", func(t *testing.T) {
		app.SwitchTheme(nil)
		assert.Equal(t, app.Main.HasPage("theme"), true)
		assert.Equal(t, app.Main.GetPageCount(), 2)
	})
}
