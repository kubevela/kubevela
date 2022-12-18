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

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestOAMObjectReference(t *testing.T) {
	r := require.New(t)
	o1 := OAMObjectReference{
		Component: "component",
		Trait:     "trait",
		Env:       "env",
	}
	obj := &unstructured.Unstructured{}
	o2 := NewOAMObjectReferenceFromObject(obj)
	r.False(o2.Equal(o1))
	o1.AddLabelsToObject(obj)
	r.Equal(3, len(obj.GetLabels()))
	o3 := NewOAMObjectReferenceFromObject(obj)
	r.True(o1.Equal(o3))
	o3.Component = "comp"
	r.False(o3.Equal(o1))

	r.True(o1.Equal(*o1.DeepCopy()))
	o4 := OAMObjectReference{}
	o1.DeepCopyInto(&o4)
	r.True(o4.Equal(o1))
}

func TestClusterObjectReference(t *testing.T) {
	r := require.New(t)
	o1 := ClusterObjectReference{
		Cluster:         "cluster",
		ObjectReference: v1.ObjectReference{Kind: "kind"},
	}
	o2 := *o1.DeepCopy()
	r.True(o1.Equal(o2))
	o2.Cluster = "c"
	r.False(o2.Equal(o1))
}

func TestContainerStateToString(t *testing.T) {
	r := require.New(t)
	r.Equal("Waiting", ContainerStateToString(v1.ContainerState{
		Waiting: &v1.ContainerStateWaiting{},
	}))
	r.Equal("Running", ContainerStateToString(v1.ContainerState{
		Running: &v1.ContainerStateRunning{},
	}))
	r.Equal("Terminated", ContainerStateToString(v1.ContainerState{
		Terminated: &v1.ContainerStateTerminated{},
	}))
	r.Equal("Unknown", ContainerStateToString(v1.ContainerState{}))
}
