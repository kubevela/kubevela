/*
Copyright 2024 The KubeVela Authors.

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
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/upgrade"
	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/filters"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// --- Shared types ---

// CompatibilityHint records a specific reason, the upgrade ID, and the KubeVela version that introduced it.
type CompatibilityHint struct {
	Introduced string `json:"introduced" yaml:"introduced"`
	ID         string `json:"id"         yaml:"id"`
	Reason     string `json:"reason"     yaml:"reason"`
}

// revisionNum returns the numeric value of a "vN" revision string for ordering.
// Non-numeric or non-"v"-prefixed strings return -1 so they sort before real revisions.
func revisionNum(rev string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(rev, "v"))
	if err != nil {
		return -1
	}
	return n
}

// parseHints parses reason strings of the form "[cue@0.14] [list-arithmetic] description"
// into structured CompatibilityHints.
func parseHints(reasons []string) []CompatibilityHint {
	var hints []CompatibilityHint
	for _, r := range reasons {
		// First bracket: source@version, e.g. "[cue@0.14]"
		ver, rest, _ := strings.Cut(r, "] ")
		introduced := strings.TrimPrefix(ver, "[")
		// Second bracket: upgrade ID, e.g. "[list-arithmetic]"
		var id, issue string
		if strings.HasPrefix(rest, "[") {
			idPart, desc, _ := strings.Cut(rest, "] ")
			id = strings.TrimPrefix(idPart, "[")
			issue = desc
		} else {
			issue = rest
		}
		hints = append(hints, CompatibilityHint{
			Introduced: introduced,
			ID:         id,
			Reason:     issue,
		})
	}
	return hints
}

// parseSelectorMap parses a comma-separated "key=value" selector string into a map.
// Returns an error if any pair is malformed.
func parseSelectorMap(sel string) (map[string]string, error) {
	if sel == "" {
		return nil, nil
	}
	m := map[string]string{}
	for _, pair := range strings.Split(sel, ",") {
		pair = strings.TrimSpace(pair)
		k, v, ok := strings.Cut(pair, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid selector %q: expected key=value", pair)
		}
		m[k] = v
	}
	return m, nil
}

// matchesSelector returns true if all key=value pairs in sel are present in meta.
func matchesSelector(meta map[string]string, sel map[string]string) bool {
	for k, v := range sel {
		if meta[k] != v {
			return false
		}
	}
	return true
}

// --- Definitions report types ---

// DefCompatReport is the serialisable report for `vela def compatibility-check definitions`.
type DefCompatReport struct {
	Total       int              `json:"total"       yaml:"total"`
	Definitions []DefCompatEntry `json:"definitions" yaml:"definitions"`
}

// DefCompatEntry groups all incompatible revisions of one definition.
type DefCompatEntry struct {
	Name      string          `json:"name"      yaml:"name"`
	Kind      string          `json:"kind"      yaml:"kind"`
	Namespace string          `json:"namespace" yaml:"namespace"`
	Issues    []DefIssueEntry `json:"issues"    yaml:"issues"`
}

// DefIssueEntry is one unique compatibility issue with the list of revisions it affects.
type DefIssueEntry struct {
	Introduced string   `json:"introduced" yaml:"introduced"`
	ID         string   `json:"id"         yaml:"id"`
	Reason     string   `json:"reason"     yaml:"reason"`
	Revisions  []string `json:"revisions"  yaml:"revisions"`
}

// defRevCompatEntry is an internal type used only during scanning, before deduplication.
type defRevCompatEntry struct {
	revision string
	reasons  []CompatibilityHint
}

// --- Applications report types ---

// AppCompatReport is the serialisable report for `vela def compatibility-check applications`.
type AppCompatReport struct {
	Total        int              `json:"total"        yaml:"total"`
	Applications []AppCompatEntry `json:"applications" yaml:"applications"`
}

// AppCompatEntry groups all incompatible definition snapshots found in one Application's active revision.
type AppCompatEntry struct {
	Application string          `json:"application" yaml:"application"`
	Namespace   string          `json:"namespace"   yaml:"namespace"`
	Revision    string          `json:"revision"    yaml:"revision"`
	Issues      []AppIssueEntry `json:"issues"      yaml:"issues"`
}

// AppIssueEntry is one unique compatibility issue with affected snapshots grouped by kind.
type AppIssueEntry struct {
	Introduced    string   `json:"introduced"               yaml:"introduced"`
	ID            string   `json:"id"                       yaml:"id"`
	Reason        string   `json:"reason"                   yaml:"reason"`
	Components    []string `json:"components,omitempty"     yaml:"components,omitempty"`
	Traits        []string `json:"traits,omitempty"         yaml:"traits,omitempty"`
	Policies      []string `json:"policies,omitempty"       yaml:"policies,omitempty"`
	WorkflowSteps []string `json:"workflowSteps,omitempty"  yaml:"workflowSteps,omitempty"`
}

// --- Command group ---

// NewDefinitionCompatibilityCommand creates the `vela def compatibility-check` subcommand group.
func NewDefinitionCompatibilityCommand(c common.Args, ioStreams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "compatibility-check",
		Aliases: []string{"compat"},
		Short:   "Scan for CUE version incompatibilities in definitions and applications",
		Long: "Subcommands for scanning CUE version incompatibilities across\n" +
			"definitions, their revision history, and active ApplicationRevisions.",
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeDefManagement,
			types.TagCommandOrder: "6",
		},
	}
	cmd.AddCommand(
		NewDefinitionCompatibilityDefinitionsCommand(c, ioStreams),
		NewDefinitionCompatibilityApplicationsCommand(c, ioStreams),
	)
	return cmd
}

// --- `vela def compatibility-check definitions` ---

// NewDefinitionCompatibilityDefinitionsCommand creates `vela def compatibility-check definitions`.
func NewDefinitionCompatibilityDefinitionsCommand(c common.Args, ioStreams util.IOStreams) *cobra.Command {
	var (
		latestOnly         bool
		outputFormat       string
		namespace          string
		labelSelector      string
		annotationSelector string
	)

	allTypes := []string{"component", "trait", "workflow-step", "policy"}

	cmd := &cobra.Command{
		Use:   "definitions",
		Short: "Scan all definitions and their revision history for CUE incompatibilities",
		Long: "Scans every definition across all namespaces for CUE syntax incompatible\n" +
			"with the current KubeVela version, grouped by definition with the affected\n" +
			"DefinitionRevisions listed under each.\n\n" +
			"Use --latest-revision-only to skip revision history and check only the\n" +
			"current live definition.",
		Example: "# Scan all definitions and all their historical revisions\n" +
			"vela def compatibility-check definitions\n\n" +
			"# Scan only the current live definition (skip revision history)\n" +
			"vela def compatibility-check definitions --latest-revision-only\n\n" +
			"# Output as YAML\n" +
			"vela def compatibility-check definitions -o yaml",
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeDefManagement,
			types.TagCommandOrder: "1",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := c.GetClient()
			if err != nil {
				return errors.Wrap(err, "failed to get k8s client")
			}
			report, err := scanDefinitions(context.Background(), k8sClient, scanDefsOptions{
				allTypes:           allTypes,
				namespace:          namespace,
				labelSelector:      labelSelector,
				annotationSelector: annotationSelector,
				latestOnly:         latestOnly,
			})
			if err != nil {
				return err
			}
			return printDefCompatReport(ioStreams, report, outputFormat)
		},
	}

	cmd.Flags().BoolVar(&latestOnly, "latest-revision-only", false,
		"Only check the current live definition; skip DefinitionRevision history.")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format. One of: table, yaml, json.")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Only scan definitions in this namespace (default: all namespaces).")
	cmd.Flags().StringVarP(&labelSelector, "label-selector", "l", "", "Filter by labels, e.g. -l app=foo,env=prod.")
	cmd.Flags().StringVar(&annotationSelector, "annotation-selector", "", "Filter by annotations, e.g. --annotation-selector owner=team-a.")
	return cmd
}

// scanDefsOptions holds parameters for scanDefinitions.
type scanDefsOptions struct {
	allTypes           []string
	namespace          string
	labelSelector      string
	annotationSelector string
	latestOnly         bool
}

// defRawEntry accumulates per-revision compat findings for a single definition.
type defRawEntry struct {
	name, kind, ns string
	revisions      []defRevCompatEntry
}

type defKey struct{ ns, name, kind string }

func normaliseDefKind(kind string) string {
	switch kind {
	case "Component":
		return v1beta1.ComponentDefinitionKind
	case "Trait":
		return v1beta1.TraitDefinitionKind
	case "WorkflowStep":
		return v1beta1.WorkflowStepDefinitionKind
	case "Policy":
		return v1beta1.PolicyDefinitionKind
	}
	return kind
}

func scanDefinitions(ctx context.Context, k8sClient client.Client, opts scanDefsOptions) (DefCompatReport, error) {
	labelSel, err := parseSelectorMap(opts.labelSelector)
	if err != nil {
		return DefCompatReport{}, errors.Wrap(err, "--label-selector")
	}
	annotationSel, err := parseSelectorMap(opts.annotationSelector)
	if err != nil {
		return DefCompatReport{}, errors.Wrap(err, "--annotation-selector")
	}

	rawEntries := map[defKey]*defRawEntry{}
	ensureRaw := func(ns, name, kind string) *defRawEntry {
		kind = normaliseDefKind(kind)
		k := defKey{ns, name, kind}
		if rawEntries[k] == nil {
			rawEntries[k] = &defRawEntry{name: name, kind: kind, ns: ns}
		}
		return rawEntries[k]
	}

	var defFilters []filters.Filter
	if len(labelSel) > 0 {
		defFilters = append(defFilters, func(obj unstructured.Unstructured) bool {
			return matchesSelector(obj.GetLabels(), labelSel)
		})
	}
	if len(annotationSel) > 0 {
		defFilters = append(defFilters, func(obj unstructured.Unstructured) bool {
			return matchesSelector(obj.GetAnnotations(), annotationSel)
		})
	}

	for _, defType := range opts.allTypes {
		defs, err := pkgdef.SearchDefinition(k8sClient, defType, opts.namespace, defFilters...)
		if err != nil {
			return DefCompatReport{}, errors.Wrapf(err, "failed to list %s", defType)
		}
		for _, def := range defs {
			template, ok := extractCUETemplate(def.Object)
			if !ok || template == "" {
				continue
			}
			needs, reasons, err := upgrade.RequiresUpgrade(template)
			if err != nil || !needs {
				continue
			}
			e := ensureRaw(def.GetNamespace(), def.GetName(), def.GetKind())
			e.revisions = append(e.revisions, defRevCompatEntry{revision: "current", reasons: parseHints(reasons)})
		}
	}

	if !opts.latestOnly {
		if err := scanDefRevisions(ctx, k8sClient, opts, labelSel, annotationSel, ensureRaw); err != nil {
			return DefCompatReport{}, err
		}
	}

	return buildDefCompatReport(rawEntries), nil
}

func scanDefRevisions(
	ctx context.Context,
	k8sClient client.Client,
	opts scanDefsOptions,
	labelSel, annotationSel map[string]string,
	ensureRaw func(ns, name, kind string) *defRawEntry,
) error {
	typeMap := map[string]oamcommon.DefinitionType{
		"component":     oamcommon.ComponentType,
		"trait":         oamcommon.TraitType,
		"workflow-step": oamcommon.WorkflowStepType,
		"policy":        oamcommon.PolicyType,
	}
	for _, t := range opts.allTypes {
		defRevs, err := pkgdef.SearchDefinitionRevisions(ctx, k8sClient, opts.namespace, "", typeMap[t], 0)
		if err != nil {
			return errors.Wrapf(err, "failed to list DefinitionRevisions for %s", t)
		}
		for _, dr := range defRevs {
			if len(labelSel) > 0 && !matchesSelector(dr.GetLabels(), labelSel) {
				continue
			}
			if len(annotationSel) > 0 && !matchesSelector(dr.GetAnnotations(), annotationSel) {
				continue
			}
			template := extractDefRevTemplate(dr)
			if template == "" {
				continue
			}
			needs, reasons, err := upgrade.RequiresUpgrade(template)
			if err != nil || !needs {
				continue
			}
			defName := defNameFromRevision(dr)
			e := ensureRaw(dr.GetNamespace(), defName, string(dr.Spec.DefinitionType))
			e.revisions = append(e.revisions, defRevCompatEntry{
				revision: fmt.Sprintf("v%d", dr.Spec.Revision),
				reasons:  parseHints(reasons),
			})
		}
	}
	return nil
}

func buildDefCompatReport(rawEntries map[defKey]*defRawEntry) DefCompatReport {
	var defList []DefCompatEntry
	for _, e := range rawEntries {
		// Promote "current" to the highest vN revision if DefinitionRevisions exist.
		highestVN := ""
		for _, r := range e.revisions {
			if r.revision != "current" && revisionNum(r.revision) > revisionNum(highestVN) {
				highestVN = r.revision
			}
		}
		if highestVN != "" {
			for i := range e.revisions {
				if e.revisions[i].revision == "current" {
					e.revisions[i].revision = highestVN
				}
			}
		}
		sort.Slice(e.revisions, func(i, j int) bool {
			return revisionNum(e.revisions[i].revision) < revisionNum(e.revisions[j].revision)
		})
		defList = append(defList, DefCompatEntry{
			Name:      e.name,
			Kind:      e.kind,
			Namespace: e.ns,
			Issues:    deduplicateDefHints(e.revisions),
		})
	}
	sort.Slice(defList, func(i, j int) bool {
		if defList[i].Namespace != defList[j].Namespace {
			return defList[i].Namespace < defList[j].Namespace
		}
		return defList[i].Name < defList[j].Name
	})
	report := DefCompatReport{Total: len(defList), Definitions: defList}
	if report.Definitions == nil {
		report.Definitions = []DefCompatEntry{}
	}
	return report
}

func deduplicateDefHints(revisions []defRevCompatEntry) []DefIssueEntry {
	type hintKey struct{ introduced, id, reason string }
	issueRevisions := map[hintKey][]string{}
	var issueOrder []hintKey
	seenIssue := map[hintKey]bool{}
	seenRev := map[string]bool{}
	for _, r := range revisions {
		if seenRev[r.revision] {
			continue
		}
		seenRev[r.revision] = true
		for _, h := range r.reasons {
			k := hintKey{h.Introduced, h.ID, h.Reason}
			issueRevisions[k] = append(issueRevisions[k], r.revision)
			if !seenIssue[k] {
				issueOrder = append(issueOrder, k)
				seenIssue[k] = true
			}
		}
	}
	var issues []DefIssueEntry
	for _, k := range issueOrder {
		issues = append(issues, DefIssueEntry{
			Introduced: k.introduced,
			ID:         k.id,
			Reason:     k.reason,
			Revisions:  issueRevisions[k],
		})
	}
	return issues
}

func printDefCompatReport(ioStreams util.IOStreams, report DefCompatReport, format string) error {
	switch format {
	case "json":
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(ioStreams.Out, string(b))
		return nil
	case outputFormatYAML:
		b, err := yaml.Marshal(report)
		if err != nil {
			return err
		}
		fmt.Fprint(ioStreams.Out, string(b))
		return nil
	default:
		if len(report.Definitions) == 0 {
			fmt.Fprintln(ioStreams.Out, "All definitions are compatible.")
			return nil
		}
		fmt.Fprintf(ioStreams.Out, "Found %d incompatible definition(s):\n", report.Total)
		for _, d := range report.Definitions {
			fmt.Fprintln(ioStreams.Out)
			fmt.Fprintf(ioStreams.Out, "  %s (%s, %s)\n", d.Name, d.Kind, d.Namespace)

			for i, issue := range d.Issues {
				fmt.Fprintf(ioStreams.Out, "    - introduced: %s\n", issue.Introduced)
				fmt.Fprintf(ioStreams.Out, "      id: %s\n", issue.ID)
				fmt.Fprintf(ioStreams.Out, "      reason: %s\n", issue.Reason)
				fmt.Fprintf(ioStreams.Out, "      revisions: %s\n", strings.Join(issue.Revisions, ", "))
				if i < len(d.Issues)-1 {
					fmt.Fprintln(ioStreams.Out)
				}
			}
		}
		fmt.Fprintln(ioStreams.Out)
		fmt.Fprintln(ioStreams.Out, "Run `vela def upgrade <file>` to fix individual definition files.")
		return nil
	}
}

// scanAppsOptions holds parameters for scanApplications.
type scanAppsOptions struct {
	allTypes           []string
	namespace          string
	labelSelector      string
	annotationSelector string
}

func scanApplications(ctx context.Context, k8sClient client.Client, opts scanAppsOptions) (AppCompatReport, error) {
	typeSet := map[string]bool{}
	for _, t := range opts.allTypes {
		typeSet[t] = true
	}
	labelSel, err := parseSelectorMap(opts.labelSelector)
	if err != nil {
		return AppCompatReport{}, errors.Wrap(err, "--label-selector")
	}
	annotationSel, err := parseSelectorMap(opts.annotationSelector)
	if err != nil {
		return AppCompatReport{}, errors.Wrap(err, "--annotation-selector")
	}

	var listOpts []client.ListOption
	if opts.namespace != "" {
		listOpts = append(listOpts, client.InNamespace(opts.namespace))
	}
	if len(labelSel) > 0 {
		listOpts = append(listOpts, client.MatchingLabels(labelSel))
	}

	appList := &v1beta1.ApplicationList{}
	if err := k8sClient.List(ctx, appList, listOpts...); err != nil {
		return AppCompatReport{}, errors.Wrap(err, "failed to list Applications")
	}

	var appEntries []AppCompatEntry
	for _, app := range appList.Items {
		if len(annotationSel) > 0 && !matchesSelector(app.GetAnnotations(), annotationSel) {
			continue
		}
		if app.Status.LatestRevision == nil || app.Status.LatestRevision.Name == "" {
			continue
		}
		rev := &v1beta1.ApplicationRevision{}
		if err := k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Status.LatestRevision.Name,
		}, rev); err != nil {
			continue
		}
		appIssues := scanAppRevision(rev, typeSet)
		if len(appIssues) == 0 {
			continue
		}
		appEntries = append(appEntries, AppCompatEntry{
			Application: app.Name,
			Namespace:   app.Namespace,
			Revision:    app.Status.LatestRevision.Name,
			Issues:      appIssues,
		})
	}
	sort.Slice(appEntries, func(i, j int) bool {
		if appEntries[i].Namespace != appEntries[j].Namespace {
			return appEntries[i].Namespace < appEntries[j].Namespace
		}
		return appEntries[i].Application < appEntries[j].Application
	})
	report := AppCompatReport{Total: len(appEntries), Applications: appEntries}
	if report.Applications == nil {
		report.Applications = []AppCompatEntry{}
	}
	return report, nil
}

func scanAppRevision(rev *v1beta1.ApplicationRevision, typeSet map[string]bool) []AppIssueEntry {
	type rawDefEntry struct {
		name    string
		kind    string
		reasons []CompatibilityHint
	}
	var rawDefs []rawDefEntry
	collect := func(name, kind, template string) {
		if needs, reasons, err := upgrade.RequiresUpgrade(template); err == nil && needs {
			rawDefs = append(rawDefs, rawDefEntry{name: name, kind: kind, reasons: parseHints(reasons)})
		}
	}
	for defName, def := range rev.Spec.ComponentDefinitions {
		if typeSet["component"] && def != nil && def.Spec.Schematic != nil && def.Spec.Schematic.CUE != nil {
			collect(defName, "component", def.Spec.Schematic.CUE.Template)
		}
	}
	for defName, def := range rev.Spec.TraitDefinitions {
		if typeSet["trait"] && def != nil && def.Spec.Schematic != nil && def.Spec.Schematic.CUE != nil {
			collect(defName, "trait", def.Spec.Schematic.CUE.Template)
		}
	}
	for defName, def := range rev.Spec.WorkflowStepDefinitions {
		if typeSet["workflow-step"] && def != nil && def.Spec.Schematic != nil && def.Spec.Schematic.CUE != nil {
			collect(defName, "workflow-step", def.Spec.Schematic.CUE.Template)
		}
	}
	for defName, def := range rev.Spec.PolicyDefinitions {
		if typeSet["policy"] && def.Spec.Schematic != nil && def.Spec.Schematic.CUE != nil {
			collect(defName, "policy", def.Spec.Schematic.CUE.Template)
		}
	}
	if len(rawDefs) == 0 {
		return nil
	}
	sort.Slice(rawDefs, func(i, j int) bool {
		if rawDefs[i].kind != rawDefs[j].kind {
			return rawDefs[i].kind < rawDefs[j].kind
		}
		return rawDefs[i].name < rawDefs[j].name
	})

	type hintKey struct{ introduced, id, reason string }
	type kindBuckets struct{ components, traits, policies, workflowSteps []string }
	issueMap := map[hintKey]*kindBuckets{}
	var issueOrder []hintKey
	seenIssue := map[hintKey]bool{}
	for _, rd := range rawDefs {
		for _, h := range rd.reasons {
			k := hintKey{h.Introduced, h.ID, h.Reason}
			if !seenIssue[k] {
				issueMap[k] = &kindBuckets{}
				issueOrder = append(issueOrder, k)
				seenIssue[k] = true
			}
			switch rd.kind {
			case "component":
				issueMap[k].components = append(issueMap[k].components, rd.name)
			case "trait":
				issueMap[k].traits = append(issueMap[k].traits, rd.name)
			case "policy":
				issueMap[k].policies = append(issueMap[k].policies, rd.name)
			case "workflow-step":
				issueMap[k].workflowSteps = append(issueMap[k].workflowSteps, rd.name)
			}
		}
	}
	var appIssues []AppIssueEntry
	for _, k := range issueOrder {
		b := issueMap[k]
		appIssues = append(appIssues, AppIssueEntry{
			Introduced:    k.introduced,
			ID:            k.id,
			Reason:        k.reason,
			Components:    b.components,
			Traits:        b.traits,
			Policies:      b.policies,
			WorkflowSteps: b.workflowSteps,
		})
	}
	return appIssues
}

// --- `vela def compatibility-check applications` ---

// NewDefinitionCompatibilityApplicationsCommand creates `vela def compatibility-check applications`.
func NewDefinitionCompatibilityApplicationsCommand(c common.Args, ioStreams util.IOStreams) *cobra.Command {
	var (
		outputFormat       string
		namespace          string
		labelSelector      string
		annotationSelector string
	)

	allTypes := []string{"component", "trait", "workflow-step", "policy"}

	cmd := &cobra.Command{
		Use:   "applications",
		Short: "Scan active ApplicationRevisions for CUE incompatibilities",
		Long: "Scans the active ApplicationRevision for every Application across all\n" +
			"namespaces, reporting incompatible definition snapshots grouped by Application.",
		Example: "# Scan all active ApplicationRevisions\n" +
			"vela def compatibility-check applications\n\n" +
			"# Output as YAML\n" +
			"vela def compatibility-check applications -o yaml",
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeDefManagement,
			types.TagCommandOrder: "2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := c.GetClient()
			if err != nil {
				return errors.Wrap(err, "failed to get k8s client")
			}
			report, err := scanApplications(context.Background(), k8sClient, scanAppsOptions{
				allTypes:           allTypes,
				namespace:          namespace,
				labelSelector:      labelSelector,
				annotationSelector: annotationSelector,
			})
			if err != nil {
				return err
			}
			return printAppCompatReport(ioStreams, report, outputFormat)
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format. One of: table, yaml, json.")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Only scan Applications in this namespace (default: all namespaces).")
	cmd.Flags().StringVarP(&labelSelector, "label-selector", "l", "", "Filter Applications by labels, e.g. -l env=prod.")
	cmd.Flags().StringVar(&annotationSelector, "annotation-selector", "", "Filter Applications by annotations, e.g. --annotation-selector owner=team-a.")
	return cmd
}

func printAppCompatReport(ioStreams util.IOStreams, report AppCompatReport, format string) error {
	switch format {
	case "json":
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(ioStreams.Out, string(b))
		return nil
	case outputFormatYAML:
		b, err := yaml.Marshal(report)
		if err != nil {
			return err
		}
		fmt.Fprint(ioStreams.Out, string(b))
		return nil
	default:
		if len(report.Applications) == 0 {
			fmt.Fprintln(ioStreams.Out, "All active ApplicationRevisions are compatible.")
			return nil
		}
		fmt.Fprintf(ioStreams.Out, "Found %d application(s) with incompatible definition snapshots:\n", report.Total)
		for _, app := range report.Applications {
			fmt.Fprintln(ioStreams.Out)
			fmt.Fprintf(ioStreams.Out, "  %s (%s)  revision: %s\n", app.Application, app.Namespace, app.Revision)
			for i, issue := range app.Issues {
				fmt.Fprintf(ioStreams.Out, "    - introduced: %s\n", issue.Introduced)
				fmt.Fprintf(ioStreams.Out, "      id: %s\n", issue.ID)
				fmt.Fprintf(ioStreams.Out, "      reason: %s\n", issue.Reason)
				if len(issue.Components) > 0 {
					fmt.Fprintf(ioStreams.Out, "      components: %s\n", strings.Join(issue.Components, ", "))
				}
				if len(issue.Traits) > 0 {
					fmt.Fprintf(ioStreams.Out, "      traits: %s\n", strings.Join(issue.Traits, ", "))
				}
				if len(issue.Policies) > 0 {
					fmt.Fprintf(ioStreams.Out, "      policies: %s\n", strings.Join(issue.Policies, ", "))
				}
				if len(issue.WorkflowSteps) > 0 {
					fmt.Fprintf(ioStreams.Out, "      workflowSteps: %s\n", strings.Join(issue.WorkflowSteps, ", "))
				}
				if i < len(app.Issues)-1 {
					fmt.Fprintln(ioStreams.Out)
				}
			}
		}
		fmt.Fprintln(ioStreams.Out)
		fmt.Fprintln(ioStreams.Out, "Run `vela def upgrade <file>` to fix individual definition files.")
		return nil
	}
}

// --- Helpers ---

// extractCUETemplate walks spec.schematic.cue.template in an unstructured object.
func extractCUETemplate(obj map[string]any) (string, bool) {
	spec, ok := obj["spec"].(map[string]any)
	if !ok {
		return "", false
	}
	schematic, ok := spec["schematic"].(map[string]any)
	if !ok {
		return "", false
	}
	cue, ok := schematic["cue"].(map[string]any)
	if !ok {
		return "", false
	}
	tmpl, ok := cue["template"].(string)
	return tmpl, ok
}

// extractDefRevTemplate returns the CUE template from a DefinitionRevision regardless of type.
func extractDefRevTemplate(dr v1beta1.DefinitionRevision) string {
	switch dr.Spec.DefinitionType {
	case oamcommon.ComponentType:
		if dr.Spec.ComponentDefinition.Spec.Schematic != nil && dr.Spec.ComponentDefinition.Spec.Schematic.CUE != nil {
			return dr.Spec.ComponentDefinition.Spec.Schematic.CUE.Template
		}
	case oamcommon.TraitType:
		if dr.Spec.TraitDefinition.Spec.Schematic != nil && dr.Spec.TraitDefinition.Spec.Schematic.CUE != nil {
			return dr.Spec.TraitDefinition.Spec.Schematic.CUE.Template
		}
	case oamcommon.WorkflowStepType:
		if dr.Spec.WorkflowStepDefinition.Spec.Schematic != nil && dr.Spec.WorkflowStepDefinition.Spec.Schematic.CUE != nil {
			return dr.Spec.WorkflowStepDefinition.Spec.Schematic.CUE.Template
		}
	case oamcommon.PolicyType:
		if dr.Spec.PolicyDefinition.Spec.Schematic != nil && dr.Spec.PolicyDefinition.Spec.Schematic.CUE != nil {
			return dr.Spec.PolicyDefinition.Spec.Schematic.CUE.Template
		}
	}
	return ""
}

// defNameFromRevision extracts the underlying definition name from a DefinitionRevision
// using the well-known label for its type (e.g. componentdefinitions.core.oam.dev/name).
func defNameFromRevision(dr v1beta1.DefinitionRevision) string {
	if labelKey, ok := pkgdef.DefinitionKindToNameLabel[dr.Spec.DefinitionType]; ok {
		if name := dr.GetLabels()[labelKey]; name != "" {
			return name
		}
	}
	// Fallback: strip the trailing "-vN" from the revision object name.
	name := dr.GetName()
	if idx := strings.LastIndex(name, "-v"); idx != -1 {
		return name[:idx]
	}
	return name
}
