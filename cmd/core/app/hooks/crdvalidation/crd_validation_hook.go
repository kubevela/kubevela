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

// Hook validates that CRDs installed in the cluster are compatible with
// enabled feature gates. This prevents silent data corruption by failing
// fast at startup if CRDs are out of date.
type Hook struct {
	client.Client
}

// NewHook creates a new CRD validation hook with the default singleton client
func NewHook() hooks.PreStartHook {
	klog.V(3).InfoS("Initializing CRD validation hook", "client", "singleton")
	return NewHookWithClient(singleton.KubeClient.Get())
}

// NewHookWithClient creates a new CRD validation hook with a specified client
// for improved testability and dependency injection
func NewHookWithClient(c client.Client) hooks.PreStartHook {
	klog.V(3).InfoS("Initializing CRD validation hook with custom client")
	return &Hook{Client: c}
}

// Name returns the hook name for logging
func (h *Hook) Name() string {
	return "CRDValidation"
}

// Run executes the CRD validation logic. It checks if compression-related
// feature gates are enabled and validates that the ApplicationRevision CRD
// supports the required compression fields.
func (h *Hook) Run(ctx context.Context) error {
	klog.InfoS("Starting CRD validation hook")

	// Add a reasonable timeout to prevent indefinite hanging while allowing
	// sufficient time for slower clusters or API servers under load.
	// 2 minutes should be more than enough for any reasonable cluster setup
	// while still protecting against indefinite hangs.
	timeout := 2 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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
		// Check if the error was due to context timeout
		if ctx.Err() == context.DeadlineExceeded {
			klog.ErrorS(err, "CRD validation timed out - API server may be slow or unresponsive",
				"timeout", timeout.String(),
				"suggestion", "Check API server health and network connectivity")
			return fmt.Errorf("CRD validation timed out after %v: %w. API server may be slow or under heavy load", timeout, err)
		}
		klog.ErrorS(err, "CRD validation failed")
		return fmt.Errorf("CRD validation failed: %w", err)
	}

	klog.InfoS("CRD validation completed successfully")
	return nil
}

// validateApplicationRevisionCRD performs a round-trip test to ensure the
// ApplicationRevision CRD supports compression fields
func (h *Hook) validateApplicationRevisionCRD(ctx context.Context, zstdEnabled, gzipEnabled bool) error {
	// Generate test resource
	testName := fmt.Sprintf("core.pre-check.%d", time.Now().UnixNano())
	namespace := k8s.GetRuntimeNamespace()

	klog.V(2).InfoS("Creating test ApplicationRevision for CRD validation",
		"name", testName,
		"namespace", namespace)

	// Ensure the namespace exists before attempting to create resources
	if err := k8s.EnsureNamespace(ctx, h.Client, namespace); err != nil {
		klog.ErrorS(err, "Failed to ensure runtime namespace exists",
			"namespace", namespace)
		return fmt.Errorf("runtime namespace %q does not exist or is not accessible: %w", namespace, err)
	}

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
			"namespace", namespace,
			"label", oam.LabelPreCheck)

		if err := h.Client.DeleteAllOf(ctx, &v1beta1.ApplicationRevision{},
			client.InNamespace(namespace),
			client.MatchingLabels{oam.LabelPreCheck: types.VelaCoreName}); err != nil {
			klog.ErrorS(err, "Failed to clean up test ApplicationRevision resources",
				"namespace", namespace)
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
		"actualName", appRev.Spec.Application.Name,
		"expectedCompression", compressionType,
		"actualCompression", appRev.Spec.Compression.Type)

	// First check that basic data survived
	if appRev.Spec.Application.Name != testName {
		klog.ErrorS(nil, "CRD round-trip validation failed - basic data corruption detected",
			"expectedName", testName,
			"actualName", appRev.Spec.Application.Name,
			"compressionType", compressionType,
			"issue", "The ApplicationRevision CRD does not support compression fields")
		return fmt.Errorf("the ApplicationRevision CRD is not updated. Compression cannot be used. Please upgrade your CRD to latest ones")
	}

	// Validate that compression fields survived the round-trip
	switch compressionType {
	case compression.Zstd:
		if appRev.Spec.Compression.Type != compression.Zstd {
			klog.ErrorS(nil, "CRD round-trip validation failed - zstd compression type lost",
				"expected", compression.Zstd,
				"actual", appRev.Spec.Compression.Type,
				"issue", "The ApplicationRevision CRD does not support zstd compression fields")
			return fmt.Errorf("ApplicationRevision CRD missing zstd compression support after round-trip; got=%v. Please upgrade your CRD to latest ones", appRev.Spec.Compression.Type)
		}
	case compression.Gzip:
		if appRev.Spec.Compression.Type != compression.Gzip {
			klog.ErrorS(nil, "CRD round-trip validation failed - gzip compression type lost",
				"expected", compression.Gzip,
				"actual", appRev.Spec.Compression.Type,
				"issue", "The ApplicationRevision CRD does not support gzip compression fields")
			return fmt.Errorf("ApplicationRevision CRD missing gzip compression support after round-trip; got=%v. Please upgrade your CRD to latest ones", appRev.Spec.Compression.Type)
		}
	case compression.Uncompressed:
		// This case should never happen as we only set Zstd or Gzip above,
		// but we need to handle it to satisfy the exhaustive linter
		klog.V(3).InfoS("Compression type is uncompressed, which is unexpected in validation",
			"compressionType", compressionType)
	}

	klog.V(2).InfoS("Round-trip validation passed - CRD supports compression",
		"compressionType", compressionType)

	return nil
}
