/*
Copyright 2023 The KubeVela Authors.

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

package sharding_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/pkg/controller/sharding"
)

func TestScheduledShardID(t *testing.T) {
	sharding.EnableSharding = true
	defer func() { sharding.EnableSharding = false }()

	o := &corev1.ConfigMap{}
	sharding.DelScheduledShardID(o)
	_, scheduled := sharding.GetScheduledShardID(o)
	require.False(t, scheduled)

	sharding.SetScheduledShardID(o, "s")
	sid, scheduled := sharding.GetScheduledShardID(o)
	require.True(t, scheduled)
	require.Equal(t, "s", sid)

	p := &corev1.Secret{}
	sharding.PropagateScheduledShardIDLabel(o, p)
	sid, scheduled = sharding.GetScheduledShardID(p)
	require.True(t, scheduled)
	require.Equal(t, "s", sid)

	sharding.DelScheduledShardID(p)
	_, scheduled = sharding.GetScheduledShardID(p)
	require.False(t, scheduled)

	sharding.PropagateScheduledShardIDLabel(p, o)
	_, scheduled = sharding.GetScheduledShardID(o)
	require.False(t, scheduled)

	sharding.ShardID = "s"
	require.Equal(t, "-s", sharding.GetShardIDSuffix())

	sharding.ShardID = "master"
	require.True(t, sharding.IsMaster())
	require.Equal(t, "", sharding.GetShardIDSuffix())
}
