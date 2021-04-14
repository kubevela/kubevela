/*
Copyright 2021 The KubeVela Authors.

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

package apis

// APIs copied from fluxcd/helm-controller/api/v2beta1 @ api/v0.7.0

/*
   Copyright 2021 The Flux CD contributors.
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

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HelmReleaseSpec defines the desired state of a Helm release.
type HelmReleaseSpec struct {
	// Chart defines the template of the v1beta1.HelmChart that should be created
	// for this HelmRelease.
	// +required
	Chart HelmChartTemplate `json:"chart"`

	// Interval at which to reconcile the Helm release.
	// make it optional in KubeVela
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`

	// KubeConfig for reconciling the HelmRelease on a remote cluster.
	// When specified, KubeConfig takes precedence over ServiceAccountName.
	// +optional
	// KubeConfig *KubeConfig `json:"kubeConfig,omitempty"`

	// Suspend tells the controller to suspend reconciliation for this HelmRelease,
	// it does not apply to already started reconciliations. Defaults to false.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// ReleaseName used for the Helm release. Defaults to a composition of
	// '[TargetNamespace-]Name'.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=53
	// +kubebuilder:validation:Optional
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// TargetNamespace to target when performing operations for the HelmRelease.
	// Defaults to the namespace of the HelmRelease.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Optional
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// StorageNamespace used for the Helm storage.
	// Defaults to the namespace of the HelmRelease.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Optional
	// +optional
	StorageNamespace string `json:"storageNamespace,omitempty"`

	// DependsOn may contain a dependency.CrossNamespaceDependencyReference slice with
	// references to HelmRelease resources that must be ready before this HelmRelease
	// can be reconciled.
	// +optional
	// DependsOn []dependency.CrossNamespaceDependencyReference `json:"dependsOn,omitempty"`

	// Timeout is the time to wait for any individual Kubernetes operation (like Jobs
	// for hooks) during the performance of a Helm action. Defaults to '5m0s'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// MaxHistory is the number of revisions saved by Helm for this HelmRelease.
	// Use '0' for an unlimited number of revisions; defaults to '10'.
	// +optional
	MaxHistory *int `json:"maxHistory,omitempty"`

	// The name of the Kubernetes service account to impersonate
	// when reconciling this HelmRelease.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Install holds the configuration for Helm install actions for this HelmRelease.
	// +optional
	Install *Install `json:"install,omitempty"`

	// Upgrade holds the configuration for Helm upgrade actions for this HelmRelease.
	// +optional
	Upgrade *Upgrade `json:"upgrade,omitempty"`

	// Test holds the configuration for Helm test actions for this HelmRelease.
	// +optional
	Test *Test `json:"test,omitempty"`

	// Rollback holds the configuration for Helm rollback actions for this HelmRelease.
	// +optional
	Rollback *Rollback `json:"rollback,omitempty"`

	// Uninstall holds the configuration for Helm uninstall actions for this HelmRelease.
	// +optional
	Uninstall *Uninstall `json:"uninstall,omitempty"`

	// ValuesFrom holds references to resources containing Helm values for this HelmRelease,
	// and information about how they should be merged.
	// ValuesFrom []ValuesReference `json:"valuesFrom,omitempty"`

	// Values holds the values for this Helm release.
	// +optional
	Values *apiextensionsv1.JSON `json:"values,omitempty"`

	// PostRenderers holds an array of Helm PostRenderers, which will be applied in order
	// of their definition.
	// +optional
	// PostRenderers []PostRenderer `json:"postRenderers,omitempty"`
}

// HelmChartTemplate defines the template from which the controller will
// generate a v1beta1.HelmChart object in the same namespace as the referenced
// v1beta1.Source.
type HelmChartTemplate struct {
	// Spec holds the template for the v1beta1.HelmChartSpec for this HelmRelease.
	// +required
	Spec HelmChartTemplateSpec `json:"spec"`
}

// HelmChartTemplateSpec defines the template from which the controller will
// generate a v1beta1.HelmChartSpec object.
type HelmChartTemplateSpec struct {
	// The name or path the Helm chart is available at in the SourceRef.
	// +required
	Chart string `json:"chart"`

	// Version semver expression, ignored for charts from v1beta1.GitRepository and
	// v1beta1.Bucket sources. Defaults to latest when omitted.
	// +kubebuilder:default:=*
	// +optional
	Version string `json:"version,omitempty"`

	// The name and namespace of the v1beta1.Source the chart is available at.
	// +required
	SourceRef CrossNamespaceObjectReference `json:"sourceRef"`

	// Interval at which to check the v1beta1.Source for updates. Defaults to
	// 'HelmReleaseSpec.Interval'.
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`

	// Alternative values file to use as the default chart values, expected to be a
	// relative path in the SourceRef. Ignored when omitted.
	// +optional
	ValuesFile string `json:"valuesFile,omitempty"`
}

// Install holds the configuration for Helm install actions performed for this
// HelmRelease.
type Install struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like
	// Jobs for hooks) during the performance of a Helm install action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Remediation holds the remediation configuration for when the Helm install
	// action for the HelmRelease fails. The default is to not perform any action.
	// +optional
	Remediation *InstallRemediation `json:"remediation,omitempty"`

	// DisableWait disables the waiting for resources to be ready after a Helm
	// install has been performed.
	// +optional
	DisableWait bool `json:"disableWait,omitempty"`

	// DisableHooks prevents hooks from running during the Helm install action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// DisableOpenAPIValidation prevents the Helm install action from validating
	// rendered templates against the Kubernetes OpenAPI Schema.
	// +optional
	DisableOpenAPIValidation bool `json:"disableOpenAPIValidation,omitempty"`

	// Replace tells the Helm install action to re-use the 'ReleaseName', but only
	// if that name is a deleted release which remains in the history.
	// +optional
	Replace bool `json:"replace,omitempty"`

	// SkipCRDs tells the Helm install action to not install any CRDs. By default,
	// CRDs are installed if not already present.
	// +optional
	SkipCRDs bool `json:"skipCRDs,omitempty"`

	// CreateNamespace tells the Helm install action to create the
	// HelmReleaseSpec.TargetNamespace if it does not exist yet.
	// On uninstall, the namespace will not be garbage collected.
	// +optional
	CreateNamespace bool `json:"createNamespace,omitempty"`
}

// InstallRemediation holds the configuration for Helm install remediation.
type InstallRemediation struct {
	// Retries is the number of retries that should be attempted on failures before
	// bailing. Remediation, using an uninstall, is performed between each attempt.
	// Defaults to '0', a negative integer equals to unlimited retries.
	// +optional
	Retries int `json:"retries,omitempty"`

	// IgnoreTestFailures tells the controller to skip remediation when the Helm
	// tests are run after an install action but fail. Defaults to
	// 'Test.IgnoreFailures'.
	// +optional
	IgnoreTestFailures *bool `json:"ignoreTestFailures,omitempty"`

	// RemediateLastFailure tells the controller to remediate the last failure, when
	// no retries remain. Defaults to 'false'.
	// +optional
	RemediateLastFailure *bool `json:"remediateLastFailure,omitempty"`
}

// Upgrade holds the configuration for Helm upgrade actions for this
// HelmRelease.
type Upgrade struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like
	// Jobs for hooks) during the performance of a Helm upgrade action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Remediation holds the remediation configuration for when the Helm upgrade
	// action for the HelmRelease fails. The default is to not perform any action.
	// +optional
	Remediation *UpgradeRemediation `json:"remediation,omitempty"`

	// DisableWait disables the waiting for resources to be ready after a Helm
	// upgrade has been performed.
	// +optional
	DisableWait bool `json:"disableWait,omitempty"`

	// DisableHooks prevents hooks from running during the Helm upgrade action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// DisableOpenAPIValidation prevents the Helm upgrade action from validating
	// rendered templates against the Kubernetes OpenAPI Schema.
	// +optional
	DisableOpenAPIValidation bool `json:"disableOpenAPIValidation,omitempty"`

	// Force forces resource updates through a replacement strategy.
	// +optional
	Force bool `json:"force,omitempty"`

	// PreserveValues will make Helm reuse the last release's values and merge in
	// overrides from 'Values'. Setting this flag makes the HelmRelease
	// non-declarative.
	// +optional
	PreserveValues bool `json:"preserveValues,omitempty"`

	// CleanupOnFail allows deletion of new resources created during the Helm
	// upgrade action when it fails.
	// +optional
	CleanupOnFail bool `json:"cleanupOnFail,omitempty"`
}

// UpgradeRemediation holds the configuration for Helm upgrade remediation.
type UpgradeRemediation struct {
	// Retries is the number of retries that should be attempted on failures before
	// bailing. Remediation, using 'Strategy', is performed between each attempt.
	// Defaults to '0', a negative integer equals to unlimited retries.
	// +optional
	Retries int `json:"retries,omitempty"`

	// IgnoreTestFailures tells the controller to skip remediation when the Helm
	// tests are run after an upgrade action but fail.
	// Defaults to 'Test.IgnoreFailures'.
	// +optional
	IgnoreTestFailures *bool `json:"ignoreTestFailures,omitempty"`

	// RemediateLastFailure tells the controller to remediate the last failure, when
	// no retries remain. Defaults to 'false' unless 'Retries' is greater than 0.
	// +optional
	RemediateLastFailure *bool `json:"remediateLastFailure,omitempty"`

	// Strategy to use for failure remediation. Defaults to 'rollback'.
	// +kubebuilder:validation:Enum=rollback;uninstall
	// +optional
	Strategy *RemediationStrategy `json:"strategy,omitempty"`
}

// RemediationStrategy returns the strategy to use to remediate a failed install
// or upgrade.
type RemediationStrategy string

const (
	// RollbackRemediationStrategy represents a Helm remediation strategy of Helm
	// rollback.
	RollbackRemediationStrategy RemediationStrategy = "rollback"

	// UninstallRemediationStrategy represents a Helm remediation strategy of Helm
	// uninstall.
	UninstallRemediationStrategy RemediationStrategy = "uninstall"
)

// Test holds the configuration for Helm test actions for this HelmRelease.
type Test struct {
	// Enable enables Helm test actions for this HelmRelease after an Helm install
	// or upgrade action has been performed.
	// +optional
	Enable bool `json:"enable,omitempty"`

	// Timeout is the time to wait for any individual Kubernetes operation during
	// the performance of a Helm test action. Defaults to 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// IgnoreFailures tells the controller to skip remediation when the Helm tests
	// are run but fail. Can be overwritten for tests run after install or upgrade
	// actions in 'Install.IgnoreTestFailures' and 'Upgrade.IgnoreTestFailures'.
	// +optional
	IgnoreFailures bool `json:"ignoreFailures,omitempty"`
}

// Rollback holds the configuration for Helm rollback actions for this
// HelmRelease.
type Rollback struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like
	// Jobs for hooks) during the performance of a Helm rollback action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// DisableWait disables the waiting for resources to be ready after a Helm
	// rollback has been performed.
	// +optional
	DisableWait bool `json:"disableWait,omitempty"`

	// DisableHooks prevents hooks from running during the Helm rollback action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// Recreate performs pod restarts for the resource if applicable.
	// +optional
	Recreate bool `json:"recreate,omitempty"`

	// Force forces resource updates through a replacement strategy.
	// +optional
	Force bool `json:"force,omitempty"`

	// CleanupOnFail allows deletion of new resources created during the Helm
	// rollback action when it fails.
	// +optional
	CleanupOnFail bool `json:"cleanupOnFail,omitempty"`
}

// Uninstall holds the configuration for Helm uninstall actions for this
// HelmRelease.
type Uninstall struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like
	// Jobs for hooks) during the performance of a Helm uninstall action. Defaults
	// to 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// DisableHooks prevents hooks from running during the Helm rollback action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// KeepHistory tells Helm to remove all associated resources and mark the
	// release as deleted, but retain the release history.
	// +optional
	KeepHistory bool `json:"keepHistory,omitempty"`
}
