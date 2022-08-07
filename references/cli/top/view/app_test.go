package view

import (
	"github.com/gdamore/tcell/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestApp(t *testing.T) {
	testEnv := &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.BoolPtr(false),
	}
	cfg, err := testEnv.Start()
	assert.NoError(t, err)
	testClient, err := client.New(cfg, client.Options{Scheme: common.Scheme})
	assert.NoError(t, err)
	app := NewApp(testClient, cfg)
	assert.Equal(t, len(app.Components), 4)

	t.Run("init", func(t *testing.T) {
		app.Init()
		assert.Equal(t, app.Main.HasPage("main"), true)
		_, ok := app.HasAction(tcell.KeyESC)
		assert.Equal(t, ok, true)
		app.content.Stack.RemoveListener(app.Crumbs())
		assert.NotEmpty(t, app.content.Stack.TopComponent())
		assert.Equal(t, app.content.Stack.Empty(), false)
		assert.Equal(t, app.content.Stack.IsLastComponent(), true)
	})
	t.Run("keyboard", func(t *testing.T) {
		evt1 := tcell.NewEventKey(tcell.KeyEsc, '/', 0)
		assert.Empty(t, app.keyboard(evt1))
		evt2 := tcell.NewEventKey(tcell.KeyTAB, '/', 0)
		assert.NotEmpty(t, app.keyboard(evt2))
		assert.Equal(t, app.keyboard(evt2), evt2)
	})
	t.Run("help view", func(t *testing.T) {
		assert.Empty(t, app.helpView(nil))
		assert.Equal(t, app.content.IsLastComponent(), false)
		assert.Empty(t, app.helpView(nil))
		assert.Equal(t, app.content.IsLastComponent(), true)
	})
	t.Run("back", func(t *testing.T) {
		assert.Empty(t, app.helpView(nil))
		app.Back(nil)
		assert.Equal(t, app.content.IsLastComponent(), true)
	})
}
