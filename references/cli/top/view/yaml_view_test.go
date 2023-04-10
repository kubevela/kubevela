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

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

func TestYamlView(t *testing.T) {
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
	view := NewYamlView(ctx, app)
	yamlView, ok := (view).(*YamlView)
	assert.Equal(t, ok, true)

	t.Run("init", func(t *testing.T) {
		yamlView.Init()
		assert.Equal(t, yamlView.GetTitle(), "[ Yaml ]")
	})

	t.Run("hint", func(t *testing.T) {
		assert.Equal(t, len(yamlView.Hint()), 2)
	})

	t.Run("start", func(t *testing.T) {
		yamlView.Start()
		content := yamlView.TextView.GetText(true)
		assert.NotEmpty(t, content)
	})

	t.Run("stop", func(t *testing.T) {
		yamlView.Stop()
	})

	t.Run("highlight text", func(t *testing.T) {
		yaml := `apiVersion: core.oam.dev/v1beta1`
		yaml = yamlView.HighlightText(yaml)
		highlightedText := fmt.Sprintf("[%s::b]apiVersion[%s::-]: [%s::]core.oam.dev/v1beta1", yamlView.app.config.Theme.Yaml.Key.String(), yamlView.app.config.Theme.Yaml.Colon.String(), yamlView.app.config.Theme.Yaml.Value.String())
		assert.Equal(t, yaml, highlightedText)
	})

}
