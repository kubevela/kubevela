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

package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	types2 "k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/types"
	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
	"github.com/oam-dev/kubevela/pkg/definition/goloader"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	velaversion "github.com/oam-dev/kubevela/version"
)

const (
	// FlagModuleVersion is the flag for module version
	FlagModuleVersion = "version"
	// FlagModuleTypes is the flag for filtering definition types
	FlagModuleTypes = "types"
	// FlagModulePrefix is the flag for definition name prefix
	FlagModulePrefix = "prefix"
	// FlagIgnorePlacement is the flag to ignore placement constraints
	FlagIgnorePlacement = "ignore-placement"
	// FlagConflictStrategy is the flag for conflict resolution strategy
	FlagConflictStrategy = "conflict"
	// FlagSkipHooks is the flag to skip all hooks
	FlagSkipHooks = "skip-hooks"
	// FlagSkipPreApply is the flag to skip pre-apply hooks
	FlagSkipPreApply = "skip-pre-apply"
	// FlagSkipPostApply is the flag to skip post-apply hooks
	FlagSkipPostApply = "skip-post-apply"
	// FlagStats is the flag to show detailed statistics
	FlagStats = "stats"
)

// ConflictStrategy represents how to handle name conflicts
type ConflictStrategy string

const (
	// ConflictStrategySkip skips definitions that already exist
	ConflictStrategySkip ConflictStrategy = "skip"
	// ConflictStrategyOverwrite overwrites existing definitions
	ConflictStrategyOverwrite ConflictStrategy = "overwrite"
	// ConflictStrategyFail fails if any definition already exists
	ConflictStrategyFail ConflictStrategy = "fail"
	// ConflictStrategyRename renames conflicting definitions with a suffix
	ConflictStrategyRename ConflictStrategy = "rename"
)

// IsValid returns true if the conflict strategy is a recognized valid value.
func (c ConflictStrategy) IsValid() bool {
	switch c {
	case ConflictStrategySkip, ConflictStrategyOverwrite, ConflictStrategyFail, ConflictStrategyRename:
		return true
	default:
		return false
	}
}

// NewDefinitionApplyModuleCommand creates the `vela def apply-module` command
// to apply a Go module containing definitions to kubernetes
func NewDefinitionApplyModuleCommand(c common.Args, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply-module",
		Short: "Apply all definitions from a Go module.",
		Long: `Apply all definitions from a Go module to kubernetes cluster.

Supports both local paths and remote Go modules:
  - Local path: ./my-definitions, /path/to/definitions
  - Go module: github.com/myorg/definitions@v1.0.0

The module can contain a module.yaml file with metadata about the module,
including name, version, description, and minimum KubeVela version requirements.`,
		Example: `# Apply definitions from a local directory
> vela def apply-module ./my-definitions

# Apply definitions from a Go module
> vela def apply-module github.com/myorg/definitions@v1.0.0

# Apply only component and trait definitions
> vela def apply-module ./my-definitions --types component,trait

# Apply with a name prefix to avoid conflicts
> vela def apply-module ./my-definitions --prefix myorg-

# Apply with conflict resolution strategy
> vela def apply-module ./my-definitions --conflict overwrite

# Dry-run to preview what would be applied
> vela def apply-module ./my-definitions --dry-run`,
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeDefModule,
			types.TagCommandOrder: "2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			dryRun, err := cmd.Flags().GetBool(FlagDryRun)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagDryRun)
			}

			namespace, err := cmd.Flags().GetString(FlagNamespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", Namespace)
			}

			version, err := cmd.Flags().GetString(FlagModuleVersion)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagModuleVersion)
			}

			typesStr, err := cmd.Flags().GetString(FlagModuleTypes)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagModuleTypes)
			}

			prefix, err := cmd.Flags().GetString(FlagModulePrefix)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagModulePrefix)
			}

			conflictStr, err := cmd.Flags().GetString(FlagConflictStrategy)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagConflictStrategy)
			}
			conflict := ConflictStrategy(conflictStr)
			if !conflict.IsValid() {
				return errors.Errorf("invalid conflict strategy %q; valid values: skip, overwrite, fail, rename", conflictStr)
			}

			ignorePlacement, err := cmd.Flags().GetBool(FlagIgnorePlacement)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagIgnorePlacement)
			}

			skipHooks, err := cmd.Flags().GetBool(FlagSkipHooks)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagSkipHooks)
			}

			skipPreApply, err := cmd.Flags().GetBool(FlagSkipPreApply)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagSkipPreApply)
			}

			skipPostApply, err := cmd.Flags().GetBool(FlagSkipPostApply)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagSkipPostApply)
			}

			showStats, err := cmd.Flags().GetBool(FlagStats)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagStats)
			}

			var defTypes []string
			if typesStr != "" {
				defTypes = strings.Split(typesStr, ",")
				for i := range defTypes {
					defTypes[i] = strings.TrimSpace(defTypes[i])
				}
			}

			return applyModule(ctx, c, streams, args[0], applyModuleOptions{
				namespace:       namespace,
				version:         version,
				types:           defTypes,
				prefix:          prefix,
				conflict:        conflict,
				dryRun:          dryRun,
				ignorePlacement: ignorePlacement,
				skipHooks:       skipHooks,
				skipPreApply:    skipPreApply,
				skipPostApply:   skipPostApply,
				showStats:       showStats,
			})
		},
	}

	cmd.Flags().BoolP(FlagDryRun, "", false, "Preview what would be applied without making changes")
	cmd.Flags().StringP(Namespace, "n", types.DefaultKubeVelaNS, "Namespace to apply definitions to")
	cmd.Flags().StringP(FlagModuleVersion, "v", "", "Version of the module to apply (for remote modules)")
	cmd.Flags().StringP(FlagModuleTypes, "t", "", "Comma-separated list of definition types to apply (component,trait,policy,workflow-step)")
	cmd.Flags().StringP(FlagModulePrefix, "p", "", "Prefix to add to all definition names")
	cmd.Flags().StringP(FlagConflictStrategy, "c", string(ConflictStrategyFail), "Conflict resolution strategy: skip, overwrite, fail, rename")
	cmd.Flags().BoolP(FlagIgnorePlacement, "", false, "Ignore placement constraints and apply all definitions")
	cmd.Flags().BoolP(FlagSkipHooks, "", false, "Skip all hooks (pre-apply and post-apply)")
	cmd.Flags().BoolP(FlagSkipPreApply, "", false, "Skip pre-apply hooks only")
	cmd.Flags().BoolP(FlagSkipPostApply, "", false, "Skip post-apply hooks only")
	cmd.Flags().BoolP(FlagStats, "", false, "Show detailed timing and statistics")

	return cmd
}

// applyModuleOptions contains options for applying a module
type applyModuleOptions struct {
	namespace       string
	version         string
	types           []string
	prefix          string
	conflict        ConflictStrategy
	dryRun          bool
	ignorePlacement bool
	skipHooks       bool
	skipPreApply    bool
	skipPostApply   bool
	showStats       bool
}

// ApplyStats tracks statistics for module application
type ApplyStats struct {
	// Timing
	StartTime         time.Time
	ModuleLoadTime    time.Duration
	PreApplyHookTime  time.Duration
	DefApplyTime      time.Duration
	PostApplyHookTime time.Duration
	TotalTime         time.Duration

	// Hook timing details
	PreApplyHookDetails  []HookTimingDetail
	PostApplyHookDetails []HookTimingDetail

	// Definition counts by type
	Components    int
	Traits        int
	Policies      int
	WorkflowSteps int

	// Definition counts by action
	Created int
	Updated int
	Skipped int
	Failed  int

	// Placement stats
	PlacementEvaluated int
	PlacementEligible  int
	PlacementSkipped   int

	// Hook stats
	HookResourcesCreated int
	HookResourcesUpdated int
	OptionalHooksFailed  int
}

// HookTimingDetail contains timing info for a single hook
type HookTimingDetail struct {
	Name     string
	Duration time.Duration
	Wait     bool
}

// NewApplyStats creates a new ApplyStats and starts the timer
func NewApplyStats() *ApplyStats {
	return &ApplyStats{
		StartTime: time.Now(),
	}
}

// applyModule loads and applies all definitions from a module
func applyModule(ctx context.Context, c common.Args, streams util.IOStreams, moduleRef string, opts applyModuleOptions) error {
	// Initialize stats tracking
	stats := NewApplyStats()

	// Load module options
	loadOpts := goloader.ModuleLoadOptions{
		Version:             opts.version,
		Types:               opts.types,
		NamePrefix:          opts.prefix,
		ResolveDependencies: true,
	}
	if len(opts.types) == 0 {
		loadOpts = goloader.DefaultModuleLoadOptions()
		loadOpts.Version = opts.version
		loadOpts.NamePrefix = opts.prefix
	}

	streams.Infof("Loading module from %s...\n", moduleRef)

	// Load the module (with timing)
	loadStart := time.Now()
	module, err := goloader.LoadModule(ctx, moduleRef, loadOpts)
	if err != nil {
		return errors.Wrapf(err, "failed to load module from %s", moduleRef)
	}
	stats.ModuleLoadTime = time.Since(loadStart)

	// Print module summary
	streams.Infof("\n%s\n", module.Summary())

	// Get kubernetes client
	config, err := c.GetConfig()
	if err != nil {
		return errors.Wrap(err, "failed to get kubernetes config")
	}
	k8sClient, err := c.GetClient()
	if err != nil {
		return errors.Wrap(err, "failed to get kubernetes client")
	}

	// Get cluster labels for placement checking (unless ignoring or dry-run)
	var clusterLabels map[string]string
	var modulePlacement placement.PlacementSpec
	checkPlacement := !opts.ignorePlacement && !opts.dryRun

	if checkPlacement {
		// Get module-level placement
		if module.Metadata.Spec.Placement != nil {
			modulePlacement = module.Metadata.Spec.Placement.ToPlacementSpec()
		}

		// Check if any placement constraints exist (module-level or definition-level)
		hasAnyPlacement := !modulePlacement.IsEmpty()
		if !hasAnyPlacement {
			for _, result := range module.Definitions {
				if result.Definition.Placement != nil && !result.Definition.Placement.IsEmpty() {
					hasAnyPlacement = true
					break
				}
			}
		}

		// Fetch cluster labels if there's any placement to check
		if hasAnyPlacement {
			var labelErr error
			clusterLabels, labelErr = placement.GetClusterLabels(ctx, k8sClient)
			if labelErr != nil {
				streams.Infof("Warning: Could not fetch cluster labels: %v\n", labelErr)
				streams.Infof("Placement constraints will not be enforced.\n\n")
				checkPlacement = false
			} else {
				streams.Infof("Checking placement constraints...\n")
				streams.Infof("Cluster labels: %s\n\n", placement.FormatClusterLabels(clusterLabels))
			}
		}
	}

	if opts.ignorePlacement {
		streams.Infof("Warning: Ignoring placement constraints (--ignore-placement)\n\n")
	}

	// Execute pre-apply hooks
	if !opts.skipHooks && !opts.skipPreApply && module.Metadata.Spec.Hooks.HasPreApply() {
		hookExecutor := goloader.NewHookExecutor(k8sClient, module.Path, opts.namespace, opts.dryRun, streams)
		hookStats, err := hookExecutor.ExecuteHooks(ctx, "pre-apply", module.Metadata.Spec.Hooks.PreApply)
		if err != nil {
			return errors.Wrap(err, "pre-apply hooks failed")
		}
		stats.PreApplyHookTime = hookStats.TotalDuration
		stats.HookResourcesCreated += hookStats.ResourcesCreated
		stats.HookResourcesUpdated += hookStats.ResourcesUpdated
		stats.OptionalHooksFailed += hookStats.OptionalFailed
		for _, detail := range hookStats.HookDetails {
			stats.PreApplyHookDetails = append(stats.PreApplyHookDetails, HookTimingDetail{
				Name:     detail.Name,
				Duration: detail.Duration,
				Wait:     detail.Wait,
			})
		}
	}

	// Track results
	var failedDefs []string
	defApplyStart := time.Now()

	// Apply each definition
	for _, result := range module.Definitions {
		if result.Error != nil {
			stats.Failed++
			failedDefs = append(failedDefs, fmt.Sprintf("%s: %v", result.Definition.FilePath, result.Error))
			continue
		}

		// Parse CUE to definition
		def := pkgdef.Definition{}
		if err := def.FromCUEString(result.CUE, config); err != nil {
			stats.Failed++
			failedDefs = append(failedDefs, fmt.Sprintf("%s: %v", result.Definition.Name, err))
			continue
		}
		def.SetNamespace(opts.namespace)

		// Apply name prefix if specified
		if opts.prefix != "" {
			def.SetName(opts.prefix + def.GetName())
		}

		// Check placement constraints (combine module-level and definition-level)
		if checkPlacement {
			// Get definition-level placement (if any)
			var defPlacement placement.PlacementSpec
			if result.Definition.Placement != nil {
				defPlacement = result.Definition.Placement.ToPlacementSpec()
			}

			// Combine with module-level placement (definition overrides module)
			effectivePlacement := placement.GetEffectivePlacement(modulePlacement, defPlacement)

			if !effectivePlacement.IsEmpty() {
				stats.PlacementEvaluated++
				placementResult := placement.Evaluate(effectivePlacement, clusterLabels)
				if !placementResult.Eligible {
					streams.Infof("  ✗ %s %s: skipped (%s)\n", def.GetKind(), def.GetName(), placementResult.Reason)
					stats.PlacementSkipped++
					continue
				}
				stats.PlacementEligible++
				streams.Infof("  ✓ %s %s: eligible\n", def.GetKind(), def.GetName())
			}
		}

		// Track definition type
		trackDefinitionType(stats, result.Definition.Type)

		// Dry-run mode: just print the YAML
		if opts.dryRun {
			s, err := prettyYAMLMarshal(def.Object)
			if err != nil {
				stats.Failed++
				failedDefs = append(failedDefs, fmt.Sprintf("%s: %v", result.Definition.Name, err))
				continue
			}
			streams.Info(s)
			streams.Info("---\n")
			stats.Created++
			continue
		}

		// Check if definition already exists
		existingDef := pkgdef.Definition{}
		existingDef.SetGroupVersionKind(def.GroupVersionKind())
		err = k8sClient.Get(ctx, types2.NamespacedName{
			Namespace: opts.namespace,
			Name:      def.GetName(),
		}, &existingDef)

		exists := err == nil
		if err != nil && !errors2.IsNotFound(err) {
			stats.Failed++
			failedDefs = append(failedDefs, fmt.Sprintf("%s: %v", def.GetName(), err))
			continue
		}

		// Handle conflicts based on strategy
		if exists {
			switch opts.conflict {
			case ConflictStrategySkip:
				streams.Infof("Skipping %s %s (already exists)\n", def.GetKind(), def.GetName())
				stats.Skipped++
				continue
			case ConflictStrategyFail:
				return errors.Errorf("definition %s %s already exists in namespace %s (use --conflict=overwrite to update)",
					def.GetKind(), def.GetName(), opts.namespace)
			case ConflictStrategyRename:
				// Find a unique name
				baseName := def.GetName()
				for i := 1; ; i++ {
					newName := fmt.Sprintf("%s-%d", baseName, i)
					existingDef := pkgdef.Definition{}
					existingDef.SetGroupVersionKind(def.GroupVersionKind())
					err = k8sClient.Get(ctx, types2.NamespacedName{
						Namespace: opts.namespace,
						Name:      newName,
					}, &existingDef)
					if errors2.IsNotFound(err) {
						def.SetName(newName)
						exists = false
						break
					}
					if i > 100 {
						return errors.Errorf("failed to find unique name for %s (tried %s-1 to %s-100)", baseName, baseName, baseName)
					}
				}
			case ConflictStrategyOverwrite:
				// Will update below
			}
		}

		// Apply the definition
		if exists {
			// Update existing - preserve resourceVersion for optimistic concurrency
			resourceVersion := existingDef.GetResourceVersion()
			uid := existingDef.GetUID()
			existingDef.Object = def.Object
			existingDef.SetResourceVersion(resourceVersion)
			existingDef.SetUID(uid)
			existingDef.SetNamespace(opts.namespace)
			if err = k8sClient.Update(ctx, &existingDef); err != nil {
				stats.Failed++
				failedDefs = append(failedDefs, fmt.Sprintf("%s: %v", def.GetName(), err))
				continue
			}
			streams.Infof("%s %s updated in namespace %s\n", def.GetKind(), def.GetName(), opts.namespace)
			stats.Updated++
		} else {
			// Create new
			if err = k8sClient.Create(ctx, &def); err != nil {
				stats.Failed++
				failedDefs = append(failedDefs, fmt.Sprintf("%s: %v", def.GetName(), err))
				continue
			}
			streams.Infof("%s %s created in namespace %s\n", def.GetKind(), def.GetName(), opts.namespace)
			stats.Created++
		}
	}
	stats.DefApplyTime = time.Since(defApplyStart)

	// Execute post-apply hooks (only if we actually applied something or this is not dry-run)
	if !opts.skipHooks && !opts.skipPostApply && module.Metadata.Spec.Hooks.HasPostApply() {
		hookExecutor := goloader.NewHookExecutor(k8sClient, module.Path, opts.namespace, opts.dryRun, streams)
		hookStats, err := hookExecutor.ExecuteHooks(ctx, "post-apply", module.Metadata.Spec.Hooks.PostApply)
		if err != nil {
			return errors.Wrap(err, "post-apply hooks failed")
		}
		stats.PostApplyHookTime = hookStats.TotalDuration
		stats.HookResourcesCreated += hookStats.ResourcesCreated
		stats.HookResourcesUpdated += hookStats.ResourcesUpdated
		stats.OptionalHooksFailed += hookStats.OptionalFailed
		for _, detail := range hookStats.HookDetails {
			stats.PostApplyHookDetails = append(stats.PostApplyHookDetails, HookTimingDetail{
				Name:     detail.Name,
				Duration: detail.Duration,
				Wait:     detail.Wait,
			})
		}
	}

	// Calculate total time
	stats.TotalTime = time.Since(stats.StartTime)

	// Print summary - basic stats are always shown
	printBasicSummary(streams, stats, failedDefs, opts.dryRun)

	// Print detailed stats if requested
	if opts.showStats {
		printDetailedStats(streams, stats)
	}

	if stats.Failed > 0 {
		return errors.Errorf("%d definitions failed to apply", stats.Failed)
	}

	return nil
}

// trackDefinitionType increments the counter for the given definition type
func trackDefinitionType(stats *ApplyStats, defType string) {
	switch defType {
	case "component":
		stats.Components++
	case "trait":
		stats.Traits++
	case "policy":
		stats.Policies++
	case "workflow-step":
		stats.WorkflowSteps++
	}
}

// printBasicSummary prints a minimal summary that is always shown
func printBasicSummary(streams util.IOStreams, stats *ApplyStats, failedDefs []string, dryRun bool) {
	streams.Infof("\n")

	// Build a concise one-line summary
	var parts []string
	if dryRun {
		if stats.Created > 0 {
			parts = append(parts, fmt.Sprintf("would apply: %d", stats.Created))
		}
	} else {
		if stats.Created > 0 {
			parts = append(parts, fmt.Sprintf("created: %d", stats.Created))
		}
		if stats.Updated > 0 {
			parts = append(parts, fmt.Sprintf("updated: %d", stats.Updated))
		}
	}
	if stats.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("skipped: %d", stats.Skipped))
	}
	if stats.PlacementSkipped > 0 {
		parts = append(parts, fmt.Sprintf("placement-skipped: %d", stats.PlacementSkipped))
	}
	if stats.Failed > 0 {
		parts = append(parts, fmt.Sprintf("failed: %d", stats.Failed))
	}

	if dryRun {
		if len(parts) > 0 {
			streams.Infof("Dry-run complete: %s\n", strings.Join(parts, ", "))
		} else {
			streams.Infof("Dry-run complete: no definitions to apply\n")
		}
	} else {
		if len(parts) > 0 {
			streams.Infof("Complete: %s\n", strings.Join(parts, ", "))
		} else {
			streams.Infof("Complete: no definitions applied\n")
		}
	}

	// Show failed definitions details
	if stats.Failed > 0 {
		for _, f := range failedDefs {
			streams.Infof("  - %s\n", f)
		}
	}
}

// printDetailedStats prints detailed statistics when --stats flag is set
func printDetailedStats(streams util.IOStreams, stats *ApplyStats) {
	streams.Infof("\n─────────────────────────────────────────\n")
	streams.Infof("Detailed Statistics\n")
	streams.Infof("─────────────────────────────────────────\n")

	// Definition counts by type
	hasTypes := stats.Components > 0 || stats.Traits > 0 || stats.Policies > 0 || stats.WorkflowSteps > 0
	if hasTypes {
		streams.Infof("\nDefinitions by type:\n")
		if stats.Components > 0 {
			streams.Infof("  Components:        %d\n", stats.Components)
		}
		if stats.Traits > 0 {
			streams.Infof("  Traits:            %d\n", stats.Traits)
		}
		if stats.Policies > 0 {
			streams.Infof("  Policies:          %d\n", stats.Policies)
		}
		if stats.WorkflowSteps > 0 {
			streams.Infof("  Workflow Steps:    %d\n", stats.WorkflowSteps)
		}
	}

	// Definition counts by action
	streams.Infof("\nDefinitions by action:\n")
	streams.Infof("  Created:           %d\n", stats.Created)
	streams.Infof("  Updated:           %d\n", stats.Updated)
	streams.Infof("  Skipped:           %d\n", stats.Skipped)
	streams.Infof("  Failed:            %d\n", stats.Failed)

	// Placement stats
	if stats.PlacementEvaluated > 0 {
		streams.Infof("\nPlacement:\n")
		streams.Infof("  Evaluated:         %d\n", stats.PlacementEvaluated)
		streams.Infof("  Eligible:          %d\n", stats.PlacementEligible)
		streams.Infof("  Skipped:           %d\n", stats.PlacementSkipped)
	}

	// Timing statistics
	streams.Infof("\nTiming:\n")
	streams.Infof("  Module loading:    %s\n", formatDuration(stats.ModuleLoadTime))

	if stats.PreApplyHookTime > 0 {
		streams.Infof("  Pre-apply hooks:   %s\n", formatDuration(stats.PreApplyHookTime))
		for _, detail := range stats.PreApplyHookDetails {
			suffix := ""
			if detail.Wait {
				suffix = " (wait)"
			}
			streams.Infof("    - %s: %s%s\n", detail.Name, formatDuration(detail.Duration), suffix)
		}
	}

	streams.Infof("  Definition apply:  %s\n", formatDuration(stats.DefApplyTime))

	if stats.PostApplyHookTime > 0 {
		streams.Infof("  Post-apply hooks:  %s\n", formatDuration(stats.PostApplyHookTime))
		for _, detail := range stats.PostApplyHookDetails {
			suffix := ""
			if detail.Wait {
				suffix = " (wait)"
			}
			streams.Infof("    - %s: %s%s\n", detail.Name, formatDuration(detail.Duration), suffix)
		}
	}

	streams.Infof("  ─────────────────\n")
	streams.Infof("  Total:             %s\n", formatDuration(stats.TotalTime))

	// Throughput
	totalApplied := stats.Created + stats.Updated
	if totalApplied > 0 && stats.DefApplyTime > 0 {
		throughput := float64(totalApplied) / stats.DefApplyTime.Seconds()
		streams.Infof("  Throughput:        %.1f definitions/sec\n", throughput)
	}

	// Hook resource stats
	if stats.HookResourcesCreated > 0 || stats.HookResourcesUpdated > 0 || stats.OptionalHooksFailed > 0 {
		streams.Infof("\nHook resources:\n")
		streams.Infof("  Created:           %d\n", stats.HookResourcesCreated)
		streams.Infof("  Updated:           %d\n", stats.HookResourcesUpdated)
		if stats.OptionalHooksFailed > 0 {
			streams.Infof("  Optional failed:   %d\n", stats.OptionalHooksFailed)
		}
	}
}

// formatDuration formats a duration for display
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

// NewDefinitionListModuleCommand creates the `vela def list-module` command
// to list definitions in a Go module without applying them
func NewDefinitionListModuleCommand(c common.Args, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-module",
		Short: "List all definitions in a Go module.",
		Long: `List all definitions in a Go module without applying them.

Supports both local paths and remote Go modules:
  - Local path: ./my-definitions, /path/to/definitions
  - Go module: github.com/myorg/definitions@v1.0.0`,
		Example: `# List definitions in a local directory
> vela def list-module ./my-definitions

# List definitions in a remote Go module
> vela def list-module github.com/myorg/definitions@v1.0.0

# List only component definitions
> vela def list-module ./my-definitions --types component`,
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeDefModule,
			types.TagCommandOrder: "3",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			version, err := cmd.Flags().GetString(FlagModuleVersion)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagModuleVersion)
			}

			typesStr, err := cmd.Flags().GetString(FlagModuleTypes)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagModuleTypes)
			}

			var defTypes []string
			if typesStr != "" {
				defTypes = strings.Split(typesStr, ",")
				for i := range defTypes {
					defTypes[i] = strings.TrimSpace(defTypes[i])
				}
			}

			return listModule(ctx, streams, args[0], listModuleOptions{
				version: version,
				types:   defTypes,
			})
		},
	}

	cmd.Flags().StringP(FlagModuleVersion, "v", "", "Version of the module (for remote modules)")
	cmd.Flags().StringP(FlagModuleTypes, "t", "", "Comma-separated list of definition types to list")

	return cmd
}

// listModuleOptions contains options for listing module contents
type listModuleOptions struct {
	version string
	types   []string
}

// listModule lists all definitions in a module
func listModule(ctx context.Context, streams util.IOStreams, moduleRef string, opts listModuleOptions) error {
	// Load module options
	loadOpts := goloader.DefaultModuleLoadOptions()
	loadOpts.Version = opts.version
	if len(opts.types) > 0 {
		loadOpts.Types = opts.types
	}

	streams.Infof("Loading module from %s...\n", moduleRef)

	// Load the module
	module, err := goloader.LoadModule(ctx, moduleRef, loadOpts)
	if err != nil {
		return errors.Wrapf(err, "failed to load module from %s", moduleRef)
	}

	// Print module info
	streams.Infof("\n%s\n", module.Summary())

	// Print definitions in table format
	if len(module.Definitions) > 0 {
		streams.Infof("Definitions:\n")
		w := tabwriter.NewWriter(streams.Out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTYPE\tFILE\tSTATUS")
		for _, def := range module.Definitions {
			status := "OK"
			if def.Error != nil {
				status = fmt.Sprintf("Error: %v", def.Error)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				def.Definition.Name,
				def.Definition.Type,
				def.Definition.FilePath,
				status)
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}

	return nil
}

// NewDefinitionValidateModuleCommand creates the `vela def validate-module` command
// to validate a Go module's definitions
func NewDefinitionValidateModuleCommand(c common.Args, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate-module",
		Short: "Validate all definitions in a Go module.",
		Long: `Validate all definitions in a Go module without applying them.

Checks:
  - All Go definition files can be parsed
  - All definitions generate valid CUE
  - Module metadata is valid (if module.yaml exists)
  - Minimum KubeVela version requirements (if specified)`,
		Example: `# Validate definitions in a local directory
> vela def validate-module ./my-definitions

# Validate definitions in a remote Go module
> vela def validate-module github.com/myorg/definitions@v1.0.0`,
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeDefModule,
			types.TagCommandOrder: "4",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			version, err := cmd.Flags().GetString(FlagModuleVersion)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagModuleVersion)
			}

			return validateModule(ctx, c, streams, args[0], version)
		},
	}

	cmd.Flags().StringP(FlagModuleVersion, "v", "", "Version of the module (for remote modules)")

	return cmd
}

// validateModule validates all definitions in a module
func validateModule(ctx context.Context, c common.Args, streams util.IOStreams, moduleRef, version string) error {
	// Load module options
	loadOpts := goloader.DefaultModuleLoadOptions()
	loadOpts.Version = version

	streams.Infof("Loading module from %s...\n", moduleRef)

	// Load the module
	module, err := goloader.LoadModule(ctx, moduleRef, loadOpts)
	if err != nil {
		return errors.Wrapf(err, "failed to load module from %s", moduleRef)
	}

	// Get kubernetes config for CUE validation
	config, err := c.GetConfig()
	if err != nil {
		// Continue without config - some validation can still be done
		streams.Infof("Warning: Could not get kubernetes config, some validations will be skipped\n")
	}

	// Collect validation errors
	var validationErrors []string

	// Validate each definition
	for _, result := range module.Definitions {
		if result.Error != nil {
			validationErrors = append(validationErrors,
				fmt.Sprintf("%s: failed to load: %v", result.Definition.FilePath, result.Error))
			continue
		}

		// Validate CUE can be parsed (config can be nil - FromCUEString doesn't use it)
		def := pkgdef.Definition{}
		if err := def.FromCUEString(result.CUE, config); err != nil {
			validationErrors = append(validationErrors,
				fmt.Sprintf("%s: invalid CUE: %v", result.Definition.Name, err))
		}
	}

	// Validate module metadata (pass current vela version for minVelaVersion check)
	errs := goloader.ValidateModule(module, velaversion.VelaVersion)
	for _, e := range errs {
		validationErrors = append(validationErrors, e.Error())
	}

	// Print results
	streams.Infof("\n%s\n", module.Summary())

	if len(validationErrors) > 0 {
		streams.Infof("Validation failed with %d error(s):\n", len(validationErrors))
		for _, e := range validationErrors {
			streams.Infof("  - %s\n", e)
		}
		return errors.Errorf("module validation failed with %d errors", len(validationErrors))
	}

	streams.Infof("Module validation passed.\n")
	return nil
}

// FlagModuleName is the flag for module name
const FlagModuleName = "name"

// FlagModuleDesc is the flag for module description
const FlagModuleDesc = "desc"

// FlagGoModule is the flag for Go module path
const FlagGoModule = "go-module"

// FlagWithExamples is the flag to include example definitions
const FlagWithExamples = "with-examples"

// Scaffold flags for creating specific definitions
const (
	// FlagComponents is the flag for component names to scaffold
	FlagComponents = "components"
	// FlagTraits is the flag for trait names to scaffold
	FlagTraits = "traits"
	// FlagPolicies is the flag for policy names to scaffold
	FlagPolicies = "policies"
	// FlagWorkflowSteps is the flag for workflow step names to scaffold
	FlagWorkflowSteps = "workflowsteps"
)

// NewDefinitionInitModuleCommand creates the `vela def init-module` command
// to initialize a new definition module with proper directory structure
func NewDefinitionInitModuleCommand(_ common.Args, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init-module",
		Short: "Initialize a new definition module.",
		Long: `Initialize a new Go-based definition module with proper directory structure.

Creates:
  - module.yaml with metadata
  - go.mod and go.sum files
  - cmd/register/main.go entry point for registry loading
  - components/, traits/, policies/, workflowsteps/ directories
  - Scaffolded definition files (optional)
  - README.md with documentation

This sets up a complete module that can be published as a Go module
and applied using 'vela def apply-module'.`,
		Example: `# Initialize a new module - creates 'my-defs' directory automatically
> vela def init-module --name my-defs

# Initialize a module in a specific directory
> vela def init-module ./my-definitions

# Initialize with custom module name and Go module path
> vela def init-module ./my-defs --name my-defs --go-module github.com/myorg/my-defs

# Initialize with example definitions
> vela def init-module --name my-defs --with-examples

# Initialize with scaffolded component definitions
> vela def init-module --name my-defs --components webservice,worker,task

# Initialize with multiple definition types
> vela def init-module --name my-defs --components api,backend --traits autoscaler --policies topology

# Initialize in current directory (if no --name or path given)
> vela def init-module`,
		Args: cobra.MaximumNArgs(1),
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeDefModule,
			types.TagCommandOrder: "1",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			moduleName, err := cmd.Flags().GetString(FlagModuleName)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagModuleName)
			}

			// Determine target directory:
			// 1. If path argument is given, use it
			// 2. If --name is given but no path, create directory with that name
			// 3. Otherwise, use current directory
			var targetDir string
			if len(args) > 0 {
				targetDir = args[0]
			} else if moduleName != "" {
				targetDir = moduleName
			} else {
				targetDir = "."
			}

			moduleDesc, err := cmd.Flags().GetString(FlagModuleDesc)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagModuleDesc)
			}

			goModule, err := cmd.Flags().GetString(FlagGoModule)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagGoModule)
			}

			withExamples, err := cmd.Flags().GetBool(FlagWithExamples)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagWithExamples)
			}

			// Parse scaffold flags
			componentsStr, err := cmd.Flags().GetString(FlagComponents)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagComponents)
			}
			traitsStr, err := cmd.Flags().GetString(FlagTraits)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagTraits)
			}
			policiesStr, err := cmd.Flags().GetString(FlagPolicies)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagPolicies)
			}
			workflowStepsStr, err := cmd.Flags().GetString(FlagWorkflowSteps)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagWorkflowSteps)
			}

			scaffold := scaffoldOptions{
				components:    parseCommaSeparated(componentsStr),
				traits:        parseCommaSeparated(traitsStr),
				policies:      parseCommaSeparated(policiesStr),
				workflowsteps: parseCommaSeparated(workflowStepsStr),
			}

			return initModule(streams, targetDir, initModuleOptions{
				name:         moduleName,
				description:  moduleDesc,
				goModule:     goModule,
				withExamples: withExamples,
				scaffold:     scaffold,
			})
		},
	}

	cmd.Flags().StringP(FlagModuleName, "n", "", "Name for the module (defaults to directory name)")
	cmd.Flags().StringP(FlagModuleDesc, "d", "", "Description of the module")
	cmd.Flags().StringP(FlagGoModule, "g", "", "Go module path (e.g., github.com/myorg/my-defs)")
	cmd.Flags().BoolP(FlagWithExamples, "e", false, "Include example definitions")
	cmd.Flags().String(FlagComponents, "", "Comma-separated component names to scaffold (e.g., webservice,worker)")
	cmd.Flags().String(FlagTraits, "", "Comma-separated trait names to scaffold (e.g., autoscaler,ingress)")
	cmd.Flags().String(FlagPolicies, "", "Comma-separated policy names to scaffold (e.g., topology,override)")
	cmd.Flags().String(FlagWorkflowSteps, "", "Comma-separated workflow step names to scaffold (e.g., deploy,notify)")

	return cmd
}

// parseCommaSeparated splits a comma-separated string into a slice of trimmed strings
func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// scaffoldOptions contains names of definitions to scaffold
type scaffoldOptions struct {
	components    []string
	traits        []string
	policies      []string
	workflowsteps []string
}

// hasAny returns true if any scaffold options are specified
func (s scaffoldOptions) hasAny() bool {
	return len(s.components) > 0 || len(s.traits) > 0 || len(s.policies) > 0 || len(s.workflowsteps) > 0
}

// initModuleOptions contains options for module initialization
type initModuleOptions struct {
	name         string
	description  string
	goModule     string
	withExamples bool
	scaffold     scaffoldOptions
}

// initModule initializes a new definition module
func initModule(streams util.IOStreams, targetDir string, opts initModuleOptions) error {
	// Create target directory if it doesn't exist
	absPath, err := filepath.Abs(targetDir)
	if err != nil {
		return errors.Wrapf(err, "failed to resolve path %s", targetDir)
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory %s", absPath)
	}

	// Derive module name from directory if not specified
	if opts.name == "" {
		opts.name = filepath.Base(absPath)
	}

	// Derive Go module path if not specified
	if opts.goModule == "" {
		opts.goModule = "github.com/myorg/" + opts.name
	}

	streams.Infof("Initializing definition module in %s...\n", absPath)

	// Create directory structure
	dirs := []string{
		"components",
		"traits",
		"policies",
		"workflowsteps",
		"cmd/register",
	}

	for _, dir := range dirs {
		dirPath := filepath.Join(absPath, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return errors.Wrapf(err, "failed to create directory %s", dirPath)
		}
		streams.Infof("  Created %s/\n", dir)
	}

	// Create doc.go files for each definition package
	// This ensures go mod tidy works correctly by making each directory a valid Go package
	packageDocs := map[string]string{
		"components": `// Package components contains KubeVela ComponentDefinition implementations.
// Each component defines a workload type that can be used in Applications.
//
// To create a new component:
//
//	func init() {
//	    defkit.Register(MyComponent())
//	}
//
//	func MyComponent() *defkit.ComponentDefinition {
//	    return defkit.NewComponent("my-component").
//	        Description("My component description").
//	        Workload("apps/v1", "Deployment").
//	        // ... configuration
//	}
package components
`,
		"traits": `// Package traits contains KubeVela TraitDefinition implementations.
// Traits modify or enhance components with additional capabilities.
//
// To create a new trait:
//
//	func init() {
//	    defkit.Register(MyTrait())
//	}
//
//	func MyTrait() *defkit.TraitDefinition {
//	    return defkit.NewTrait("my-trait").
//	        Description("My trait description").
//	        AppliesToWorkloads("deployments.apps").
//	        // ... configuration
//	}
package traits
`,
		"policies": `// Package policies contains KubeVela PolicyDefinition implementations.
// Policies define application-level behaviors and constraints.
//
// To create a new policy:
//
//	func init() {
//	    defkit.Register(MyPolicy())
//	}
//
//	func MyPolicy() *defkit.PolicyDefinition {
//	    return defkit.NewPolicy("my-policy").
//	        Description("My policy description").
//	        // ... configuration
//	}
package policies
`,
		"workflowsteps": `// Package workflowsteps contains KubeVela WorkflowStepDefinition implementations.
// Workflow steps define actions that can be executed in application workflows.
//
// To create a new workflow step:
//
//	func init() {
//	    defkit.Register(MyStep())
//	}
//
//	func MyStep() *defkit.WorkflowStepDefinition {
//	    return defkit.NewWorkflowStep("my-step").
//	        Description("My workflow step description").
//	        // ... configuration
//	}
package workflowsteps
`,
	}

	for pkg, doc := range packageDocs {
		docPath := filepath.Join(absPath, pkg, "doc.go")
		if err := os.WriteFile(docPath, []byte(doc), 0644); err != nil {
			return errors.Wrapf(err, "failed to write %s/doc.go", pkg)
		}
	}

	// Create module.yaml
	moduleYAML := generateModuleYAML(opts)
	if err := os.WriteFile(filepath.Join(absPath, "module.yaml"), []byte(moduleYAML), 0644); err != nil {
		return errors.Wrapf(err, "failed to write module.yaml")
	}
	streams.Infof("  Created module.yaml\n")

	// Create go.mod
	goMod := generateGoMod(opts)
	if err := os.WriteFile(filepath.Join(absPath, "go.mod"), []byte(goMod), 0644); err != nil {
		return errors.Wrapf(err, "failed to write go.mod")
	}
	streams.Infof("  Created go.mod\n")

	// Create cmd/register/main.go - the registry entry point
	registerMain := generateRegisterMain(opts)
	if err := os.WriteFile(filepath.Join(absPath, "cmd/register/main.go"), []byte(registerMain), 0644); err != nil {
		return errors.Wrapf(err, "failed to write cmd/register/main.go")
	}
	streams.Infof("  Created cmd/register/main.go\n")

	// Create README.md
	readme := generateModuleReadme(opts)
	if err := os.WriteFile(filepath.Join(absPath, "README.md"), []byte(readme), 0644); err != nil {
		return errors.Wrapf(err, "failed to write README.md")
	}
	streams.Infof("  Created README.md\n")

	// Create scaffolded definitions if specified
	if opts.scaffold.hasAny() {
		if err := createScaffoldedDefinitions(streams, absPath, opts); err != nil {
			return errors.Wrap(err, "failed to create scaffolded definitions")
		}
	}

	// Create example definitions if requested
	if opts.withExamples {
		if err := createExampleDefinitions(streams, absPath, opts); err != nil {
			return errors.Wrap(err, "failed to create example definitions")
		}
	}

	// Create .gitignore
	gitignore := `# Binaries
*.exe
*.dll
*.so
*.dylib

# Test binary
*.test

# Output
*.out

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db
`
	if err := os.WriteFile(filepath.Join(absPath, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return errors.Wrapf(err, "failed to write .gitignore")
	}
	streams.Infof("  Created .gitignore\n")

	streams.Infof("\nModule initialized successfully!\n\n")
	streams.Infof("Next steps:\n")
	streams.Infof("  1. Add your definitions to the appropriate directories\n")
	streams.Infof("  2. Run 'go mod tidy' to download dependencies\n")
	streams.Infof("  3. Test locally: vela def apply-module %s --dry-run\n", targetDir)
	streams.Infof("  4. Validate: vela def validate-module %s\n", targetDir)
	streams.Infof("  5. List definitions: vela def list-module %s\n", targetDir)

	return nil
}

// generateRegisterMain generates the cmd/register/main.go content
func generateRegisterMain(opts initModuleOptions) string {
	return fmt.Sprintf(`/*
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

// Package main is the entry point for the definition registry.
// It outputs all registered definitions as JSON for CLI consumption.
//
// Usage: go run ./cmd/register
//
// Each definition package (components, traits, policies, workflowsteps)
// registers its definitions via init() functions that call defkit.Register().
// Importing those packages triggers registration automatically.
package main

import (
	"fmt"
	"os"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"

	// Import packages to trigger init() registration
	_ "%s/components"
	_ "%s/traits"
	_ "%s/policies"
	_ "%s/workflowsteps"
)

func main() {
	output, err := defkit.ToJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to serialize registry: %%v\n", err)
		os.Exit(1)
	}
	fmt.Print(string(output))
}
`, opts.goModule, opts.goModule, opts.goModule, opts.goModule)
}

// createScaffoldedDefinitions creates scaffolded definition files
func createScaffoldedDefinitions(streams util.IOStreams, basePath string, opts initModuleOptions) error {
	// Create component scaffolds
	for _, name := range opts.scaffold.components {
		content := generateComponentScaffold(name)
		filename := toSnakeCase(name) + ".go"
		if err := os.WriteFile(filepath.Join(basePath, "components", filename), []byte(content), 0644); err != nil {
			return errors.Wrapf(err, "failed to write components/%s", filename)
		}
		streams.Infof("  Created components/%s\n", filename)
	}

	// Create trait scaffolds
	for _, name := range opts.scaffold.traits {
		content := generateTraitScaffold(name)
		filename := toSnakeCase(name) + ".go"
		if err := os.WriteFile(filepath.Join(basePath, "traits", filename), []byte(content), 0644); err != nil {
			return errors.Wrapf(err, "failed to write traits/%s", filename)
		}
		streams.Infof("  Created traits/%s\n", filename)
	}

	// Create policy scaffolds
	for _, name := range opts.scaffold.policies {
		content := generatePolicyScaffold(name)
		filename := toSnakeCase(name) + ".go"
		if err := os.WriteFile(filepath.Join(basePath, "policies", filename), []byte(content), 0644); err != nil {
			return errors.Wrapf(err, "failed to write policies/%s", filename)
		}
		streams.Infof("  Created policies/%s\n", filename)
	}

	// Create workflowstep scaffolds
	for _, name := range opts.scaffold.workflowsteps {
		content := generateWorkflowStepScaffold(name)
		filename := toSnakeCase(name) + ".go"
		if err := os.WriteFile(filepath.Join(basePath, "workflowsteps", filename), []byte(content), 0644); err != nil {
			return errors.Wrapf(err, "failed to write workflowsteps/%s", filename)
		}
		streams.Infof("  Created workflowsteps/%s\n", filename)
	}

	return nil
}

// toSnakeCase converts a name to snake_case for filenames
func toSnakeCase(name string) string {
	// Simple conversion: replace hyphens with underscores, lowercase
	result := strings.ToLower(name)
	result = strings.ReplaceAll(result, "-", "_")
	return result
}

// toCamelCase converts a name to CamelCase for Go exported function names
func toCamelCase(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, "")
}

// toLowerCamelCase converts a name to lowerCamelCase for Go unexported function names
func toLowerCamelCase(name string) string {
	camel := toCamelCase(name)
	if len(camel) == 0 {
		return camel
	}
	return strings.ToLower(camel[:1]) + camel[1:]
}

// generateComponentScaffold generates a component definition scaffold
func generateComponentScaffold(name string) string {
	funcName := toCamelCase(name)
	return fmt.Sprintf(`/*
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

package components

import (
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

func init() {
	defkit.Register(%s())
}

// %s creates the %s component definition.
func %s() *defkit.ComponentDefinition {
	// Define parameters using the defkit fluent API
	// TODO: Add your component parameters here

	return defkit.NewComponent("%s").
		Description("TODO: Add description for %s component").
		Workload("apps/v1", "Deployment").
		// TODO: Add parameters and template
		Template(%sTemplate)
}

// %sTemplate defines the template function for %s.
func %sTemplate(tpl *defkit.Template) {
	// TODO: Implement component output logic
	// Example:
	// image := defkit.String("image")
	// replicas := defkit.Int("replicas")
	// deployment := defkit.NewResource("apps/v1", "Deployment").
	//     Set("spec.replicas", replicas).
	//     Set("spec.template.spec.containers[0].image", image)
	// tpl.Output(deployment)
}
`, funcName, funcName, name, funcName, name, name, toLowerCamelCase(name), toLowerCamelCase(name), name, toLowerCamelCase(name))
}

// generateTraitScaffold generates a trait definition scaffold
func generateTraitScaffold(name string) string {
	funcName := toCamelCase(name)
	return fmt.Sprintf(`/*
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

package traits

import (
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

func init() {
	defkit.Register(%s())
}

// %s creates the %s trait definition.
func %s() *defkit.TraitDefinition {
	// Define parameters using the defkit fluent API
	// TODO: Add your trait parameters here

	return defkit.NewTrait("%s").
		Description("TODO: Add description for %s trait").
		AppliesTo("deployments.apps", "statefulsets.apps").
		// TODO: Add parameters and template
		Template(%sTemplate)
}

// %sTemplate defines the template function for %s.
func %sTemplate(tpl *defkit.Template) {
	// TODO: Implement trait patch logic
	// Example:
	// labels := defkit.Object("labels")
	// patch := defkit.NewPatch().
	//     SpreadIf(labels.IsSet(), "spec.template.metadata.labels", labels)
	// tpl.PatchOutput(patch)
}
`, funcName, funcName, name, funcName, name, name, toLowerCamelCase(name), toLowerCamelCase(name), name, toLowerCamelCase(name))
}

// generatePolicyScaffold generates a policy definition scaffold
func generatePolicyScaffold(name string) string {
	funcName := toCamelCase(name)
	return fmt.Sprintf(`/*
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

package policies

import (
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

func init() {
	defkit.Register(%s())
}

// %s creates the %s policy definition.
func %s() *defkit.PolicyDefinition {
	// Define parameters using the defkit fluent API
	// TODO: Add your policy parameters here

	return defkit.NewPolicy("%s").
		Description("TODO: Add description for %s policy")
		// TODO: Add parameters and implementation
}
`, funcName, funcName, name, funcName, name, name)
}

// generateWorkflowStepScaffold generates a workflow step definition scaffold
func generateWorkflowStepScaffold(name string) string {
	funcName := toCamelCase(name)
	return fmt.Sprintf(`/*
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

package workflowsteps

import (
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

func init() {
	defkit.Register(%s())
}

// %s creates the %s workflow step definition.
func %s() *defkit.WorkflowStepDefinition {
	// Define parameters using the defkit fluent API
	// TODO: Add your workflow step parameters here

	return defkit.NewWorkflowStep("%s").
		Description("TODO: Add description for %s workflow step")
		// TODO: Add parameters and implementation
}
`, funcName, funcName, name, funcName, name, name)
}

// generateModuleYAML generates the module.yaml content
func generateModuleYAML(opts initModuleOptions) string {
	desc := opts.description
	if desc == "" {
		desc = "KubeVela definition module"
	}

	return fmt.Sprintf(`# KubeVela Definition Module
# This file contains metadata about your definition module.
# See: https://kubevela.io/docs/platform-engineers/definition-module
#
# Note: Version is automatically derived from git tags.
# Use 'git tag v1.0.0' to set the version.

apiVersion: core.oam.dev/v1beta1
kind: DefinitionModule
metadata:
  name: %s
spec:
  description: %s
  maintainers:
    - name: Your Name
      email: your.email@example.com
  # Minimum KubeVela version required (optional)
  # minVelaVersion: v1.9.0
  # Categories for organization (optional)
  categories:
    - custom
  # Dependencies on other modules (optional)
  # dependencies:
  #   - module: github.com/other/module
  #     version: v1.0.0
  # Placement constraints for cluster-aware deployment (optional)
  # Definitions in this module will only be applied to clusters matching these conditions.
  # See: https://kubevela.io/docs/platform-engineers/definition-placement
  # placement:
  #   runOn:
  #     - key: provider
  #       operator: Eq
  #       values: ["aws"]
  #     - key: environment
  #       operator: In
  #       values: ["production", "staging"]
  #   notRunOn:
  #     - key: cluster-type
  #       operator: Eq
  #       values: ["vcluster"]
`, opts.name, desc)
}

// generateGoMod generates the go.mod content
func generateGoMod(opts initModuleOptions) string {
	return fmt.Sprintf(`module %s

go 1.23.8

require github.com/oam-dev/kubevela v1.9.0
`, opts.goModule)
}

// generateModuleReadme generates the README.md content
func generateModuleReadme(opts initModuleOptions) string {
	desc := opts.description
	if desc == "" {
		desc = "A collection of KubeVela X-Definitions."
	}

	return fmt.Sprintf(`# %s

%s

## Overview

This module contains Go-based KubeVela X-Definitions that can be applied to any KubeVela cluster.

## Directory Structure

- **components/** - ComponentDefinitions for workload types
- **traits/** - TraitDefinitions for operational behaviors
- **policies/** - PolicyDefinitions for application policies
- **workflowsteps/** - WorkflowStepDefinitions for delivery workflows

## Usage

### Apply all definitions

`+"```bash"+`
vela def apply-module %s
`+"```"+`

### List definitions

`+"```bash"+`
vela def list-module %s
`+"```"+`

### Validate definitions

`+"```bash"+`
vela def validate-module %s
`+"```"+`

### Apply with namespace

`+"```bash"+`
vela def apply-module %s --namespace my-namespace
`+"```"+`

### Dry-run (preview without applying)

`+"```bash"+`
vela def apply-module %s --dry-run
`+"```"+`

## Adding New Definitions

1. Create a new Go file in the appropriate directory
2. Add an init() function that registers your definition
3. Use the defkit package fluent API to define your component/trait/policy/workflow-step
4. Run `+"`go mod tidy`"+` to update dependencies
5. Validate with `+"`vela def validate-module .`"+`

Example component definition:

`+"```go"+`
package components

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func init() {
    defkit.Register(MyComponent())
}

func MyComponent() *defkit.ComponentDefinition {
    image := defkit.String("image").Required().Description("Container image")
    replicas := defkit.Int("replicas").Default(1).Description("Number of replicas")

    return defkit.NewComponent("my-component").
        Description("My custom component").
        Workload("apps/v1", "Deployment").
        Params(image, replicas).
        Template(myComponentTemplate)
}

func myComponentTemplate(tpl *defkit.Template) {
    vela := defkit.VelaCtx()
    image := defkit.String("image")
    replicas := defkit.Int("replicas")

    deployment := defkit.NewResource("apps/v1", "Deployment").
        Set("spec.replicas", replicas).
        Set("spec.selector.matchLabels[app.oam.dev/component]", vela.Name()).
        Set("spec.template.spec.containers[0].name", vela.Name()).
        Set("spec.template.spec.containers[0].image", image)

    tpl.Output(deployment)
}
`+"```"+`

## Version

Version is automatically derived from git tags. Use semantic versioning tags (e.g., v1.0.0) to set the module version.

## License

Apache License 2.0
`, opts.name, desc, opts.goModule, opts.goModule, opts.goModule, opts.goModule, opts.goModule)
}

// createExampleDefinitions creates example definition files
func createExampleDefinitions(streams util.IOStreams, basePath string, opts initModuleOptions) error {
	// Example component - using correct defkit API
	componentExample := `/*
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

// Package components contains component definitions.
// This demonstrates how to create a simple component definition using the defkit package.
package components

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func init() {
	defkit.Register(ExampleComponent())
}

// ExampleComponent creates a simple container-based component.
func ExampleComponent() *defkit.ComponentDefinition {
	// Define parameters using the defkit fluent API
	image := defkit.String("image").Required().Description("Container image to run")
	port := defkit.Int("port").Default(80).Description("Container port")
	replicas := defkit.Int("replicas").Default(1).Description("Number of replicas")

	return defkit.NewComponent("example-component").
		Description("An example component definition").
		Workload("apps/v1", "Deployment").
		CustomStatus(defkit.DeploymentStatus().Build()).
		HealthPolicy(defkit.DeploymentHealth().Build()).
		Params(image, port, replicas).
		Template(exampleComponentTemplate)
}

// exampleComponentTemplate defines the template function for example-component.
func exampleComponentTemplate(tpl *defkit.Template) {
	vela := defkit.VelaCtx()
	image := defkit.String("image")
	port := defkit.Int("port")
	replicas := defkit.Int("replicas")

	// Primary output: Deployment
	deployment := defkit.NewResource("apps/v1", "Deployment").
		Set("spec.replicas", replicas).
		Set("spec.selector.matchLabels[app.oam.dev/component]", vela.Name()).
		Set("spec.template.metadata.labels[app.oam.dev/name]", vela.AppName()).
		Set("spec.template.metadata.labels[app.oam.dev/component]", vela.Name()).
		Set("spec.template.spec.containers[0].name", vela.Name()).
		Set("spec.template.spec.containers[0].image", image).
		Set("spec.template.spec.containers[0].ports[0].containerPort", port)

	tpl.Output(deployment)
}
`
	if err := os.WriteFile(filepath.Join(basePath, "components", "example.go"), []byte(componentExample), 0644); err != nil {
		return errors.Wrap(err, "failed to write example component")
	}
	streams.Infof("  Created components/example.go\n")

	// Example trait - using correct defkit API
	traitExample := `/*
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

// Package traits contains trait definitions.
// This demonstrates how to create a simple trait definition using the defkit package.
package traits

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func init() {
	defkit.Register(ExampleLabels())
}

// ExampleLabels creates a simple labels trait.
func ExampleLabels() *defkit.TraitDefinition {
	// Define parameters using the defkit fluent API
	labels := defkit.StringKeyMap("labels").Description("Labels to add to the workload")

	return defkit.NewTrait("example-labels").
		Description("Adds custom labels to the workload").
		AppliesTo("deployments.apps", "statefulsets.apps").
		Params(labels).
		Template(exampleLabelsTemplate)
}

// exampleLabelsTemplate defines the template function for example-labels.
func exampleLabelsTemplate(tpl *defkit.Template) {
	labels := defkit.Object("labels")

	// Patch output: add labels to pod template
	patch := defkit.NewPatch().
		SpreadIf(labels.IsSet(), "spec.template.metadata.labels", labels)

	tpl.PatchOutput(patch)
}
`
	if err := os.WriteFile(filepath.Join(basePath, "traits", "example.go"), []byte(traitExample), 0644); err != nil {
		return errors.Wrap(err, "failed to write example trait")
	}
	streams.Infof("  Created traits/example.go\n")

	return nil
}

// FlagOutputDir is the flag for output directory
const FlagOutputDir = "output"

// NewDefinitionGenModuleCommand creates the `vela def gen-module` command
// to generate CUE code from Go definitions in a module
func NewDefinitionGenModuleCommand(_ common.Args, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gen-module",
		Short: "Generate CUE code from Go definitions in a module.",
		Long: `Generate CUE code from all Go definitions in a module.

This command loads a Go definition module, compiles all definitions to CUE,
and writes the generated CUE files to an output directory organized by type.

Output structure:
  <output>/
    components/
      webservice.cue
      worker.cue
    traits/
      scaler.cue
    policies/
      topology.cue
    workflowsteps/
      deploy.cue`,
		Example: `# Generate CUE from a local module (output to ./cue-generated)
> vela def gen-module ./my-definitions

# Generate CUE to a specific output directory
> vela def gen-module ./my-definitions -o ./generated-cue

# Generate only specific definition types
> vela def gen-module ./my-definitions -o ./output --types component,trait

# Generate from a remote Go module
> vela def gen-module github.com/myorg/definitions@v1.0.0 -o ./output`,
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeDefModule,
			types.TagCommandOrder: "5",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			outputDir, err := cmd.Flags().GetString(FlagOutputDir)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagOutputDir)
			}

			version, err := cmd.Flags().GetString(FlagModuleVersion)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagModuleVersion)
			}

			typesStr, err := cmd.Flags().GetString(FlagModuleTypes)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagModuleTypes)
			}

			var defTypes []string
			if typesStr != "" {
				defTypes = strings.Split(typesStr, ",")
				for i := range defTypes {
					defTypes[i] = strings.TrimSpace(defTypes[i])
				}
			}

			return genModule(ctx, streams, args[0], genModuleOptions{
				outputDir: outputDir,
				version:   version,
				types:     defTypes,
			})
		},
	}

	cmd.Flags().StringP(FlagOutputDir, "o", "cue-generated", "Output directory for generated CUE files")
	cmd.Flags().StringP(FlagModuleVersion, "v", "", "Version of the module (for remote modules)")
	cmd.Flags().StringP(FlagModuleTypes, "t", "", "Comma-separated list of definition types to generate (component,trait,policy,workflow-step)")

	return cmd
}

// genModuleOptions contains options for generating CUE from a module
type genModuleOptions struct {
	outputDir string
	version   string
	types     []string
}

// genModule loads a module and generates CUE files for all definitions
func genModule(ctx context.Context, streams util.IOStreams, moduleRef string, opts genModuleOptions) error {
	// Load module options
	loadOpts := goloader.ModuleLoadOptions{
		Version:             opts.version,
		Types:               opts.types,
		ResolveDependencies: true,
	}
	if len(opts.types) == 0 {
		loadOpts = goloader.DefaultModuleLoadOptions()
		loadOpts.Version = opts.version
	}

	streams.Infof("Loading module from %s...\n", moduleRef)

	// Load the module
	module, err := goloader.LoadModule(ctx, moduleRef, loadOpts)
	if err != nil {
		return errors.Wrapf(err, "failed to load module from %s", moduleRef)
	}

	// Print module summary
	streams.Infof("\n%s\n", module.Summary())

	if len(module.Definitions) == 0 {
		streams.Infof("No definitions found in module.\n")
		return nil
	}

	// Resolve output directory
	absOutputDir, err := filepath.Abs(opts.outputDir)
	if err != nil {
		return errors.Wrapf(err, "failed to resolve output path %s", opts.outputDir)
	}

	// Create output directory structure
	typeDirs := map[string]string{
		"component":     "components",
		"trait":         "traits",
		"policy":        "policies",
		"workflow-step": "workflowsteps",
	}

	for _, dir := range typeDirs {
		dirPath := filepath.Join(absOutputDir, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return errors.Wrapf(err, "failed to create directory %s", dirPath)
		}
	}

	streams.Infof("Generating CUE files to %s...\n\n", absOutputDir)

	// Track results
	var generated, failed int
	var failedDefs []string

	// Generate CUE for each definition
	for _, result := range module.Definitions {
		if result.Error != nil {
			failed++
			failedDefs = append(failedDefs, fmt.Sprintf("%s: %v", result.Definition.FilePath, result.Error))
			continue
		}

		// Determine output directory based on definition type
		typeDir, ok := typeDirs[result.Definition.Type]
		if !ok {
			typeDir = result.Definition.Type + "s" // fallback
		}

		// Generate filename from definition name
		filename := result.Definition.Name + ".cue"
		outputPath := filepath.Join(absOutputDir, typeDir, filename)

		// Write CUE content
		if err := os.WriteFile(outputPath, []byte(result.CUE), 0644); err != nil {
			failed++
			failedDefs = append(failedDefs, fmt.Sprintf("%s: failed to write: %v", result.Definition.Name, err))
			continue
		}

		streams.Infof("  Generated %s/%s\n", typeDir, filename)
		generated++
	}

	// Print summary
	streams.Infof("\nGeneration complete:\n")
	streams.Infof("  Generated: %d\n", generated)
	if failed > 0 {
		streams.Infof("  Failed:    %d\n", failed)
		for _, f := range failedDefs {
			streams.Infof("    - %s\n", f)
		}
		return errors.Errorf("%d definitions failed to generate", failed)
	}

	streams.Infof("\nOutput directory: %s\n", absOutputDir)
	return nil
}
