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
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// MigrateCommandOptions are the options shared by `config-template migrate` and
// `config migrate`.
type MigrateCommandOptions struct {
	Name      string
	Namespace string
	All       bool
	DryRun    bool
}

// validate applies the name-or-all rule. what names the object kind in messages.
func (o *MigrateCommandOptions) validate(args []string, what string) error {
	if len(args) == 1 {
		o.Name = args[0]
	}
	switch {
	case o.Name != "" && o.All:
		return fmt.Errorf("specify a %s name or --all, not both", what)
	case o.Name == "" && !o.All:
		return fmt.Errorf("specify a %s name or --all", what)
	}
	return nil
}

// migrationRunner runs one migration against the library.
type migrationRunner func(ctx context.Context, m *config.Migrator, opts config.MigrationOptions) (config.MigrationReport, error)

// NewTemplateMigrateCommand migrates one legacy template and its configs, or every
// legacy template in a namespace, into CRs.
func NewTemplateMigrateCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options MigrateCommandOptions
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: i18n.T("Adopt a legacy config template and its configs into ConfigTemplate and Config CRs."),
		Long: i18n.T(`Adopt a legacy config template (a config-template-<name> ConfigMap) and every legacy
config made from it, in any namespace, into ConfigTemplate and Config CRs. Nothing is deleted:
each config Secret becomes owned by its Config, the template ConfigMap is annotated with the
ConfigTemplate that replaced it, and a second run skips both. --all does this for every legacy
template in the namespace, then for the legacy configs left there. Objects a GitOps tool applies
are adopted too; the report says which, and their new CRs belong where the legacy ones are kept.`),
		Example: `  vela config-template migrate my-template -n vela-system
  vela config-template migrate my-template -n vela-system --dry-run
  vela config-template migrate --all -n vela-system`,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeExtension,
		},
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := options.validate(args, "template"); err != nil {
				return err
			}
			return runMigration(cmd.Context(), f, streams, options, templateMigrationKinds, func(ctx context.Context, m *config.Migrator, opts config.MigrationOptions) (config.MigrationReport, error) {
				if options.All {
					return m.MigrateNamespace(ctx, options.Namespace, opts)
				}
				return m.MigrateTemplate(ctx, options.Namespace, options.Name, opts)
			})
		},
	}
	addMigrateFlags(cmd, &options, "template")
	return cmd
}

// NewConfigMigrateCommand migrates one legacy config, or every legacy config in a
// namespace, into Config CRs. Templates are left as they are.
func NewConfigMigrateCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options MigrateCommandOptions
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: i18n.T("Adopt a legacy config into a Config CR."),
		Long: i18n.T(`Adopt a legacy config (a velacore-config Secret) into a Config CR of the same name.
The template may already be a ConfigTemplate CR or still a legacy ConfigMap; both work, unless a
ConfigTemplate that migration did not create shadows the legacy ConfigMap, which is a conflict. The
Secret is kept and becomes owned by the Config; a second run skips it. --all does this for
every legacy config in the namespace, with or without a template. Secrets a GitOps tool applies
are adopted too; the report says which, and their new Configs belong where the Secrets are kept.`),
		Example: `  vela config migrate my-config -n my-team
  vela config migrate my-config -n my-team --dry-run
  vela config migrate --all -n my-team`,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := options.validate(args, "config"); err != nil {
				return err
			}
			return runMigration(cmd.Context(), f, streams, options, configMigrationKinds, func(ctx context.Context, m *config.Migrator, opts config.MigrationOptions) (config.MigrationReport, error) {
				if options.All {
					return m.MigrateConfigs(ctx, options.Namespace, opts)
				}
				return m.MigrateConfig(ctx, options.Namespace, options.Name, opts)
			})
		},
	}
	addMigrateFlags(cmd, &options, "config")
	return cmd
}

func addMigrateFlags(cmd *cobra.Command, options *MigrateCommandOptions, what string) {
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the "+what)
	cmd.Flags().BoolVarP(&options.All, "all", "", false, "migrate every legacy "+what+" in the namespace")
	cmd.Flags().BoolVarP(&options.DryRun, "dry-run", "", false, "print the CRs that would be created and the objects that would be adopted, and change nothing")
}

// crdProbe pairs a kind a command writes with the error for a cluster that does
// not serve it. The two CRDs install separately, so each command probes only what
// it writes: template migration writes both kinds, config migration Configs only
// (its template lookup already tolerates a missing ConfigTemplate type).
type crdProbe struct {
	kind    schema.GroupVersionKind
	missing error
}

var (
	templateMigrationKinds = []crdProbe{
		{configv1alpha1.ConfigTemplateGroupVersionKind, errConfigTemplateCRDMissing},
		{configv1alpha1.ConfigGroupVersionKind, errConfigCRDMissing},
	}
	configMigrationKinds = []crdProbe{
		{configv1alpha1.ConfigGroupVersionKind, errConfigCRDMissing},
	}
)

// runMigration probes the CRDs the command writes, runs the migration, prints the
// dry-run documents and the result table, and turns conflicts and failures into a
// non-zero exit.
func runMigration(ctx context.Context, f velacmd.Factory, streams util.IOStreams, options MigrateCommandOptions, probes []crdProbe, run migrationRunner) error {
	for _, probe := range probes {
		if !kindServed(f, probe.kind) {
			return probe.missing
		}
	}
	opts := config.MigrationOptions{DryRun: options.DryRun}
	if !options.DryRun {
		opts.Recorder = &syncEventRecorder{ctx: ctx, cli: f.Client()}
	}
	report, err := run(ctx, config.NewMigrator(f.Client()), opts)
	if err != nil {
		return err
	}
	if options.DryRun {
		if err := printMigrationTargets(streams, report); err != nil {
			return err
		}
	}
	if err := printMigrationReport(streams, report, options.DryRun); err != nil {
		return err
	}
	return migrationExitError(report)
}

const eventComponent = "vela-cli"

// syncEventRecorder writes each Event straight to the API server. client-go's
// broadcaster hands Events to a goroutine and a short-lived CLI exits before that
// goroutine has posted them, so migration Events went missing. A failed write is
// logged and dropped: the migration itself has already happened.
type syncEventRecorder struct {
	ctx context.Context
	cli client.Client
}

var _ record.EventRecorder = &syncEventRecorder{}

func (r *syncEventRecorder) Event(object runtime.Object, eventType, reason, message string) {
	ref, err := reference.GetReference(r.cli.Scheme(), object)
	if err != nil {
		klog.Warningf("could not record event %s on %T: %v", reason, object, err)
		return
	}
	now := metav1.NewTime(time.Now())
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s.%x", ref.Name, now.UnixNano()),
			Namespace: ref.Namespace,
		},
		InvolvedObject:      *ref,
		Reason:              reason,
		Message:             message,
		Type:                eventType,
		Source:              corev1.EventSource{Component: eventComponent},
		FirstTimestamp:      now,
		LastTimestamp:       now,
		Count:               1,
		ReportingController: eventComponent,
		ReportingInstance:   eventComponent,
	}
	if err := r.cli.Create(r.ctx, event); err != nil {
		klog.Warningf("could not record event %s on %s %s/%s: %v", reason, ref.Kind, ref.Namespace, ref.Name, err)
	}
}

func (r *syncEventRecorder) Eventf(object runtime.Object, eventType, reason, messageFmt string, args ...interface{}) {
	r.Event(object, eventType, reason, fmt.Sprintf(messageFmt, args...))
}

func (r *syncEventRecorder) AnnotatedEventf(object runtime.Object, _ map[string]string, eventType, reason, messageFmt string, args ...interface{}) {
	r.Eventf(object, eventType, reason, messageFmt, args...)
}

// printMigrationTargets prints the CRs a dry run would create, one YAML document each.
func printMigrationTargets(streams util.IOStreams, report config.MigrationReport) error {
	for _, res := range report.Results {
		if res.Outcome != config.MigrationAdopted || res.Target == nil {
			continue
		}
		body, err := yaml.Marshal(res.Target)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(streams.Out, "---\n%s", body); err != nil {
			return err
		}
	}
	return nil
}

// printMigrationReport prints the result table and the summary line.
func printMigrationReport(streams util.IOStreams, report config.MigrationReport, dryRun bool) error {
	table := newUITable()
	table.AddRow("KIND", "NAME", "NAMESPACE", "RESULT", "REASON")
	for _, res := range report.Results {
		result := string(res.Outcome)
		if dryRun && res.Outcome == config.MigrationAdopted {
			result = "would adopt"
		}
		table.AddRow(res.Kind, res.Name, res.Namespace, result, res.Reason)
	}
	if _, err := streams.Out.Write(table.Bytes()); err != nil {
		return err
	}
	_, err := fmt.Fprintf(streams.Out, "\nadopted %d, skipped %d, conflict %d, failed %d\n",
		report.Count(config.MigrationAdopted), report.Count(config.MigrationSkipped),
		report.Count(config.MigrationConflict), report.Count(config.MigrationFailed))
	return err
}

// migrationExitError is the error a run ends with when anything conflicted or
// failed, so scripts see a non-zero exit and the count.
func migrationExitError(report config.MigrationReport) error {
	var parts []string
	if n := report.Count(config.MigrationConflict); n > 0 {
		parts = append(parts, fmt.Sprintf("%d object(s) in conflict", n))
	}
	if n := report.Count(config.MigrationFailed); n > 0 {
		parts = append(parts, fmt.Sprintf("%d object(s) failed", n))
	}
	if len(parts) == 0 {
		return nil
	}
	return fmt.Errorf("%s; see the table above", strings.Join(parts, " and "))
}
