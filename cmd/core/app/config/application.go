/*
Copyright 2025 The KubeVela Authors.

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

package config

import (
	"time"

	"github.com/spf13/pflag"

	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
)

// ApplicationConfig contains application-specific configuration.
type ApplicationConfig struct {
	ReSyncPeriod              time.Duration
	DefaultAutoRevision       bool
	DefaultApplyPolicyUpdates bool
	DefaultApplyDataChanges   bool
	// AllowNamespaceGlobalPolicies lets global policies outside vela-system apply.
	AllowNamespaceGlobalPolicies bool
	// AllowSkipGlobalPolicies lets an Application opt out of global policies (skip-global).
	AllowSkipGlobalPolicies bool
}

// NewApplicationConfig creates a new ApplicationConfig with defaults.
func NewApplicationConfig() *ApplicationConfig {
	return &ApplicationConfig{
		ReSyncPeriod:              commonconfig.ApplicationReSyncPeriod,
		DefaultAutoRevision:       commonconfig.ApplicationDefaultAutoRevision,
		DefaultApplyPolicyUpdates: commonconfig.ApplicationDefaultApplyPolicyUpdates,
		DefaultApplyDataChanges:   commonconfig.ApplicationDefaultApplyDataChanges,

		AllowNamespaceGlobalPolicies: commonconfig.AllowNamespaceGlobalPolicies,
		AllowSkipGlobalPolicies:      commonconfig.AllowSkipGlobalPolicies,
	}
}

// AddFlags registers application configuration flags.
func (c *ApplicationConfig) AddFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&c.ReSyncPeriod,
		"application-re-sync-period",
		c.ReSyncPeriod,
		"Re-sync period for application to re-sync, also known as the state-keep interval.")
	fs.BoolVar(&c.DefaultAutoRevision,
		"application-default-auto-revision",
		c.DefaultAutoRevision,
		"Default for a policy's autoRevision setting, used when the application has no valid policy.oam.dev/auto-revision "+
			"annotation and the policy template sets no config.autoRevision. When true, "+
			"Application-scoped policy changes to an application's spec cut a new ApplicationRevision; an application "+
			"can opt out with the annotation set to \"false\".")
	fs.BoolVar(&c.DefaultApplyPolicyUpdates,
		"application-default-apply-policy-updates",
		c.DefaultApplyPolicyUpdates,
		"Default for a policy's applyPolicyUpdates setting, used when the application has no valid policy.oam.dev/apply-policy-updates "+
			"annotation and the policy template sets no config.applyPolicyUpdates. When true, "+
			"applications adopt new versions of the Application-scoped policies they run.")
	fs.BoolVar(&c.DefaultApplyDataChanges,
		"application-default-apply-data-changes",
		c.DefaultApplyDataChanges,
		"Default for a policy's applyDataChanges setting, used when the application has no valid policy.oam.dev/apply-data-changes "+
			"annotation and the policy template sets no config.applyDataChanges. When true, "+
			"applications take changed output from an unchanged Application-scoped policy version.")
	fs.BoolVar(&c.AllowNamespaceGlobalPolicies,
		"allow-namespace-global-policies",
		c.AllowNamespaceGlobalPolicies,
		"Whether global Application-scoped PolicyDefinitions outside vela-system apply to the applications in their "+
			"namespace. When false, global policies are platform-wide only: they must be in vela-system, the webhook "+
			"rejects them elsewhere, and any that exist are ignored.")
	fs.BoolVar(&c.AllowSkipGlobalPolicies,
		"allow-skip-global-policies",
		c.AllowSkipGlobalPolicies,
		"Whether an application may opt out of global policies with the policy.oam.dev/skip-global annotation. "+
			"When false, global policies are mandatory: the webhook rejects the annotation and the controller ignores it.")
}

// SyncToApplicationGlobals syncs the parsed configuration values to application package global variables.
// This should be called after flag parsing to ensure the application controller uses the configured values.
//
// NOTE: This method exists for backward compatibility with legacy code that depends on global
// variables in the commonconfig package. Ideally, configuration should be injected rather than using globals.
//
// The flow is: CLI flags -> ApplicationConfig struct fields -> commonconfig globals (via this method)
func (c *ApplicationConfig) SyncToApplicationGlobals() {
	commonconfig.ApplicationReSyncPeriod = c.ReSyncPeriod
	commonconfig.ApplicationDefaultAutoRevision = c.DefaultAutoRevision
	commonconfig.ApplicationDefaultApplyPolicyUpdates = c.DefaultApplyPolicyUpdates
	commonconfig.ApplicationDefaultApplyDataChanges = c.DefaultApplyDataChanges
	commonconfig.AllowNamespaceGlobalPolicies = c.AllowNamespaceGlobalPolicies
	commonconfig.AllowSkipGlobalPolicies = c.AllowSkipGlobalPolicies
}
