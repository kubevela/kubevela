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

func TestK8SView(t *testing.T) {
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
	ctx := context.Background()
	ctx = context.WithValue(ctx, &model.CtxKeyAppName, "")
	ctx = context.WithValue(ctx, &model.CtxKeyNamespace, "")
	ctx = context.WithValue(ctx, &model.CtxKeyCluster, "")

	view := NewK8SView(ctx, app)
	k8sView, ok := (view).(*K8SView)
	assert.Equal(t, ok, true)

	t.Run("init", func(t *testing.T) {
		k8sView.Init()
		assert.Equal(t, k8sView.Table.GetTitle(), "[ K8S-Object ]")
		assert.Equal(t, k8sView.GetCell(0, 0).Text, "Name")
	})

	t.Run("colorize text", func(t *testing.T) {
		testData := [][]string{
			{"app", "ns", "", "", "", "Healthy"},
			{"app", "ns", "", "", "", "UnHealthy"},
			{"app", "ns", "", "", "", "Progressing"},
			{"app", "ns", "", "", "", "UnKnown"}}
		for i := 0; i < len(testData); i++ {
			for j := 0; j < len(testData[i]); j++ {
				k8sView.Table.SetCell(1+i, j, tview.NewTableCell(testData[i][j]))
			}
		}
		k8sView.ColorizeStatusText(4)
		assert.Equal(t, k8sView.GetCell(1, 5).Text, "[green::]Healthy")
		assert.Equal(t, k8sView.GetCell(2, 5).Text, "[red::]UnHealthy")
		assert.Equal(t, k8sView.GetCell(3, 5).Text, "[blue::]Progressing")
		assert.Equal(t, k8sView.GetCell(4, 5).Text, "[gray::]UnKnown")
	})

	t.Run("hint", func(t *testing.T) {
		t.Run("hint", func(t *testing.T) {
			assert.Equal(t, len(k8sView.Hint()), 2)
		})
	})

}
