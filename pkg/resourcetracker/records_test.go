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

package resourcetracker

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestRecordAndDeleteManifestsInResourceTracker(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(testScheme).Build()
	rt := &v1beta1.ResourceTracker{ObjectMeta: v1.ObjectMeta{Name: "rt"}}
	r.NoError(cli.Create(context.Background(), rt))
	n := 10
	var objs []*unstructured.Unstructured
	for i := 0; i < n; i++ {
		obj := &unstructured.Unstructured{}
		obj.SetName(fmt.Sprintf("workload-%d", i))
		objs = append(objs, obj)
		r.NoError(RecordManifestsInResourceTracker(context.Background(), cli, rt, []*unstructured.Unstructured{obj}, rand.Int()%2 == 0, false, ""))
	}
	rand.Shuffle(len(objs), func(i, j int) { objs[i], objs[j] = objs[j], objs[i] })
	for i := 0; i < n; i++ {
		r.NoError(DeletedManifestInResourceTracker(context.Background(), cli, rt, objs[i], true))
		r.Equal(n-i-1, len(rt.Spec.ManagedResources))
	}
}

// testScheme is the library's own: client-go's types plus core.oam.dev, rather than
// KubeVela's full pkg/utils/common.Scheme (which imports this package's configuration).
var testScheme = func() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = v1alpha1.AddToScheme(s)
	_ = v1beta1.AddToScheme(s)
	return s
}()
