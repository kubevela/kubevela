/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/application"
	velacommon "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	// Output format constants
	outputFormatTable   = "table"
	outputFormatJSON    = "json"
	outputFormatYAML    = "yaml"
	outputFormatSummary = "summary"
	outputFormatDiff    = "diff"
)

// PolicyCommandGroup creates the `policy` command group
func PolicyCommandGroup(c velacommon.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Manage and debug Application-scoped policies.",
		Long:  "Commands for viewing and testing Application-scoped PolicyDefinitions (both global and explicit) applied to Applications.",
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeApp,
			types.TagCommandOrder: order,
		},
	}

	cmd.AddCommand(
		NewPolicyViewCommand(c, ioStreams),
		NewPolicyDryRunCommand(c, ioStreams),
	)

	return cmd
}

// NewPolicyViewCommand creates the `vela policy view` command
func NewPolicyViewCommand(c velacommon.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "view <app-name>",
		Short: "View applied Application-scoped policies and their effects.",
		Long:  "View which Application-scoped policies (global and explicit) were applied to an Application and what changes they made.",
		Example: `  # View policies applied to an Application
  vela policy view my-app

  # View in JSON format
  vela policy view my-app --output json

  # View in YAML format
  vela policy view my-app --output yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName := args[0]
			namespace, err := cmd.Flags().GetString("namespace")
			if err != nil {
				return err
			}
			if namespace == "" {
				namespace = "default"
			}

			ctx := context.Background()
			return runPolicyView(ctx, c, appName, namespace, outputFormat, ioStreams)
		},
	}

	addNamespaceAndEnvArg(cmd)
	cmd.Flags().StringVarP(&outputFormat, "output", "o", outputFormatTable, "Output format: table, json, yaml")

	return cmd
}

// NewPolicyDryRunCommand creates the `vela policy dry-run` command
func NewPolicyDryRunCommand(c velacommon.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var (
		outputFormat          string
		policies              []string
		includeGlobalPolicies bool
		includeAppPolicies    bool
	)

	cmd := &cobra.Command{
		Use:   "dry-run <app-name>",
		Short: "Preview policy effects before applying.",
		Long: `Simulate policy application to see what changes would be made.

Modes:
  - Isolated (--policies only): Test specified policies in isolation
  - Additive (--policies + --include-global-policies): Test with existing globals
  - Full (no --policies): Simulate complete policy chain
  - Full + Extra (--policies + both flags): Everything plus test policies`,
		Example: `  # Full simulation (all policies that would apply)
  vela policy dry-run my-app

  # Test specific policy in isolation
  vela policy dry-run my-app --policies inject-sidecar

  # Test new policy with existing globals
  vela policy dry-run my-app --policies new-policy --include-global-policies

  # Show only metadata changes (labels, annotations, context)
  vela policy dry-run my-app --output summary

  # Show unified diff
  vela policy dry-run my-app --output diff

  # JSON output for CI/CD
  vela policy dry-run my-app --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName := args[0]
			namespace, err := cmd.Flags().GetString("namespace")
			if err != nil {
				return err
			}
			if namespace == "" {
				namespace = "default"
			}

			ctx := context.Background()
			return runPolicyDryRun(ctx, c, appName, namespace, policies, includeGlobalPolicies, includeAppPolicies, outputFormat, ioStreams)
		},
	}

	addNamespaceAndEnvArg(cmd)
	cmd.Flags().StringVarP(&outputFormat, "output", "o", outputFormatTable, "Output format: table, summary, diff, json, yaml")
	cmd.Flags().StringSliceVar(&policies, "policies", nil, "Comma-separated list of policies to test")
	cmd.Flags().BoolVar(&includeGlobalPolicies, "include-global-policies", false, "Include existing global policies")
	cmd.Flags().BoolVar(&includeAppPolicies, "include-app-policies", false, "Include policies from Application spec")

	return cmd
}

// runPolicyView implements the view command logic
func runPolicyView(ctx context.Context, c velacommon.Args, appName, namespace, outputFormat string, ioStreams cmdutil.IOStreams) error {
	cli, err := c.GetClient()
	if err != nil {
		return err
	}

	// Get the Application
	app := &v1beta1.Application{}
	if err := cli.Get(ctx, client.ObjectKey{Name: appName, Namespace: namespace}, app); err != nil {
		return errors.Wrapf(err, "failed to get Application %s/%s", namespace, appName)
	}

	// Check if any policies were applied
	if len(app.Status.AppliedApplicationPolicies) == 0 {
		ioStreams.Info(fmt.Sprintf("No Application-scoped policies applied to Application '%s'\n", appName))
		ioStreams.Info("\nThis could be because:\n")
		ioStreams.Info("  • No global policies exist in vela-system or the application namespace\n")
		ioStreams.Info("  • No explicit Application-scoped policies in Application spec\n")
		ioStreams.Info("  • Application has annotation: policy.oam.dev/skip-global: \"true\"\n")
		ioStreams.Info("  • Global policies feature is disabled (feature gate not enabled)\n")
		return nil
	}

	// Get the diffs ConfigMap if it exists
	var diffsConfigMap *corev1.ConfigMap
	if app.Status.ApplicationPoliciesConfigMap != "" {
		cm := &corev1.ConfigMap{}
		err := cli.Get(ctx, client.ObjectKey{Name: app.Status.ApplicationPoliciesConfigMap, Namespace: namespace}, cm)
		if err == nil {
			diffsConfigMap = cm
		}
	}

	// Display based on output format
	switch outputFormat {
	case outputFormatJSON:
		return outputPolicyViewJSON(app, diffsConfigMap, ioStreams)
	case outputFormatYAML:
		return outputPolicyViewYAML(app, diffsConfigMap, ioStreams)
	case outputFormatTable:
		return outputPolicyViewTable(app, diffsConfigMap, ioStreams)
	default:
		return fmt.Errorf("unknown output format: %s (supported: table, json, yaml)", outputFormat)
	}
}

// outputPolicyViewTable displays policy view in interactive table format
func outputPolicyViewTable(app *v1beta1.Application, diffsConfigMap *corev1.ConfigMap, ioStreams cmdutil.IOStreams) error {
	policies := app.Status.AppliedApplicationPolicies

	// Count applied vs skipped
	applied := 0
	skipped := 0
	for _, p := range policies {
		if p.Applied {
			applied++
		} else {
			skipped++
		}
	}

	// Display header
	ioStreams.Info(fmt.Sprintf("Applied Application Policies: %s applied, %s skipped\n\n",
		color.GreenString("%d", applied),
		color.YellowString("%d", skipped)))

	// Create table
	table := tablewriter.NewWriter(ioStreams.Out)
	table.SetHeader([]string{"Seq", "Policy", "Namespace", "Source", "Priority", "Applied", "Spec", "Labels", "Annot.", "Context"})
	table.SetBorder(true)
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, policy := range policies {
		seq := "-"
		source := "-"
		priority := "-"
		specChanges := "No"
		labels := "0"
		annot := "0"
		ctx := "No"

		if policy.Applied {
			// Try to get sequence, source, priority from ConfigMap if available
			if diffsConfigMap != nil {
				for key, value := range diffsConfigMap.Data {
					if strings.HasSuffix(key, "-"+policy.Name) {
						var record map[string]interface{}
						if err := json.Unmarshal([]byte(value), &record); err == nil {
							if seqVal, ok := record["sequence"].(float64); ok {
								seq = fmt.Sprintf("%.0f", seqVal)
							}
							if srcVal, ok := record["source"].(string); ok {
								source = srcVal
							}
							if priVal, ok := record["priority"].(float64); ok {
								priority = fmt.Sprintf("%.0f", priVal)
							}
						}
						break
					}
				}
			}

			// Spec changes (from status summary)
			if policy.SpecModified {
				specChanges = "Yes"
			}

			// Labels count (from status summary)
			labels = fmt.Sprintf("%d", policy.LabelsCount)

			// Annotations count (from status summary)
			annot = fmt.Sprintf("%d", policy.AnnotationsCount)

			// Context (from status summary)
			if policy.HasContext {
				ctx = "Yes"
			}
		}

		appliedStatus := "Yes"
		if !policy.Applied {
			appliedStatus = "No"
		}

		table.Append([]string{
			seq,
			policy.Name,
			policy.Namespace,
			source,
			priority,
			appliedStatus,
			specChanges,
			labels,
			annot,
			ctx,
		})
	}

	table.Render()

	// Show skipped policies
	var skippedPolicies []common.AppliedApplicationPolicy
	for _, p := range policies {
		if !p.Applied {
			skippedPolicies = append(skippedPolicies, p)
		}
	}

	if len(skippedPolicies) > 0 {
		ioStreams.Info(fmt.Sprintf("\nSkipped (%d):\n", len(skippedPolicies)))
		for _, p := range skippedPolicies {
			ioStreams.Info(fmt.Sprintf("  • %s: %s\n", p.Name, p.Reason))
		}
	}

	// Show summary (using status summary counts)
	totalLabels := 0
	totalAnnotations := 0
	specModCount := 0
	contextCount := 0

	for _, p := range policies {
		if p.Applied {
			totalLabels += p.LabelsCount
			totalAnnotations += p.AnnotationsCount
			if p.SpecModified {
				specModCount++
			}
			if p.HasContext {
				contextCount++
			}
		}
	}

	ioStreams.Info("\nSummary:\n")
	ioStreams.Info(fmt.Sprintf("  Total Policies: %d (%d applied, %d skipped)\n", len(policies), applied, skipped))
	ioStreams.Info(fmt.Sprintf("  Spec Changes:   %d policies\n", specModCount))
	ioStreams.Info(fmt.Sprintf("  Labels Added:   %d total\n", totalLabels))
	ioStreams.Info(fmt.Sprintf("  Annotations:    %d total\n", totalAnnotations))
	ioStreams.Info(fmt.Sprintf("  Context Data:   %d policies\n", contextCount))

	if diffsConfigMap != nil {
		ioStreams.Info(fmt.Sprintf("\nView detailed diffs:\n  kubectl get configmap %s -o json | jq '.data'\n", app.Status.ApplicationPoliciesConfigMap))
	}

	return nil
}

// outputPolicyViewJSON outputs policy view in JSON format
func outputPolicyViewJSON(app *v1beta1.Application, diffsConfigMap *corev1.ConfigMap, ioStreams cmdutil.IOStreams) error {
	output := map[string]interface{}{
		"application":            app.Name,
		"namespace":              app.Namespace,
		"appliedPolicies":        app.Status.AppliedApplicationPolicies,
		"policyDiffsConfigMap":   app.Status.ApplicationPoliciesConfigMap,
	}

	// Add summary
	applied := 0
	skipped := 0
	specMod := 0
	totalLabels := 0
	totalAnnotations := 0

	for _, p := range app.Status.AppliedApplicationPolicies {
		if p.Applied {
			applied++
			if p.SpecModified {
				specMod++
			}
			totalLabels += p.LabelsCount
			totalAnnotations += p.AnnotationsCount
		} else {
			skipped++
		}
	}

	output["summary"] = map[string]interface{}{
		"totalDiscovered":    len(app.Status.AppliedApplicationPolicies),
		"applied":            applied,
		"skipped":            skipped,
		"specModifications":  specMod,
		"labelsAdded":        totalLabels,
		"annotationsAdded":   totalAnnotations,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal to JSON")
	}

	ioStreams.Info("%s\n", string(data))
	return nil
}

// outputPolicyViewYAML outputs policy view in YAML format
func outputPolicyViewYAML(app *v1beta1.Application, diffsConfigMap *corev1.ConfigMap, ioStreams cmdutil.IOStreams) error {
	output := map[string]interface{}{
		"application":            app.Name,
		"namespace":              app.Namespace,
		"appliedPolicies":        app.Status.AppliedApplicationPolicies,
		"policyDiffsConfigMap":   app.Status.ApplicationPoliciesConfigMap,
	}

	data, err := yaml.Marshal(output)
	if err != nil {
		return errors.Wrap(err, "failed to marshal to YAML")
	}

	ioStreams.Info("%s\n", string(data))
	return nil
}

// runPolicyDryRun implements the dry-run command logic
func runPolicyDryRun(ctx context.Context, c velacommon.Args, appName, namespace string, policies []string, includeGlobal, includeApp bool, outputFormat string, ioStreams cmdutil.IOStreams) error {
	cli, err := c.GetClient()
	if err != nil {
		return err
	}

	// Load the Application from cluster
	app := &v1beta1.Application{}
	if err := cli.Get(ctx, client.ObjectKey{Name: appName, Namespace: namespace}, app); err != nil {
		return errors.Wrapf(err, "failed to get Application %s/%s", namespace, appName)
	}

	// Determine dry-run mode based on flags
	var mode application.PolicyDryRunMode
	if len(policies) > 0 && !includeGlobal && !includeApp {
		mode = application.DryRunModeIsolated
	} else if len(policies) > 0 && includeGlobal {
		mode = application.DryRunModeAdditive
	} else {
		mode = application.DryRunModeFull
	}

	// Build simulation options
	opts := application.PolicyDryRunOptions{
		Mode:               mode,
		SpecifiedPolicies:  policies,
		IncludeAppPolicies: includeApp || mode == application.DryRunModeFull,
	}

	// Run simulation
	ioStreams.Info("Dry-run Simulation\n")
	ioStreams.Info("Application: %s (namespace: %s)\n", appName, namespace)

	modeStr := string(mode)
	switch mode {
	case application.DryRunModeIsolated:
		modeStr = "Isolated (testing specified policies only)"
	case application.DryRunModeAdditive:
		modeStr = "Additive (specified policies + existing global policies)"
	case application.DryRunModeFull:
		modeStr = "Full simulation (all policies that would apply)"
	}
	ioStreams.Info("Mode: %s\n\n", modeStr)

	result, err := application.SimulatePolicyApplication(ctx, cli, app, opts)
	if err != nil {
		return errors.Wrap(err, "simulation failed")
	}

	// Display results based on output format
	switch outputFormat {
	case outputFormatTable:
		return outputDryRunTable(result, ioStreams)
	case outputFormatSummary:
		return outputDryRunSummary(result, ioStreams)
	case outputFormatDiff:
		return outputDryRunDiff(result, ioStreams)
	case outputFormatJSON:
		return outputDryRunJSON(result, ioStreams)
	case outputFormatYAML:
		return outputDryRunYAML(result, ioStreams)
	default:
		return fmt.Errorf("unknown output format: %s", outputFormat)
	}
}
// outputDryRunTable outputs dry-run results in table format
func outputDryRunTable(result *application.PolicyDryRunResult, ioStreams cmdutil.IOStreams) error {
	// Show execution plan
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ioStreams.Info("Execution Plan\n")
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if len(result.ExecutionPlan) == 0 {
		ioStreams.Info("No policies to apply\n\n")
	} else {
		for _, step := range result.ExecutionPlan {
			ioStreams.Info("  %d. %-30s (%s, priority: %d) [%s]\n",
				step.Sequence, step.PolicyName, step.PolicyNamespace, step.Priority, step.Source)
		}
		ioStreams.Info("\n")
	}

	// Show warnings and errors
	if len(result.Warnings) > 0 {
		ioStreams.Info("%s:\n", color.YellowString("Warnings"))
		for _, w := range result.Warnings {
			ioStreams.Info("  • %s\n", w)
		}
		ioStreams.Info("\n")
	}

	if len(result.Errors) > 0 {
		ioStreams.Info("%s:\n", color.RedString("Errors"))
		for _, e := range result.Errors {
			ioStreams.Info("  • %s\n", e)
		}
		ioStreams.Info("\n")
		return nil // Don't continue if there were errors
	}

	// Apply policies
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ioStreams.Info("Applying Policies...\n")
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	for _, policyResult := range result.PolicyResults {
		ioStreams.Info("[%d/%d] %s\n", policyResult.Sequence, len(result.PolicyResults), policyResult.PolicyName)

		if policyResult.Error != "" {
			ioStreams.Info("  %s Error: %s\n\n", color.RedString("✗"), policyResult.Error)
			continue
		}

		if !policyResult.Enabled {
			ioStreams.Info("  %s Skipped: %s\n\n", color.YellowString("⊘"), policyResult.SkipReason)
			continue
		}

		ioStreams.Info("  %s Policy enabled\n", color.GreenString("✓"))
		ioStreams.Info("\n  Changes:\n")

		specChangesStr := "No"
		if policyResult.SpecModified {
			specChangesStr = fmt.Sprintf("Yes (see diffs)")
		}
		ioStreams.Info("    Spec Modified:     %s\n", specChangesStr)
		ioStreams.Info("    Labels Added:      %d\n", len(policyResult.AddedLabels))
		if len(policyResult.AddedLabels) > 0 {
			for k, v := range policyResult.AddedLabels {
				ioStreams.Info("      • %s: %s\n", k, v)
			}
		}
		ioStreams.Info("    Annotations Added: %d\n", len(policyResult.AddedAnnotations))
		if len(policyResult.AddedAnnotations) > 0 {
			for k, v := range policyResult.AddedAnnotations {
				ioStreams.Info("      • %s: %s\n", k, v)
			}
		}

		contextStr := "None"
		if policyResult.AdditionalContext != nil && len(policyResult.AdditionalContext.Raw) > 0 {
			var contextMap map[string]interface{}
			if err := json.Unmarshal(policyResult.AdditionalContext.Raw, &contextMap); err == nil {
				var keys []string
				for k := range contextMap {
					keys = append(keys, k)
				}
				contextStr = strings.Join(keys, ", ")
			}
		}
		ioStreams.Info("    Context Data:      %s\n", contextStr)
		ioStreams.Info("\n")
	}

	// Summary
	applied := 0
	skipped := 0
	specMod := 0
	totalLabels := 0
	totalAnnotations := 0

	for _, pr := range result.PolicyResults {
		if pr.Applied {
			applied++
			if pr.SpecModified {
				specMod++
			}
			totalLabels += len(pr.AddedLabels)
			totalAnnotations += len(pr.AddedAnnotations)
		} else {
			skipped++
		}
	}

	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ioStreams.Info("Summary\n")
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	ioStreams.Info("Policies Applied:       %d\n", applied)
	ioStreams.Info("Policies Skipped:       %d\n", skipped)
	ioStreams.Info("Spec Modifications:     %d\n", specMod)
	ioStreams.Info("Labels Added:           %d\n", totalLabels)
	ioStreams.Info("Annotations Added:      %d\n", totalAnnotations)
	ioStreams.Info("\n")

	// Final state
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ioStreams.Info("Final Application State\n")
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Show labels
	if len(result.Application.Labels) > 0 {
		ioStreams.Info("Labels (%d total):\n", len(result.Application.Labels))
		for k, v := range result.Application.Labels {
			// Find which policy added this label
			var policyName string
			for _, pr := range result.PolicyResults {
				if pr.AddedLabels != nil {
					if _, exists := pr.AddedLabels[k]; exists {
						policyName = pr.PolicyName
						break
					}
				}
			}
			if policyName != "" {
				ioStreams.Info("  %-30s %s  (%s)\n", k+":", v, policyName)
			} else {
				ioStreams.Info("  %-30s %s\n", k+":", v)
			}
		}
	} else {
		ioStreams.Info("Labels: (none)\n")
	}
	ioStreams.Info("\n")

	// Show annotations
	if len(result.Application.Annotations) > 0 {
		ioStreams.Info("Annotations (%d total):\n", len(result.Application.Annotations))
		for k, v := range result.Application.Annotations {
			var policyName string
			for _, pr := range result.PolicyResults {
				if pr.AddedAnnotations != nil {
					if _, exists := pr.AddedAnnotations[k]; exists {
						policyName = pr.PolicyName
						break
					}
				}
			}
			if policyName != "" {
				ioStreams.Info("  %-30s %s  (%s)\n", k+":", v, policyName)
			} else {
				ioStreams.Info("  %-30s %s\n", k+":", v)
			}
		}
	} else {
		ioStreams.Info("Annotations: (none)\n")
	}
	ioStreams.Info("\n")

	// Show application spec (YAML)
	ioStreams.Info("Application Spec:\n")
	specYAML, err := yaml.Marshal(result.Application.Spec)
	if err != nil {
		return errors.Wrap(err, "failed to marshal Application spec")
	}
	ioStreams.Info("%s\n", string(specYAML))

	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	ioStreams.Info("This is a dry-run. No changes were applied to the cluster.\n\n")

	return nil
}

// outputDryRunSummary outputs only metadata (labels, annotations, context)
func outputDryRunSummary(result *application.PolicyDryRunResult, ioStreams cmdutil.IOStreams) error {
	applied := 0
	skipped := 0
	for _, pr := range result.PolicyResults {
		if pr.Applied {
			applied++
		} else {
			skipped++
		}
	}

	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ioStreams.Info("Summary\n")
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	ioStreams.Info("Policies Applied:       %d\n", applied)
	ioStreams.Info("Policies Skipped:       %d\n", skipped)
	ioStreams.Info("\n")

	// Show labels table
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ioStreams.Info("Labels\n")
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if len(result.Application.Labels) > 0 {
		table := tablewriter.NewWriter(ioStreams.Out)
		table.SetHeader([]string{"Key", "Value", "Added By"})
		table.SetBorder(true)

		for k, v := range result.Application.Labels {
			var policyName string
			for _, pr := range result.PolicyResults {
				if pr.AddedLabels != nil {
					if _, exists := pr.AddedLabels[k]; exists {
						policyName = pr.PolicyName
						break
					}
				}
			}
			if policyName == "" {
				policyName = "(existing)"
			}
			table.Append([]string{k, v, policyName})
		}
		table.Render()
	} else {
		ioStreams.Info("  (none)\n")
	}
	ioStreams.Info("\n")

	// Show annotations table
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ioStreams.Info("Annotations\n")
	ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if len(result.Application.Annotations) > 0 {
		table := tablewriter.NewWriter(ioStreams.Out)
		table.SetHeader([]string{"Key", "Value", "Added By"})
		table.SetBorder(true)

		for k, v := range result.Application.Annotations {
			var policyName string
			for _, pr := range result.PolicyResults {
				if pr.AddedAnnotations != nil {
					if _, exists := pr.AddedAnnotations[k]; exists {
						policyName = pr.PolicyName
						break
					}
				}
			}
			if policyName == "" {
				policyName = "(existing)"
			}
			table.Append([]string{k, v, policyName})
		}
		table.Render()
	} else {
		ioStreams.Info("  (none)\n")
	}
	ioStreams.Info("\n")

	return nil
}

// outputDryRunDiff outputs unified diff format
func outputDryRunDiff(result *application.PolicyDryRunResult, ioStreams cmdutil.IOStreams) error {
	// TODO: Implement unified diff output
	// For now, just show JSON patches
	ioStreams.Info("Policy Diffs:\n\n")

	if len(result.Diffs) == 0 {
		ioStreams.Info("No spec modifications\n")
		return nil
	}

	for policyName, diff := range result.Diffs {
		ioStreams.Info("Policy: %s\n", policyName)
		ioStreams.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		ioStreams.Info("%s\n\n", string(diff))
	}

	return nil
}

// outputDryRunJSON outputs JSON format
func outputDryRunJSON(result *application.PolicyDryRunResult, ioStreams cmdutil.IOStreams) error {
	output := map[string]interface{}{
		"application":    result.Application.Name,
		"namespace":      result.Application.Namespace,
		"executionPlan":  result.ExecutionPlan,
		"policyResults":  result.PolicyResults,
		"warnings":       result.Warnings,
		"errors":         result.Errors,
		"finalSpec":      result.Application.Spec,
		"finalLabels":    result.Application.Labels,
		"finalAnnotations": result.Application.Annotations,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal to JSON")
	}

	ioStreams.Info("%s\n", string(data))
	return nil
}

// outputDryRunYAML outputs YAML format
func outputDryRunYAML(result *application.PolicyDryRunResult, ioStreams cmdutil.IOStreams) error {
	output := map[string]interface{}{
		"application":    result.Application.Name,
		"namespace":      result.Application.Namespace,
		"executionPlan":  result.ExecutionPlan,
		"policyResults":  result.PolicyResults,
		"warnings":       result.Warnings,
		"errors":         result.Errors,
		"finalSpec":      result.Application.Spec,
		"finalLabels":    result.Application.Labels,
		"finalAnnotations": result.Application.Annotations,
	}

	data, err := yaml.Marshal(output)
	if err != nil {
		return errors.Wrap(err, "failed to marshal to YAML")
	}

	ioStreams.Info("%s\n", string(data))
	return nil
}
