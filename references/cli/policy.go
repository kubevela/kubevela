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

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/aryann/difflib"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	v1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/application"
	velacommon "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	outputFormatTable   = "table"
	outputFormatJSON    = "json"
	outputFormatYAML    = "yaml"
	outputFormatSummary = "summary"
)

// PolicyCommandGroup creates the `policy` command group
func PolicyCommandGroup(c velacommon.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Manage and debug Application-scoped policies.",
		Long:  "Commands for viewing and testing Application-scoped PolicyDefinitions applied to Applications.",
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
	var wide, outcome, details bool
	var policyFilter string

	cmd := &cobra.Command{
		Use:   "view <app-name>",
		Short: "View applied Application-scoped policies and their effects.",
		Long:  "View which Application-scoped policies were applied to an Application and what changes they made.",
		Example: `  # View policies applied to an Application
  vela policy view my-app

  # Wide output (includes labels/annotations/context columns)
  vela policy view my-app --wide

  # Include per-policy details (context, labels, annotations, spec diff)
  vela policy view my-app --details

  # Include outcome (final spec/labels/annotations/context)
  vela policy view my-app --outcome

  # Filter to a specific policy
  vela policy view my-app -p governance

  # Include outcome in JSON output
  vela policy view my-app --output json --outcome`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := cmd.Flags().GetString("namespace")
			if err != nil {
				return err
			}
			if namespace == "" {
				namespace = "default"
			}
			return runPolicyView(context.Background(), c, args[0], namespace, outputFormat, wide, details, outcome, policyFilter, ioStreams)
		},
	}

	addNamespaceAndEnvArg(cmd)
	cmd.Flags().StringVarP(&outputFormat, "output", "o", outputFormatTable, "Output format: table, json, yaml")
	cmd.Flags().BoolVar(&wide, "wide", false, "Show additional columns (labels, annotations, context)")
	cmd.Flags().BoolVar(&details, "details", false, "Include per-policy output details (context, labels, annotations, spec transforms)")
	cmd.Flags().BoolVar(&outcome, "outcome", false, "Include final spec, labels, annotations and context in output")
	cmd.Flags().StringVarP(&policyFilter, "policy", "p", "", "Filter output to policies matching a name or glob pattern (e.g. \"add-env-*\")")

	return cmd
}

// policyOutputSpec holds spec snapshots nested under output.spec in the ConfigMap.
type policyOutputSpec struct {
	Before *json.RawMessage `json:"before,omitempty"`
	After  *json.RawMessage `json:"after,omitempty"`
}

// policyOutput is the per-policy output block stored in the ConfigMap.
type policyOutput struct {
	Name        string                 `json:"name"`
	Namespace   string                 `json:"namespace"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Annotations map[string]string      `json:"annotations,omitempty"`
	Ctx         map[string]interface{} `json:"ctx,omitempty"`
	Spec        *policyOutputSpec      `json:"spec,omitempty"`
}

type policyOutputWrapper struct {
	Name                   string       `json:"name"`
	Namespace              string       `json:"namespace"`
	Priority               int32        `json:"priority,omitempty"`
	Output                 policyOutput `json:"output,omitempty"`
	DefinitionRevisionName string       `json:"definitionRevisionName,omitempty"`
	Revision               int64        `json:"revision,omitempty"`
	RevisionHash           string       `json:"revisionHash,omitempty"`
}

type policyInfoRecord struct {
	RenderedAt string `json:"rendered_at"`
}

func printSpecDiff(before, after, indent string, ioStreams cmdutil.IOStreams) {
	// Convert JSON to YAML for readable diff
	var beforeObj, afterObj interface{}
	beforeYAML, afterYAML := before, after
	if err := json.Unmarshal([]byte(before), &beforeObj); err == nil {
		if b, err := yaml.Marshal(beforeObj); err == nil {
			beforeYAML = string(b)
		}
	}
	if err := json.Unmarshal([]byte(after), &afterObj); err == nil {
		if b, err := yaml.Marshal(afterObj); err == nil {
			afterYAML = string(b)
		}
	}

	diffs := difflib.Diff(strings.Split(beforeYAML, "\n"), strings.Split(afterYAML, "\n"))

	anyChange := false
	for _, d := range diffs {
		if d.Delta != difflib.Common {
			anyChange = true
			break
		}
	}
	if !anyChange {
		fmt.Fprintf(ioStreams.Out, "%s(no spec changes)\n", indent)
		return
	}

	for _, d := range diffs {
		switch d.Delta {
		case difflib.LeftOnly:
			fmt.Fprintf(ioStreams.Out, "%s\n", color.RedString("%s- %s", indent, d.Payload))
		case difflib.RightOnly:
			fmt.Fprintf(ioStreams.Out, "%s\n", color.GreenString("%s+ %s", indent, d.Payload))
		case difflib.Common:
			fmt.Fprintf(ioStreams.Out, "%s  %s\n", indent, d.Payload)
		}
	}
}

// buildPolicyDetailsFromConfigMap parses per-policy entries from a ConfigMap into the
// same map[string]map[string]any shape that PolicyDryRunResult.PolicyDetails uses.
// This allows view --details to call printPolicyDetails just like dry-run does.
func buildPolicyDetailsFromConfigMap(cm *corev1.ConfigMap) map[string]map[string]any {
	if cm == nil {
		return nil
	}
	result := map[string]map[string]any{}
	for key, val := range cm.Data {
		if key == "info" || key == "rendered_spec" || key == "applied_spec" || key == "metadata" {
			continue
		}
		var wrapper policyOutputWrapper
		if err := json.Unmarshal([]byte(val), &wrapper); err != nil {
			continue
		}

		// Parse the nested output block
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(val), &raw); err == nil {
			if outputRaw, ok := raw["output"]; ok {
				_ = json.Unmarshal(outputRaw, &wrapper.Output)
			}
		}

		// Build the details map in the same shape as PolicyDryRunResult.PolicyDetails
		outputMap := map[string]interface{}{}
		if len(wrapper.Output.Labels) > 0 {
			// Convert map[string]string to map[string]interface{} for consistent handling
			labelsAny := make(map[string]interface{}, len(wrapper.Output.Labels))
			for k, v := range wrapper.Output.Labels {
				labelsAny[k] = v
			}
			outputMap["labels"] = labelsAny
		}
		if len(wrapper.Output.Annotations) > 0 {
			annotationsAny := make(map[string]interface{}, len(wrapper.Output.Annotations))
			for k, v := range wrapper.Output.Annotations {
				annotationsAny[k] = v
			}
			outputMap["annotations"] = annotationsAny
		}
		if len(wrapper.Output.Ctx) > 0 {
			outputMap["ctx"] = wrapper.Output.Ctx
		}
		if wrapper.Output.Spec != nil && wrapper.Output.Spec.Before != nil && wrapper.Output.Spec.After != nil {
			var beforeObj, afterObj interface{}
			_ = json.Unmarshal(*wrapper.Output.Spec.Before, &beforeObj)
			_ = json.Unmarshal(*wrapper.Output.Spec.After, &afterObj)
			outputMap["spec"] = map[string]interface{}{
				"before": beforeObj,
				"after":  afterObj,
			}
		}

		details := map[string]any{
			"output":   outputMap,
			"priority": wrapper.Priority,
		}
		if wrapper.DefinitionRevisionName != "" {
			details["definitionRevisionName"] = wrapper.DefinitionRevisionName
			details["revision"] = wrapper.Revision
			details["revisionHash"] = wrapper.RevisionHash
		}
		result[wrapper.Name] = details
	}
	return result
}

// filterPolicies returns only the policies whose names match the given glob pattern.
func filterPolicies(policies []common.AppliedApplicationPolicy, pattern string) []common.AppliedApplicationPolicy {
	var filtered []common.AppliedApplicationPolicy
	for _, p := range policies {
		if matched, _ := path.Match(pattern, p.Name); matched {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// NewPolicyDryRunCommand creates the `vela policy dry-run` command
func NewPolicyDryRunCommand(c velacommon.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var outputFormat string
	var file, policyFilter string
	var details, outcome bool

	cmd := &cobra.Command{
		Use:   "dry-run [app-name]",
		Short: "Preview what policies would do to an Application without applying changes.",
		Long:  "Simulates the full policy application using the same code path as the controller. No changes are made to the cluster.",
		Example: `  # Simulate policies for a live Application (fetched from cluster)
  vela policy dry-run my-app

  # Test policies using a local Application file
  vela policy dry-run -f app.yaml

  # Focus on a specific policy
  vela policy dry-run -f app.yaml -p my-policy

  # Summary view (labels and annotations only)
  vela policy dry-run my-app --output summary

  # JSON output for CI/CD
  vela policy dry-run -f app.yaml --output json

  # Include per-policy output details
  vela policy dry-run -f app.yaml --details

  # Include outcome (final spec/labels/annotations/context)
  vela policy dry-run -f app.yaml --outcome`,
		Args: cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := cmd.Flags().GetString("namespace")
			if err != nil {
				return err
			}
			if namespace == "" {
				namespace = "default"
			}
			if file != "" {
				return runPolicyDryRunFile(context.Background(), c, file, namespace, outputFormat, policyFilter, details, outcome, ioStreams)
			}
			if len(args) == 0 {
				return fmt.Errorf("must provide either an app name or -f <file>")
			}
			return runPolicyDryRun(context.Background(), c, args[0], namespace, outputFormat, policyFilter, details, outcome, ioStreams)
		},
	}

	addNamespaceAndEnvArg(cmd)
	cmd.Flags().StringVarP(&outputFormat, "output", "o", outputFormatTable, "Output format: table, summary, json, yaml")
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to a local Application YAML file")
	cmd.Flags().StringVarP(&policyFilter, "policy", "p", "", "Filter output to policies matching a name or glob pattern (e.g. \"add-env-*\")")
	cmd.Flags().BoolVar(&details, "details", false, "Include per-policy output details (context, labels, annotations, spec transforms)")
	cmd.Flags().BoolVar(&outcome, "outcome", false, "Include outcome (final spec, labels, annotations and context) in output")

	return cmd
}

// runPolicyView implements the view command logic
func runPolicyView(ctx context.Context, c velacommon.Args, appName, namespace, outputFormat string, wide, details, outcome bool, policyFilter string, ioStreams cmdutil.IOStreams) error {
	cli, err := c.GetClient()
	if err != nil {
		return err
	}

	app := &v1beta1.Application{}
	if err := cli.Get(ctx, client.ObjectKey{Name: appName, Namespace: namespace}, app); err != nil {
		return errors.Wrapf(err, "failed to get Application %s/%s", namespace, appName)
	}

	if len(app.Status.AppliedApplicationPolicies) == 0 && len(app.Spec.Policies) == 0 {
		fmt.Fprintf(ioStreams.Out, "No policies found on '%s'\n", appName)
		return nil
	}

	// Fetch the diffs ConfigMap if available
	var diffsConfigMap *corev1.ConfigMap
	if app.Status.ApplicationPoliciesConfigMap != "" {
		cm := &corev1.ConfigMap{}
		if err := cli.Get(ctx, client.ObjectKey{Name: app.Status.ApplicationPoliciesConfigMap, Namespace: namespace}, cm); err == nil {
			diffsConfigMap = cm
		}
	}

	// Apply policy filter
	policies := app.Status.AppliedApplicationPolicies
	if policyFilter != "" {
		policies = filterPolicies(policies, policyFilter)
		if len(policies) == 0 {
			return fmt.Errorf("no policies matching %q found", policyFilter)
		}
	}

	// Build policyDetails from ConfigMap when --details is requested
	var policyDetails map[string]map[string]any
	if details {
		policyDetails = buildPolicyDetailsFromConfigMap(diffsConfigMap)
	}

	switch outputFormat {
	case outputFormatJSON:
		return outputPolicyViewJSON(app, policies, diffsConfigMap, details, outcome, policyDetails, ioStreams)
	case outputFormatYAML:
		return outputPolicyViewYAML(app, policies, diffsConfigMap, details, outcome, policyDetails, ioStreams)
	default:
		return outputPolicyViewTable(app, policies, diffsConfigMap, wide, details, outcome, policyDetails, ioStreams)
	}
}

func outputPolicyViewTable(app *v1beta1.Application, policies []common.AppliedApplicationPolicy, diffsConfigMap *corev1.ConfigMap, wide, details, outcome bool, policyDetails map[string]map[string]any, ioStreams cmdutil.IOStreams) error {
	// Build type map from spec.policies for cross-referencing global policies
	specPolicyType := make(map[string]string)
	for _, p := range app.Spec.Policies {
		specPolicyType[p.Name] = p.Type
	}
	// Resolve type for each applied policy
	pTypes := make([]string, len(policies))
	for i, p := range policies {
		pTypes[i] = p.Type
		if pTypes[i] == "" {
			pTypes[i] = specPolicyType[p.Name]
		}
	}

	printPolicyTable(policies, pTypes, wide, ioStreams)

	// Rendered at timestamp from ConfigMap info block
	if diffsConfigMap != nil {
		if infoRaw, ok := diffsConfigMap.Data["info"]; ok {
			var info policyInfoRecord
			if err := json.Unmarshal([]byte(infoRaw), &info); err == nil && info.RenderedAt != "" {
				fmt.Fprintf(ioStreams.Out, "Last rendered: %s\n\n", info.RenderedAt)
			}
		}
	}

	if outcome {
		oSpec, oLabels, oAnnotations, oCtx := extractOutcomeFromConfigMap(diffsConfigMap, app.Spec, app.Labels, app.Annotations)
		printOutcomeBlock(oSpec, oLabels, oAnnotations, oCtx, ioStreams)
	}

	if details {
		printPolicyDetails(policies, policyDetails, ioStreams)
	}

	return nil
}

// printPolicyTable renders the shared policy table used by both view and dry-run.
// types is a parallel slice of resolved policy type strings (may be empty strings).
func printPolicyTable(policies []common.AppliedApplicationPolicy, types []string, wide bool, ioStreams cmdutil.IOStreams) {
	table := tablewriter.NewWriter(ioStreams.Out)
	if wide {
		table.SetHeader([]string{"#", "Policy", "Type", "Namespace", "Source", "Enabled", "Error", "Labels", "Annotations", "Context", "Spec Modified"})
	} else {
		table.SetHeader([]string{"#", "Policy", "Type", "Namespace", "Source", "Enabled"})
	}
	table.SetBorder(true)
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for i, p := range policies {
		enabledStr := color.GreenString("Yes")
		if !p.Applied {
			enabledStr = color.YellowString("No")
		}
		pType := ""
		if i < len(types) {
			pType = types[i]
		}
		row := []string{
			fmt.Sprintf("%d", i+1),
			p.Name,
			pType,
			p.Namespace,
			p.Source,
			enabledStr,
		}
		if wide {
			row = append(row,
				boolStr(p.Error),
				fmt.Sprintf("%d", p.LabelsCount),
				fmt.Sprintf("%d", p.AnnotationsCount),
				boolStr(p.HasContext),
				boolStr(p.SpecModified),
			)
		}
		table.Append(row)
	}
	table.Render()
	fmt.Fprintln(ioStreams.Out)
}

// printPolicyDetails renders the per-policy details section. It is shared between
// `vela policy view --details` (reading from ConfigMap) and `vela policy dry-run --details`
// (reading from PolicyDryRunResult.PolicyDetails).
func printPolicyDetails(policies []common.AppliedApplicationPolicy, policyDetails map[string]map[string]any, ioStreams cmdutil.IOStreams) {
	if len(policyDetails) == 0 {
		return
	}

	fmt.Fprintf(ioStreams.Out, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(ioStreams.Out, "Per-Policy Details\n")
	fmt.Fprintf(ioStreams.Out, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	for i, p := range policies {
		header := fmt.Sprintf("Policy %d: %s", i+1, p.Name)
		if p.Source != "" {
			header += fmt.Sprintf(" [%s]", p.Source)
		}

		if !p.Applied {
			disabledHeader := header + "  (disabled)"
			fmt.Fprintf(ioStreams.Out, "%s\n%s\n", color.YellowString(disabledHeader), strings.Repeat("─", len(disabledHeader)))
			fmt.Fprintf(ioStreams.Out, "  (policy was not applied)\n\n")
			continue
		}

		details, ok := policyDetails[p.Name]
		if !ok {
			continue
		}

		fmt.Fprintf(ioStreams.Out, "%s\n%s\n", color.CyanString(header), strings.Repeat("─", len(header)))

		// Version / priority metadata
		if v, ok := details["definitionRevisionName"].(string); ok && v != "" {
			rev, _ := details["revision"].(int64)
			hash, _ := details["revisionHash"].(string)
			fmt.Fprintf(ioStreams.Out, "  Version:  %s (revision %d, hash: %s)\n", v, rev, hash)
		}
		if pri, ok := details["priority"]; ok {
			fmt.Fprintf(ioStreams.Out, "  Priority: %v\n", pri)
		}

		output, _ := details["output"].(map[string]interface{})

		if output != nil {
			if ctx, ok := output["ctx"].(map[string]interface{}); ok && len(ctx) > 0 {
				fmt.Fprintf(ioStreams.Out, "  %s\n", color.CyanString("Context:"))
				ctxYAML, _ := yaml.Marshal(ctx)
				fmt.Fprintf(ioStreams.Out, "%s\n", indentLines(string(ctxYAML), "    "))
			}
			if rawLabels, ok := output["labels"].(map[string]interface{}); ok && len(rawLabels) > 0 {
				fmt.Fprintf(ioStreams.Out, "  %s\n", color.CyanString(fmt.Sprintf("Labels (%d):", len(rawLabels))))
				for k, v := range rawLabels {
					fmt.Fprintf(ioStreams.Out, "    %-40s %v\n", k+":", v)
				}
				fmt.Fprintln(ioStreams.Out)
			}
			if rawAnnotations, ok := output["annotations"].(map[string]interface{}); ok && len(rawAnnotations) > 0 {
				fmt.Fprintf(ioStreams.Out, "  %s\n", color.CyanString(fmt.Sprintf("Annotations (%d):", len(rawAnnotations))))
				for k, v := range rawAnnotations {
					fmt.Fprintf(ioStreams.Out, "    %-40s %v\n", k+":", v)
				}
				fmt.Fprintln(ioStreams.Out)
			}
			if specBlock, ok := output["spec"].(map[string]interface{}); ok {
				before, hasBefore := specBlock["before"]
				after, hasAfter := specBlock["after"]
				if hasBefore && hasAfter {
					beforeJSON, _ := json.Marshal(before)
					afterJSON, _ := json.Marshal(after)
					fmt.Fprintf(ioStreams.Out, "  %s\n", color.CyanString("Spec Diff:"))
					printSpecDiff(string(beforeJSON), string(afterJSON), "    ", ioStreams)
				}
			}
		}

		fmt.Fprintln(ioStreams.Out)
	}
}

// printOutcomeBlock renders the Outcome section for table output.
func printOutcomeBlock(spec v1beta1.ApplicationSpec, labels, annotations map[string]string, finalCtx map[string]interface{}, ioStreams cmdutil.IOStreams) {
	fmt.Fprintf(ioStreams.Out, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(ioStreams.Out, "Outcome\n")
	fmt.Fprintf(ioStreams.Out, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if len(labels) > 0 {
		printSubheader(fmt.Sprintf("Labels (%d)", len(labels)), ioStreams)
		for k, v := range labels {
			fmt.Fprintf(ioStreams.Out, "  %-40s %s\n", k+":", v)
		}
		fmt.Fprintln(ioStreams.Out)
	}
	if len(annotations) > 0 {
		printSubheader(fmt.Sprintf("Annotations (%d)", len(annotations)), ioStreams)
		for k, v := range annotations {
			fmt.Fprintf(ioStreams.Out, "  %-40s %s\n", k+":", v)
		}
		fmt.Fprintln(ioStreams.Out)
	}
	if len(finalCtx) > 0 {
		printSubheader("Context", ioStreams)
		ctxYAML, _ := yaml.Marshal(finalCtx)
		fmt.Fprintf(ioStreams.Out, "%s\n", indentLines(string(ctxYAML), "  "))
	}

	printSubheader("Spec", ioStreams)
	specYAML, _ := yaml.Marshal(spec)
	fmt.Fprintf(ioStreams.Out, "%s\n", indentLines(string(specYAML), "  "))
}

func outputPolicyViewJSON(app *v1beta1.Application, policies []common.AppliedApplicationPolicy, cm *corev1.ConfigMap, details, outcome bool, policyDetails map[string]map[string]any, ioStreams cmdutil.IOStreams) error {
	var detailsArg map[string]map[string]any
	if details {
		detailsArg = policyDetails
	}
	oSpec, oLabels, oAnnotations, oCtx := extractOutcomeFromConfigMap(cm, app.Spec, app.Labels, app.Annotations)
	out := buildPolicyOutput(app.Name, app.Namespace, policies, oSpec, oLabels, oAnnotations, oCtx, outcome, nil, detailsArg)
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal JSON")
	}
	fmt.Fprintf(ioStreams.Out, "%s\n", data)
	return nil
}

func outputPolicyViewYAML(app *v1beta1.Application, policies []common.AppliedApplicationPolicy, cm *corev1.ConfigMap, details, outcome bool, policyDetails map[string]map[string]any, ioStreams cmdutil.IOStreams) error {
	var detailsArg map[string]map[string]any
	if details {
		detailsArg = policyDetails
	}
	oSpec, oLabels, oAnnotations, oCtx := extractOutcomeFromConfigMap(cm, app.Spec, app.Labels, app.Annotations)
	out := buildPolicyOutput(app.Name, app.Namespace, policies, oSpec, oLabels, oAnnotations, oCtx, outcome, nil, detailsArg)
	data, err := yaml.Marshal(out)
	if err != nil {
		return errors.Wrap(err, "failed to marshal YAML")
	}
	fmt.Fprintf(ioStreams.Out, "%s\n", data)
	return nil
}

// extractMetadataFromConfigMap parses the "metadata" key from the ConfigMap (merged labels, annotations, context).
func extractMetadataFromConfigMap(cm *corev1.ConfigMap) map[string]interface{} {
	if cm == nil {
		return nil
	}
	metaRaw, ok := cm.Data["metadata"]
	if !ok {
		return nil
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(metaRaw), &meta); err != nil {
		return nil
	}
	return meta
}

// extractOutcomeFromConfigMap reads the policy-applied outcome (labels, annotations, context, spec) from the ConfigMap.
// labels and annotations reflect only what policies contributed — not controller-added values.
// Falls back to the provided app spec/labels/annotations if the ConfigMap is unavailable.
func extractOutcomeFromConfigMap(cm *corev1.ConfigMap, fallbackSpec v1beta1.ApplicationSpec, fallbackLabels, fallbackAnnotations map[string]string) (v1beta1.ApplicationSpec, map[string]string, map[string]string, map[string]interface{}) {
	spec := fallbackSpec

	if cm == nil {
		return spec, fallbackLabels, fallbackAnnotations, nil
	}

	// applied_spec is app.Spec after policy application
	if specRaw, ok := cm.Data["applied_spec"]; ok {
		var s v1beta1.ApplicationSpec
		if err := json.Unmarshal([]byte(specRaw), &s); err == nil {
			spec = s
		}
	}

	// metadata contains only policy-contributed labels/annotations/context — always use it when present,
	// even if empty (an empty map means no policies added labels/annotations).
	meta := extractMetadataFromConfigMap(cm)
	if meta == nil {
		return spec, fallbackLabels, fallbackAnnotations, nil
	}

	labels := make(map[string]string)
	if l, ok := meta["labels"].(map[string]interface{}); ok {
		for k, v := range l {
			if s, ok := v.(string); ok {
				labels[k] = s
			}
		}
	}
	annotations := make(map[string]string)
	if a, ok := meta["annotations"].(map[string]interface{}); ok {
		for k, v := range a {
			if s, ok := v.(string); ok {
				annotations[k] = s
			}
		}
	}
	var finalCtx map[string]interface{}
	if ctx, ok := meta["context"].(map[string]interface{}); ok && len(ctx) > 0 {
		finalCtx = ctx
	}
	return spec, labels, annotations, finalCtx
}

// runPolicyDryRun implements the dry-run command logic (cluster mode)
func runPolicyDryRun(ctx context.Context, c velacommon.Args, appName, namespace, outputFormat, policyFilter string, details, outcome bool, ioStreams cmdutil.IOStreams) error {
	cli, err := c.GetClient()
	if err != nil {
		return err
	}

	app := &v1beta1.Application{}
	if err := cli.Get(ctx, client.ObjectKey{Name: appName, Namespace: namespace}, app); err != nil {
		return errors.Wrapf(err, "failed to get Application %s/%s", namespace, appName)
	}

	if outputFormat == outputFormatTable || outputFormat == "" {
		fmt.Fprintf(ioStreams.Out, "Dry-run: %s/%s\n\n", namespace, appName)
	}

	result, err := application.SimulatePolicyApplication(ctx, cli, app)
	if err != nil {
		return errors.Wrap(err, "simulation failed")
	}

	return outputDryRunResult(result, outputFormat, policyFilter, details, outcome, ioStreams)
}

// runPolicyDryRunFile implements the dry-run command logic (file mode)
func runPolicyDryRunFile(ctx context.Context, c velacommon.Args, filePath, namespace, outputFormat, policyFilter string, details, outcome bool, ioStreams cmdutil.IOStreams) error {
	data, err := os.ReadFile(filePath) //nolint:gosec // filePath is supplied by the user via CLI flag
	if err != nil {
		return errors.Wrapf(err, "failed to read file %s", filePath)
	}

	app := &v1beta1.Application{}
	if err := yaml.Unmarshal(data, app); err != nil {
		return errors.Wrapf(err, "failed to parse Application from %s", filePath)
	}
	if app.Namespace == "" {
		app.Namespace = namespace
	}

	cli, err := c.GetClient()
	if err != nil {
		return err
	}

	if outputFormat == outputFormatTable || outputFormat == "" {
		fmt.Fprintf(ioStreams.Out, "Dry-run: %s (from file)\n\n", filePath)
	}

	result, err := application.SimulatePolicyApplication(ctx, cli, app)
	if err != nil {
		return errors.Wrap(err, "simulation failed")
	}

	return outputDryRunResult(result, outputFormat, policyFilter, details, outcome, ioStreams)
}

func outputDryRunResult(result *application.PolicyDryRunResult, outputFormat, policyFilter string, details, outcome bool, ioStreams cmdutil.IOStreams) error {
	if policyFilter != "" {
		result = filterDryRunResult(result, policyFilter)
		if result == nil {
			return fmt.Errorf("no policies matching %q found in simulation results", policyFilter)
		}
	}
	switch outputFormat {
	case outputFormatSummary:
		return outputDryRunSummary(result, ioStreams)
	case outputFormatJSON:
		return outputDryRunJSON(result, details, outcome, ioStreams)
	case outputFormatYAML:
		return outputDryRunYAML(result, details, outcome, ioStreams)
	default:
		return outputDryRunTable(result, details, outcome, ioStreams)
	}
}

func outputDryRunTable(result *application.PolicyDryRunResult, details, outcome bool, ioStreams cmdutil.IOStreams) error {
	if len(result.Errors) > 0 {
		fmt.Fprintf(ioStreams.Out, "%s\n", color.RedString("Errors:"))
		for _, e := range result.Errors {
			fmt.Fprintf(ioStreams.Out, "  • %s\n", e)
		}
		fmt.Fprintln(ioStreams.Out)
	}

	fmt.Fprintf(ioStreams.Out, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(ioStreams.Out, "Policy Results\n")
	fmt.Fprintf(ioStreams.Out, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if len(result.PolicyResults) == 0 {
		fmt.Fprintf(ioStreams.Out, "No Application-scoped policies applied.\n\n")
	} else {
		types := make([]string, len(result.PolicyResults))
		for i, p := range result.PolicyResults {
			types[i] = p.Type
		}
		printPolicyTable(result.PolicyResults, types, true, ioStreams)
	}

	if outcome {
		printOutcomeBlock(result.Application.Spec, result.Application.Labels, result.Application.Annotations, result.FinalContext, ioStreams)
	}

	if details {
		printPolicyDetails(result.PolicyResults, result.PolicyDetails, ioStreams)
	}

	fmt.Fprintf(ioStreams.Out, "This is a dry-run. No changes were applied to the cluster.\n")

	return nil
}

func outputDryRunSummary(result *application.PolicyDryRunResult, ioStreams cmdutil.IOStreams) error {
	applied, skipped := countApplied(result.PolicyResults)
	fmt.Fprintf(ioStreams.Out, "Policies: %d enabled, %d disabled\n\n", applied, skipped)
	printLabelsAnnotations(result.Application.Labels, result.Application.Annotations, ioStreams)
	return nil
}

func outputDryRunJSON(result *application.PolicyDryRunResult, details, outcome bool, ioStreams cmdutil.IOStreams) error {
	var policyDetails map[string]map[string]any
	if details {
		policyDetails = result.PolicyDetails
	}
	out := buildPolicyOutput(result.Application.Name, result.Application.Namespace, result.PolicyResults, result.Application.Spec, result.Application.Labels, result.Application.Annotations, result.FinalContext, outcome, result.Errors, policyDetails)
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal JSON")
	}
	fmt.Fprintf(ioStreams.Out, "%s\n", data)
	return nil
}

func outputDryRunYAML(result *application.PolicyDryRunResult, details, outcome bool, ioStreams cmdutil.IOStreams) error {
	var policyDetails map[string]map[string]any
	if details {
		policyDetails = result.PolicyDetails
	}
	out := buildPolicyOutput(result.Application.Name, result.Application.Namespace, result.PolicyResults, result.Application.Spec, result.Application.Labels, result.Application.Annotations, result.FinalContext, outcome, result.Errors, policyDetails)
	data, err := yaml.Marshal(out)
	if err != nil {
		return errors.Wrap(err, "failed to marshal YAML")
	}
	fmt.Fprintf(ioStreams.Out, "%s\n", data)
	return nil
}

// buildPolicyOutput constructs the shared JSON/YAML output for both `vela policy view` and
// `vela policy dry-run`. errors and policyDetails are nil for the live view case.
func buildPolicyOutput(appName, namespace string, policies []common.AppliedApplicationPolicy, spec v1beta1.ApplicationSpec, labels, annotations map[string]string, finalCtx map[string]interface{}, outcome bool, errs []string, policyDetails map[string]map[string]any) map[string]any {
	applied, skipped := countApplied(policies)
	totalLabels, totalAnnotations, specMod, _ := summarize(policies)

	// Merge status entries with rich detail data into a single policies list.
	merged := make([]map[string]any, 0, len(policies))
	for i, p := range policies {
		entry := map[string]any{
			"order":     i + 1,
			"name":      p.Name,
			"type":      p.Type,
			"namespace": p.Namespace,
			"source":    p.Source,
			"applied":   p.Applied,
		}
		if p.Error {
			entry["error"] = true
		}
		if p.Message != "" {
			entry["message"] = p.Message
		}
		if p.SpecModified {
			entry["specModified"] = true
		}
		if p.LabelsCount > 0 {
			entry["labelsCount"] = p.LabelsCount
		}
		if p.AnnotationsCount > 0 {
			entry["annotationsCount"] = p.AnnotationsCount
		}
		if p.HasContext {
			entry["hasContext"] = true
		}
		if p.DefinitionRevisionName != "" {
			entry["definitionRevisionName"] = p.DefinitionRevisionName
		}
		if p.Revision > 0 {
			entry["revision"] = p.Revision
		}
		if p.RevisionHash != "" {
			entry["revisionHash"] = p.RevisionHash
		}
		// Merge rich output from policyDetails when available (--details mode).
		if details, ok := policyDetails[p.Name]; ok {
			if priority, ok := details["priority"]; ok {
				entry["priority"] = priority
			}
			if output, ok := details["output"]; ok {
				entry["output"] = output
			}
		}
		merged = append(merged, entry)
	}

	out := map[string]any{
		"application": appName,
		"namespace":   namespace,
		"policies":    merged,
		"summary": map[string]any{
			"enabled":           applied,
			"disabled":          skipped,
			"specModifications": specMod,
			"labelsAdded":       totalLabels,
			"annotationsAdded":  totalAnnotations,
		},
	}
	if outcome {
		outcomeBlock := map[string]any{
			"spec":        spec,
			"labels":      labels,
			"annotations": annotations,
		}
		if len(finalCtx) > 0 {
			outcomeBlock["context"] = finalCtx
		}
		out["outcome"] = outcomeBlock
	}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	return out
}

// filterDryRunResult returns a shallow copy of result with PolicyResults and PolicyDetails
// filtered to policies whose names match the given glob pattern (e.g. "add-env-*").
// Returns nil if no policies match.
func filterDryRunResult(result *application.PolicyDryRunResult, pattern string) *application.PolicyDryRunResult {
	filtered := filterPolicies(result.PolicyResults, pattern)
	if len(filtered) == 0 {
		return nil
	}
	out := *result
	out.PolicyResults = filtered
	if result.PolicyDetails != nil {
		out.PolicyDetails = map[string]map[string]any{}
		for _, p := range filtered {
			if d, ok := result.PolicyDetails[p.Name]; ok {
				out.PolicyDetails[p.Name] = d
			}
		}
	}
	return &out
}

// helpers

func countApplied(policies []common.AppliedApplicationPolicy) (applied, skipped int) {
	for _, p := range policies {
		if p.Applied {
			applied++
		} else {
			skipped++
		}
	}
	return
}

func summarize(policies []common.AppliedApplicationPolicy) (totalLabels, totalAnnotations, specMod, ctxCount int) {
	for _, p := range policies {
		if !p.Applied {
			continue
		}
		totalLabels += p.LabelsCount
		totalAnnotations += p.AnnotationsCount
		if p.SpecModified {
			specMod++
		}
		if p.HasContext {
			ctxCount++
		}
	}
	return
}

func boolStr(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func printLabelsAnnotations(labels, annotations map[string]string, ioStreams cmdutil.IOStreams) {
	if len(labels) > 0 {
		fmt.Fprintf(ioStreams.Out, "Labels (%d):\n", len(labels))
		for k, v := range labels {
			fmt.Fprintf(ioStreams.Out, "  %-40s %s\n", k+":", v)
		}
		fmt.Fprintln(ioStreams.Out)
	}
	if len(annotations) > 0 {
		fmt.Fprintf(ioStreams.Out, "Annotations (%d):\n", len(annotations))
		for k, v := range annotations {
			fmt.Fprintf(ioStreams.Out, "  %-40s %s\n", k+":", v)
		}
		fmt.Fprintln(ioStreams.Out)
	}
}

func printSubheader(title string, ioStreams cmdutil.IOStreams) {
	fmt.Fprintf(ioStreams.Out, "%s\n%s\n", color.CyanString(title), strings.Repeat("─", len(title)))
}


func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}
