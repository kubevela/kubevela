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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// helmchartComponentType is the component-definition type string the helm
// provider handles. The fingerprint helper only walks components of this type.
// Defined in vela-templates/definitions/internal/component/helmchart.cue.
const helmchartComponentType = "helmchart"

// defaultValuesFromKey matches the helm provider's defaultValuesKey at
// pkg/cue/cuex/providers/helm/helm.go (see the const there). Falls back to
// "values.yaml" when the user does not specify a key, matching FluxCD and
// Helm CLI conventions. The two constants must stay in sync; if helm.go
// changes its default, update this one too.
const defaultValuesFromKey = "values.yaml"

// valuesFromRef captures the subset of the helmchart valuesFrom entry the
// fingerprint helper needs.
type valuesFromRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Key       string `json:"key,omitempty"`
	Optional  bool   `json:"optional,omitempty"`
}

// helmchartProperties is the minimal subset of helmchart component properties
// the fingerprint helper needs. Other fields are ignored to keep the helper
// resilient to future schema additions.
type helmchartProperties struct {
	Release struct {
		Namespace string `json:"namespace,omitempty"`
	} `json:"release,omitempty"`
	ValuesFrom []valuesFromRef `json:"valuesFrom,omitempty"`
}

// resolvedRef is a valuesFromRef plus the namespace context (Application's
// namespace and the release namespace, derived from the helmchart properties)
// needed to resolve the source's effective namespace later.
type resolvedRef struct {
	ref              valuesFromRef
	appNamespace     string
	releaseNamespace string
}

// computeValuesFromContentFingerprint walks every helmchart component in the
// Application, reads its referenced ConfigMaps/Secrets, and returns a stable
// sha256 hex digest covering all referenced content. Returns "" when the App
// has no helmchart-with-valuesFrom components, so the workflow gate is left
// unchanged for apps that do not opt in.
//
// The digest is intended to be appended to desiredRev as a suffix so external
// CM/Secret edits move the gate without changing the AppRevision hash.
func computeValuesFromContentFingerprint(ctx context.Context, app *v1beta1.Application) (string, error) {
	if app == nil || len(app.Spec.Components) == 0 {
		return "", nil
	}

	var refs []resolvedRef
	for _, comp := range app.Spec.Components {
		if comp.Type != helmchartComponentType {
			continue
		}
		if comp.Properties == nil || len(comp.Properties.Raw) == 0 {
			continue
		}
		var props helmchartProperties
		if err := json.Unmarshal(comp.Properties.Raw, &props); err != nil {
			// Properties not parseable as our minimal shape — let the render
			// layer surface the schema error. Skip fingerprinting this
			// component so we do not gate on a parser disagreement.
			continue
		}
		if len(props.ValuesFrom) == 0 {
			continue
		}
		releaseNamespace := props.Release.Namespace
		if releaseNamespace == "" {
			releaseNamespace = app.Namespace
		}
		for _, ref := range props.ValuesFrom {
			refs = append(refs, resolvedRef{
				ref:              ref,
				appNamespace:     app.Namespace,
				releaseNamespace: releaseNamespace,
			})
		}
	}

	if len(refs) == 0 {
		return "", nil
	}

	lines := make([]string, 0, len(refs))
	for _, r := range refs {
		line, err := hashOneSource(ctx, r.ref, r.appNamespace, r.releaseNamespace)
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h[:]), nil
}

// hashOneSource resolves one valuesFrom reference and returns a per-source
// "kind:ns/name/key=hex" line for the aggregate digest. The hex is sha256 of
// canonical JSON of the parsed YAML content, matching the helm provider's
// computeReleaseFingerprint contract so cosmetic-only YAML edits do not move
// the digest.
//
// singleton.KubeClient.Get() returns an UNCACHED client (see helm.go's
// loadConfigMapValues for the rationale): switching to a cached reader would
// register a cluster-wide ConfigMap/Secret informer and bloat controller
// memory. Direct API reads per source per reconcile are the intended
// trade-off.
func hashOneSource(ctx context.Context, ref valuesFromRef, appNamespace, releaseNamespace string) (string, error) {
	ns, err := resolveNamespace(ref, appNamespace, releaseNamespace)
	if err != nil {
		return "", err
	}
	key := ref.Key
	if key == "" {
		key = defaultValuesFromKey
	}

	k8s := singleton.KubeClient.Get()

	switch ref.Kind {
	case "ConfigMap":
		cm := &corev1.ConfigMap{}
		if err := k8s.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, cm); err != nil {
			if apierrors.IsNotFound(err) {
				if ref.Optional {
					return missingLine(ref.Kind, ns, ref.Name, key), nil
				}
				return "", fmt.Errorf("configmap %s/%s not found", ns, ref.Name)
			}
			return "", fmt.Errorf("failed to read ConfigMap %s/%s: %w", ns, ref.Name, err)
		}
		raw, ok := cm.Data[key]
		if !ok {
			if ref.Optional {
				return missingLine(ref.Kind, ns, ref.Name, key), nil
			}
			return "", fmt.Errorf("configmap %s/%s key %q not found", ns, ref.Name, key)
		}
		return canonicalLine(ref.Kind, ns, ref.Name, key, []byte(raw))
	case "Secret":
		sec := &corev1.Secret{}
		if err := k8s.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, sec); err != nil {
			if apierrors.IsNotFound(err) {
				if ref.Optional {
					return missingLine(ref.Kind, ns, ref.Name, key), nil
				}
				return "", fmt.Errorf("secret %s/%s not found", ns, ref.Name)
			}
			return "", fmt.Errorf("failed to read Secret %s/%s: %w", ns, ref.Name, err)
		}
		raw, ok := sec.Data[key]
		if !ok {
			if ref.Optional {
				return missingLine(ref.Kind, ns, ref.Name, key), nil
			}
			return "", fmt.Errorf("secret %s/%s key %q not found", ns, ref.Name, key)
		}
		// Kubernetes already base64-decodes Secret.Data on read, so the bytes
		// are consumed as-is. Error messages intentionally never include raw
		// secret content (matches the helm provider's convention).
		return canonicalLine(ref.Kind, ns, ref.Name, key, raw)
	default:
		// OCIRepository is deferred and remains unsupported
		// here. The helm provider's loadValuesFromSource is the source of
		// truth for kind validation.
		return "", fmt.Errorf("unsupported valuesFrom kind %q: only ConfigMap and Secret are currently supported by the fingerprint helper", ref.Kind)
	}
}

// canonicalLine parses raw as YAML, re-marshals as JSON (Go's encoding/json
// sorts map keys deterministically when marshalling map[string]interface{}),
// sha256s the canonical bytes, and returns a "kind:ns/name/key=hex" line.
//
// The parse-then-canonicalise round trip is intentional: it makes the digest
// invariant under whitespace and comment edits in the source YAML, matching
// the helm provider's computeReleaseFingerprint contract (which hashes the
// JSON of the parsed values map).
func canonicalLine(kind, ns, name, key string, raw []byte) (string, error) {
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("%s %s/%s key %q: invalid YAML: %w", kind, ns, name, key, err)
	}
	if parsed == nil {
		parsed = map[string]interface{}{}
	}
	canonical, err := json.Marshal(parsed)
	if err != nil {
		return "", fmt.Errorf("%s %s/%s key %q: canonicalise failed: %w", kind, ns, name, key, err)
	}
	h := sha256.Sum256(canonical)
	return fmt.Sprintf("%s:%s/%s/%s=%s", kind, ns, name, key, hex.EncodeToString(h[:])), nil
}

// missingLine is the per-source line emitted when an optional valuesFrom
// source is absent (resource missing or key missing). The literal "<missing>"
// keeps the aggregate fingerprint stable across reconciles while the source
// stays absent, and moves the digest the moment the source appears.
func missingLine(kind, ns, name, key string) string {
	return fmt.Sprintf("%s:%s/%s/%s=<missing>", kind, ns, name, key)
}

// resolveNamespace mirrors the helm provider's resolveValuesFromNamespace.
// An empty Namespace defaults to releaseNamespace. An explicit Namespace is
// accepted only if it equals releaseNamespace or appNamespace; any other
// value is rejected to block cross-tenant Secret reads via the controller's
// cluster-wide RBAC.
func resolveNamespace(ref valuesFromRef, appNamespace, releaseNamespace string) (string, error) {
	if ref.Namespace == "" {
		return releaseNamespace, nil
	}
	if ref.Namespace == releaseNamespace || ref.Namespace == appNamespace {
		return ref.Namespace, nil
	}
	return "", fmt.Errorf("cross-namespace valuesFrom sources are not permitted: %s %q requested namespace %q but Application is in %q and release is in %q",
		ref.Kind, ref.Name, ref.Namespace, appNamespace, releaseNamespace)
}
