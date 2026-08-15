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

// Helm post-renderer that injects KubeVela ownership labels and Helm release annotations on every rendered resource.

import (
	"bytes"
	"context"
	stderrors "errors"
	"io"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/kubevela/pkg/cue/util"
	"github.com/kubevela/workflow/pkg/cue/model/sets"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
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
// rendered manifests, injects KubeVela ownership labels/annotations, defaults
// the namespace for namespaced kinds that omitted it, and returns the modified
// manifests. The namespace defaulting runs even when the KubeVela ownership
// context is absent, since direct callers (tests, CLI tooling) can still
// supply a release namespace and benefit from the defaulting fix.
func (r *velaLabelPostRenderer) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	if r.context == nil && r.releaseNamespace == "" && r.releaseName == "" {
		// Nothing to inject and no namespace to default — return unchanged.
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

		// Inject KubeVela ownership labels when context is available.
		if r.context != nil {
			labels := obj.GetLabels()
			if labels == nil {
				labels = make(map[string]string)
			}
			labels["app.oam.dev/name"] = r.context.AppName
			labels["app.oam.dev/namespace"] = r.context.AppNamespace
			labels["app.oam.dev/component"] = r.context.Name
			obj.SetLabels(labels)
		}

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

		// Inject ownership annotations (both KubeVela and Helm). Helm ownership
		// annotations are stamped regardless of velaCtx so that resources
		// re-applied from KubeVela's ResourceTracker retain Helm adoption
		// metadata. Without these, a subsequent helm install would fail with:
		//   "cannot be imported into the current release: invalid ownership metadata"
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		if r.context != nil {
			annotations["app.oam.dev/owner"] = "helm-provider"
		}
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

// helmPostRenderer is satisfied by any post-renderer in this package. It matches
// helm.sh/helm/v3/pkg/postrender.PostRenderer structurally, so values can be
// assigned to action.Install.PostRenderer without importing that package here.
type helmPostRenderer interface {
	Run(*bytes.Buffer) (*bytes.Buffer, error)
}

// cuePatchFieldName is the field a post-render template must produce. It mirrors
// the trait authoring model, where a trait patches its workload via `patch: {...}`.
const cuePatchFieldName = "patch"

// cuePostRenderer applies a user-supplied CUE template to every rendered
// resource. The template is evaluated once per resource with that resource bound
// to context.resource, and its patch field is merged back into the resource.
//
// The context.Context is carried on the struct because helm's PostRenderer
// interface has no ctx parameter, and CUE evaluation needs one to stay
// cancellable alongside the surrounding install/upgrade.
type cuePostRenderer struct {
	ctx     context.Context
	params  *CUEParams
	velaCtx *ContextParams
}

// Run implements helmPostRenderer. Resources whose template produces no patch
// field pass through untouched, which is how a template scopes itself to a
// subset of kinds (a bare `if context.resource.kind == "Deployment"` guard
// leaves patch undefined for everything else).
func (r *cuePostRenderer) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	if r.params == nil || strings.TrimSpace(r.params.Template) == "" || renderedManifests.Len() == 0 {
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
			return nil, errors.Wrap(err, "cue post-renderer: failed to decode manifest")
		}
		if len(obj.Object) == 0 {
			continue
		}

		patched, err := r.patchResource(obj.Object)
		if err != nil {
			return nil, err
		}

		data, err := yaml.Marshal(patched)
		if err != nil {
			return nil, errors.Wrapf(err, "cue post-renderer: failed to marshal %s/%s", obj.GetKind(), obj.GetName())
		}
		out.WriteString("---\n")
		out.Write(data)
	}

	return out, nil
}

// patchResource evaluates the user template against a single resource and
// returns the merged result, or the resource unchanged when the template
// produces no patch for it.
func (r *cuePostRenderer) patchResource(resource map[string]interface{}) (map[string]interface{}, error) {
	kind, _ := resource["kind"].(string)

	if err := r.ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "cue post-renderer: cancelled")
	}

	// A post-render template is evaluated as plain CUE rather than through a
	// cuex compiler. It is a pure data transform over an already-rendered
	// resource, so it needs no vela provider packages — and reaching for one
	// would both re-enter the helm provider that is mid-render and drag in the
	// kube client singleton, which hard-exits the process when no kubeconfig is
	// present. The full CUE standard library (strings, list, regexp, ...)
	// remains available.
	cctx := cuecontext.New()
	ctxData, err := util.ToString(cctx.CompileString("").FillPath(cue.ParsePath("context"), r.templateContext(resource)))
	if err != nil {
		return nil, errors.Wrapf(err, "cue post-renderer: failed to bind context for %s", kind)
	}

	val := cctx.CompileString(strings.Join([]string{r.params.Template, ctxData}, "\n"))
	if err := val.Err(); err != nil {
		return nil, errors.Wrapf(err, "cue post-renderer: invalid template for %s", kind)
	}

	patcher := val.LookupPath(cue.ParsePath(cuePatchFieldName))
	if !patcher.Exists() {
		return resource, nil
	}
	if err := patcher.Err(); err != nil {
		return nil, errors.Wrapf(err, "cue post-renderer: invalid patch for %s", kind)
	}

	base := cctx.Encode(resource)
	if err := base.Err(); err != nil {
		return nil, errors.Wrapf(err, "cue post-renderer: failed to encode %s", kind)
	}

	merged, err := sets.StrategyUnify(base, patcher, sets.CreateUnifyOptionsForPatcher(patcher)...)
	if err != nil {
		return nil, errors.Wrapf(err, "cue post-renderer: failed to merge patch into %s", kind)
	}

	data, err := merged.MarshalJSON()
	if err != nil {
		return nil, errors.Wrapf(err, "cue post-renderer: patched %s is incomplete", kind)
	}
	// Decode with the apimachinery decoder rather than encoding/json: it keeps
	// integral values as int64 instead of widening them to float64, so fields
	// like spec.replicas survive the round-trip as integers.
	patched := map[string]interface{}{}
	if err := json.Unmarshal(data, &patched); err != nil {
		return nil, errors.Wrapf(err, "cue post-renderer: failed to decode patched %s", kind)
	}
	return patched, nil
}

// templateContext builds the `context` struct visible to the user template. It
// carries the resource under evaluation plus the KubeVela identifiers already
// available elsewhere in definition templates, so a post-render template can
// stamp application-derived values without threading them through values.
func (r *cuePostRenderer) templateContext(resource map[string]interface{}) map[string]interface{} {
	ctxData := map[string]interface{}{"resource": resource}
	if r.velaCtx != nil {
		ctxData["appName"] = r.velaCtx.AppName
		ctxData["appNamespace"] = r.velaCtx.AppNamespace
		ctxData["name"] = r.velaCtx.Name
		ctxData["namespace"] = r.velaCtx.Namespace
	}
	return ctxData
}

// compositePostRenderer chains post-renderers in order, feeding each renderer's
// output as the next one's input.
type compositePostRenderer struct {
	renderers []helmPostRenderer
}

// Run implements helmPostRenderer.
func (r *compositePostRenderer) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	buf := renderedManifests
	for _, rdr := range r.renderers {
		var err error
		if buf, err = rdr.Run(buf); err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// newPostRenderer builds the post-render chain for a release. User-supplied
// transformations run first and velaLabelPostRenderer runs last, so ownership
// labels and Helm adoption annotations are stamped on the final resource set and
// cannot be patched away by a user template. When no post-rendering is
// configured the bare vela renderer is returned unchanged.
func newPostRenderer(ctx context.Context, postRender *PostRenderParams, velaCtx *ContextParams, releaseName, releaseNamespace string) helmPostRenderer {
	velaRenderer := &velaLabelPostRenderer{
		context:          velaCtx,
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}
	if postRender == nil || postRender.CUE == nil {
		return velaRenderer
	}
	return &compositePostRenderer{
		renderers: []helmPostRenderer{
			&cuePostRenderer{ctx: ctx, params: postRender.CUE, velaCtx: velaCtx},
			velaRenderer,
		},
	}
}

// isOwnedByVela checks whether a Helm release was installed/managed by THIS
// KubeVela Application + component triple by comparing the release Secret's
// stored ownership labels against the current render context. Matching only
// on a non-empty app.oam.dev/name would let one Application's release pass
// as already owned when another Application's component happens to target
// the same release name in the same namespace; that path skips adoption and
// can corrupt cross-tenant ownership tracking. An external release (vanilla
// `helm install` on the CLI) carries no ownership labels at all and falls
// through to the adoption code path.
func isOwnedByVela(rel *release.Release, velaCtx *ContextParams) bool {
	if rel == nil || velaCtx == nil {
		return false
	}
	if rel.Labels == nil {
		return false
	}
	return rel.Labels["app.oam.dev/name"] == velaCtx.AppName &&
		rel.Labels["app.oam.dev/namespace"] == velaCtx.AppNamespace &&
		rel.Labels["app.oam.dev/component"] == velaCtx.Name
}
