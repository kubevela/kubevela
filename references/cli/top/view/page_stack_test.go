package view

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestPageStack(t *testing.T) {
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

	stack := NewPageStack(app)
	stack.Init()

	t.Run("stack push", func(t *testing.T) {
		helpView := NewHelpView(app)
		stack.StackPush(helpView)
	})
	t.Run("stack pop", func(t *testing.T) {
		stack.StackPop(nil, nil)
	})
}
