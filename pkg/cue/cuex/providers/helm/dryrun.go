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
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"k8s.io/client-go/discovery"
)

// kubeVersionTimeout bounds the one request a dry-run makes, for the target cluster's version, so
// an unreachable cluster cannot hold an admission request open.
const kubeVersionTimeout = 5 * time.Second

// dryRunRender performs a client-only Helm template render that installs
// nothing. Used during webhook validation and vela dry-run to verify the chart
// can be fetched, values are valid, and templates render without errors,
// without blocking on real resource creation, hooks, or waiting. Its only
// request is for the target cluster's Kubernetes version.
func (p *Provider) dryRunRender(ctx context.Context, ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams, velaCtx *ContextParams) (string, string, error) {
	install := action.NewInstall(&action.Configuration{})
	install.ReleaseName = releaseName
	install.Namespace = releaseNamespace
	install.DryRun = true
	install.ClientOnly = true

	// Charts and templates see the target cluster's Kubernetes version, or the
	// version this build of client-go targets when it cannot be read; the real
	// install during reconciliation reads the cluster's own.
	if kv := p.getKubeVersion(ctx); kv != nil {
		install.KubeVersion = kv
	} else if kv := fallbackKubeVersion(); kv != nil {
		install.KubeVersion = kv
	}

	install.PostRenderer = &velaLabelPostRenderer{
		context:          velaCtx,
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}

	if options != nil {
		if options.SkipHooks != nil {
			install.DisableHooks = *options.SkipHooks
		}
	}

	rel, err := install.Run(ch, values)
	if err != nil {
		return "", "", errors.Wrapf(err, "dry-run render failed for chart %s", ch.Name())
	}

	return rel.Manifest, rel.Info.Notes, nil
}

// getKubeVersion queries the Kubernetes version of the cluster being rendered
// for, for use in dry-run rendering. Returns nil if it cannot be read.
func (p *Provider) getKubeVersion(ctx context.Context) *chartutil.KubeVersion {
	getter, err := p.restClientGetter(ctx)
	if err != nil {
		return nil
	}
	cfg, err := getter.ToRESTConfig()
	if err != nil {
		return nil
	}
	cfg.Timeout = kubeVersionTimeout
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil
	}
	info, err := dc.ServerVersion()
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

// fallbackKubeVersion is the Kubernetes version this build's client-go targets
// (client-go v0.X.Y ships with Kubernetes v1.X.Y), or nil when the build
// records no module versions.
var fallbackKubeVersion = sync.OnceValue(func() *chartutil.KubeVersion {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	for _, dep := range info.Deps {
		if dep.Path != "k8s.io/client-go" {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(dep.Version, "v0."), ".", 2)
		if len(parts) != 2 || !strings.HasPrefix(dep.Version, "v0.") {
			return nil
		}
		patch, _, _ := strings.Cut(parts[1], "-")
		return &chartutil.KubeVersion{Version: fmt.Sprintf("v1.%s.%s", parts[0], patch), Major: "1", Minor: parts[0]}
	}
	return nil
})
