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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AddonPhase represents the current lifecycle phase of an Addon as observed
// by the controller. Reported on status.phase.
type AddonPhase string

const (
	// AddonPhaseInstalling indicates the addon has been created and the
	// controller is performing its initial install sequence (source fetch,
	// render, Application apply, auxiliary apply, definitions apply).
	AddonPhaseInstalling AddonPhase = "installing"
	// AddonPhaseUpgrading indicates spec.version differs from
	// status.installedVersion and the controller is running the upgrade path.
	AddonPhaseUpgrading AddonPhase = "upgrading"
	// AddonPhaseRunning is the terminal good state. All API lines are ready
	// and the Ready condition is True.
	AddonPhaseRunning AddonPhase = "running"
	// AddonPhaseFailed indicates a non-recoverable failure in one of the
	// reconcile tiers. Inspect status.conditions for the specific cause
	// (DefinitionConflict, ApplicationHealthy, AuxiliaryReady, ...).
	AddonPhaseFailed AddonPhase = "failed"
)

// AddonUpgradePolicy controls how the controller acts on a newly resolved
// version when spec.version is a semver constraint (tracking mode). Ignored
// when spec.version is an exact tag.
type AddonUpgradePolicy string

const (
	// AddonUpgradePolicyManual (the default) writes any newer resolved
	// version to status.availableUpgrade and sets an UpgradeAvailable
	// condition, but does not upgrade. The operator applies the upgrade
	// explicitly by updating spec.version. This preserves GitOps determinism.
	AddonUpgradePolicyManual AddonUpgradePolicy = "Manual"
	// AddonUpgradePolicyAuto upgrades immediately whenever the constraint
	// resolves to a newer version. spec.version is not mutated; the
	// installed version is visible only in status.installedVersion. Not
	// recommended for GitOps environments because upgrades do not appear in
	// git history.
	AddonUpgradePolicyAuto AddonUpgradePolicy = "Auto"
)

// AddonDeletionPolicy controls the finalizer behaviour when an Addon CR is
// deleted.
type AddonDeletionPolicy string

const (
	// AddonDeletionPolicyProtect (the default) blocks deletion of the Addon
	// CR while any Application on the cluster references a definition
	// installed by this addon. Once no referencing Applications remain, the
	// finalizer deletes the owned Application and ResourceTracker GC removes
	// every tracked resource.
	AddonDeletionPolicyProtect AddonDeletionPolicy = "Protect"
	// AddonDeletionPolicyForce releases the finalizer immediately regardless
	// of active references. The owned Application is deleted and
	// ResourceTracker GC removes every tracked resource. Existing
	// Applications that reference the addon's definitions will break.
	// Intended for cluster teardown or emergency cleanup.
	AddonDeletionPolicyForce AddonDeletionPolicy = "Force"
	// AddonDeletionPolicyOrphan releases the finalizer without deleting the
	// owned Application. All addon-installed resources remain on the cluster
	// as unmanaged resources. Use when decommissioning addon management
	// while keeping the capability running.
	AddonDeletionPolicyOrphan AddonDeletionPolicy = "Orphan"
)

// Addon is the declarative delivery unit for a versioned set of X-Definition
// APIs (KEP-2.13). Creating an Addon CR causes the controller to install the
// addon's resources, definitions, and auxiliary objects onto the cluster and
// to continuously reconcile their drift.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=addon
// +kubebuilder:subresource:status
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Addon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AddonSpec   `json:"spec,omitempty"`
	Status AddonStatus `json:"status,omitempty"`
}

// AddonSpec defines the desired state of an Addon. The field set tracks
// KEP-2.13 §API Changes (Extended Addon CR Spec).
type AddonSpec struct {
	// Version is an exact semver tag (e.g. "v1.2.0") or a semver constraint
	// (e.g. ">=1.2.0", "~2.1.0", "^1.0.0").
	//
	// Exact tag selects pinned mode: the controller installs exactly this
	// version and never changes it autonomously. This is the recommended
	// default for GitOps workflows.
	//
	// A semver constraint selects tracking mode: the controller resolves the
	// constraint against the registry on every periodic reconcile. The
	// upgrade behaviour is then governed by spec.upgradePolicy.
	Version string `json:"version,omitempty"`

	// UpgradePolicy controls autonomous upgrade behaviour when spec.version
	// is a semver constraint (tracking mode). Ignored when spec.version is an
	// exact tag. Defaults to Manual.
	UpgradePolicy AddonUpgradePolicy `json:"upgradePolicy,omitempty"`

	// Registry names the source registry from which to fetch the addon. Must
	// match a registry already registered with KubeVela.
	Registry string `json:"registry,omitempty"`

	// Parameters are input values injected into the addon's parameter.cue at
	// render time. They are equivalent to the old `--set` flags on the CLI.
	// The schema is intentionally free-form; the addon's own parameter.cue
	// constrains what is valid.
	// +kubebuilder:pruning:PreserveUnknownFields
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`

	// Clusters lists the cluster names to deploy the addon's Application to
	// in multi-cluster KubeVela setups. Translated at reconcile time into an
	// OAM topology policy on the owned Application.
	//
	// Only takes effect when the addon's parameter.cue declares a "clusters"
	// input. Omitting the field deploys to all registered clusters
	// (consistent with existing addon behaviour).
	Clusters []string `json:"clusters,omitempty"`

	// OverrideDefinitions allows the addon to overwrite definitions that
	// already exist on the cluster under a different owner. When false
	// (default), any such definition fails reconciliation immediately with a
	// DefinitionConflict condition listing each conflict and its current
	// owner.
	OverrideDefinitions bool `json:"overrideDefinitions,omitempty"`

	// SkipVersionCheck bypasses the minKubeVelaVersion compatibility check
	// declared in the addon's metadata. Use with caution; skipping the check
	// may result in installing an addon against an incompatible KubeVela
	// version.
	SkipVersionCheck bool `json:"skipVersionCheck,omitempty"`

	// DeletionPolicy controls what happens when the Addon CR is deleted.
	// Defaults to Protect.
	DeletionPolicy AddonDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// AddonStatus defines the observed state of an Addon. Story 1.1 ships only the
// minimum surface (phase, observedGeneration); the remaining status fields
// (conditions, installedResources, resolvedSourceDigest, modules) land in
// story 1.2.
type AddonStatus struct {
	// Phase is the high-level lifecycle state observed by the controller.
	// See AddonPhase for the allowed values.
	Phase AddonPhase `json:"phase,omitempty"`

	// ObservedGeneration is the metadata.generation that the controller has
	// most recently reconciled. Consumers compare this to the resource's
	// current metadata.generation to detect whether the controller has
	// caught up to the latest spec change.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// AddonList contains a list of Addon.
//
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Addon `json:"items"`
}
