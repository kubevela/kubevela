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
	"errors"
	"time"

	"github.com/oam-dev/kubevela/pkg/sources"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/config"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	sourceCacheSyncAtKey   = apitypes.AnnotationConfigLastSyncAt
	sourceCacheAccessedKey = apitypes.AnnotationConfigLastAccessed
)

type configAPISourceCacheStore struct {
	client           client.Client
	factory          config.Factory
	templateBySource map[string]string
}

func newConfigAPISourceCacheStore(cli client.Client, templateBySource map[string]string) *configAPISourceCacheStore {
	if templateBySource == nil {
		templateBySource = map[string]string{}
	}
	return &configAPISourceCacheStore{
		client:           cli,
		factory:          config.NewConfigFactory(cli),
		templateBySource: templateBySource,
	}
}

// sourceTemplateRefsByType maps each referenced SourceDefinition type to its
// versioned ConfigTemplate name. The appfile's RelatedSourceDefinitions have
// their Status wiped during parsing (consistent with Component/TraitDefinition,
// so volatile status does not leak into the ApplicationRevision snapshot), so
// ConfigTemplateRef is read from the LIVE SourceDefinition via the client. This
// name is used to label cache Config entries with config.oam.dev/type, linking
// them back to the ConfigTemplate version. A missing/not-yet-populated ref is
// skipped; the cache store then falls back to the raw source type label.
func sourceTemplateRefsByType(ctx context.Context, cli client.Client, af *appfile.Appfile) map[string]string {
	out := map[string]string{}
	if af == nil || cli == nil {
		return out
	}
	for sourceType := range af.RelatedSourceDefinitions {
		def := &v1beta1.SourceDefinition{}
		if err := cli.Get(ctx, client.ObjectKey{Namespace: af.Namespace, Name: sourceType}, def); err != nil {
			if !apierrors.IsNotFound(err) {
				continue
			}
			if err := cli.Get(ctx, client.ObjectKey{Namespace: oam.SystemDefinitionNamespace, Name: sourceType}, def); err != nil {
				continue
			}
		}
		if def.Status.ConfigTemplateRef == nil || def.Status.ConfigTemplateRef.Name == "" {
			continue
		}
		out[sourceType] = def.Status.ConfigTemplateRef.Name
	}
	return out
}

func (s *configAPISourceCacheStore) Read(ctx context.Context, cacheKey string, ttl time.Duration) (map[string]interface{}, bool, bool, time.Time, error) {
	cfg, err := s.factory.GetConfig(ctx, sources.CacheNamespace(), cacheKey, false)
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			return nil, false, false, time.Time{}, nil
		}
		return nil, false, false, time.Time{}, err
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	lastSyncAt := cfg.CreateTime
	if cfg.Secret != nil && cfg.Secret.Annotations != nil {
		if raw, ok := cfg.Secret.Annotations[sourceCacheSyncAtKey]; ok && raw != "" {
			if parsed, parseErr := time.Parse(time.RFC3339, raw); parseErr == nil {
				lastSyncAt = parsed
			}
		}
	}
	expiresAt := lastSyncAt.Add(ttl)
	return cfg.Properties, true, time.Now().After(expiresAt), expiresAt, nil
}

func (s *configAPISourceCacheStore) Write(ctx context.Context, cacheKey, sourceType string, data map[string]interface{}, meta velaprocess.SourceCacheWriteMeta) error {
	// The template the render actually used, not the one the live definition
	// points at now. A published ApplicationRevision resolves against the
	// definition it snapshotted, so once the live definition's schema changed
	// the entry was parsed against the wrong template and ParseConfig rejected
	// it. The live lookup stays as the fallback for a resolver that reported
	// none - a source with no schema to name one.
	templateName := meta.TemplateName
	if templateName == "" {
		templateName = s.templateBySource[sourceType]
	}
	template := config.NamespacedName{}
	if templateName != "" {
		template = config.NamespacedName{
			Name:      templateName,
			Namespace: sources.CacheNamespace(),
		}
	}
	cfg, err := s.factory.ParseConfig(ctx, template, config.Metadata{
		NamespacedName: config.NamespacedName{
			Name:      cacheKey,
			Namespace: sources.CacheNamespace(),
		},
		Properties: data,
	})
	if err != nil {
		return err
	}
	// Stamp identity + lifetime metadata for the GC sweep. sourceType is always
	// recorded as the config type; the resolved template name (if any) is kept in
	// its own annotation rather than overloading the type label.
	//
	// Stamped onto the Config rather than onto the Secret it happens to write.
	// The factory merges them, leaving the labels it set itself alone - which is
	// what keeps ParseConfig's template name as the config type here.
	stampMeta := meta
	stampMeta.TemplateName = templateName
	carrier := &metav1.ObjectMeta{}
	sources.ApplySourceCacheMetadata(carrier, sourceType, stampMeta)
	if carrier.Annotations == nil {
		carrier.Annotations = map[string]string{}
	}
	carrier.Annotations[sourceCacheSyncAtKey] = time.Now().UTC().Format(time.RFC3339)
	cfg.Labels, cfg.Annotations = carrier.Labels, carrier.Annotations

	// Values first, declaration second. The validating webhook resolves
	// spec.propertiesFrom when a Config is admitted, so declaring the entry
	// before its Secret exists is denied outright: "failed to load
	// spec.propertiesFrom secret".
	//
	// The values go in the Secret, not in spec.properties: a source schema can
	// mark fields +sensitive and the resolver redacts them out of status, so they
	// must be assumed sensitive, and a CR spec is plainly readable.
	// propertiesFrom is the field for exactly that, and the Config controller
	// only ever reads what it points at.
	if err := s.factory.CreateOrUpdateConfig(ctx, cfg, sources.CacheNamespace()); err != nil {
		return err
	}
	declared, err := s.declareConfig(ctx, cacheKey, templateName, carrier)
	if err != nil {
		return err
	}
	// The owner reference needs the Config's UID, so it can only be set once the
	// Config exists - which is after the Secret it points at. A second write,
	// but only on a cache miss, and only until the reference is in place.
	return s.adoptCacheSecret(ctx, declared, cacheKey)
}

// adoptCacheSecret points the entry's Secret at the Config that declares it, so
// deleting either takes both.
func (s *configAPISourceCacheStore) adoptCacheSecret(ctx context.Context,
	declared *configv1alpha1.Config, cacheKey string) error {
	if declared == nil || s.client == nil {
		return nil
	}
	secret := &corev1.Secret{}
	if err := s.client.Get(ctx, client.ObjectKey{
		Namespace: sources.CacheNamespace(), Name: cacheKey}, secret); err != nil {
		return client.IgnoreNotFound(err)
	}
	for _, ref := range secret.OwnerReferences {
		if ref.UID == declared.UID {
			return nil
		}
	}
	secret.OwnerReferences = append(secret.OwnerReferences,
		*metav1.NewControllerRef(declared, configv1alpha1.SchemeGroupVersion.WithKind(configv1alpha1.ConfigKind)))
	return s.client.Update(ctx, secret)
}

// declareConfig creates or updates the Config that declares a cache entry, and
// returns it so the entry's Secret can be owned by it.
func (s *configAPISourceCacheStore) declareConfig(ctx context.Context, cacheKey, templateName string,
	meta *metav1.ObjectMeta) (*configv1alpha1.Config, error) {
	if s.client == nil {
		return nil, nil
	}
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: cacheKey, Namespace: sources.CacheNamespace()},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, s.client, cfg, func() error {
		if cfg.Labels == nil {
			cfg.Labels = map[string]string{}
		}
		for key, value := range meta.Labels {
			cfg.Labels[key] = value
		}
		if cfg.Annotations == nil {
			cfg.Annotations = map[string]string{}
		}
		for key, value := range meta.Annotations {
			cfg.Annotations[key] = value
		}
		cfg.Spec.Properties = nil
		cfg.Spec.PropertiesFrom = &configv1alpha1.PropertiesReference{
			SecretRef: configv1alpha1.SecretKeySelector{
				Name: cacheKey,
				Key:  config.SaveInputPropertiesKey,
			},
		}
		if templateName != "" {
			cfg.Spec.TemplateRef = &configv1alpha1.ConfigTemplateReference{
				Name:      templateName,
				Namespace: sources.CacheNamespace(),
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Touch advances the last-accessed marker on a stale cache entry that is being
// served. It is throttled against the entry's TTL to bound write amplification.
func (s *configAPISourceCacheStore) Touch(ctx context.Context, cacheKey string) error {
	if s.client == nil {
		return nil
	}
	secret := &corev1.Secret{}
	if err := s.client.Get(ctx, client.ObjectKey{Namespace: sources.CacheNamespace(), Name: cacheKey}, secret); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !sources.ShouldTouchSourceCache(secret.Annotations, time.Now()) {
		return nil
	}
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[sourceCacheAccessedKey] = time.Now().UTC().Format(time.RFC3339)
	return s.client.Update(ctx, secret)
}
