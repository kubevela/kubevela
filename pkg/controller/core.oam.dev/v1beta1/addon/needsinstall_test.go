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
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// controlledTracker returns a ResourceTracker whose controller owner-reference
// points at ad, i.e. one this addon instance owns.
func controlledTracker(ad *v1beta1.Addon) *v1beta1.ResourceTracker {
	controller := true
	return &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{
		OwnerReferences: []metav1.OwnerReference{{
			Name: ad.Name, UID: ad.UID, Controller: &controller,
		}},
	}}
}

func TestNeedsInstall(t *testing.T) {
	r := &Reconciler{}
	ad := &v1beta1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "fluxcd", UID: "addon-uid"},
		Spec:       v1beta1.AddonSpec{Version: "1.0.0"},
	}
	rt := controlledTracker(ad)

	// no tracker -> needs install (first install)
	assert.True(t, r.needsInstall(ad, nil))

	// tracker present and owned, versions equal -> heal path, no install
	ad.Status.InstalledVersion = "1.0.0"
	assert.False(t, r.needsInstall(ad, rt))

	// tracker present and owned, version changed -> install (upgrade)
	ad.Spec.Version = "1.1.0"
	assert.True(t, r.needsInstall(ad, rt))

	// tracker present and owned, spec.version set but installedVersion empty
	// (prior readBack failure) -> heal, not reinstall
	ad.Spec.Version = "1.0.0"
	ad.Status.InstalledVersion = ""
	assert.False(t, r.needsInstall(ad, rt))

	// tracker NOT controlled by this addon instance (stale, from a previous
	// addon of the same name mid-GC) -> needs install, do not heal from it
	ad.Status.InstalledVersion = "1.0.0"
	stale := &v1beta1.ResourceTracker{}
	assert.True(t, r.needsInstall(ad, stale))
}
