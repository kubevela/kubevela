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
// +kubebuilder:printcolumn:name="PHASE",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="VERSION",type=string,JSONPath=`.status.installedVersion`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
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

// AddonStatus defines the observed state of an Addon. The fields track
// KEP-2.13 §API Changes (Addon CR Status). Writers (reconciler, source
// resolve, drift correction, finalizer) populate them; this type just
// declares the data shape.
type AddonStatus struct {
	// Phase is the high-level lifecycle state observed by the controller.
	// See AddonPhase for the allowed values.
	Phase AddonPhase `json:"phase,omitempty"`

	// ObservedGeneration is the metadata.generation that the controller has
	// most recently reconciled. Consumers compare this to the resource's
	// current metadata.generation to detect whether the controller has
	// caught up to the latest spec change.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastReconciledAt is the wall-clock timestamp of the most recent
	// reconcile. Useful for "is the controller stuck?" diagnostics.
	LastReconciledAt *metav1.Time `json:"lastReconciledAt,omitempty"`

	// InstalledVersion is the addon version actually running on the cluster.
	// Compared against spec.version to detect drift or upgrade triggers.
	InstalledVersion string `json:"installedVersion,omitempty"`

	// InstalledRegistry is the registry that provided the installed version.
	InstalledRegistry string `json:"installedRegistry,omitempty"`

	// AvailableUpgrade is populated in tracking mode with Manual upgrade
	// policy when the controller resolves a constraint to a newer version
	// than InstalledVersion. Cleared once the upgrade is applied.
	AvailableUpgrade string `json:"availableUpgrade,omitempty"`

	// ResolvedSourceDigest is the content-addressable identifier of the
	// addon source artifact resolved at the last install or upgrade. For
	// OCI sources this is the manifest digest (e.g. "sha256:abc..."); for
	// Git sources this is the full commit SHA. The controller compares this
	// value against the current remote digest on each periodic reconcile to
	// short-circuit re-fetch when the source has not changed.
	ResolvedSourceDigest string `json:"resolvedSourceDigest,omitempty"`

	// ApplicationName is the name of the owned Application in vela-system
	// (typically "addon-{name}"). Provided so tooling can navigate from the
	// Addon CR to its payload.
	ApplicationName string `json:"applicationName,omitempty"`

	// ApplicationHealthy is a quick boolean indicator of the owned
	// Application's health. Supplemented by the ApplicationHealthy condition
	// which carries transition time and reason.
	ApplicationHealthy bool `json:"applicationHealthy,omitempty"`

	// Conditions provides structured Kubernetes-native status reporting and
	// integrates with kubectl wait, GitOps health checks, and other
	// status.conditions tooling. See the AddonCondition* constants for the
	// type strings used.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// InstalledResources is the inventory of metadata resources the addon
	// installed at the end of the last reconcile. The staleness diff
	// compares the freshly rendered set against this inventory to identify
	// resources to clean up. Per-API-line auxiliary resources are tracked
	// separately on AddonModuleLineStatus.AuxiliaryResources.
	InstalledResources AddonInstalledResources `json:"installedResources,omitempty"`

	// Modules records per-module API line states (see KEP-2.20). Populated
	// only for addons that use the modules/ directory; empty for addons that
	// use only definitions/.
	Modules []AddonModuleStatus `json:"modules,omitempty"`
}

// AddonModuleStatus records the reconcile state of a single module within
// the addon. A module is the unit of API-line versioning under modules/ in
// the addon source tree (see KEP-2.20).
type AddonModuleStatus struct {
	// Name is the module directory name under modules/ (e.g. "aws-s3").
	Name string `json:"name"`

	// Lines reports the reconcile state per enabled API line in this
	// module. Each entry corresponds to a versioned subdirectory under
	// modules/<name>/ (e.g. v1, v2).
	Lines []AddonModuleLineStatus `json:"lines,omitempty"`
}

// AddonModuleLineStatus records the reconcile state of a single API line
// within a module. Defined in KEP-2.20; extended here with AuxiliaryResources
// to track the per-line auxiliary set applied this cycle.
type AddonModuleLineStatus struct {
	// APIVersion is the line identifier (e.g. "v1", "v2") taken from the
	// versioned subdirectory under modules/<name>/.
	APIVersion string `json:"apiVersion"`

	// Enabled reflects whether the line's _version.cue `enabled` expression
	// resolved to true against the current cluster context.
	Enabled bool `json:"enabled"`

	// Deprecated is true when the line is marked deprecated per the
	// KEP-2.20 deprecation lifecycle. Existing consumers keep working but
	// new Applications referencing this line's definitions are blocked by
	// admission.
	Deprecated bool `json:"deprecated"`

	// DeprecationReason carries a free-form explanation for the
	// deprecation, surfaced to operators inspecting the addon.
	DeprecationReason string `json:"deprecationReason,omitempty"`

	// AuxiliaryResources tracks the auxiliary/ resources applied for this
	// API line (e.g. Crossplane Compositions, KRO ResourceGraphDefinitions).
	// Used by the staleness diff scoped to this line.
	AuxiliaryResources []AddonResourceRef `json:"auxiliaryResources,omitempty"`

	// ResolvedSourceVersion records, for lines that reference an external
	// module via _version.cue `source`, which module version was pulled at
	// the last reconcile.
	ResolvedSourceVersion string `json:"resolvedSourceVersion,omitempty"`

	// Message carries any free-form information the reconciler wants to
	// surface for this line (e.g. why a line is currently disabled).
	Message string `json:"message,omitempty"`
}

// AddonInstalledResources is the addon-wide inventory of resources the
// controller applied at the last reconcile. Used by the staleness diff
// (see KEP-2.13 §Reconciliation Semantics).
type AddonInstalledResources struct {
	// Definitions lists every X-Definition (ComponentDefinition,
	// TraitDefinition, WorkflowStepDefinition, PolicyDefinition) the addon
	// installed.
	Definitions []AddonResourceRef `json:"definitions,omitempty"`

	// VelaQLViews lists every VelaQL View shipped by the addon.
	VelaQLViews []AddonResourceRef `json:"velaQLViews,omitempty"`

	// ConfigTemplates lists every ConfigTemplate shipped by the addon.
	ConfigTemplates []AddonResourceRef `json:"configTemplates,omitempty"`

	// Schemas lists every UI Schema ConfigMap shipped by the addon for
	// VelaUX form rendering.
	Schemas []AddonResourceRef `json:"schemas,omitempty"`

	// Packages tracks CUE package files installed from packages/. Populated
	// once the packages/ feature is implemented.
	Packages []AddonResourceRef `json:"packages,omitempty"`
}

// AddonInclude controls which addon asset categories are installed when the
// addon is consumed via the `addon` component type (KEP-2.13 Addon-of-Addons
// Composition). Every knob defaults to "include" semantically: a nil pointer
// means the category is installed; only an explicit false opts the category
// out. Pointer-to-bool is used so we can distinguish "unset" from "explicit
// false" without surprising defaults.
type AddonInclude struct {
	// Definitions controls installation of definitions/ content (Component,
	// Trait, WorkflowStep, Policy). Default: included.
	Definitions *bool `json:"definitions,omitempty"`

	// ConfigTemplates controls installation of config-templates/ resources.
	// Default: included.
	ConfigTemplates *bool `json:"configTemplates,omitempty"`

	// Views controls installation of views/ (VelaQL views).
	// Default: included.
	Views *bool `json:"views,omitempty"`

	// Schemas controls installation of UI schema files from schemas/
	// (ConfigMaps consumed by VelaUX to render parameter forms).
	// Default: included.
	Schemas *bool `json:"schemas,omitempty"`

	// Packages controls installation of CUE package files from packages/
	// (shared CUE libraries imported by definitions in this addon).
	// Future feature; reserved until packages/ is implemented.
	// Default: included.
	Packages *bool `json:"packages,omitempty"`

	// Resources controls installation of top-level resources/ (the owned
	// Application that installs addon infrastructure such as operators and
	// CRDs). Skipping this means no owned Application is created or
	// updated; the addon's definitions and auxiliary still install.
	// Default: included.
	Resources *bool `json:"resources,omitempty"`

	// Auxiliary controls installation of per-API-line auxiliary/ resources
	// (e.g. Crossplane Compositions, KRO ResourceGraphDefinitions). These
	// are applied after the owned Application is healthy. Skipping leaves
	// the addon with definitions and top-level resources but no per-line
	// compositions. Default: included.
	Auxiliary *bool `json:"auxiliary,omitempty"`
}

// AddonResourceRef identifies one resource within the addon inventory. Name
// and Kind are unique within an AddonInstalledResources category; the group
// and namespace are implied by the category bucket and the addon scope.
type AddonResourceRef struct {
	// Name of the resource as it appears on the cluster.
	Name string `json:"name"`

	// Kind of the resource (e.g. "ComponentDefinition", "View",
	// "ConfigTemplate", "ConfigMap").
	Kind string `json:"kind"`

	// Deprecated indicates the resource is marked deprecated per the
	// KEP-2.20 deprecation lifecycle. Consumers are still allowed to use
	// it, but new Applications referencing it are blocked by admission.
	Deprecated bool `json:"deprecated,omitempty"`

	// DeprecatedAt is the timestamp (RFC3339) at which Deprecated was set.
	// Free-form string rather than metav1.Time so it round-trips cleanly
	// when annotations carry it as a label value.
	DeprecatedAt string `json:"deprecatedAt,omitempty"`
}

// Standard condition types set on the Addon CR. Plain string constants per
// the metav1.Condition.Type contract; AddonConditionReady is the rollup
// answer that `kubectl wait --for=condition=Ready` checks.
//
// AuxiliaryReady, ModulesSynced, and DefinitionConflict are declared now
// for surface stability; the controller logic that writes them lands in
// follow-up epics under GWCP-100436.
const (
	// AddonConditionReady is True when all modules are synced and all
	// enabled API lines have their auxiliary resources ready and definitions
	// applied.
	AddonConditionReady = "Ready"
	// AddonConditionSourceResolved is True when the source artifact was
	// successfully fetched and its digest resolved. False indicates a
	// registry or network failure; the reconciler retries with exponential
	// backoff and does not apply stale outputs.
	AddonConditionSourceResolved = "SourceResolved"
	// AddonConditionApplicationHealthy is True when the owned Application
	// has reached Healthy status. False blocks auxiliary and definition
	// application (the tier-1 gate of the reconcile loop).
	AddonConditionApplicationHealthy = "ApplicationHealthy"
	// AddonConditionAuxiliaryReady is True when all enabled API line
	// auxiliary resources across all modules have reached Ready status.
	AddonConditionAuxiliaryReady = "AuxiliaryReady"
	// AddonConditionModulesSynced is True when all modules have been
	// evaluated and their definitions applied without error on the last
	// reconcile cycle.
	AddonConditionModulesSynced = "ModulesSynced"
	// AddonConditionDefinitionConflict is True when one or more definitions
	// this addon would install already exist on the cluster under a
	// different owner and spec.overrideDefinitions is false. The condition
	// message lists each conflicting definition name and its current owner.
	AddonConditionDefinitionConflict = "DefinitionConflict"
)

// AddonList contains a list of Addon.
//
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Addon `json:"items"`
}
