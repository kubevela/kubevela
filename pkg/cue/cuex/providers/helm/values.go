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

package helm

// Resolves inline and valuesFrom value sources from ConfigMaps and Secrets, with the cross-namespace guard.

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chartutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/pkg/util/singleton"
)

// defaultValuesKey is the key looked up in a ConfigMap/Secret when the user
// does not specify one explicitly. Matches the FluxCD and Helm CLI convention.
const defaultValuesKey = "values.yaml"

// valueSourceMissingError is returned by loaders when a ConfigMap/Secret or the
// requested key inside it does not exist. mergeValues uses this sentinel type to
// decide whether source.Optional allows the source to be skipped. Parse errors
// and other failures produce different error types, so Optional never swallows
// them — a common source of silent misconfiguration bugs.
type valueSourceMissingError struct {
	kind, name, namespace, key string
	cause                      error
}

func (e *valueSourceMissingError) Error() string {
	if e.key != "" {
		return fmt.Sprintf("%s %s/%s key %q not found: %v", e.kind, e.namespace, e.name, e.key, e.cause)
	}
	return fmt.Sprintf("%s %s/%s not found: %v", e.kind, e.namespace, e.name, e.cause)
}

func (e *valueSourceMissingError) Unwrap() error { return e.cause }

func isValueSourceMissing(err error) bool {
	var target *valueSourceMissingError
	return stderrors.As(err, &target)
}

// errCrossNamespaceValuesFrom is returned when a valuesFrom source references a
// namespace other than the Application's own namespace. The controller has
// cluster-scoped read on ConfigMaps/Secrets, so without this guard a tenant could
// read Secrets from any namespace by submitting a crafted Application.
var errCrossNamespaceValuesFrom = stderrors.New("cross-namespace valuesFrom sources are not permitted")

// mergeValues merges inline `values` and any `valuesFrom` sources into a single
// map. Priority (highest wins): inline > valuesFrom[N] > valuesFrom[N-1] > ... >
// valuesFrom[0]. Later entries override earlier ones. The merge is a deep-merge
// of map keys via chartutil.CoalesceTables; arrays are replaced wholesale (not
// concatenated), and `null` values are preserved (not treated as delete), so
// semantics diverge slightly from `helm CLI --values a.yaml --values b.yaml`
// which uses chartutil.CoalesceValues.
//
// A valuesFrom entry that omits `namespace` resolves to releaseNamespace (the
// natural co-location with the chart's deployed resources). An entry that sets
// an explicit Namespace is only allowed if it matches either releaseNamespace
// or appNamespace; any other namespace is rejected to block cross-tenant reads
// via the controller's cluster-wide RBAC.
func (p *Provider) mergeValues(ctx context.Context, baseValues interface{}, valuesFrom []ValuesFromParams, appNamespace, releaseNamespace string) (map[string]interface{}, error) {
	accumulated := map[string]interface{}{}

	for _, source := range valuesFrom {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		values, err := p.loadValuesFromSource(ctx, source, appNamespace, releaseNamespace)
		if err != nil {
			if source.Optional && isValueSourceMissing(err) {
				klog.V(2).Infof("Helm provider: skipping optional values source %s %q: %v", source.Kind, source.Name, err)
				continue
			}
			return nil, errors.Wrapf(err, "failed to load values from %s %q", source.Kind, source.Name)
		}
		// CoalesceTables(dst, src) treats dst as authoritative. `values` is the
		// newer source, so it's passed as dst to override `accumulated` (older).
		accumulated = chartutil.CoalesceTables(values, accumulated)
	}

	// Inline values override everything from valuesFrom. Clone before merging
	// because CoalesceTables mutates dst in place, and dst here is the caller's
	// map (renderParams.Values). A shallow copy is not enough: nested maps
	// would still be shared with the caller and could be mutated by deep
	// keys CoalesceTables fills in.
	if inline, ok := baseValues.(map[string]interface{}); ok {
		accumulated = chartutil.CoalesceTables(deepCloneValues(inline), accumulated)
	}

	return accumulated, nil
}

// deepCloneValues returns a deep copy of a values map.
// CoalesceTables mutates the destination map (including nested entries) in
// place, so we must isolate the caller's map at every depth, not just the
// top level. We walk the structure by hand rather than using
// runtime.DeepCopyJSON because that helper only accepts the narrow set of
// types json.Unmarshal returns (float64, not int) and panics on the int /
// int32 / int64 values CUE evaluation routinely produces. Plain
// json.Marshal/Unmarshal would coerce every number to float64 and break
// Helm templates that depend on integer type assertions like
// `{{ .Values.replicaCount | int }}`.
//
// Maps and slices are recreated; scalars (numbers, strings, bool, nil)
// are immutable in Go and so the interface copy is sufficient.
func deepCloneValues(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = deepCloneValue(v)
	}
	return out
}

func deepCloneValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return deepCloneValues(val)
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, item := range val {
			out[i] = deepCloneValue(item)
		}
		return out
	default:
		return v
	}
}

// loadValuesFromSource dispatches to the appropriate loader based on source.Kind.
func (p *Provider) loadValuesFromSource(ctx context.Context, source ValuesFromParams, appNamespace, releaseNamespace string) (map[string]interface{}, error) {
	switch source.Kind {
	case "ConfigMap":
		return p.loadConfigMapValues(ctx, source, appNamespace, releaseNamespace)
	case "Secret":
		return p.loadSecretValues(ctx, source, appNamespace, releaseNamespace)
	default:
		return nil, fmt.Errorf("unsupported values source kind: %s", source.Kind)
	}
}

// resolveValuesFromNamespace returns the effective namespace for a valuesFrom
// entry. The default (empty) resolves to releaseNamespace — the natural place
// to co-locate chart values with the chart's resources. An explicit namespace
// is accepted only if it matches releaseNamespace or appNamespace; any other
// value is rejected so a tenant cannot coerce the controller's cluster-wide
// RBAC into reading Secrets from unrelated namespaces.
func resolveValuesFromNamespace(source ValuesFromParams, appNamespace, releaseNamespace string) (string, error) {
	if source.Namespace == "" {
		return releaseNamespace, nil
	}
	if source.Namespace == releaseNamespace || source.Namespace == appNamespace {
		return source.Namespace, nil
	}
	return "", fmt.Errorf("%w: %s %q requested namespace %q but Application is in %q and release is in %q",
		errCrossNamespaceValuesFrom, source.Kind, source.Name, source.Namespace, appNamespace, releaseNamespace)
}

// loadConfigMapValues reads a ConfigMap in the Application namespace and parses
// the requested key as YAML. When source.Key is empty it falls back to
// "values.yaml" (Helm/FluxCD convention). Not-found errors (missing ConfigMap
// or missing key) are returned as valueSourceMissingError so optional sources
// can skip them; parse errors are surfaced as-is and are never swallowed by
// optional.
//
// singleton.KubeClient.Get() here is the kubevela-pkg default client built via
// controller-runtime's client.New — this is an UNCACHED client that reads
// directly from the API server. Do NOT switch this to manager.GetClient() or
// a cached reader: that would register a cluster-wide ConfigMap/Secret
// informer on first use and load every CM/Secret cluster-wide into the
// controller's memory. Direct API reads per valuesFrom entry are the intended
// trade-off.
func (p *Provider) loadConfigMapValues(ctx context.Context, source ValuesFromParams, appNamespace, releaseNamespace string) (map[string]interface{}, error) {
	ns, err := resolveValuesFromNamespace(source, appNamespace, releaseNamespace)
	if err != nil {
		return nil, err
	}
	key := source.Key
	if key == "" {
		key = defaultValuesKey
	}

	k8s := singleton.KubeClient.Get()
	cm := &corev1.ConfigMap{}
	if err := k8s.Get(ctx, client.ObjectKey{Name: source.Name, Namespace: ns}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, &valueSourceMissingError{kind: "ConfigMap", name: source.Name, namespace: ns, cause: err}
		}
		return nil, errors.Wrapf(err, "failed to read ConfigMap %s/%s", ns, source.Name)
	}

	raw, ok := cm.Data[key]
	if !ok {
		// If the key lives in binaryData (kubectl create cm --from-file of
		// non-UTF-8 content), reject explicitly. Helm values are textual; a
		// binary blob is unparseable. The clear message saves operators from
		// chasing a mismatch when `kubectl get cm` shows the key under
		// binaryData and the loader reports "not found".
		if _, isBinary := cm.BinaryData[key]; isBinary {
			return nil, errors.Errorf("ConfigMap %s/%s key %q is in binaryData; valuesFrom requires a textual YAML value in .data",
				ns, source.Name, key)
		}
		return nil, &valueSourceMissingError{
			kind: "ConfigMap", name: source.Name, namespace: ns, key: key,
			cause: fmt.Errorf("key not found in .data"),
		}
	}

	var values map[string]interface{}
	if err := yaml.Unmarshal([]byte(raw), &values); err != nil {
		return nil, errors.Wrapf(err, "ConfigMap %s/%s key %q: invalid YAML", ns, source.Name, key)
	}
	return values, nil
}

// loadSecretValues reads a Secret in the Application namespace and parses the
// requested key as YAML. Kubernetes already base64-decodes Secret.Data on read,
// so the bytes are consumed as-is. Error messages intentionally never include
// raw secret bytes.
func (p *Provider) loadSecretValues(ctx context.Context, source ValuesFromParams, appNamespace, releaseNamespace string) (map[string]interface{}, error) {
	ns, err := resolveValuesFromNamespace(source, appNamespace, releaseNamespace)
	if err != nil {
		return nil, err
	}
	key := source.Key
	if key == "" {
		key = defaultValuesKey
	}

	k8s := singleton.KubeClient.Get()
	secret := &corev1.Secret{}
	if err := k8s.Get(ctx, client.ObjectKey{Name: source.Name, Namespace: ns}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, &valueSourceMissingError{kind: "Secret", name: source.Name, namespace: ns, cause: err}
		}
		return nil, errors.Wrapf(err, "failed to read Secret %s/%s", ns, source.Name)
	}

	raw, ok := secret.Data[key]
	if !ok {
		return nil, &valueSourceMissingError{
			kind: "Secret", name: source.Name, namespace: ns, key: key,
			cause: fmt.Errorf("key not found in .data"),
		}
	}

	var values map[string]interface{}
	if err := yaml.Unmarshal(raw, &values); err != nil {
		return nil, errors.Wrapf(err, "Secret %s/%s key %q: invalid YAML", ns, source.Name, key)
	}
	return values, nil
}
