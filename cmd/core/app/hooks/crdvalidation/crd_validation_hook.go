/*
Copyright 2022 The KubeVela Authors.

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

package crdvalidation

import (
	"context"
	"fmt"
	"time"

	"github.com/kubevela/pkg/util/compression"
	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/singleton"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/cmd/core/app/hooks"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// CRDValidation validates that CRDs installed in the cluster are compatible with
// enabled feature gates. This prevents silent data corruption by failing
// fast at startup if CRDs are out of date.
type CRDValidation struct {
	client.Client
}

// NewHook creates a new CRD validation hook
func NewHook() hooks.PreStartHook {
	klog.V(3).InfoS("Initializing CRD validation hook", "client", "singleton")
	return &CRDValidation{Client: singleton.KubeClient.Get()}
}

// Name returns the hook name for logging
func (h *CRDValidation) Name() string {
	return "CRDValidation"
}

// Run executes the CRD validation logic. It checks if compression-related
// feature gates are enabled and validates that the ApplicationRevision CRD
// supports the required compression fields.
func (h *CRDValidation) Run(ctx context.Context) error {
	klog.InfoS("Starting CRD validation hook")

	zstdEnabled := feature.DefaultMutableFeatureGate.Enabled(features.ZstdApplicationRevision)
	gzipEnabled := feature.DefaultMutableFeatureGate.Enabled(features.GzipApplicationRevision)

	klog.V(2).InfoS("Checking compression feature gates",
		"zstdEnabled", zstdEnabled,
		"gzipEnabled", gzipEnabled)

	if !zstdEnabled && !gzipEnabled {
		klog.InfoS("No compression features enabled, skipping CRD validation")
		return nil
	}

	klog.InfoS("Compression features enabled, validating ApplicationRevision CRD compatibility")

	if err := h.validateApplicationRevisionCRD(ctx, zstdEnabled, gzipEnabled); err != nil {
		klog.ErrorS(err, "CRD validation failed")
		return fmt.Errorf("CRD validation failed: %w", err)
	}

	klog.InfoS("CRD validation completed successfully")
	return nil
}

// validateApplicationRevisionCRD performs a round-trip test to ensure the
// ApplicationRevision CRD supports compression fields
func (h *CRDValidation) validateApplicationRevisionCRD(ctx context.Context, zstdEnabled, gzipEnabled bool) error {
	// Generate test resource
	testName := fmt.Sprintf("core.pre-check.%d", time.Now().UnixNano())
	namespace := k8s.GetRuntimeNamespace()

	klog.V(2).InfoS("Creating test ApplicationRevision for CRD validation",
		"name", testName,
		"namespace", namespace)

	appRev := &v1beta1.ApplicationRevision{}
	appRev.Name = testName
	appRev.Namespace = namespace
	appRev.SetLabels(map[string]string{oam.LabelPreCheck: types.VelaCoreName})
	appRev.Spec.Application.Name = testName
	appRev.Spec.Application.Spec.Components = []common.ApplicationComponent{}

	// Set compression type based on enabled features
	var compressionType compression.Type
	if zstdEnabled {
		compressionType = compression.Zstd
		appRev.Spec.Compression.SetType(compression.Zstd)
		klog.V(3).InfoS("Setting compression type", "type", "zstd")
	} else if gzipEnabled {
		compressionType = compression.Gzip
		appRev.Spec.Compression.SetType(compression.Gzip)
		klog.V(3).InfoS("Setting compression type", "type", "gzip")
	}

	// Register cleanup function
	defer func() {
		klog.V(2).InfoS("Cleaning up test ApplicationRevisions",
			"namespace", types.DefaultKubeVelaNS,
			"label", oam.LabelPreCheck)

		if err := h.Client.DeleteAllOf(ctx, &v1beta1.ApplicationRevision{},
			client.InNamespace(types.DefaultKubeVelaNS),
			client.MatchingLabels{oam.LabelPreCheck: types.VelaCoreName}); err != nil {
			klog.ErrorS(err, "Failed to clean up test ApplicationRevision resources",
				"namespace", types.DefaultKubeVelaNS)
		} else {
			klog.V(3).InfoS("Successfully cleaned up test ApplicationRevision resources")
		}
	}()

	// Create test resource
	klog.V(2).InfoS("Writing test ApplicationRevision to cluster")
	if err := h.Client.Create(ctx, appRev); err != nil {
		klog.ErrorS(err, "Failed to create test ApplicationRevision",
			"name", testName,
			"namespace", namespace)
		return fmt.Errorf("failed to create test ApplicationRevision: %w", err)
	}
	klog.V(3).InfoS("Test ApplicationRevision created successfully")

	// Read back the resource
	key := client.ObjectKeyFromObject(appRev)
	klog.V(2).InfoS("Reading back test ApplicationRevision from cluster",
		"key", key.String())

	if err := h.Client.Get(ctx, key, appRev); err != nil {
		klog.ErrorS(err, "Failed to read back test ApplicationRevision",
			"key", key.String())
		return fmt.Errorf("failed to read test ApplicationRevision: %w", err)
	}
	klog.V(3).InfoS("Test ApplicationRevision read successfully")

	// Validate round-trip integrity
	klog.V(2).InfoS("Validating round-trip data integrity",
		"expectedName", testName,
		"actualName", appRev.Spec.Application.Name)

	if appRev.Spec.Application.Name != testName {
		klog.ErrorS(nil, "CRD round-trip validation failed - data corruption detected",
			"expectedName", testName,
			"actualName", appRev.Spec.Application.Name,
			"compressionType", compressionType,
			"issue", "The ApplicationRevision CRD does not support compression fields")
		return fmt.Errorf("the ApplicationRevision CRD is not updated. Compression cannot be used. Please upgrade your CRD to latest ones")
	}

	klog.V(2).InfoS("Round-trip validation passed - CRD supports compression",
		"compressionType", compressionType)

	return nil
}
