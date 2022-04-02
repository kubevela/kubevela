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
	"github.com/fatih/color"
	"github.com/gosuri/uitable"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/pkg/workflow/debug"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/custom"
	"github.com/oam-dev/kubevela/references/appfile"
)

type debugOpts struct {
	step  string
	focus string
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
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			app, err := appfile.LoadApplication(namespace, args[0], c)
			if err != nil {
				return err
			}
			return dOpts.debugApplication(ctx, client, config, app, ioStreams)
		},
	}
	addNamespaceAndEnvArg(cmd)
	cmd.Flags().StringVarP(&dOpts.step, "step", "s", "", "specify the step to debug")
	cmd.Flags().StringVarP(&dOpts.focus, "focus", "f", "", "specify the focus value to debug")
	return cmd
}

func (d *debugOpts) debugApplication(ctx context.Context, cli client.Client, config *rest.Config, app *v1beta1.Application, ioStreams cmdutil.IOStreams) error {
	if d.step == "" {
		if err := d.getDebugStep(app); err != nil {
			return err
		}
	}

	rawValue, err := d.getDebugRawValue(ctx, cli, config, app)
	if err != nil {
		return err
	}

	if err := d.handleCueSteps(rawValue, ioStreams); err != nil {
		return err
	}
	return nil
}

func (d *debugOpts) getDebugStep(app *v1beta1.Application) error {
	s := "components"
	stepList := make([]string, 0)
	if app.Spec.Workflow != nil && len(app.Spec.Workflow.Steps) > 0 {
		s = "workflow steps"
		for _, step := range app.Spec.Workflow.Steps {
			stepList = append(stepList, step.Name)
		}
	} else {
		for _, component := range app.Spec.Components {
			stepList = append(stepList, component.Name)
		}
	}
	prompt := &survey.Select{
		Message: fmt.Sprintf("Select the %s to debug:", s),
		Options: stepList,
	}
	var step string
	err := survey.AskOne(prompt, &step, survey.WithValidator(survey.Required))
	if err != nil {
		return fmt.Errorf("failed to select %s: %w", s, err)
	}
	d.step = step
	return nil
}

func (d *debugOpts) getDebugRawValue(ctx context.Context, cli client.Client, config *rest.Config, app *v1beta1.Application) (*value.Value, error) {
	debugCM := &corev1.ConfigMap{}
	if err := cli.Get(ctx, client.ObjectKey{Name: debug.GenerateContextName(app.Name, d.step), Namespace: app.Namespace}, debugCM); err != nil {
		return nil, fmt.Errorf("failed to get debug configmap: %w", err)
	}

	if debugCM.Data == nil || debugCM.Data["debug"] == "" {
		return nil, fmt.Errorf("debug configmap is empty")
	}
	pd, err := packages.NewPackageDiscover(config)
	if err != nil {
		return nil, err
	}
	v, err := value.NewValue(debugCM.Data["debug"], pd, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse debug configmap: %w", err)
	}
	return v, nil
}

func (d *debugOpts) handleCueSteps(v *value.Value, ioStreams cmdutil.IOStreams) error {
	if d.focus != "" {
		f, err := v.LookupValue(strings.Split(d.focus, ".")...)
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

	if err := separateBySteps(v, ioStreams); err != nil {
		return err
	}
	return nil
}

func separateBySteps(v *value.Value, ioStreams cmdutil.IOStreams) error {
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

	opts := fieldList
	opts = append(opts, "all fields", "exit debug mode")
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
		if field == "exit debug mode" {
			break
		}
		if field == "all fields" {
			for _, field := range fieldList {
				ioStreams.Info(color.New(color.FgCyan).Sprint("\n", field, "\n"))
				rendered, err := renderFields(fieldMap[field])
				if err != nil {
					return err
				}
				ioStreams.Info(rendered, "\n")
			}
			continue
		}
		ioStreams.Info(color.New(color.FgCyan).Sprint("\n", field, "\n"))
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
	table.MaxColWidth = 100
	table.Wrap = true

	if err := v.StepByFields(func(fieldName string, in *value.Value) (bool, error) {
		if custom.OpTpy(in) != "" {
			rendered, err := renderFields(in)
			if err != nil {
				return false, err
			}
			renderValuesInRow(table, fieldName, rendered)
			return false, nil
		}

		vStr, err := in.String()
		if err != nil {
			return false, err
		}
		renderValuesInRow(table, fieldName, vStr)
		return false, nil
	}); err != nil {
		vStr, err := v.String()
		if err != nil {
			return "", err
		}
		return vStr, nil
	}

	return table.String(), nil
}

func renderValuesInRow(table *uitable.Table, k, v string) {
	v = strings.TrimSpace(v)
	c := color.New(color.FgGreen)
	if k == "#do" || k == "#provider" {
		c = color.New(color.FgYellow)
	}
	table.AddRow(c.Sprint(k, ":"), v)
}
