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
	"encoding/json"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

type secretSourceCacheStore struct {
	client client.Client
}

// NewSecretSourceCacheStore creates a Secret-backed source cache store.
func NewSecretSourceCacheStore(cli client.Client) velaprocess.SourceCacheStore {
	if cli == nil {
		return nil
	}
	return &secretSourceCacheStore{client: cli}
}

func (s *secretSourceCacheStore) Read(ctx context.Context, cacheKey string, ttl time.Duration) (map[string]interface{}, bool, bool, time.Time, error) {
	if s == nil || s.client == nil || cacheKey == "" {
		return nil, false, false, time.Time{}, nil
	}
	secret := &corev1.Secret{}
	if err := s.client.Get(ctx, ktypes.NamespacedName{Namespace: CacheNamespace(), Name: cacheKey}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, false, time.Time{}, nil
		}
		return nil, false, false, time.Time{}, err
	}
	if secret.Data == nil || len(secret.Data[sourceCacheDataKey]) == 0 {
		return nil, false, false, time.Time{}, nil
	}
	properties := map[string]interface{}{}
	if err := json.Unmarshal(secret.Data[sourceCacheDataKey], &properties); err != nil {
		return nil, false, false, time.Time{}, err
	}
	lastSync := secret.CreationTimestamp.Time
	if ts := secret.Annotations[sourceCacheSyncAtKey]; ts != "" {
		if t, parseErr := time.Parse(time.RFC3339, ts); parseErr == nil {
			lastSync = t
		}
	}
	if ttl <= 0 {
		ttl = sourceCacheTTL
	}
	stale := time.Since(lastSync) > ttl
	expiresAt := lastSync.Add(ttl)
	return properties, stale, true, expiresAt, nil
}

func (s *secretSourceCacheStore) Write(ctx context.Context, cacheKey, sourceType string, data map[string]interface{}, meta velaprocess.SourceCacheWriteMeta) error {
	if s == nil || s.client == nil || cacheKey == "" {
		return nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339)
	key := ktypes.NamespacedName{Namespace: CacheNamespace(), Name: cacheKey}
	secret := &corev1.Secret{}
	if err := s.client.Get(ctx, key, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		secret = &corev1.Secret{}
		secret.Namespace = CacheNamespace()
		secret.Name = cacheKey
		secret.Type = corev1.SecretTypeOpaque
		secret.Labels = map[string]string{}
		secret.Annotations = map[string]string{}
		secret.Data = map[string][]byte{}
		ApplySourceCacheMetadata(secret, sourceType, meta)
		secret.Annotations[sourceCacheSyncAtKey] = now
		secret.Data[sourceCacheDataKey] = raw
		return s.client.Create(ctx, secret)
	}
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	ApplySourceCacheMetadata(secret, sourceType, meta)
	secret.Annotations[sourceCacheSyncAtKey] = now
	secret.Data[sourceCacheDataKey] = raw
	return s.client.Update(ctx, secret)
}

// Touch records that a stale cache entry was served, advancing the last-accessed
// annotation so the GC sweep does not collect an entry that is still in use. It
// is throttled: if the existing last-accessed marker is already recent relative
// to the entry's own TTL, the update is skipped to bound write amplification on
// the render path.
func (s *secretSourceCacheStore) Touch(ctx context.Context, cacheKey string) error {
	if s == nil || s.client == nil || cacheKey == "" {
		return nil
	}
	secret := &corev1.Secret{}
	if err := s.client.Get(ctx, ktypes.NamespacedName{Namespace: CacheNamespace(), Name: cacheKey}, secret); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !ShouldTouchSourceCache(secret.Annotations, time.Now()) {
		return nil
	}
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[sourceCacheAccessedKey] = time.Now().Format(time.RFC3339)
	return s.client.Update(ctx, secret)
}
