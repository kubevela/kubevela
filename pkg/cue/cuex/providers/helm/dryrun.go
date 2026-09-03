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

// Client-only dry-run renderer used by webhook validation, plus the cluster Kubernetes-version lookup.

import (
	"fmt"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"k8s.io/client-go/kubernetes"
)

// dryRunRender performs a client-only Helm template render without touching the
// cluster. Used during webhook validation to verify the chart can be fetched,
// values are valid, and templates render without errors — without blocking on
// real resource creation, hooks, or waiting.
func (p *Provider) dryRunRender(ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams, velaCtx *ContextParams) (string, string, error) {
	install := action.NewInstall(&action.Configuration{})
	install.ReleaseName = releaseName
	install.Namespace = releaseNamespace
	install.DryRun = true
	install.ClientOnly = true

	// Set Kubernetes version capabilities so charts with kubeVersion constraints
	// don't fail against Helm's default v1.20.0. We query the real cluster version
	// via the REST config. If unreachable, the kubeVersion check is skipped —
	// the real install during reconciliation will validate it.
	if kv := p.getKubeVersion(); kv != nil {
		install.KubeVersion = kv
	}

	var postRender *PostRenderParams
	if options != nil {
		postRender = options.PostRender
		if options.SkipHooks != nil {
			install.DisableHooks = *options.SkipHooks
		}
	}
	install.PostRenderer = newPostRenderer(postRender, velaCtx, releaseName, releaseNamespace)

	rel, err := install.Run(ch, values)
	if err != nil {
		return "", "", errors.Wrapf(err, "dry-run render failed for chart %s", ch.Name())
	}

	return rel.Manifest, rel.Info.Notes, nil
}

// getKubeVersion queries the cluster's Kubernetes version for use in dry-run
// rendering. Returns nil if the cluster is unreachable — Helm will then skip
// the kubeVersion constraint check, deferring it to the real install.
func (p *Provider) getKubeVersion() *chartutil.KubeVersion {
	cfg, err := p.helmClient.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return nil
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil
	}
	info, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return nil
	}
	// Prefer the full GitVersion (e.g. "v1.31.4") so charts with patch-level
	// kubeVersion constraints validate correctly. Fall back to "vMAJOR.MINOR"
	// only if GitVersion is empty.
	version := info.GitVersion
	if version == "" {
		version = fmt.Sprintf("v%s.%s", info.Major, info.Minor)
	}
	return &chartutil.KubeVersion{
		Version: version,
		Major:   info.Major,
		Minor:   info.Minor,
	}
}
