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

package addon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestCollectAndWriteTracker(t *testing.T) {
	scheme := newTestScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{Client: cli, Scheme: scheme}
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fluxcd", UID: "u1"}}

	collector := newObjectCollector()
	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(schema.FromAPIVersionAndKind("v1", "ConfigMap"))
	cm.SetNamespace("vela-system")
	cm.SetName("c1")
	collector.sink(cm)

	assert.NoError(t, r.writeTracker(context.Background(), ad, collector.objects()))
	rt, _ := r.loadTracker(context.Background(), "fluxcd")
	assert.Len(t, rt.Spec.ManagedResources, 1)
}
