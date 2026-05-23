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

import (
	"bytes"
	stderrors "errors"
	"io"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

// getActionConfig initializes a Helm action.Configuration with a real Kubernetes
// REST client and a secrets-based storage driver so that releases persist in-cluster.
func (p *Provider) getActionConfig(namespace string) (*action.Configuration, error) {
	actionConfig := &action.Configuration{}
	if err := actionConfig.Init(
		p.helmClient.RESTClientGetter(),
		namespace,
		"secret",
		klog.Infof,
	); err != nil {
		return nil, errors.Wrap(err, "failed to initialize helm action configuration")
	}
	return actionConfig, nil
}

// velaLabelPostRenderer implements postrender.PostRenderer.
// It injects KubeVela ownership labels and annotations into every resource
// before Helm deploys them, enabling KubeVela to adopt the resources.
// It also injects Helm ownership annotations (meta.helm.sh/release-name and
// meta.helm.sh/release-namespace) so that resources re-applied from
// KubeVela's ResourceTracker can be adopted by a subsequent helm install.
type velaLabelPostRenderer struct {
	context          *ContextParams
	releaseName      string
	releaseNamespace string
}

// Run implements postrender.PostRenderer. It parses each YAML document in the
// rendered manifests, injects KubeVela ownership labels/annotations, and returns
// the modified manifests.
func (r *velaLabelPostRenderer) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	if r.context == nil {
		return renderedManifests, nil
	}

	out := &bytes.Buffer{}
	decoder := kyaml.NewYAMLOrJSONDecoder(bytes.NewReader(renderedManifests.Bytes()), 4096)

	for {
		obj := &unstructured.Unstructured{}
		if err := decoder.Decode(obj); err != nil {
			if stderrors.Is(err, io.EOF) {
				break
			}
			return nil, errors.Wrap(err, "post-renderer: failed to decode manifest")
		}

		if len(obj.Object) == 0 {
			continue
		}

		// Inject KubeVela ownership labels
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["app.oam.dev/name"] = r.context.AppName
		labels["app.oam.dev/namespace"] = r.context.AppNamespace
		labels["app.oam.dev/component"] = r.context.Name
		obj.SetLabels(labels)

		// Default metadata.namespace for namespaced rendered resources whose
		// template omitted it. Upstream charts (Bitnami, podinfo, ...)
		// typically rely on `helm install --namespace` for placement instead
		// of templating metadata.namespace, and Helm's own apply step then
		// uses the kube client's default namespace (which under KubeVela
		// resolves to the controller's own ns, vela-system) rather than the
		// release namespace. Stamping the namespace here makes every output
		// in the rendered manifest carry the right placement before helm's
		// kube.Client.Create runs, and before KubeVela's resource tracker
		// re-applies it. Cluster-scoped kinds (CRDs, ClusterRoles,
		// Namespaces, ...) are left as-is so the API server does not reject
		// them.
		if r.releaseNamespace != "" && obj.GetNamespace() == "" && !isClusterScopedGVK(obj.GroupVersionKind()) {
			obj.SetNamespace(r.releaseNamespace)
		}

		// Inject ownership annotations (both KubeVela and Helm)
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["app.oam.dev/owner"] = "helm-provider"
		// Inject Helm ownership annotations so that resources re-applied from
		// KubeVela's ResourceTracker retain Helm adoption metadata. Without
		// these, a subsequent helm install would fail with:
		//   "cannot be imported into the current release: invalid ownership metadata"
		if r.releaseName != "" {
			annotations["meta.helm.sh/release-name"] = r.releaseName
		}
		if r.releaseNamespace != "" {
			annotations["meta.helm.sh/release-namespace"] = r.releaseNamespace
		}
		obj.SetAnnotations(annotations)

		// Serialize back to YAML
		data, err := yaml.Marshal(obj.Object)
		if err != nil {
			return nil, errors.Wrap(err, "post-renderer: failed to marshal resource")
		}

		out.WriteString("---\n")
		out.Write(data)
	}

	return out, nil
}

// velaOwnerLabels returns KubeVela ownership labels suitable for embedding in Helm
// action Labels (which are written onto the Kubernetes release Secret). Returns nil
// when velaCtx is nil so callers can skip the label map safely.
func velaOwnerLabels(velaCtx *ContextParams) map[string]string {
	if velaCtx == nil {
		return nil
	}
	labels := map[string]string{
		"app.oam.dev/name":      velaCtx.AppName,
		"app.oam.dev/namespace": velaCtx.AppNamespace,
		"app.oam.dev/component": velaCtx.Name,
	}
	// Embed the publishVersion pin in the release labels so subsequent
	// reconciles can short-circuit when the App is at a stable pin and the
	// release was already installed at that pin.
	if velaCtx.PublishVersion != "" {
		labels["app.oam.dev/publishVersion"] = velaCtx.PublishVersion
	}
	return labels
}

// isOwnedByVela checks whether a Helm release was installed/managed by KubeVela
// by looking for the app.oam.dev/name label on the release's metadata (which is
// stored on the Kubernetes release Secret via install.Labels / upgrade.Labels).
// An external release (installed via `helm install` on the CLI) won't have this label.
func isOwnedByVela(rel *release.Release, velaCtx *ContextParams) bool {
	if rel == nil || velaCtx == nil {
		return false
	}
	if rel.Labels == nil {
		return false
	}
	return rel.Labels["app.oam.dev/name"] != ""
}
