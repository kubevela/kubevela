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

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestHelpView(t *testing.T) {
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
	view := NewHelpView(app)
	helpView, ok := (view).(*HelpView)
	assert.Equal(t, ok, true)

	t.Run("init", func(t *testing.T) {
		helpView.Init()
		assert.Equal(t, helpView.GetTitle(), "[ Help ]")
		assert.Equal(t, len(helpView.Hint()), 2)
	})

	t.Run("start", func(t *testing.T) {
		assert.Equal(t, helpView.GetText(false), "")
		helpView.Start()
		assert.Equal(t, helpView.GetTitle(), "[ Help ]")
		assert.NotEqual(t, helpView.GetText(false), "")
	})
}
