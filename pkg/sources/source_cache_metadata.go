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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	apitypes "github.com/oam-dev/kubevela/apis/types"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/cue/render"
)

// ApplySourceCacheMetadata stamps identity and lifetime metadata onto a source
// cache object so a context-free GC sweep can reason about it. It is strictly
// additive: it never overwrites the config.oam.dev/type label, which callers
// (e.g. the config-API store via ParseConfig) set to the ConfigTemplate name and
// which the config factory relies on for its change-template guard. The
// ttl/template/sourcedefinition markers are new.
func ApplySourceCacheMetadata(obj metav1.Object, sourceType string, meta velaprocess.SourceCacheWriteMeta) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[apitypes.LabelConfigCatalog] = apitypes.VelaCoreConfig
	// Preserve an existing type (the template name set by ParseConfig); only fall
	// back to the source type when nothing linked a template (the Secret-store
	// path, which has no ConfigTemplate).
	if labels[apitypes.LabelConfigType] == "" {
		labels[apitypes.LabelConfigType] = sourceType
	}
	if meta.SourceDefName != "" {
		labels[apitypes.LabelSourceDefinitionName] = meta.SourceDefName
	}
	if meta.SourceDefNamespace != "" {
		labels[apitypes.LabelSourceDefinitionNamespace] = meta.SourceDefNamespace
	}
	for k, v := range render.ContextLabels(meta.Context) {
		labels[k] = v
	}
	obj.SetLabels(labels)

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	if meta.TTL > 0 {
		annotations[sourceCacheTTLKey] = meta.TTL.String()
	}
	if meta.TemplateName != "" {
		annotations[sourceCacheTemplateKey] = meta.TemplateName
	}
	if meta.TemplateHash != "" {
		annotations[apitypes.AnnotationSourceTemplateHash] = meta.TemplateHash
	}
	if len(meta.KeyInputs) > 0 {
		if raw, err := json.Marshal(meta.KeyInputs); err == nil {
			annotations[apitypes.AnnotationSourceKeyInputs] = string(raw)
		}
	}
	if len(meta.Context) > 0 {
		if raw, err := json.Marshal(meta.Context); err == nil {
			annotations[apitypes.AnnotationSourceContext] = string(raw)
		}
	}
	if len(meta.Properties) > 0 {
		if raw, truncated, err := render.Properties(meta.Properties); err == nil {
			annotations[apitypes.AnnotationSourceProperties] = raw
			if truncated {
				// Say so explicitly, so a clipped value is never mistaken for the
				// real one when someone is comparing two entries.
				annotations[apitypes.AnnotationSourcePropertiesTruncated] = "true"
			}
		}
	}
	obj.SetAnnotations(annotations)
}

// sourceCacheTemplateName reproduces the ConfigTemplate name the SourceDefinition
// controller derives from (sourceType, schema) so a cache entry can be stamped
// with the template it was rendered against without a client round-trip. It must
// stay in sync with buildSchemaTemplateName in the sourcedefinition controller.
// Returns "" when there is no schema (no template is generated in that case).
func sourceCacheTemplateName(sourceType, schemaExpr string) string {
	if schemaExpr == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(schemaExpr))
	shortHash := hex.EncodeToString(sum[:])[:8]
	safeName := sanitizeSourceName(sourceType)
	if safeName == "" {
		safeName = "source"
	}
	const prefix = "source-"
	suffix := "-" + shortHash
	maxNameLen := 63 - len(prefix) - len(suffix)
	if maxNameLen < 1 {
		maxNameLen = 1
	}
	if len(safeName) > maxNameLen {
		safeName = strings.Trim(safeName[:maxNameLen], "-")
		if safeName == "" {
			safeName = "source"
		}
	}
	return prefix + safeName + suffix
}

func sanitizeSourceName(name string) string {
	s := strings.ToLower(name)
	var b strings.Builder
	b.Grow(len(s))
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// ShouldTouchSourceCache throttles last-accessed updates: it returns true only
// when no marker exists yet or the existing one is older than half the entry's
// TTL, so a hot stale entry is not rewritten on every reconcile. The TTL is read
// from the entry's own annotation, defaulting to sourceCacheTTL.
func ShouldTouchSourceCache(annotations map[string]string, now time.Time) bool {
	if annotations == nil {
		return true
	}
	raw := annotations[sourceCacheAccessedKey]
	if raw == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return true
	}
	ttl := sourceCacheTTL
	if t := annotations[sourceCacheTTLKey]; t != "" {
		if parsed, perr := time.ParseDuration(t); perr == nil && parsed > 0 {
			ttl = parsed
		}
	}
	return now.Sub(last) >= ttl/2
}
