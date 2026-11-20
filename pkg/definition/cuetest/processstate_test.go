/*
Copyright 2026 The KubeVela Authors.

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

package cuetest

import (
	"context"
	"testing"

	"github.com/kubevela/pkg/util/singleton"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/velaconfig"
	"github.com/oam-dev/kubevela/pkg/registry"
)

func TestStopExecClusterRestoresProcessState(t *testing.T) {
	// The cluster starts afresh, so what it saves is what this test set.
	require.NoError(t, StopExecCluster())
	t.Cleanup(saveProcessState())
	t.Cleanup(func() { _ = StopExecCluster() })
	priorClient := fake.NewClientBuilder().Build()
	priorConfig := &rest.Config{Host: "https://prior.example.com"}
	singleton.KubeClient.Set(priorClient)
	singleton.KubeConfig.Set(priorConfig)
	_, hadReader := registry.Get[velaconfig.Reader]()

	cl := testCluster(t)
	cl.runtimeContext(context.Background())
	require.Same(t, cl.Config, singleton.KubeConfig.Get())
	_, hasReader := registry.Get[velaconfig.Reader]()
	require.True(t, hasReader)

	require.NoError(t, StopExecCluster())
	require.Same(t, priorConfig, singleton.KubeConfig.Get())
	require.Equal(t, priorClient, singleton.KubeClient.Get())
	_, hasReader = registry.Get[velaconfig.Reader]()
	require.Equal(t, hadReader, hasReader)
}
