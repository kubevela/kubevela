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

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/dryrun"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/pkg/workflow/debug"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/custom"
	"github.com/oam-dev/kubevela/references/appfile"
)

type debugOpts struct {
	step   string
	focus  string
	errMsg string
	// TODO: (fog) add watch flag
	// watch bool
}

// NewDebugCommand create `debug` command
func NewDebugCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	dOpts := &debugOpts{}
	cmd := &cobra.Command{
		Use:     "debug",
		Aliases: []string{"debug"},
		Short:   "Debug running application",
		Long:    "Debug running application with debug policy.",
		Example: `vela debug <application-name>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify application name")
			}
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			app, err := appfile.LoadApplication(namespace, args[0], c)
			if err != nil {
				return err
			}
			return dOpts.debugApplication(ctx, c, app, ioStreams)
		},
	}
	addNamespaceAndEnvArg(cmd)
	cmd.Flags().StringVarP(&dOpts.step, "step", "s", "", "specify the step to debug")
	cmd.Flags().StringVarP(&dOpts.focus, "focus", "f", "", "specify the focus value to debug")
	return cmd
}

func (d *debugOpts) debugApplication(ctx context.Context, c common.Args, app *v1beta1.Application, ioStreams cmdutil.IOStreams) error {
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

	s, opts, errMap := d.getDebugOptions(app)
	if s == "workflow steps" {
		if d.step == "" {
			prompt := &survey.Select{
				Message: fmt.Sprintf("Select the %s to debug:", s),
				Options: opts,
			}
			var step string
			err := survey.AskOne(prompt, &step, survey.WithValidator(survey.Required))
			if err != nil {
				return fmt.Errorf("failed to select %s: %w", s, err)
			}
			d.step = unwrapStepName(step)
			d.errMsg = errMap[d.step]
		}

		// debug workflow steps
		rawValue, err := d.getDebugRawValue(ctx, cli, pd, app)
		if err != nil {
			return err
		}

		if err := d.handleCueSteps(rawValue, ioStreams); err != nil {
			return err
		}
	} else {
		// dry run components
		dm, err := discoverymapper.New(config)
		if err != nil {
			return err
		}
		dryRunOpt := dryrun.NewDryRunOption(cli, config, dm, pd, []oam.Object{})
		comps, err := dryRunOpt.ExecuteDryRun(ctx, app)
		if err != nil {
			ioStreams.Info(color.RedString("%s%s", emojiFail, err.Error()))
			return nil
		}
		if err := d.debugComponents(opts, comps, ioStreams); err != nil {
			return err
		}
	}
	return nil
}

func (d *debugOpts) debugComponents(compList []string, comps []*types.ComponentManifest, ioStreams cmdutil.IOStreams) error {
	opts := compList
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
			for _, step := range compList {
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

func (d *debugOpts) getDebugOptions(app *v1beta1.Application) (string, []string, map[string]string) {
	s := "components"
	stepList := make([]string, 0)
	if app.Spec.Workflow != nil && len(app.Spec.Workflow.Steps) > 0 {
		s = "workflow steps"
	}
	errMap := make(map[string]string)
	switch {
	case app.Status.Workflow != nil:
		for _, step := range app.Status.Workflow.Steps {
			stepName := step.Name
			switch step.Phase {
			case apicommon.WorkflowStepPhaseSucceeded:
				stepName = emojiSucceed + step.Name
			case apicommon.WorkflowStepPhaseFailed:
				stepName = emojiFail + step.Name
				errMap[step.Name] = step.Message
			default:
			}
			stepList = append(stepList, stepName)
		}
	case app.Spec.Workflow != nil && len(app.Spec.Workflow.Steps) > 0:
		for _, step := range app.Spec.Workflow.Steps {
			stepList = append(stepList, step.Name)
		}
	default:
		for _, c := range app.Spec.Components {
			stepList = append(stepList, c.Name)
		}
	}
	return s, stepList, errMap
}

func unwrapStepName(step string) string {
	if strings.HasPrefix(step, emojiSucceed) {
		return strings.TrimPrefix(step, emojiSucceed)
	}
	if strings.HasPrefix(step, emojiFail) {
		return strings.TrimPrefix(step, emojiFail)
	}
	return step
}

func (d *debugOpts) getDebugRawValue(ctx context.Context, cli client.Client, pd *packages.PackageDiscover, app *v1beta1.Application) (*value.Value, error) {
	debugCM := &corev1.ConfigMap{}
	if err := cli.Get(ctx, client.ObjectKey{Name: debug.GenerateContextName(app.Name, d.step), Namespace: app.Namespace}, debugCM); err != nil {
		return nil, fmt.Errorf("failed to get debug configmap: %w", err)
	}

	if debugCM.Data == nil || debugCM.Data["debug"] == "" {
		return nil, fmt.Errorf("debug configmap is empty")
	}
	v, err := value.NewValue(debugCM.Data["debug"], pd, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse debug configmap: %w", err)
	}
	return v, nil
}

func (d *debugOpts) handleCueSteps(v *value.Value, ioStreams cmdutil.IOStreams) error {
	if d.focus != "" {
		f, err := v.LookupValue(d.focus)
		if err != nil {
			return err
		}
		ioStreams.Info(color.New(color.FgCyan).Sprint("\n", d.focus, "\n"))
		rendered, err := renderFields(f)
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
			return true, errors.New(errInfo + "(bottom kind)")
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
				rendered, err := renderFields(fieldMap[field])
				if err != nil {
					return err
				}
				ioStreams.Info(rendered, "\n")
			}
			continue
		}
		field = unwrapStepName(field)
		ioStreams.Info(color.CyanString("\n▫️ %s", field))
		rendered, err := renderFields(fieldMap[field])
		if err != nil {
			return err
		}
		ioStreams.Info(rendered, "\n")
	}
	return nil
}

func renderFields(v *value.Value) (string, error) {
	table := uitable.New()
	table.MaxColWidth = 200
	table.Wrap = true
	i := 0

	if err := v.StepByFields(func(fieldName string, in *value.Value) (bool, error) {
		if custom.OpTpy(in) != "" {
			rendered, err := renderFields(in)
			if err != nil {
				return false, err
			}
			i++
			key := fmt.Sprintf("%v.%s", i, fieldName)
			if !strings.Contains(fieldName, "#") {
				if err := v.FillObject(in, fieldName); err != nil {
					renderValuesInRow(table, key, rendered, false)
					return false, err
				}
			}
			renderValuesInRow(table, key, rendered, true)
			return false, nil
		}

		vStr, err := in.String()
		if err != nil {
			return false, err
		}
		i++
		key := fmt.Sprintf("%v.%s", i, fieldName)
		if !strings.Contains(fieldName, "#") {
			if err := v.FillObject(in, fieldName); err != nil {
				renderValuesInRow(table, key, vStr, false)
				return false, err
			}
		}

		renderValuesInRow(table, key, vStr, true)
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
