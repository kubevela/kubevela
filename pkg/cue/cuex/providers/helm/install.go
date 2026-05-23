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
	"strings"
	"time"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/klog/v2"
)

// freshInstall performs a helm install with retry logic for orphaned/corrupted state.
func (p *Provider) freshInstall(ctx context.Context, actionConfig *action.Configuration, ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams, postRenderer *velaLabelPostRenderer, releaseLabels map[string]string, velaCtx *ContextParams) (*release.Release, error) {
	install := p.newInstallAction(actionConfig, releaseName, releaseNamespace, options, postRenderer, releaseLabels)

	klog.Infof("Helm provider [%s]: Installing release %s in namespace %s", velaContextStr(velaCtx), releaseName, releaseNamespace)
	rel, err := install.RunWithContext(ctx, ch, values)
	if err == nil {
		return rel, nil
	}

	// If install fails due to corrupted/orphaned release secrets or ownership
	// conflicts, clean up the broken state and retry once.
	if !isRetryableInstallError(err) {
		return nil, errors.Wrapf(err, "failed to install helm release %s", releaseName)
	}

	klog.Warningf("Helm provider [%s]: Install failed for %s due to orphaned state (%v), cleaning up and retrying", velaContextStr(velaCtx), releaseName, err)
	if cleanErr := p.cleanOrphanedReleaseSecrets(actionConfig, releaseName, releaseNamespace, velaCtx); cleanErr != nil {
		klog.Warningf("Helm provider [%s]: Failed to clean orphaned secrets for %s: %v", velaContextStr(velaCtx), releaseName, cleanErr)
		return nil, errors.Wrapf(err, "failed to install helm release %s (cleanup also failed: %v)", releaseName, cleanErr)
	}

	retry := p.newInstallAction(actionConfig, releaseName, releaseNamespace, options, postRenderer, releaseLabels)
	klog.Infof("Helm provider [%s]: Retrying install for release %s after cleanup", velaContextStr(velaCtx), releaseName)
	rel, err = retry.RunWithContext(ctx, ch, values)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to install helm release %s after cleanup retry", releaseName)
	}
	return rel, nil
}

// newInstallAction creates a configured helm install action.
func (p *Provider) newInstallAction(actionConfig *action.Configuration, releaseName, releaseNamespace string, options *RenderOptionsParams, postRenderer *velaLabelPostRenderer, releaseLabels map[string]string) *action.Install {
	install := action.NewInstall(actionConfig)
	install.ReleaseName = releaseName
	install.Namespace = releaseNamespace
	install.DryRun = false
	install.ClientOnly = false
	install.PostRenderer = postRenderer
	install.CreateNamespace = true
	install.Labels = releaseLabels
	applyCommonInstallOptions(install, options)
	return install
}

// applyCommonInstallOptions wires user-facing options onto action.Install.
// Helm SDK v3.14.4 does not expose MaxHistory, CleanupOnFail, or Recreate on
// action.Install; those take effect only on upgrade. The asymmetry between
// this applier and applyCommonUpgradeOptions in upgrade.go is the SDK's by
// design: a fresh install has no prior state to clean, no history to bound,
// and no pods to recreate.
func applyCommonInstallOptions(install *action.Install, opts *RenderOptionsParams) {
	if opts == nil {
		return
	}
	if opts.IncludeCRDs != nil {
		install.SkipCRDs = !*opts.IncludeCRDs
	}
	if opts.Force {
		install.Force = true
	}
	if opts.Atomic {
		install.Atomic = true
	}
	if opts.Wait || opts.Atomic {
		install.Wait = true
	}
	if opts.Timeout != "" {
		if d, err := time.ParseDuration(opts.Timeout); err == nil {
			install.Timeout = d
		}
	}
	if opts.CreateNamespace != nil {
		install.CreateNamespace = *opts.CreateNamespace
	}
	if opts.SkipHooks != nil {
		install.DisableHooks = *opts.SkipHooks
	}
}

// isRetryableInstallError returns true if the error indicates orphaned state
// that can be fixed by cleaning up and retrying.
func isRetryableInstallError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "cannot be imported") ||
		strings.Contains(msg, "invalid ownership metadata") ||
		strings.Contains(msg, "no revision for release") ||
		strings.Contains(msg, "release: already exists")
}
