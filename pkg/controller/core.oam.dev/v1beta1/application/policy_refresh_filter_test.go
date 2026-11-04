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

package application

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
)

// refreshArgsKeeper records what RefreshRecordedMetadata is asked to write.
type refreshArgsKeeper struct {
	resourcekeeper.ResourceKeeper
	labels, annotations map[string]string
	removed             resourcekeeper.MetadataRemoval
}

func (k *refreshArgsKeeper) RefreshRecordedMetadata(_ context.Context, labels, annotations map[string]string, removed resourcekeeper.MetadataRemoval) (bool, error) {
	k.labels, k.annotations, k.removed = labels, annotations, removed
	return false, nil
}

func TestRefreshPolicyMetadataLeavesFilteredKeysAlone(t *testing.T) {
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Annotations: map[string]string{
		oam.AnnotationFilterLabelKeys:      "team, tier",
		oam.AnnotationFilterAnnotationKeys: "note",
	}}}
	keeper := &refreshArgsKeeper{}
	h := &AppHandler{
		Client:          fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build(),
		resourceKeeper:  keeper,
		dataLabels:      map[string]string{"team": "platform", "tier": "gold", "region": "eu"},
		dataAnnotations: map[string]string{"note": "b", "owner": "ops"},
		dataRemovedMetadata: resourcekeeper.MetadataRemoval{
			Labels:      map[string]string{"team": "platform", "zone": "a"},
			Annotations: map[string]string{"note": "a", "audit": "on"},
		},
	}
	r := &Reconciler{Recorder: event.NewNopRecorder()}

	r.refreshPolicyMetadata(monitorContext.NewTraceContext(context.Background(), "test"), h, app)

	require.Equal(t, map[string]string{"region": "eu"}, keeper.labels)
	require.Equal(t, map[string]string{"owner": "ops"}, keeper.annotations)
	require.Equal(t, map[string]string{"zone": "a"}, keeper.removed.Labels)
	require.Equal(t, map[string]string{"audit": "on"}, keeper.removed.Annotations)
}
