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

package sourcedefinition

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
)

const (
	// defaultSourceCacheTTL mirrors the render-time default when a source cache
	// entry carries no config.oam.dev/ttl annotation.
	defaultSourceCacheTTL = 15 * time.Minute
	// sourceCacheStaleMultiplier is applied to an entry's TTL to decide how long a
	// stale, unaccessed entry is retained before collection. An entry is deleted
	// only once it is both stale (past one TTL since last sync) and has not been
	// served for staleMultiplier×TTL since it was last accessed.
	sourceCacheStaleMultiplier = 3
)

// cacheGCResult summarizes one sweep, for logging and tests.
type cacheGCResult struct {
	ConfigsScanned    int
	ConfigsDeleted    int
	TemplatesScanned  int
	TemplatesDeleted  int
	TemplatesRetained int
}

// sweepSourceCache performs one mark-and-sweep pass over source cache Secrets and
// auto-generated ConfigTemplates in vela-system. It is context-free: every
// decision is made from metadata stamped on the objects themselves plus the set
// of live SourceDefinition references, so it can run on a timer without any
// application/render context.
//
// Pass 1 deletes cache Secrets that are both stale and unaccessed past
// staleMultiplier×TTL, and records the templates still referenced by survivors.
// Pass 2 deletes source ConfigTemplates that are referenced by neither a
// surviving cache Secret nor a live SourceDefinition, once they are older than a
// grace window (so a freshly created template whose first cache write has not yet
// landed is never collected).
func (r *Reconciler) sweepSourceCache(ctx context.Context) (cacheGCResult, error) {
	now := time.Now()
	res := cacheGCResult{}

	referencedTemplates, err := r.sweepCacheSecrets(ctx, now, &res)
	if err != nil {
		return res, err
	}

	liveRefs, err := r.liveTemplateRefs(ctx)
	if err != nil {
		return res, err
	}
	for name := range liveRefs {
		referencedTemplates[name] = struct{}{}
	}

	if err := r.sweepTemplates(ctx, now, referencedTemplates, &res); err != nil {
		return res, err
	}

	klog.InfoS("source cache GC sweep complete",
		"configsScanned", res.ConfigsScanned, "configsDeleted", res.ConfigsDeleted,
		"templatesScanned", res.TemplatesScanned, "templatesDeleted", res.TemplatesDeleted,
		"templatesRetained", res.TemplatesRetained)
	return res, nil
}

// sweepCacheSecrets applies the Config GC predicate and returns the set of
// ConfigTemplate names still referenced by surviving cache Secrets.
func (r *Reconciler) sweepCacheSecrets(ctx context.Context, now time.Time, res *cacheGCResult) (map[string]struct{}, error) {
	referenced := map[string]struct{}{}
	var secrets corev1.SecretList
	if err := r.List(ctx, &secrets,
		client.InNamespace(sourceTemplateNamespace),
		client.MatchingLabels{apitypes.LabelConfigCatalog: apitypes.VelaCoreConfig}); err != nil {
		return referenced, err
	}
	// An entry declared through the Config API keeps its lifetime metadata on the
	// Config, not on the Secret: the Config controller's output Secret is named
	// for the Config, so it lands on the very Secret propertiesFrom points at and
	// replaces its labels and annotations wholesale. The Config is the only half
	// nothing else writes.
	var declared configv1alpha1.ConfigList
	if err := r.List(ctx, &declared,
		client.InNamespace(sourceTemplateNamespace),
		client.HasLabels{apitypes.LabelSourceDefinitionName}); err != nil {
		return referenced, err
	}
	collected := map[string]struct{}{}
	for i := range declared.Items {
		cfg := &declared.Items[i]
		if _, ok := cfg.Annotations[apitypes.AnnotationConfigLastSyncAt]; !ok {
			continue
		}
		res.ConfigsScanned++
		if shouldCollectCacheSecret(cfg.Annotations, now) {
			// Both halves, rather than leaving the Secret to the owner reference:
			// the staleness decision is already made, and an entry still present
			// is still served.
			if err := r.Delete(ctx, cfg); err != nil && !apierrors.IsNotFound(err) {
				klog.ErrorS(err, "failed to delete a stale source cache Config", "name", cfg.Name)
			} else {
				collected[cfg.Name] = struct{}{}
				res.ConfigsDeleted++
				continue
			}
		}
		if tmpl := cfg.Annotations[apitypes.AnnotationConfigTemplate]; tmpl != "" {
			referenced[tmpl] = struct{}{}
		}
	}

	for i := range secrets.Items {
		s := &secrets.Items[i]
		// The Secret half of an entry already collected above.
		if _, done := collected[s.Name]; done {
			if err := r.Delete(ctx, s); err != nil && !apierrors.IsNotFound(err) {
				klog.ErrorS(err, "failed to delete a stale source cache secret", "name", s.Name)
			}
			continue
		}
		// A Secret a Config declares is accounted for above, whether or not it
		// was collected.
		if owningConfig(s) != "" {
			continue
		}
		// What remains is an entry the admission-side Secret store wrote, which
		// carries its own metadata. Only those have last-sync-at; other
		// velacore-config secrets (distributed configs, say) are not ours.
		if _, ok := s.Annotations[apitypes.AnnotationConfigLastSyncAt]; !ok {
			continue
		}
		res.ConfigsScanned++
		if shouldCollectCacheSecret(s.Annotations, now) {
			if err := r.Delete(ctx, s); err != nil && !apierrors.IsNotFound(err) {
				klog.ErrorS(err, "failed to delete stale source cache secret", "name", s.Name)
				// It is still here, so it still references its template. Falling
				// through to record that keeps the template from being collected
				// out from under a secret that survived.
			} else {
				res.ConfigsDeleted++
				continue
			}
		}
		if tmpl := s.Annotations[apitypes.AnnotationConfigTemplate]; tmpl != "" {
			referenced[tmpl] = struct{}{}
		}
	}
	return referenced, nil
}

// liveTemplateRefs returns the set of ConfigTemplate names referenced by the
// status of every live SourceDefinition in the cluster.
func (r *Reconciler) liveTemplateRefs(ctx context.Context) (map[string]struct{}, error) {
	refs := map[string]struct{}{}
	var defs v1beta1.SourceDefinitionList
	if err := r.List(ctx, &defs); err != nil {
		return refs, err
	}
	for i := range defs.Items {
		ref := defs.Items[i].Status.ConfigTemplateRef
		if ref != nil && ref.Name != "" {
			refs[ref.Name] = struct{}{}
		}
	}
	return refs, nil
}

// sweepTemplates deletes source ConfigTemplates that are referenced by neither a
// surviving cache Secret nor a live SourceDefinition and are older than the grace
// window.
func (r *Reconciler) sweepTemplates(ctx context.Context, now time.Time, referenced map[string]struct{}, res *cacheGCResult) error {
	grace := sourceCacheStaleMultiplier * defaultSourceCacheTTL
	// Select on the owning-SourceDefinition label the controller stamps, not on
	// the name. "config-template-source-" is a convention, and a ConfigTemplate
	// somebody happened to call source-of-truth would otherwise be swept: it
	// matches the prefix, no SourceDefinition references it, and it ages past the
	// grace window like anything else. The label is written by us, so it says
	// ownership rather than resemblance.
	var templates configv1alpha1.ConfigTemplateList
	if err := r.List(ctx, &templates, client.InNamespace(sourceTemplateNamespace),
		client.HasLabels{apitypes.LabelSourceDefinitionName}); err != nil {
		return err
	}
	for i := range templates.Items {
		ct := &templates.Items[i]
		res.TemplatesScanned++
		// A ConfigTemplate CR is named for the template, so there is no prefix to
		// strip and nothing to misparse.
		if _, ok := referenced[ct.Name]; ok {
			res.TemplatesRetained++
			continue
		}
		if now.Sub(ct.CreationTimestamp.Time) < grace {
			// Too young: a cache write for it may not have landed yet.
			res.TemplatesRetained++
			continue
		}
		if err := r.Delete(ctx, ct); err != nil && !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to delete orphaned source ConfigTemplate", "name", ct.Name)
			continue
		}
		res.TemplatesDeleted++
	}
	return nil
}

// owningConfig names the Config that declares this cache entry, or "" when
// nothing does - which is every entry written by the Secret store.
func owningConfig(obj metav1.Object) string {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Kind == configv1alpha1.ConfigKind &&
			ref.APIVersion == configv1alpha1.SchemeGroupVersion.String() {
			return ref.Name
		}
	}
	return ""
}

// shouldCollectCacheSecret returns true when a source cache entry is both stale
// (now past one TTL since last sync) and has not been served for
// staleMultiplier×TTL since it was last accessed. last-accessed defaults to
// last-sync-at when never stamped, so an entry nobody reads is collected
// staleMultiplier×TTL after its last sync.
func shouldCollectCacheSecret(annotations map[string]string, now time.Time) bool {
	if annotations == nil {
		return false
	}
	lastSyncRaw := annotations[apitypes.AnnotationConfigLastSyncAt]
	if lastSyncRaw == "" {
		return false
	}
	lastSync, err := time.Parse(time.RFC3339, lastSyncRaw)
	if err != nil {
		return false
	}
	ttl := defaultSourceCacheTTL
	if t := annotations[apitypes.AnnotationConfigTTL]; t != "" {
		if parsed, perr := time.ParseDuration(t); perr == nil && parsed > 0 {
			ttl = parsed
		}
	}
	// Not stale yet.
	if now.Sub(lastSync) <= ttl {
		return false
	}
	lastAccess := lastSync
	if a := annotations[apitypes.AnnotationConfigLastAccessed]; a != "" {
		if parsed, perr := time.Parse(time.RFC3339, a); perr == nil {
			lastAccess = parsed
		}
	}
	return now.Sub(lastAccess) > time.Duration(sourceCacheStaleMultiplier)*ttl
}
