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

package common

import (
	"strings"

	"github.com/kubevela/pkg/util/compression"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// init wires the library for every KubeVela binary: they all import this package for the
// Scheme. ConfigureLibrary is exported so a binary outside this repo can wire it itself.
func init() {
	ConfigureLibrary()
}

// ConfigureLibrary gives the apply / ResourceTracker / GC library its switches and scheme;
// it takes them as options rather than reading KubeVela's feature gates itself. The switches
// read the gates on each call, so flags parsed after start-up, and gates set in tests, apply.
func ConfigureLibrary() {
	apply.Configure(apply.Config{
		ReplaceOnUpdate:       gate(features.ApplyResourceByReplace),
		LegacyOwnerValidation: gate(features.LegacyResourceOwnerValidation),
		Scheme:                Scheme,
		// Sharer keys written without a kind (<namespace>/<name>, <name>) are Applications':
		// shared-by annotations have always been written that way.
		DefaultOwnerKind: v1beta1.ApplicationKind,
		// Migration: owner labels. A resource carrying app.oam.dev/* only is still its
		// Application's, until that Application re-applies it.
		LegacyControlledBy: applicationFromAppLabels,
	})
	resourcetracker.Configure(resourcetracker.Config{
		// zstd wins when both gates are on.
		Compression: func() compression.Type {
			switch {
			case utilfeature.DefaultMutableFeatureGate.Enabled(features.ZstdResourceTracker):
				return compression.Zstd
			case utilfeature.DefaultMutableFeatureGate.Enabled(features.GzipResourceTracker):
				return compression.Gzip
			default:
				return ""
			}
		},
		OnList: func(kind string) {
			// Lower case, so an Application still counts under "application" as it always has.
			metrics.ListResourceTrackerCounter.WithLabelValues(strings.ToLower(kind)).Inc()
		},
		// An Application labels its resources app.oam.dev/* as well as owner.oam.dev/*.
		KindLabels: applicationKindLabels,
	})
}

func applicationKindLabels(kind, namespace, name string) map[string]string {
	if kind != v1beta1.ApplicationKind {
		return nil
	}
	return map[string]string{oam.LabelAppName: name, oam.LabelAppNamespace: namespace}
}

func applicationFromAppLabels(obj client.Object) string {
	name, namespace := obj.GetLabels()[oam.LabelAppName], obj.GetLabels()[oam.LabelAppNamespace]
	if name == "" || namespace == "" {
		return ""
	}
	return v1beta1.ApplicationKind + "/" + namespace + "/" + name
}

// gate reads a feature gate when called.
func gate(f featuregate.Feature) func() bool {
	return func() bool { return utilfeature.DefaultMutableFeatureGate.Enabled(f) }
}
