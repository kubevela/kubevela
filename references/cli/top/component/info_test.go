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

package component

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestInfo(t *testing.T) {
	testEnv := &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.Bool(false),
	}
	cfg, err := testEnv.Start()
	assert.NoError(t, err)

	k8sClient, err := client.New(cfg, client.Options{Scheme: common.Scheme})
	assert.NoError(t, err)

	info := NewInfo(&themeConfig)
	info.Init(k8sClient, cfg)

	assert.Equal(t, info.GetColumnCount(), 7)
	assert.Equal(t, info.GetRowCount(), 6)

	assert.Equal(t, info.GetCell(0, 0).Text, "Context:")
	assert.Equal(t, info.GetCell(1, 0).Text, "K8S Version:")
	assert.Equal(t, info.GetCell(2, 0).Text, "VelaCLI Version:")
	assert.Equal(t, info.GetCell(3, 0).Text, "VelaCore Version:")

	assert.Equal(t, info.GetCell(0, 0).Color, themeConfig.Info.Title.Color())

	assert.NoError(t, testEnv.Stop())
}
