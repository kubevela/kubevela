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
	"io"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wfTypes "github.com/kubevela/workflow/pkg/types"
	wfUtils "github.com/kubevela/workflow/pkg/utils"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	"github.com/oam-dev/kubevela/pkg/workflow/operation"
	"github.com/oam-dev/kubevela/references/appfile"
)

// NewWorkflowCommand create `workflow` command
func NewWorkflowCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Operate application delivery workflow.",
		Long:  "Operate the Workflow during Application Delivery. Note that workflow command is both valid for Application Workflow and WorkflowRun. The command will try to find the Application first, if not found, it will try to find WorkflowRun. You can also specify the resource type by using --type flag.",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
	}
	cmd.AddCommand(
		NewWorkflowSuspendCommand(c, ioStreams),
		NewWorkflowResumeCommand(c, ioStreams),
		NewWorkflowTerminateCommand(c, ioStreams),
		NewWorkflowRestartCommand(c, ioStreams),
		NewWorkflowRollbackCommand(c, ioStreams),
		NewWorkflowLogsCommand(c, ioStreams),
	)
	return cmd
}

// NewWorkflowSuspendCommand create workflow suspend command
func NewWorkflowSuspendCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suspend",
		Short:   "Suspend an application workflow.",
		Long:    "Suspend an application workflow in cluster.",
		Example: "vela workflow suspend <application-name>",
		PreRun:  checkApplicationNotRunning(c),
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

			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			config.Wrap(pkgmulticluster.NewTransportWrapper())
			cli, err := c.GetClient()
			if err != nil {
				return err
			}

			wo := operation.NewWorkflowOperator(cli, cmd.OutOrStdout())
			return wo.Suspend(context.Background(), app)
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// NewWorkflowResumeCommand create workflow resume command
func NewWorkflowResumeCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resume",
		Short:   "Resume a suspend application workflow.",
		Long:    "Resume a suspend application workflow in cluster.",
		Example: "vela workflow resume <application-name>",
		PreRun:  checkApplicationNotRunning(c),
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

			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			config.Wrap(pkgmulticluster.NewTransportWrapper())
			cli, err := c.GetClient()
			if err != nil {
				return err
			}

			wo := operation.NewWorkflowOperator(cli, cmd.OutOrStdout())
			return wo.Resume(context.Background(), app)
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// NewWorkflowTerminateCommand create workflow terminate command
func NewWorkflowTerminateCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "terminate",
		Short:   "Terminate an application workflow.",
		Long:    "Terminate an application workflow in cluster.",
		Example: "vela workflow terminate <application-name>",
		PreRun:  checkApplicationNotRunning(c),
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
			if app.Status.Workflow == nil {
				return fmt.Errorf("the workflow in application is not running")
			}
			cli, err := c.GetClient()
			if err != nil {
				return err
			}
			wo := operation.NewWorkflowOperator(cli, cmd.OutOrStdout())
			return wo.Terminate(context.Background(), app)
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// NewWorkflowRestartCommand create workflow restart command
func NewWorkflowRestartCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "restart",
		Short:   "Restart an application workflow.",
		Long:    "Restart an application workflow in cluster.",
		Example: "vela workflow restart <application-name>",
		PreRun:  checkApplicationNotRunning(c),
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
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			config.Wrap(pkgmulticluster.NewTransportWrapper())
			cli, err := c.GetClient()
			if err != nil {
				return err
			}

			wo := operation.NewWorkflowOperator(cli, cmd.OutOrStdout())
			return wo.Restart(context.Background(), app)
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// NewWorkflowRollbackCommand create workflow rollback command
func NewWorkflowRollbackCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rollback",
		Short:   "Rollback an application workflow to the latest revision.",
		Long:    "Rollback an application workflow to the latest revision.",
		Example: "vela workflow rollback <application-name>",
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
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			config.Wrap(pkgmulticluster.NewTransportWrapper())
			cli, err := c.GetClient()
			if err != nil {
				return err
			}
			if app.Status.Workflow != nil && !app.Status.Workflow.Terminated && !app.Status.Workflow.Suspend && !app.Status.Workflow.Finished {
				return fmt.Errorf("can not rollback a running workflow")
			}
			if oam.GetPublishVersion(app) == "" {
				if app.Status.LatestRevision == nil || app.Status.LatestRevision.Name == "" {
					return fmt.Errorf("the latest revision is not set: %s", app.Name)
				}
				// get the last revision
				revision := &v1beta1.ApplicationRevision{}
				if err := cli.Get(context.TODO(), k8stypes.NamespacedName{Name: app.Status.LatestRevision.Name, Namespace: app.Namespace}, revision); err != nil {
					return fmt.Errorf("failed to get the latest revision: %w", err)
				}

				app.Spec = revision.Spec.Application.Spec
				if err := cli.Status().Update(context.TODO(), app); err != nil {
					return err
				}

				fmt.Printf("Successfully rollback workflow to the latest revision: %s\n", app.Name)
				return nil
			}
			wo := operation.NewWorkflowOperator(cli, cmd.OutOrStdout())
			return wo.Rollback(context.Background(), app)
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// NewWorkflowLogsCommand create workflow logs command
func NewWorkflowLogsCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	wargs := &WorkflowArgs{Args: c}
	cmd := &cobra.Command{
		Use:     "logs",
		Short:   "Tail logs for workflow steps",
		Long:    "Tail logs for workflow steps, note that you need to use op.#Logs in step definition to set the log config of the step.",
		Example: "vela workflow logs <workflow-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify Application or WorkflowRun name")
			}
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			cli, err := c.GetClient()
			if err != nil {
				return err
			}
			ctx := context.Background()
			if err := wargs.getWorkflowInstance(ctx, cli, namespace, args[0]); err != nil {
				return err
			}
			return wargs.printStepLogs(ctx, cli, ioStream)
		},
	}
	cmd.Flags().StringVarP(&wargs.StepName, "step", "s", "", "specify the step name in the workflow")
	cmd.Flags().StringVarP(&wargs.Output, "output", "o", "default", "output format for logs, support: [default, raw, json]")
	cmd.Flags().StringVarP(&wargs.Type, "type", "t", "", "the type of the resource, support: [app, workflow]")
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// WorkflowArgs is the args for workflow command
type WorkflowArgs struct {
	Type             string
	Output           string
	ControllerLabels map[string]string
	Args             common.Args
	StepName         string
	App              *v1beta1.Application
	WorkflowRun      *workflowv1alpha1.WorkflowRun
	WorkflowInstance *wfTypes.WorkflowInstance
}

const (
	instanceTypeApplication string = "app"
	instanceTypeWorkflowRun string = "workflow"
)

func (w *WorkflowArgs) getWorkflowInstance(ctx context.Context, cli client.Client, namespace, name string) error {
	switch w.Type {
	case "":
		app := &v1beta1.Application{}
		if err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, app); err == nil {
			w.Type = instanceTypeApplication
			w.App = app
		} else {
			wr := &workflowv1alpha1.WorkflowRun{}
			if err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, wr); err == nil {
				w.Type = instanceTypeWorkflowRun
				w.WorkflowRun = wr
			}
		}
		if w.Type == "" {
			return fmt.Errorf("can't find application or workflowrun %s", name)
		}
	case instanceTypeApplication:
		app := &v1beta1.Application{}
		if err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, app); err != nil {
			return err
		}
		w.App = app
	case instanceTypeWorkflowRun:
		wr := &workflowv1alpha1.WorkflowRun{}
		if err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, wr); err != nil {
			return err
		}
		w.WorkflowRun = wr
	default:
	}
	return w.generateWorkflowInstance(ctx, cli)
}

func (w *WorkflowArgs) generateWorkflowInstance(ctx context.Context, cli client.Client) error {
	switch w.Type {
	case instanceTypeApplication:
		if w.App.Status.Workflow == nil {
			return fmt.Errorf("the workflow in application %s is not start", w.App.Name)
		}
		status := w.App.Status.Workflow
		w.WorkflowInstance = &wfTypes.WorkflowInstance{
			WorkflowMeta: wfTypes.WorkflowMeta{
				Name:      w.App.Name,
				Namespace: w.App.Namespace,
			},
			Steps: w.App.Spec.Workflow.Steps,
			Status: workflowv1alpha1.WorkflowRunStatus{
				Phase:          status.Phase,
				Message:        status.Message,
				Suspend:        status.Suspend,
				SuspendState:   status.SuspendState,
				Terminated:     status.Terminated,
				Finished:       status.Finished,
				ContextBackend: status.ContextBackend,
				Steps:          status.Steps,
				StartTime:      status.StartTime,
				EndTime:        status.EndTime,
			},
		}
		w.ControllerLabels = map[string]string{"app.kubernetes.io/name": "vela-core"}
	case instanceTypeWorkflowRun:
		var steps []workflowv1alpha1.WorkflowStep
		if w.WorkflowRun.Spec.WorkflowRef != "" {
			workflow := &workflowv1alpha1.Workflow{}
			if err := cli.Get(ctx, client.ObjectKey{Namespace: w.WorkflowRun.Namespace, Name: w.WorkflowRun.Spec.WorkflowRef}, workflow); err != nil {
				return err
			}
			steps = workflow.Steps
		} else {
			steps = w.WorkflowRun.Spec.WorkflowSpec.Steps
		}
		w.WorkflowInstance = &wfTypes.WorkflowInstance{
			WorkflowMeta: wfTypes.WorkflowMeta{
				Name:      w.WorkflowRun.Name,
				Namespace: w.WorkflowRun.Namespace,
			},
			Steps:  steps,
			Status: w.WorkflowRun.Status,
		}
		w.ControllerLabels = map[string]string{"app.kubernetes.io/name": "vela-workflow"}
	default:
		return fmt.Errorf("unknown workflow instance type %s", w.Type)
	}
	return nil
}

func (w *WorkflowArgs) printStepLogs(ctx context.Context, cli client.Client, ioStreams cmdutil.IOStreams) error {
	if w.StepName == "" {
		if err := w.selectWorkflowStep(); err != nil {
			return err
		}
	}
	if w.WorkflowInstance.Status.ContextBackend == nil {
		return fmt.Errorf("the workflow context backend is not set")
	}
	logConfig, err := wfUtils.GetLogConfigFromStep(ctx, cli, w.WorkflowInstance.Status.ContextBackend.Name, w.WorkflowInstance.Name, w.WorkflowInstance.Namespace, w.StepName)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("step [%s]", w.StepName))
	}
	if err := selectStepLogSource(logConfig); err != nil {
		return err
	}
	switch {
	case logConfig.Data:
		return w.printResourceLogs(ctx, cli, ioStreams, []wfTypes.Resource{{
			Namespace:     types.DefaultKubeVelaNS,
			LabelSelector: w.ControllerLabels,
		}}, []string{fmt.Sprintf(`step_name="%s"`, w.StepName), fmt.Sprintf("%s/%s", w.WorkflowInstance.Namespace, w.WorkflowInstance.Name), "cue logs"})
	case logConfig.Source != nil:
		if len(logConfig.Source.Resources) > 0 {
			return w.printResourceLogs(ctx, cli, ioStreams, logConfig.Source.Resources, nil)
		}
		if logConfig.Source.URL != "" {
			readCloser, err := wfUtils.GetLogsFromURL(ctx, logConfig.Source.URL)
			if err != nil {
				return err
			}
			//nolint:errcheck
			defer readCloser.Close()
			if _, err := io.Copy(ioStreams.Out, readCloser); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *WorkflowArgs) selectWorkflowStep() error {
	stepsKey := make([]string, 0)
	for _, step := range w.WorkflowInstance.Status.Steps {
		stepsKey = append(stepsKey, wrapStepName(step.StepStatus))
		for _, sub := range step.SubStepsStatus {
			stepsKey = append(stepsKey, fmt.Sprintf("  %s", wrapStepName(sub)))
		}
	}
	if len(stepsKey) == 0 {
		return fmt.Errorf("Workflow is not start")
	}

	prompt := &survey.Select{
		Message: "Select a step to show logs:",
		Options: stepsKey,
	}
	var stepName string
	err := survey.AskOne(prompt, &stepName, survey.WithValidator(survey.Required))
	if err != nil {
		return fmt.Errorf("failed to select step %s: %w", unwrapStepName(w.StepName), err)
	}
	w.StepName = unwrapStepName(stepName)
	return nil
}

func selectStepLogSource(logConfig *wfTypes.LogConfig) error {
	var source string
	if logConfig.Data && logConfig.Source != nil {
		prompt := &survey.Select{
			Message: "Select logs from data or source",
			Options: []string{"data", "source"},
		}
		err := survey.AskOne(prompt, &source, survey.WithValidator(survey.Required))
		if err != nil {
			return fmt.Errorf("failed to select %s: %w", source, err)
		}
		if source != "data" {
			logConfig.Data = false
		}
	}
	return nil
}

func (w *WorkflowArgs) printResourceLogs(ctx context.Context, cli client.Client, ioStreams cmdutil.IOStreams, resources []wfTypes.Resource, filters []string) error {
	pods, err := wfUtils.GetPodListFromResources(ctx, cli, resources)
	if err != nil {
		return err
	}
	podList := make([]querytypes.PodBase, 0)
	for _, pod := range pods {
		podBase := querytypes.PodBase{}
		podBase.Metadata.Name = pod.Name
		podBase.Metadata.Namespace = pod.Namespace
		podList = append(podList, podBase)
	}
	if len(pods) == 0 {
		return errors.New("no pod found")
	}
	var selectPod *querytypes.PodBase
	if len(pods) > 1 {
		selectPod, err = AskToChooseOnePod(podList)
		if err != nil {
			return err
		}
	} else {
		selectPod = &podList[0]
	}
	l := Args{
		Args:   w.Args,
		Output: w.Output,
	}
	return l.printPodLogs(ctx, ioStreams, selectPod, filters)
}

func checkApplicationNotRunning(c common.Args) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		// Any error will be returned to let the normal execution report the error
		if len(args) < 1 {
			return
		}
		namespace, err := GetFlagNamespaceOrEnv(cmd, c)
		if err != nil {
			return
		}
		app, err := appfile.LoadApplication(namespace, args[0], c)
		if err != nil {
			return
		}
		if app.Status.Phase == common2.ApplicationRunning {
			cmd.Printf("%s workflow not allowed because application %s is running\n", cmd.Use, args[0])
			os.Exit(1)
		}
	}
}
