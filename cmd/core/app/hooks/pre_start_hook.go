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

package hooks

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
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// PreStartHook hook that should be run before controller start working
type PreStartHook interface {
	Run(ctx context.Context) error
}

// SystemCRDValidationHook checks if the crd in the system are valid to run the current controller
type SystemCRDValidationHook struct {
	client.Client
}

// NewSystemCRDValidationHook .
func NewSystemCRDValidationHook() PreStartHook {
	return &SystemCRDValidationHook{Client: singleton.KubeClient.Get()}
}

// Run .
func (in *SystemCRDValidationHook) Run(ctx context.Context) error {
	if feature.DefaultMutableFeatureGate.Enabled(features.ZstdApplicationRevision) ||
		feature.DefaultMutableFeatureGate.Enabled(features.GzipApplicationRevision) {
		appRev := &v1beta1.ApplicationRevision{}
		appRev.Name = fmt.Sprintf("core.pre-check.%d", time.Now().UnixNano())
		appRev.Namespace = k8s.GetRuntimeNamespace()
		key := client.ObjectKeyFromObject(appRev)
		appRev.SetLabels(map[string]string{oam.LabelPreCheck: types.VelaCoreName})
		appRev.Spec.Application.Name = appRev.Name
		appRev.Spec.Application.Spec.Components = []common.ApplicationComponent{}
		if feature.DefaultMutableFeatureGate.Enabled(features.ZstdApplicationRevision) {
			appRev.Spec.Compression.SetType(compression.Zstd)
		} else if feature.DefaultMutableFeatureGate.Enabled(features.GzipApplicationRevision) {
			appRev.Spec.Compression.SetType(compression.Gzip)
		}
		if err := in.Client.Create(ctx, appRev); err != nil {
			return err
		}
		defer func() {
			if err := in.Client.DeleteAllOf(ctx, &v1beta1.ApplicationRevision{},
				client.InNamespace(types.DefaultKubeVelaNS),
				client.MatchingLabels{oam.LabelPreCheck: types.VelaCoreName}); err != nil {
				klog.Errorf("failed to recycle pre-check ApplicationRevision: %w", err)
			}
		}()
		if err := in.Client.Get(ctx, key, appRev); err != nil {
			return err
		}
		if appRev.Spec.Application.Name != appRev.Name {
			return fmt.Errorf("the ApplicationRevision CRD is not updated. Compression cannot be used. Please upgrade your CRD to latest ones")
		}
	}
	return nil
}
