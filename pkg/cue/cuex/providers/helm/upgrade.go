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
	"context"
	"time"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/klog/v2"
)

// performUpgrade runs `helm upgrade` against an existing release with the
// resolved chart and merged values. The caller (installOrUpgradeChart) has
// already decided that an upgrade is warranted (fingerprint differs, the
// release needs adoption, or the release is not in a clean deployed state).
func (p *Provider) performUpgrade(ctx context.Context, actionConfig *action.Configuration, ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams, postRenderer *velaLabelPostRenderer, releaseLabels map[string]string, velaCtx *ContextParams) (*release.Release, error) {
	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Namespace = releaseNamespace
	upgrade.PostRenderer = postRenderer
	upgrade.Labels = releaseLabels
	applyCommonUpgradeOptions(upgrade, options)

	klog.Infof("Helm provider [%s]: Upgrading release %s in namespace %s", velaContextStr(velaCtx), releaseName, releaseNamespace)
	rel, err := upgrade.RunWithContext(ctx, releaseName, ch, values)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to upgrade helm release %s", releaseName)
	}
	klog.Infof("Helm provider [%s]: Successfully upgraded release %s", velaContextStr(velaCtx), releaseName)
	return rel, nil
}

// applyCommonUpgradeOptions wires user-facing options onto action.Upgrade.
// MaxHistory, CleanupOnFail, and Recreate are upgrade-only fields per the
// Helm SDK shape and so live here rather than in applyCommonInstallOptions.
func applyCommonUpgradeOptions(upgrade *action.Upgrade, opts *RenderOptionsParams) {
	if opts == nil {
		return
	}
	if opts.Atomic {
		upgrade.Atomic = true
	}
	if opts.Wait || opts.Atomic {
		upgrade.Wait = true
	}
	if opts.Timeout != "" {
		if d, err := time.ParseDuration(opts.Timeout); err == nil {
			upgrade.Timeout = d
		}
	}
	if opts.Force {
		upgrade.Force = true
	}
	if opts.CleanupOnFail {
		upgrade.CleanupOnFail = true
	}
	if opts.RecreatePods {
		upgrade.Recreate = true
	}
	if opts.MaxHistory > 0 {
		upgrade.MaxHistory = opts.MaxHistory
	}
	if opts.SkipHooks != nil {
		upgrade.DisableHooks = *opts.SkipHooks
	}
}
