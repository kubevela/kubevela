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

package v1beta1

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kubevela/pkg/util/compression"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/errors"
)

func TestManagedResource_DeepCopyEqual(t *testing.T) {
	r := require.New(t)
	mr := ManagedResource{
		ClusterObjectReference: common.ClusterObjectReference{Cluster: "cluster"},
		OAMObjectReference:     common.OAMObjectReference{Component: "component"},
		Data:                   &runtime.RawExtension{Raw: []byte("data")},
	}
	r.True(mr.Equal(*mr.DeepCopy()))
}

func TestManagedResource_Equal(t *testing.T) {
	testCases := map[string]struct {
		input1 ManagedResource
		input2 ManagedResource
		equal  bool
	}{
		"equal": {
			input1: ManagedResource{
				ClusterObjectReference: common.ClusterObjectReference{Cluster: "cluster"},
				OAMObjectReference:     common.OAMObjectReference{Component: "component"},
				Data:                   &runtime.RawExtension{Raw: []byte("data")},
			},
			input2: ManagedResource{
				ClusterObjectReference: common.ClusterObjectReference{Cluster: "cluster"},
				OAMObjectReference:     common.OAMObjectReference{Component: "component"},
				Data:                   &runtime.RawExtension{Raw: []byte("data")},
			},
			equal: true,
		},
		"ClusterObjectReference not equal": {
			input1: ManagedResource{
				ClusterObjectReference: common.ClusterObjectReference{Cluster: "cluster"},
			},
			input2: ManagedResource{
				ClusterObjectReference: common.ClusterObjectReference{Cluster: "c"},
			},
			equal: false,
		},
		"OAMObjectReference not equal": {
			input1: ManagedResource{
				OAMObjectReference: common.OAMObjectReference{Component: "component"},
			},
			input2: ManagedResource{
				OAMObjectReference: common.OAMObjectReference{Component: "c"},
			},
			equal: false,
		},
		"Data content not equal": {
			input1: ManagedResource{
				Data: &runtime.RawExtension{Raw: []byte("data")},
			},
			input2: ManagedResource{
				Data: &runtime.RawExtension{Raw: []byte("d")},
			},
			equal: false,
		},
		"one data empty, one data not empty": {
			input1: ManagedResource{Data: nil},
			input2: ManagedResource{
				Data: &runtime.RawExtension{Raw: []byte("d")},
			},
			equal: false,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(tc.equal, tc.input1.Equal(tc.input2))
			r.Equal(tc.equal, tc.input2.Equal(tc.input1))
		})
	}
}

func TestManagedResourceKeys(t *testing.T) {
	r := require.New(t)
	input := ManagedResource{
		ClusterObjectReference: common.ClusterObjectReference{
			Cluster: "cluster",
			ObjectReference: corev1.ObjectReference{
				Namespace:  "namespace",
				Name:       "name",
				APIVersion: appsv1.SchemeGroupVersion.String(),
				Kind:       "Deployment",
			},
		},
		OAMObjectReference: common.OAMObjectReference{
			Env:       "env",
			Component: "component",
			Trait:     "trait",
		},
	}
	r.Equal("namespace/name", input.NamespacedName().String())
	r.Equal("apps/Deployment/cluster/namespace/name", input.ResourceKey())
	r.Equal("env/component", input.ComponentKey())
	r.Equal("Deployment name (Cluster: cluster, Namespace: namespace)", input.DisplayName())
	var deploy1, deploy2 appsv1.Deployment
	deploy1.Spec.Replicas = pointer.Int32(5)
	bs, err := json.Marshal(deploy1)
	r.NoError(err)
	r.ErrorIs(input.UnmarshalTo(&deploy2), errors.ManagedResourceHasNoDataError{})
	_, err = input.ToUnstructuredWithData()
	r.ErrorIs(err, errors.ManagedResourceHasNoDataError{})
	input.Data = &runtime.RawExtension{Raw: bs}
	r.NoError(input.UnmarshalTo(&deploy2))
	r.Equal(deploy1, deploy2)
	obj := input.ToUnstructured()
	r.Equal("Deployment", obj.GetKind())
	r.Equal("apps/v1", obj.GetAPIVersion())
	r.Equal("name", obj.GetName())
	r.Equal("namespace", obj.GetNamespace())
	r.Equal("cluster", oam.GetCluster(obj))
	obj, err = input.ToUnstructuredWithData()
	r.NoError(err)
	val, correct, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	r.NoError(err)
	r.True(correct)
	r.Equal(int64(5), val)
}

func TestResourceTracker_ManagedResource(t *testing.T) {
	r := require.New(t)
	input := &ResourceTracker{}
	deploy1 := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deploy1"}}
	input.AddManagedResource(&deploy1, true, false, "")
	r.Equal(1, len(input.Spec.ManagedResources))
	cm2 := corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm2"}}
	input.AddManagedResource(&cm2, false, false, "")
	r.Equal(2, len(input.Spec.ManagedResources))
	pod3 := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod3"}}
	input.AddManagedResource(&pod3, false, false, "")
	r.Equal(3, len(input.Spec.ManagedResources))
	deploy1.Spec.Replicas = pointer.Int32(5)
	input.AddManagedResource(&deploy1, false, false, "")
	r.Equal(3, len(input.Spec.ManagedResources))
	input.DeleteManagedResource(&cm2, false)
	r.Equal(3, len(input.Spec.ManagedResources))
	r.True(input.Spec.ManagedResources[1].Deleted)
	input.DeleteManagedResource(&cm2, true)
	r.Equal(2, len(input.Spec.ManagedResources))
	input.DeleteManagedResource(&deploy1, true)
	r.Equal(1, len(input.Spec.ManagedResources))
	input.DeleteManagedResource(&pod3, true)
	r.Equal(0, len(input.Spec.ManagedResources))
	secret4 := corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret4"}}
	input.DeleteManagedResource(&secret4, true)
	r.Equal(0, len(input.Spec.ManagedResources))
	input.DeleteManagedResource(&secret4, false)
	r.Equal(1, len(input.Spec.ManagedResources))
}

func TestResourceTrackerCompression(t *testing.T) {
	count := 20
	r := require.New(t)

	// Load some real CRDs, and other test data to simulate real use-cases.
	// The user must have some large resourcetrackers if they use compression,
	// so we load some large CRDs.
	var data []string
	paths := []string{
		"../../../charts/vela-core/crds/core.oam.dev_applicationrevisions.yaml",
		"../../../charts/vela-core/crds/core.oam.dev_applications.yaml",
		"../../../charts/vela-core/crds/core.oam.dev_definitionrevisions.yaml",
		"../../../charts/vela-core/crds/core.oam.dev_healthscopes.yaml",
		"../../../charts/vela-core/crds/core.oam.dev_traitdefinitions.yaml",
		"../../../charts/vela-core/crds/core.oam.dev_componentdefinitions.yaml",
		"../../../charts/vela-core/crds/core.oam.dev_workloaddefinitions.yaml",
		"../../../charts/vela-core/crds/standard.oam.dev_rollouts.yaml",
		"../../../charts/vela-core/templates/kubevela-controller.yaml",
		"../../../charts/vela-core/README.md",
		"../../../pkg/velaql/providers/query/testdata/machinelearning.seldon.io_seldondeployments.yaml",
		"../../../legacy/charts/vela-core-legacy/crds/standard.oam.dev_podspecworkloads.yaml",
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		r.NoError(err)
		data = append(data, string(b))
	}
	size := len(data)

	// Gzip
	var (
		gzipCompressTime int64
		gzipSize         int
		gzipBs           []byte
	)
	for c := 0; c < count; c++ {
		var err error
		rtGzip := &ResourceTracker{}
		for i := 0; i < size; i++ {
			rtGzip.AddManagedResource(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("cm%d", i)}, Data: map[string]string{"1": data[i]}}, false, false, "")
			rtGzip.AddManagedResource(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("secret%d", i)}}, true, false, "")
		}
		rtGzip.Spec.Compression.Type = compression.Gzip
		// Compress
		t0 := time.Now()
		gzipBs, err = json.Marshal(rtGzip)
		elapsed := time.Since(t0).Nanoseconds()
		if gzipCompressTime == 0 {
			gzipCompressTime = elapsed
		} else {
			gzipCompressTime = (elapsed + gzipCompressTime) / 2
		}
		if gzipSize == 0 {
			gzipSize = len(gzipBs)
		} else {
			gzipSize = (len(gzipBs) + gzipSize) / 2
		}
		r.NoError(err)
		r.Contains(string(gzipBs), `"type":"gzip","data":`)
	}

	// Zstd
	var (
		zstdCompressTime int64
		zstdSize         int
		zstdBs           []byte
	)
	for c := 0; c < count; c++ {
		var err error
		rtZstd := &ResourceTracker{}
		for i := 0; i < size; i++ {
			rtZstd.AddManagedResource(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("cm%d", i)}, Data: map[string]string{"1": data[i]}}, false, false, "")
			rtZstd.AddManagedResource(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("secret%d", i)}}, true, false, "")
		}
		rtZstd.Spec.Compression.Type = compression.Zstd
		t0 := time.Now()
		zstdBs, err = json.Marshal(rtZstd)
		elapsed := time.Since(t0).Nanoseconds()
		if zstdCompressTime == 0 {
			zstdCompressTime = elapsed
		} else {
			zstdCompressTime = (elapsed + zstdCompressTime) / 2
		}
		if zstdSize == 0 {
			zstdSize = len(zstdBs)
		} else {
			zstdSize = (len(zstdBs) + zstdSize) / 2
		}
		r.NoError(err)
		r.Contains(string(zstdBs), `"type":"zstd","data":`)
	}

	rtUncmp := &ResourceTracker{}
	r.NoError(json.Unmarshal(gzipBs, rtUncmp))
	r.Equal(size*2, len(rtUncmp.Spec.ManagedResources))
	for i, rsc := range rtUncmp.Spec.ManagedResources {
		r.Equal(i%2 == 1, rsc.Data == nil)
	}
	r.NoError(json.Unmarshal(zstdBs, rtUncmp))
	r.Equal(size*2, len(rtUncmp.Spec.ManagedResources))
	for i, rsc := range rtUncmp.Spec.ManagedResources {
		r.Equal(i%2 == 1, rsc.Data == nil)
	}
	// No compression
	var (
		uncmpTime int64
		uncmpSize int
	)
	rtUncmp.Spec.Compression.Type = compression.Uncompressed
	for c := 0; c < count; c++ {
		t0 := time.Now()
		_bs, err := json.Marshal(rtUncmp)
		if uncmpTime == 0 {
			uncmpTime = time.Since(t0).Nanoseconds()
		} else {
			uncmpTime = (time.Since(t0).Nanoseconds() + uncmpTime) / 2
		}
		if uncmpSize == 0 {
			uncmpSize = len(_bs)
		} else {
			uncmpSize = (len(_bs) + uncmpSize) / 2
		}
		r.NoError(err)
		before, after := len(_bs), len(zstdBs)
		r.Less(after, before)
		before, after = len(_bs), len(gzipBs)
		r.Less(after, before)
	}

	fmt.Printf(`Compressed Size:
  uncompressed: %d bytes	100.00%%
  gzip:         %d bytes	%.2f%%
  zstd:         %d bytes	%.2f%%
`,
		uncmpSize,
		gzipSize, float64(gzipSize)*100.0/float64(uncmpSize),
		zstdSize, float64(zstdSize)*100.0/float64(uncmpSize))

	fmt.Printf(`Marshal Time:
  no compression: %d ns	1.00x
  gzip:           %d ns	%.2fx
  zstd:           %d ns	%.2fx
`,
		uncmpTime,
		gzipCompressTime, float64(gzipCompressTime)/float64(uncmpTime),
		zstdCompressTime, float64(zstdCompressTime)/float64(uncmpTime),
	)
}

func TestResourceTrackerInvalidMarshal(t *testing.T) {
	r := require.New(t)
	rt := &ResourceTracker{}
	rt.Spec.Compression.Type = "invalid"
	_, err := json.Marshal(rt)
	r.ErrorIs(err, compression.NewUnsupportedCompressionTypeError("invalid"))
	r.True(strings.Contains(err.Error(), "invalid"))
	r.ErrorIs(json.Unmarshal([]byte(`{"spec":{"compression":{"type":"invalid"}}}`), rt), compression.NewUnsupportedCompressionTypeError("invalid"))
	r.NotNil(json.Unmarshal([]byte(`{"spec":{"compression":{"type":"gzip","data":"xxx"}}}`), rt))
	r.NotNil(json.Unmarshal([]byte(`{"spec":["invalid"]}`), rt))
}
