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

package cli

import (
	"context"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/AlecAivazis/survey/v2"
	"github.com/FogDong/uitable"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/cue/model/sets"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/packages"
	"github.com/kubevela/workflow/pkg/debug"
	"github.com/kubevela/workflow/pkg/tasks/custom"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/dryrun"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

type debugOpts struct {
	step   string
	focus  string
	errMsg string
	opts   []string
	errMap map[string]string
	// TODO: (fog) add watch flag
	// watch bool
}

// NewDebugCommand create `debug` command
func NewDebugCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	dOpts := &debugOpts{}
	wargs := &WorkflowArgs{
		Args:   c,
		Writer: ioStreams.Out,
	}
	cmd := &cobra.Command{
		Use:     "debug",
		Aliases: []string{"debug"},
		Short:   "Debug running application",
		Long:    "Debug running application with debug policy.",
		Example: `vela debug <application-name>`,
		PreRun:  wargs.checkDebugMode(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify application name")
			}
			if err := wargs.getWorkflowInstance(ctx, cmd, args); err != nil {
				return err
			}
			if wargs.Type == instanceTypeWorkflowRun {
				return fmt.Errorf("please use `vela workflow debug <name>` instead")
			}
			if wargs.App == nil {
				return fmt.Errorf("application %s not found", args[0])
			}

			return dOpts.debugApplication(ctx, wargs, c, ioStreams)
		},
	}
	addNamespaceAndEnvArg(cmd)
	cmd.Flags().StringVarP(&dOpts.step, "step", "s", "", "specify the step or component to debug")
	cmd.Flags().StringVarP(&dOpts.focus, "focus", "f", "", "specify the focus value to debug, only valid for application with workflow")
	return cmd
}

func (d *debugOpts) debugApplication(ctx context.Context, wargs *WorkflowArgs, c common.Args, ioStreams cmdutil.IOStreams) error {
	app := wargs.App
	cli, err := c.GetClient()
	if err != nil {
		return err
	}
	config, err := c.GetConfig()
	if err != nil {
		return err
	}
	pd, err := c.GetPackageDiscover()
	if err != nil {
		return err
	}
	d.opts = wargs.getWorkflowSteps()
	d.errMap = wargs.ErrMap
	if app.Spec.Workflow != nil && len(app.Spec.Workflow.Steps) > 0 {
		return d.debugWorkflow(ctx, wargs, cli, pd, ioStreams)
	}

	dm, err := discoverymapper.New(config)
	if err != nil {
		return err
	}
	dryRunOpt := dryrun.NewDryRunOption(cli, config, dm, pd, []oam.Object{}, false)
	comps, _, err := dryRunOpt.ExecuteDryRun(ctx, app)
	if err != nil {
		ioStreams.Info(color.RedString("%s%s", emojiFail, err.Error()))
		return nil
	}
	if err := d.debugComponents(comps, ioStreams); err != nil {
		return err
	}
	return nil
}

func (d *debugOpts) debugWorkflow(ctx context.Context, wargs *WorkflowArgs, cli client.Client, pd *packages.PackageDiscover, ioStreams cmdutil.IOStreams) error {
	if d.step == "" {
		prompt := &survey.Select{
			Message: "Select the workflow step to debug:",
			Options: d.opts,
		}
		var step string
		err := survey.AskOne(prompt, &step, survey.WithValidator(survey.Required))
		if err != nil {
			return fmt.Errorf("failed to select workflow step: %w", err)
		}
		d.step = unwrapStepID(step, wargs.WorkflowInstance)
		d.errMsg = d.errMap[d.step]
	} else {
		d.step = unwrapStepID(d.step, wargs.WorkflowInstance)
	}

	// debug workflow steps
	rawValue, data, err := d.getDebugRawValue(ctx, cli, pd, wargs.WorkflowInstance)
	if err != nil {
		if data != "" {
			ioStreams.Info(color.RedString("%s%s", emojiFail, err.Error()))
			ioStreams.Info(color.GreenString("Original Data in Debug:\n"), data)
			return nil
		}
		return err
	}

	if err := d.handleCueSteps(rawValue, ioStreams); err != nil {
		ioStreams.Info(color.RedString("%s%s", emojiFail, err.Error()))
		ioStreams.Info(color.GreenString("Original Data in Debug:\n"), data)
		return nil
	}
	return nil
}

func (d *debugOpts) debugComponents(comps []*types.ComponentManifest, ioStreams cmdutil.IOStreams) error {
	opts := d.opts
	all := color.YellowString("all fields")
	exit := color.CyanString("exit debug mode")
	opts = append(opts, all, exit)

	var components = make(map[string]*unstructured.Unstructured)
	var traits = make(map[string][]*unstructured.Unstructured)
	for _, comp := range comps {
		components[comp.Name] = comp.StandardWorkload
		traits[comp.Name] = comp.Traits
	}

	if d.step != "" {
		return renderComponents(d.step, components[d.step], traits[d.step], ioStreams)
	}
	for {
		prompt := &survey.Select{
			Message: "Select the components to debug:",
			Options: opts,
		}
		var step string
		err := survey.AskOne(prompt, &step, survey.WithValidator(survey.Required))
		if err != nil {
			return fmt.Errorf("failed to select components: %w", err)
		}

		if step == exit {
			break
		}
		if step == all {
			for _, step := range d.opts {
				step = unwrapStepName(step)
				if err := renderComponents(step, components[step], traits[step], ioStreams); err != nil {
					return err
				}
			}
			continue
		}
		step = unwrapStepName(step)
		if err := renderComponents(step, components[step], traits[step], ioStreams); err != nil {
			return err
		}
	}
	return nil
}

func renderComponents(compName string, comp *unstructured.Unstructured, traits []*unstructured.Unstructured, ioStreams cmdutil.IOStreams) error {
	ioStreams.Info(color.CyanString("\n▫️ %s", compName))
	result, err := yaml.Marshal(comp)
	if err != nil {
		return errors.WithMessage(err, "marshal result for component "+compName+" object in yaml format")
	}
	ioStreams.Info(string(result), "\n")
	for _, t := range traits {
		result, err := yaml.Marshal(t)
		if err != nil {
			return errors.WithMessage(err, "marshal result for component "+compName+" object in yaml format")
		}
		ioStreams.Info(string(result), "\n")
	}
	return nil
}

func wrapStepName(step workflowv1alpha1.StepStatus) string {
	var stepName string
	switch step.Phase {
	case workflowv1alpha1.WorkflowStepPhaseSucceeded:
		stepName = emojiSucceed + step.Name
	case workflowv1alpha1.WorkflowStepPhaseFailed:
		stepName = emojiFail + step.Name
	case workflowv1alpha1.WorkflowStepPhaseSkipped:
		stepName = emojiSkip + step.Name
	default:
		stepName = emojiExecuting + step.Name
	}
	return stepName
}

func unwrapStepName(step string) string {
	step = strings.TrimPrefix(step, "  ")
	switch {
	case strings.HasPrefix(step, emojiSucceed):
		return strings.TrimPrefix(step, emojiSucceed)
	case strings.HasPrefix(step, emojiFail):
		return strings.TrimPrefix(step, emojiFail)
	case strings.HasPrefix(step, emojiSkip):
		return strings.TrimPrefix(step, emojiSkip)
	case strings.HasPrefix(step, emojiExecuting):
		return strings.TrimPrefix(step, emojiExecuting)
	default:
		return step
	}
}

func unwrapStepID(step string, instance *wfTypes.WorkflowInstance) string {
	step = unwrapStepName(step)
	for _, status := range instance.Status.Steps {
		if status.Name == step {
			return status.ID
		}
		for _, sub := range status.SubStepsStatus {
			if sub.Name == step {
				return sub.ID
			}
		}
	}
	return step
}

func (d *debugOpts) getDebugRawValue(ctx context.Context, cli client.Client, pd *packages.PackageDiscover, instance *wfTypes.WorkflowInstance) (*value.Value, string, error) {
	debugCM := &corev1.ConfigMap{}
	if err := cli.Get(ctx, client.ObjectKey{Name: debug.GenerateContextName(instance.Name, d.step, string(instance.UID)), Namespace: instance.Namespace}, debugCM); err != nil {
		for _, step := range instance.Status.Steps {
			if step.Name == d.step && (step.Type == wfTypes.WorkflowStepTypeSuspend || step.Type == wfTypes.WorkflowStepTypeStepGroup) {
				return nil, "", fmt.Errorf("no debug data for a suspend or step-group step, please choose another step")
			}
			for _, sub := range step.SubStepsStatus {
				if sub.Name == d.step && sub.Type == wfTypes.WorkflowStepTypeSuspend {
					return nil, "", fmt.Errorf("no debug data for a suspend step, please choose another step")
				}
			}
		}
		return nil, "", fmt.Errorf("failed to get debug configmap, please make sure the you're in the debug mode`: %w", err)
	}

	if debugCM.Data == nil || debugCM.Data["debug"] == "" {
		return nil, "", fmt.Errorf("debug configmap is empty")
	}
	v, err := value.NewValue(debugCM.Data["debug"], pd, "")
	if err != nil {
		return nil, debugCM.Data["debug"], fmt.Errorf("failed to parse debug configmap: %w", err)
	}
	return v, debugCM.Data["debug"], nil
}

func (d *debugOpts) handleCueSteps(v *value.Value, ioStreams cmdutil.IOStreams) error {
	if d.focus != "" {
		f, err := v.LookupValue(strings.Split(d.focus, ".")...)
		if err != nil {
			return err
		}
		ioStreams.Info(color.New(color.FgCyan).Sprint("\n", d.focus, "\n"))
		rendered, err := renderFields(f, &renderOptions{})
		if err != nil {
			return err
		}
		ioStreams.Info(rendered, "\n")
		return nil
	}

	if err := d.separateBySteps(v, ioStreams); err != nil {
		return err
	}
	return nil
}

func (d *debugOpts) separateBySteps(v *value.Value, ioStreams cmdutil.IOStreams) error {
	fieldMap := make(map[string]*value.Value)
	fieldList := make([]string, 0)
	if err := v.StepByFields(func(fieldName string, in *value.Value) (bool, error) {
		if in.CueValue().IncompleteKind() == cue.BottomKind {
			errInfo, err := sets.ToString(in.CueValue())
			if err != nil {
				errInfo = "value is _|_"
			}
			return true, errors.New(errInfo + "value is _|_ (bottom kind)")
		}
		fieldList = append(fieldList, fieldName)
		fieldMap[fieldName] = in
		return false, nil
	}); err != nil {
		return fmt.Errorf("failed to parse debug configmap by field: %w", err)
	}

	errStep := ""
	if d.errMsg != "" {
		s := strings.Split(d.errMsg, ":")
		errStep = strings.TrimPrefix(s[0], "step ")
	}
	opts := make([]string, 0)
	for _, field := range fieldList {
		if field == errStep {
			opts = append(opts, emojiFail+field)
		} else {
			opts = append(opts, emojiSucceed+field)
		}
	}
	all := color.YellowString("all fields")
	exit := color.CyanString("exit debug mode")
	opts = append(opts, all, exit)
	for {
		prompt := &survey.Select{
			Message: "Select the field to debug: ",
			Options: opts,
		}
		var field string
		err := survey.AskOne(prompt, &field, survey.WithValidator(survey.Required))
		if err != nil {
			return fmt.Errorf("failed to select: %w", err)
		}
		if field == exit {
			break
		}
		if field == all {
			for _, field := range fieldList {
				ioStreams.Info(color.CyanString("\n▫️ %s", field))
				rendered, err := renderFields(fieldMap[field], &renderOptions{})
				if err != nil {
					return err
				}
				ioStreams.Info(rendered, "\n")
			}
			continue
		}
		field = unwrapStepName(field)
		ioStreams.Info(color.CyanString("\n▫️ %s", field))
		rendered, err := renderFields(fieldMap[field], &renderOptions{})
		if err != nil {
			return err
		}
		ioStreams.Info(rendered, "\n")
	}
	return nil
}

type renderOptions struct {
	hideIndex    bool
	filterFields []string
}

func renderFields(v *value.Value, opt *renderOptions) (string, error) {
	table := uitable.New()
	table.MaxColWidth = 200
	table.Wrap = true
	i := 0

	if err := v.StepByFields(func(fieldName string, in *value.Value) (bool, error) {
		key := ""
		if custom.OpTpy(in) != "" {
			rendered, err := renderFields(in, opt)
			if err != nil {
				return false, err
			}
			i++
			if !opt.hideIndex {
				key += fmt.Sprintf("%v.", i)
			}
			key += fieldName
			if !strings.Contains(fieldName, "#") {
				if err := v.FillObject(in, fieldName); err != nil {
					renderValuesInRow(table, key, rendered, false)
					return false, err
				}
			}
			if len(opt.filterFields) > 0 {
				for _, filter := range opt.filterFields {
					if filter != fieldName {
						renderValuesInRow(table, key, rendered, true)
					}
				}
			} else {
				renderValuesInRow(table, key, rendered, true)
			}
			return false, nil
		}

		vStr, err := in.String()
		if err != nil {
			return false, err
		}
		i++
		if !opt.hideIndex {
			key += fmt.Sprintf("%v.", i)
		}
		key += fieldName
		if !strings.Contains(fieldName, "#") {
			if err := v.FillObject(in, fieldName); err != nil {
				renderValuesInRow(table, key, vStr, false)
				return false, err
			}
		}

		if len(opt.filterFields) > 0 {
			for _, filter := range opt.filterFields {
				if filter != fieldName {
					renderValuesInRow(table, key, vStr, true)
				}
			}
		} else {
			renderValuesInRow(table, key, vStr, true)
		}
		return false, nil
	}); err != nil {
		vStr, serr := v.String()
		if serr != nil {
			return "", serr
		}
		if strings.Contains(err.Error(), "(type string) as struct") {
			return strings.TrimSpace(vStr), nil
		}
	}

	return table.String(), nil
}

func renderValuesInRow(table *uitable.Table, k, v string, isPass bool) {
	v = strings.TrimSpace(v)
	if isPass {
		if strings.Contains(k, "#do") || strings.Contains(k, "#provider") {
			k = color.YellowString("%s:", k)
		} else {
			k = color.GreenString("%s:", k)
		}
	} else {
		k = color.RedString("%s:", k)
		v = color.RedString("%s%s", emojiFail, v)
	}
	if v == `"steps"` {
		v = color.BlueString(v)
	}

	table.AddRow(k, v)
}
