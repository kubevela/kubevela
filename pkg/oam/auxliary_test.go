/*
Copyright 2021 The KubeVela Authors.

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

package oam

import (
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/apps/v1"
)

func TestGetSetCluster(t *testing.T) {
	r := require.New(t)
	deploy := &v1.Deployment{}
	r.Equal("", GetCluster(deploy))
	clusterName := "cluster"
	SetCluster(deploy, clusterName)
	r.Equal(clusterName, GetCluster(deploy))
}
