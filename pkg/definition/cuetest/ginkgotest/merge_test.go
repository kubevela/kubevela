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

package ginkgotest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/definition/cuetest"
)

// Every DescribeDefinitions call's CRDs reach the one cluster the suite
// starts; different envtest binaries cannot both be used.
func TestMergeClusterOptions(t *testing.T) {
	var shared cuetest.ClusterOptions
	require.NoError(t, mergeClusterOptions(&shared, cuetest.ClusterOptions{CRDs: []string{"crds/a"}}))
	require.NoError(t, mergeClusterOptions(&shared, cuetest.ClusterOptions{CRDs: []string{"crds/b", "crds/a"}, Assets: "/bin/k8s"}))
	require.Equal(t, cuetest.ClusterOptions{CRDs: []string{"crds/a", "crds/b"}, Assets: "/bin/k8s"}, shared)
	require.NoError(t, mergeClusterOptions(&shared, cuetest.ClusterOptions{Assets: "/bin/k8s"}), "the same binaries may be named twice")
	require.ErrorContains(t, mergeClusterOptions(&shared, cuetest.ClusterOptions{Assets: "/other"}), "conflict")
	require.Equal(t, cuetest.ClusterOptions{CRDs: []string{"crds/a", "crds/b"}, Assets: "/bin/k8s"}, shared, "a conflict changes nothing")
}
