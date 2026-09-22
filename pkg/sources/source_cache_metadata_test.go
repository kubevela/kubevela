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

package sources

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apitypes "github.com/oam-dev/kubevela/apis/types"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

func TestSourceCacheTemplateNameMatchesController(t *testing.T) {
	// Same (sourceType, schema) must yield a stable name; empty schema -> no template.
	a := sourceCacheTemplateName("img-source", "{image: string}")
	b := sourceCacheTemplateName("img-source", "{image: string}")
	assert.Equal(t, a, b)
	assert.Contains(t, a, "source-img-source-")
	assert.Equal(t, "", sourceCacheTemplateName("img-source", ""))
}

func TestApplySourceCacheMetadata(t *testing.T) {
	meta := velaprocess.SourceCacheWriteMeta{
		TTL:                10 * time.Minute,
		SourceDefName:      "img-source",
		SourceDefNamespace: "vela-system",
		TemplateName:       "source-img-source-abcd1234",
	}

	// No pre-existing type (Secret-store path): fall back to the source type.
	secret := &corev1.Secret{}
	ApplySourceCacheMetadata(secret, "img-source", meta)
	assert.Equal(t, "img-source", secret.Labels[apitypes.LabelConfigType])

	// A pre-set type (config-API store path, set by ParseConfig to the template
	// name) must be PRESERVED, not clobbered to the source type.
	preset := &corev1.Secret{}
	preset.Labels = map[string]string{apitypes.LabelConfigType: "source-img-source-abcd1234"}
	ApplySourceCacheMetadata(preset, "img-source", meta)
	assert.Equal(t, "source-img-source-abcd1234", preset.Labels[apitypes.LabelConfigType],
		"config.oam.dev/type must not be overwritten")

	assert.Equal(t, apitypes.VelaCoreConfig, secret.Labels[apitypes.LabelConfigCatalog])
	assert.Equal(t, "img-source", secret.Labels[apitypes.LabelSourceDefinitionName])
	assert.Equal(t, "vela-system", secret.Labels[apitypes.LabelSourceDefinitionNamespace])
	assert.Equal(t, "10m0s", secret.Annotations[apitypes.AnnotationConfigTTL])
	assert.Equal(t, "source-img-source-abcd1234", secret.Annotations[apitypes.AnnotationConfigTemplate])
}

func TestShouldTouchSourceCacheThrottle(t *testing.T) {
	now := time.Now()
	// No marker yet -> touch.
	assert.True(t, ShouldTouchSourceCache(nil, now))
	assert.True(t, ShouldTouchSourceCache(map[string]string{}, now))
	// Recent marker relative to TTL -> skip.
	assert.False(t, ShouldTouchSourceCache(map[string]string{
		apitypes.AnnotationConfigLastAccessed: now.Add(-1 * time.Minute).Format(time.RFC3339),
		apitypes.AnnotationConfigTTL:          "10m",
	}, now))
	// Older than half the TTL -> touch.
	assert.True(t, ShouldTouchSourceCache(map[string]string{
		apitypes.AnnotationConfigLastAccessed: now.Add(-6 * time.Minute).Format(time.RFC3339),
		apitypes.AnnotationConfigTTL:          "10m",
	}, now))
}

func TestSecretStoreWriteStampsMetadata(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, corev1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	store := &secretSourceCacheStore{client: cli}

	err := store.Write(context.Background(), "source-cache-x", "img-source",
		map[string]interface{}{"region": "eu"},
		velaprocess.SourceCacheWriteMeta{
			TTL:                10 * time.Minute,
			SourceDefName:      "img-source",
			SourceDefNamespace: "vela-system",
			TemplateName:       "source-img-source-abcd1234",
		})
	assert.NoError(t, err)

	got := &corev1.Secret{}
	assert.NoError(t, cli.Get(context.Background(), types.NamespacedName{Namespace: CacheNamespace(), Name: "source-cache-x"}, got))
	assert.Equal(t, "10m0s", got.Annotations[apitypes.AnnotationConfigTTL])
	assert.Equal(t, "source-img-source-abcd1234", got.Annotations[apitypes.AnnotationConfigTemplate])
	assert.Equal(t, "img-source", got.Labels[apitypes.LabelSourceDefinitionName])
	assert.NotEmpty(t, got.Annotations[apitypes.AnnotationConfigLastSyncAt])
}

func TestSecretStoreTouchThrottled(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, corev1.AddToScheme(scheme))

	// Seed a stale-served entry whose last-accessed is already recent -> Touch skips.
	recent := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source-cache-recent",
			Namespace: CacheNamespace(),
			Annotations: map[string]string{
				apitypes.AnnotationConfigTTL:          "10m",
				apitypes.AnnotationConfigLastAccessed: time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
			},
		},
	}
	// And one whose marker is stale enough -> Touch updates.
	old := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source-cache-old",
			Namespace: CacheNamespace(),
			Annotations: map[string]string{
				apitypes.AnnotationConfigTTL:          "10m",
				apitypes.AnnotationConfigLastAccessed: time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
			},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(recent, old).Build()
	store := &secretSourceCacheStore{client: cli}

	before := recent.Annotations[apitypes.AnnotationConfigLastAccessed]
	assert.NoError(t, store.Touch(context.Background(), "source-cache-recent"))
	gotRecent := &corev1.Secret{}
	assert.NoError(t, cli.Get(context.Background(), types.NamespacedName{Namespace: CacheNamespace(), Name: "source-cache-recent"}, gotRecent))
	assert.Equal(t, before, gotRecent.Annotations[apitypes.AnnotationConfigLastAccessed], "recent marker should not be rewritten")

	oldBefore := old.Annotations[apitypes.AnnotationConfigLastAccessed]
	assert.NoError(t, store.Touch(context.Background(), "source-cache-old"))
	gotOld := &corev1.Secret{}
	assert.NoError(t, cli.Get(context.Background(), types.NamespacedName{Namespace: CacheNamespace(), Name: "source-cache-old"}, gotOld))
	assert.NotEqual(t, oldBefore, gotOld.Annotations[apitypes.AnnotationConfigLastAccessed], "stale marker should be advanced")

	// Missing entry is a no-op, not an error.
	assert.NoError(t, store.Touch(context.Background(), "source-cache-missing"))
}
