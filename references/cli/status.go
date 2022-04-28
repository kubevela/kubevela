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
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	pkgappfile "github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/policy"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile"
)

// HealthStatus represents health status strings.
type HealthStatus = v1alpha2.HealthStatus

const (
	// HealthStatusNotDiagnosed means there's no health scope referred or unknown health status returned
	HealthStatusNotDiagnosed HealthStatus = "NOT DIAGNOSED"
)

const (
	// HealthStatusHealthy represents healthy status.
	HealthStatusHealthy = v1alpha2.StatusHealthy
	// HealthStatusUnhealthy represents unhealthy status.
	HealthStatusUnhealthy = v1alpha2.StatusUnhealthy
	// HealthStatusUnknown represents unknown status.
	HealthStatusUnknown = v1alpha2.StatusUnknown
)

// WorkloadHealthCondition holds health status of any resource
type WorkloadHealthCondition = v1alpha2.WorkloadHealthCondition

// ScopeHealthCondition holds health condition of a scope
type ScopeHealthCondition = v1alpha2.ScopeHealthCondition

// CompStatus represents the status of a component during "vela init"
type CompStatus int

// Enums of CompStatus
const (
	compStatusDeploying CompStatus = iota
	compStatusDeployFail
	compStatusDeployed
	compStatusUnknown
)

// Error msg used in `status` command
const (
	// ErrNotLoadAppConfig display the error message load
	ErrNotLoadAppConfig = "cannot load the application"
)

const (
	trackingInterval time.Duration = 1 * time.Second
	deployTimeout    time.Duration = 10 * time.Second
)

// NewAppStatusCommand creates `status` command for showing status
func NewAppStatusCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "status APP_NAME",
		Short:   "Show status of an application.",
		Long:    "Show status of vela application.",
		Example: `vela status APP_NAME`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// check args
			argsLength := len(args)
			if argsLength == 0 {
				return fmt.Errorf("please specify an application")
			}
			appName := args[0]
			// get namespace
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			if printTree, err := cmd.Flags().GetBool("tree"); err == nil && printTree {
				return printApplicationTree(c, cmd, appName, namespace)
			}
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			showEndpoints, err := cmd.Flags().GetBool("endpoint")
			if showEndpoints && err == nil {
				component, _ := cmd.Flags().GetString("component")
				f := Filter{
					Component: component,
				}
				return printAppEndpoints(ctx, newClient, appName, namespace, f, c)
			}
			return printAppStatus(ctx, newClient, ioStreams, appName, namespace, cmd, c)
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
	}
	cmd.Flags().StringP("svc", "s", "", "service name")
	cmd.Flags().BoolP("endpoint", "p", false, "show all service endpoints of the application")
	cmd.Flags().StringP("component", "c", "", "filter service endpoints by component name")
	cmd.Flags().BoolP("tree", "t", false, "display the application resources into tree structure")
	cmd.Flags().BoolP("detail", "d", false, "display the realtime details of application resources")
	cmd.Flags().StringP("detail-format", "", "inline", "the format for displaying details. Can be one of inline (default), wide, list, table, raw.")
	addNamespaceAndEnvArg(cmd)
	return cmd
}

func printAppStatus(_ context.Context, c client.Client, ioStreams cmdutil.IOStreams, appName string, namespace string, cmd *cobra.Command, velaC common.Args) error {
	app, err := appfile.LoadApplication(namespace, appName, velaC)
	if err != nil {
		return err
	}

	cmd.Printf("About:\n\n")
	table := newUITable()
	table.AddRow("  Name:", appName)
	table.AddRow("  Namespace:", namespace)
	table.AddRow("  Created at:", app.CreationTimestamp.String())
	table.AddRow("  Status:", getAppPhaseColor(app.Status.Phase).Sprint(app.Status.Phase))
	cmd.Printf("%s\n\n", table.String())
	if err := printWorkflowStatus(c, ioStreams, appName, namespace); err != nil {
		return err
	}
	cmd.Printf("Services:\n\n")
	return loopCheckStatus(c, ioStreams, appName, namespace)
}

func printAppEndpoints(ctx context.Context, client client.Client, appName string, namespace string, f Filter, velaC common.Args) error {
	endpoints, err := GetServiceEndpoints(ctx, client, appName, namespace, velaC, f)
	if err != nil {
		return err
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetColWidth(100)
	table.SetHeader([]string{"Cluster", "Component", "Ref(Kind/Namespace/Name)", "Endpoint"})
	for _, endpoint := range endpoints {
		if endpoint.Cluster == "" {
			endpoint.Cluster = multicluster.ClusterLocalName
		}
		table.Append([]string{endpoint.Cluster, endpoint.Component, fmt.Sprintf("%s/%s/%s", endpoint.Ref.Kind, endpoint.Ref.Namespace, endpoint.Ref.Name), endpoint.String()})
	}
	table.Render()
	return nil
}

func loadRemoteApplication(c client.Client, ns string, name string) (*v1beta1.Application, error) {
	app := new(v1beta1.Application)
	err := c.Get(context.Background(), client.ObjectKey{
		Namespace: ns,
		Name:      name,
	}, app)
	return app, err
}

func getComponentType(app *v1beta1.Application, name string) string {
	for _, c := range app.Spec.Components {
		if c.Name == name {
			return c.Type
		}
	}
	return "webservice"
}

func printWorkflowStatus(c client.Client, ioStreams cmdutil.IOStreams, appName string, namespace string) error {
	remoteApp, err := loadRemoteApplication(c, namespace, appName)
	if err != nil {
		return err
	}
	workflowStatus := remoteApp.Status.Workflow
	if workflowStatus != nil {
		ioStreams.Info("Workflow:\n")
		ioStreams.Infof("  mode: %s\n", workflowStatus.Mode)
		ioStreams.Infof("  finished: %t\n", workflowStatus.Finished)
		ioStreams.Infof("  Suspend: %t\n", workflowStatus.Suspend)
		ioStreams.Infof("  Terminated: %t\n", workflowStatus.Terminated)
		ioStreams.Info("  Steps")
		for _, step := range workflowStatus.Steps {
			ioStreams.Infof("  - id:%s\n", step.ID)
			ioStreams.Infof("    name:%s\n", step.Name)
			ioStreams.Infof("    type:%s\n", step.Type)
			ioStreams.Infof("    phase:%s \n", getWfStepColor(step.Phase).Sprint(step.Phase))
			ioStreams.Infof("    message:%s\n", step.Message)
		}
		ioStreams.Infof("\n")
	}
	return nil
}

func loopCheckStatus(c client.Client, ioStreams cmdutil.IOStreams, appName string, namespace string) error {
	remoteApp, err := loadRemoteApplication(c, namespace, appName)
	if err != nil {
		return err
	}
	for _, comp := range remoteApp.Status.Services {
		compName := comp.Name
		envStat := ""
		if comp.Env != "" {
			envStat = "Env: " + comp.Env
		}
		if comp.Cluster == "" {
			comp.Cluster = "local"
		}
		nsStat := ""
		if comp.Namespace != "" {
			nsStat = "Namespace: " + comp.Namespace
		}
		ioStreams.Infof(fmt.Sprintf("  - Name: %s  %s\n", compName, envStat))
		ioStreams.Infof(fmt.Sprintf("    Cluster: %s  %s\n", comp.Cluster, nsStat))
		ioStreams.Infof("    Type: %s\n", getComponentType(remoteApp, compName))
		healthColor := getHealthStatusColor(comp.Healthy)
		healthInfo := strings.ReplaceAll(comp.Message, "\n", "\n\t") // format healthInfo output
		healthstats := "Healthy"
		if !comp.Healthy {
			healthstats = "Unhealthy"
		}
		ioStreams.Infof("    %s %s\n", healthColor.Sprint(healthstats), healthColor.Sprint(healthInfo))

		// load it again after health check
		remoteApp, err = loadRemoteApplication(c, namespace, appName)
		if err != nil {
			return err
		}
		// workload Must found
		if len(comp.Traits) > 0 {
			ioStreams.Infof("    Traits:\n")
		} else {
			ioStreams.Infof("    No trait applied\n")
		}
		for _, tr := range comp.Traits {
			traitBase := ""
			if tr.Healthy {
				traitBase = fmt.Sprintf("      %s%s", emojiSucceed, white.Sprint(tr.Type))
			} else {
				traitBase = fmt.Sprintf("      %s%s", emojiFail, white.Sprint(tr.Type))
			}
			if tr.Message != "" {
				traitBase += ": " + tr.Message
			}
			ioStreams.Infof(traitBase)
		}
		ioStreams.Info("")
	}
	return nil
}

func printTrackingDeployStatus(c common.Args, ioStreams cmdutil.IOStreams, appName string, namespace string) (CompStatus, error) {
	sDeploy := newTrackingSpinnerWithDelay("Checking Status ...", trackingInterval)
	sDeploy.Start()
	defer sDeploy.Stop()
TrackDeployLoop:
	for {
		time.Sleep(trackingInterval)
		deployStatus, failMsg, err := TrackDeployStatus(c, appName, namespace)
		if err != nil {
			return compStatusUnknown, err
		}
		switch deployStatus {
		case compStatusDeploying:
			continue
		case compStatusDeployed:
			ioStreams.Info(green.Sprintf("\n%sApplication Deployed Successfully!", emojiSucceed))
			break TrackDeployLoop
		case compStatusDeployFail:
			ioStreams.Info(red.Sprintf("\n%sApplication Failed to Deploy!", emojiFail))
			ioStreams.Info(red.Sprintf("Reason: %s", failMsg))
			return compStatusDeployFail, nil
		default:
			continue
		}
	}
	return compStatusDeployed, nil
}

// TrackDeployStatus will only check AppConfig is deployed successfully,
func TrackDeployStatus(c common.Args, appName string, namespace string) (CompStatus, string, error) {
	appObj, err := appfile.LoadApplication(namespace, appName, c)
	if err != nil {
		return compStatusUnknown, "", err
	}
	if appObj == nil {
		return compStatusUnknown, "", errors.New(ErrNotLoadAppConfig)
	}
	condition := appObj.Status.Conditions
	if len(condition) < 1 {
		return compStatusDeploying, "", nil
	}

	// If condition is true, we can regard appConfig is deployed successfully
	if appObj.Status.Phase == commontypes.ApplicationRunning {
		return compStatusDeployed, "", nil
	}

	// if not found workload status in AppConfig
	// then use age to check whether the workload controller is running
	if time.Since(appObj.GetCreationTimestamp().Time) > deployTimeout {
		return compStatusDeployFail, condition[0].Message, nil
	}
	return compStatusDeploying, "", nil
}

func getHealthStatusColor(s bool) *color.Color {
	if s {
		return green
	}
	return yellow
}

func getWfStepColor(phase commontypes.WorkflowStepPhase) *color.Color {
	switch phase {
	case commontypes.WorkflowStepPhaseSucceeded:
		return green
	case commontypes.WorkflowStepPhaseFailed:
		return red
	default:
		return yellow
	}
}

func getAppPhaseColor(appPhase commontypes.ApplicationPhase) *color.Color {
	if appPhase == commontypes.ApplicationRunning {
		return green
	}
	return yellow
}

func printApplicationTree(c common.Args, cmd *cobra.Command, appName string, appNs string) error {
	config, err := c.GetConfig()
	if err != nil {
		return err
	}
	config.Wrap(multicluster.NewSecretModeMultiClusterRoundTripper)
	cli, err := c.GetClient()
	if err != nil {
		return err
	}
	pd, err := c.GetPackageDiscover()
	if err != nil {
		return err
	}
	dm, err := discoverymapper.New(config)
	if err != nil {
		return err
	}

	app, err := loadRemoteApplication(cli, appNs, appName)
	if err != nil {
		return err
	}
	ctx := context.Background()
	_, currentRT, historyRTs, _, err := resourcetracker.ListApplicationResourceTrackers(ctx, cli, app)
	if err != nil {
		return err
	}

	svc, err := multicluster.GetClusterGatewayService(context.Background(), cli)
	if err != nil {
		return errors.Wrapf(err, "failed to get cluster secret namespace, please ensure cluster gateway is correctly deployed")
	}
	multicluster.ClusterGatewaySecretNamespace = svc.Namespace
	clusterMapper, err := multicluster.NewClusterMapper(ctx, cli)
	if err != nil {
		return errors.Wrapf(err, "failed to get cluster mapper")
	}

	var placements []v1alpha1.PlacementDecision
	af, err := pkgappfile.NewApplicationParser(cli, dm, pd).GenerateAppFile(context.Background(), app)
	if err == nil {
		placements, _ = policy.GetPlacementsFromTopologyPolicies(context.Background(), cli, app.GetNamespace(), af.Policies, true)
	}
	format, _ := cmd.Flags().GetString("detail-format")
	var maxWidth *int
	if w, _, err := term.GetSize(0); err == nil && w > 0 {
		maxWidth = pointer.Int(w)
	}
	options := resourcetracker.ResourceTreePrintOptions{MaxWidth: maxWidth, Format: format, ClusterMapper: clusterMapper}
	printDetails, _ := cmd.Flags().GetBool("detail")
	if printDetails {
		msgRetriever, err := resourcetracker.RetrieveKubeCtlGetMessageGenerator(config)
		if err != nil {
			return err
		}
		options.DetailRetriever = msgRetriever
	}
	options.PrintResourceTree(cmd.OutOrStdout(), placements, currentRT, historyRTs)
	return nil
}
