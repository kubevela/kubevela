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

	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/logging"
)

// defaultCompatChecker is the production compatibility check. It resolves the addon
// meta from the registry and validates its SystemRequirements. It returns nil to
// allow on any resolve or registry error (fail open) and a non-nil *field.Error only
// on a concrete compatibility mismatch. cli and restConfig come from the manager
// registering the webhook, not a process-wide singleton.
func defaultCompatChecker(ctx context.Context, cli client.Client, restConfig *rest.Config, addonName, version, registry string) *field.Error {
	logger := logging.WithContext(ctx).
		WithStep("validate-addon-compatibility").
		WithValues("addon", addonName, "version", version, "registry", registry)

	var registries []string
	if registry != "" {
		registries = []string{registry}
	}

	pkgs, err := pkgaddon.FindAddonPackagesDetailFromRegistry(ctx, cli, []string{addonName}, registries)
	if err != nil {
		logger.Info("Skipping addon compatibility validation", "reason", "registry-resolution-failed", "error", err)
		return nil
	}
	if len(pkgs) == 0 {
		logger.Info("Skipping addon compatibility validation", "reason", "addon-not-found")
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
			logger.Info("Skipping addon compatibility validation", "reason", "version-resolution-failed", "error", err)
			return nil
		}
		require = exact.SystemRequirements
	}
	if require == nil {
		return nil
	}

	var dc *discovery.DiscoveryClient
	if restConfig != nil {
		d, err := discovery.NewDiscoveryClientForConfig(restConfig)
		if err != nil {
			// Fail open on the kubernetes-version portion: without a discovery client
			// ValidateSystemRequirements still checks the vela versions.
			logger.Info("Skipping addon Kubernetes compatibility validation", "reason", "discovery-client-failed", "error", err)
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
			logger.Info("Skipping addon compatibility validation", "reason", "requirement-lookup-failed", "error", err)
			return nil
		}
		return field.Invalid(field.NewPath("spec", "components"), addonName,
			fmt.Sprintf("addon %q is incompatible with the current environment: %v", addonName, err))
	}
	return nil
}
