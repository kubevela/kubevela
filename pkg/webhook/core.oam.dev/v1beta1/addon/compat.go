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

package addon

import (
	"context"
	"errors"
	"fmt"

	"github.com/kubevela/pkg/util/singleton"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/discovery"
	"k8s.io/klog/v2"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
)

// defaultCompatChecker is the production compatibility check. It resolves the addon
// meta from the registry and validates its SystemRequirements. It returns nil to
// allow on any resolve or registry error (fail open) and a non-nil *field.Error only
// on a concrete compatibility mismatch.
func (h *ValidatingHandler) defaultCompatChecker(ctx context.Context, addonName, version, registry string) *field.Error {
	var registries []string
	if registry != "" {
		registries = []string{registry}
	}

	cli := singleton.KubeClient.Get()

	pkgs, err := pkgaddon.FindAddonPackagesDetailFromRegistry(ctx, cli, []string{addonName}, registries)
	if err != nil {
		klog.Infof("skip addon %q compatibility check (fail-open): resolve from registry failed: %v", addonName, err)
		return nil
	}
	if len(pkgs) == 0 {
		klog.Infof("skip addon %q compatibility check (fail-open): addon not found in registries %v", addonName, registries)
		return nil
	}

	// FindAddonPackagesDetailFromRegistry returns the latest version. When the
	// component pins a specific version, validate that version's requirements
	// instead of the latest, so a valid pin is not falsely rejected, and an
	// incompatible pin is rejected at admission rather than only at render time.
	require := pkgs[0].InstallPackage.SystemRequirements
	if version != "" && version != pkgs[0].InstallPackage.Version {
		exact, err := pkgaddon.GetAddonInstallPackageFromRegistry(ctx, cli, pkgs[0].RegistryName, addonName, version)
		if err != nil {
			klog.Infof("skip addon %q version %q compatibility check (fail-open): resolve version failed: %v", addonName, version, err)
			return nil
		}
		require = exact.SystemRequirements
	}
	if require == nil {
		return nil
	}

	var dc *discovery.DiscoveryClient
	if cfg := singleton.KubeConfig.Get(); cfg != nil {
		d, err := discovery.NewDiscoveryClientForConfig(cfg)
		if err != nil {
			// Fail open on the kubernetes-version portion: without a discovery client
			// ValidateSystemRequirements still checks the vela versions.
			klog.Infof("addon %q kubernetes version check skipped (fail-open): build discovery client failed: %v", addonName, err)
		} else {
			dc = d
		}
	}

	if err := pkgaddon.ValidateSystemRequirements(ctx, require, cli, dc); err != nil {
		// ValidateSystemRequirements reports two very different things through one
		// error: a requirement that was evaluated and not met, and a lookup that
		// could not be performed at all (reading the vela-core image tag, querying
		// the discovery API, or parsing a malformed constraint). Only the former is
		// a reason to deny; denying on the latter would let an API blip block
		// applies, and this webhook runs with failurePolicy: Fail.
		if !errors.Is(err, pkgaddon.ErrVersionMismatch) {
			klog.Infof("skip addon %q compatibility check (fail-open): requirement lookup failed: %v", addonName, err)
			return nil
		}
		return field.Invalid(field.NewPath("spec", "components"), addonName,
			fmt.Sprintf("addon %q is incompatible with the current environment: %v", addonName, err))
	}
	return nil
}
