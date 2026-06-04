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

package v1beta1

import (
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestAddonAdmission covers BDD Scenario 6 of KEP-2.13: the Reconcile slice
// only supports pinned exact-tag versions and Protect-only deletion, so the
// CEL admission rules on AddonSpec must reject semver constraints and the
// Force/Orphan deletion policies at CREATE time.
func TestAddonAdmission(t *testing.T) {
	if k8sClient == nil {
		t.Skip("envtest unavailable (KUBEBUILDER_ASSETS unset); admission is also covered by the live-cluster smoke")
	}

	cases := []struct {
		name        string
		version     string
		deletion    AddonDeletionPolicy
		wantReject  bool
		errContains string
	}{
		{
			name:        "semver constraint is rejected",
			version:     ">=1.2.0",
			wantReject:  true,
			errContains: "semver constraints are not supported",
		},
		{
			name:    "exact tag is accepted",
			version: "v1.2.0",
		},
		{
			name:        "Force deletion policy is rejected",
			version:     "v1.0.0",
			deletion:    AddonDeletionPolicyForce,
			wantReject:  true,
			errContains: "Force and Orphan deletion policies are not supported",
		},
		{
			name:        "Orphan deletion policy is rejected",
			version:     "v1.0.0",
			deletion:    AddonDeletionPolicyOrphan,
			wantReject:  true,
			errContains: "Force and Orphan deletion policies are not supported",
		},
		{
			name:     "Protect deletion policy is accepted",
			version:  "v1.0.0",
			deletion: AddonDeletionPolicyProtect,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addon := &Addon{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "admission-test-"},
				Spec:       AddonSpec{Version: tc.version, DeletionPolicy: tc.deletion},
			}

			err := k8sClient.Create(testCtx, addon)

			if tc.wantReject {
				if err == nil {
					t.Fatalf("expected admission rejection, got none")
				}
				if !apierrors.IsInvalid(err) {
					t.Fatalf("expected an Invalid (admission) error, got %T: %v", err, err)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("error message %q does not contain %q", err.Error(), tc.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("expected the object to be accepted, got error: %v", err)
			}
			_ = k8sClient.Delete(testCtx, addon)
		})
	}
}

// TestAddonDeletionPolicyDefault confirms an omitted deletionPolicy is defaulted
// to Protect server-side (which is also why the CEL rule must allow the empty
// string: defaulting runs before validation).
func TestAddonDeletionPolicyDefault(t *testing.T) {
	if k8sClient == nil {
		t.Skip("envtest unavailable (KUBEBUILDER_ASSETS unset); defaulting is also covered by the live-cluster smoke")
	}

	addon := &Addon{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "admission-default-"},
		Spec:       AddonSpec{Version: "v1.0.0"},
	}
	if err := k8sClient.Create(testCtx, addon); err != nil {
		t.Fatalf("expected the object to be accepted, got error: %v", err)
	}
	defer func() { _ = k8sClient.Delete(testCtx, addon) }()

	if addon.Spec.DeletionPolicy != AddonDeletionPolicyProtect {
		t.Fatalf("expected deletionPolicy to default to %q, got %q", AddonDeletionPolicyProtect, addon.Spec.DeletionPolicy)
	}
}
