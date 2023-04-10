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

func TestLogView(t *testing.T) {
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
	ctx = context.WithValue(ctx, &model.CtxKeyCluster, "")
	ctx = context.WithValue(ctx, &model.CtxKeyPod, "")
	ctx = context.WithValue(ctx, &model.CtxKeyNamespace, "")

	view := NewLogView(ctx, app)
	logView, ok := (view).(*LogView)
	assert.Equal(t, ok, true)

	t.Run("init", func(t *testing.T) {
		logView.Init()
		assert.Equal(t, logView.GetTitle(), "[ Log ]")
	})

	t.Run("hint", func(t *testing.T) {
		assert.Equal(t, len(logView.Hint()), 2)
	})

	t.Run("start", func(t *testing.T) {
		logView.Start()
		assert.NotEmpty(t, logView.writer)
		logView.writer.Write([]byte("test"))
		content := logView.TextView.GetText(true)
		assert.NotEmpty(t, content)
	})

	t.Run("stop", func(t *testing.T) {
		logView.Stop()
		content := logView.TextView.GetText(true)
		assert.Empty(t, content)
	})
}
