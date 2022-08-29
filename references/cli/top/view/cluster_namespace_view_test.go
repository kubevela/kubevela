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

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

func TestClusterNamespaceView(t *testing.T) {
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
	cnsView, ok := NewClusterNamespaceView(ctx, app).(*ClusterNamespaceView)
	assert.Equal(t, ok, true)

	t.Run("init", func(t *testing.T) {
		cnsView.Init()
		assert.Equal(t, cnsView.GetTitle(), "[ ClusterNamespace ]")
	})

	t.Run("start", func(t *testing.T) {
		cnsView.Start()
		assert.Equal(t, cnsView.GetCell(0, 0).Text, "Name")
		assert.Equal(t, cnsView.GetCell(1, 0).Text, "all")
	})
	t.Run("stop", func(t *testing.T) {
		cnsView.Stop()
	})

	t.Run("hint", func(t *testing.T) {
		assert.Equal(t, len(cnsView.Hint()), 3)
	})

	t.Run("object view", func(t *testing.T) {
		cnsView.Table.Table = cnsView.Table.Select(1, 1)
		assert.Empty(t, cnsView.managedResourceView(nil))
	})
}
