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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile"
	"github.com/oam-dev/kubevela/references/appfile/api"
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
	compStatusHealthChecking
	compStatusHealthCheckDone
	compStatusUnknown
)

// Error msg used in `status` command
const (
	ErrNotLoadAppConfig  = "cannot load the application"
	ErrFmtNotInitialized = "service: %s not ready"
	ErrServiceNotFound   = "service %s not found in app"
)

const (
	trackingInterval      time.Duration = 1 * time.Second
	deployTimeout         time.Duration = 10 * time.Second
	healthCheckBufferTime time.Duration = 120 * time.Second
)

// NewAppStatusCommand creates `status` command for showing status
func NewAppStatusCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "status APP_NAME",
		Short:   "Show status of an application",
		Long:    "Show status of an application, including workloads and traits of each service.",
		Example: `vela status APP_NAME`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength == 0 {
				ioStreams.Errorf("Hint: please specify an application")
				os.Exit(1)
			}
			appName := args[0]
			env, err := GetFlagEnvOrCurrent(cmd, c)
			if err != nil {
				ioStreams.Errorf("Error: failed to get Env: %s", err)
				return err
			}
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			return printAppStatus(ctx, newClient, ioStreams, appName, env, cmd, c)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.Flags().StringP("svc", "s", "", "service name")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printAppStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, appName string, env *types.EnvMeta, cmd *cobra.Command, velaC common.Args) error {
	app, err := appfile.LoadApplication(env.Namespace, appName, velaC)
	if err != nil {
		return err
	}
	namespace := env.Namespace

	cmd.Printf("About:\n\n")
	table := newUITable()
	table.AddRow("  Name:", appName)
	table.AddRow("  Namespace:", namespace)
	table.AddRow("  Created at:", app.CreationTimestamp.String())
	cmd.Printf("%s\n\n", table.String())

	cmd.Printf("Services:\n\n")
	return loopCheckStatus(ctx, c, ioStreams, appName, env)
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

func loopCheckStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, appName string, env *types.EnvMeta) error {
	remoteApp, err := loadRemoteApplication(c, env.Namespace, appName)
	if err != nil {
		return err
	}
	for _, comp := range remoteApp.Status.Services {
		compName := comp.Name

		ioStreams.Infof(white.Sprintf("  - Name: %s  Env: %s\n", compName, comp.Env))
		ioStreams.Infof("    Type: %s\n", getComponentType(remoteApp, compName))
		healthColor := getHealthStatusColor(comp.Healthy)
		healthInfo := strings.ReplaceAll(comp.Message, "\n", "\n\t") // format healthInfo output
		healthstats := "healthy"
		if !comp.Healthy {
			healthstats = "unhealthy"
		}
		ioStreams.Infof("    %s %s\n", healthColor.Sprint(healthstats), healthColor.Sprint(healthInfo))

		// load it again after health check
		remoteApp, err = loadRemoteApplication(c, env.Namespace, appName)
		if err != nil {
			return err
		}
		// workload Must found
		ioStreams.Infof("    Traits:\n")
		for _, tr := range comp.Traits {
			if tr.Message != "" {
				if tr.Healthy {
					ioStreams.Infof("      - %s%s: %s", emojiSucceed, white.Sprint(tr.Type), tr.Message)
				} else {
					ioStreams.Infof("      - %s%s: %s", emojiFail, white.Sprint(tr.Type), tr.Message)
				}
				continue
			}
		}
		ioStreams.Info("")
	}
	return nil
}

func healthCheckLoop(ctx context.Context, c client.Client, compName, appName string, env *types.EnvMeta) (HealthStatus, string, error) {
	// Health Check Loop For Workload
	var healthInfo string
	var healthStatus HealthStatus
	var err error

	sHealthCheck := newTrackingSpinner("Checking health status ...")
	sHealthCheck.Start()
	defer sHealthCheck.Stop()
HealthCheckLoop:
	for {
		time.Sleep(trackingInterval)
		var healthcheckStatus CompStatus
		healthcheckStatus, healthStatus, healthInfo, err = trackHealthCheckingStatus(ctx, c, compName, appName, env)
		if err != nil {
			healthInfo = red.Sprintf("Health checking failed!")
			return "", healthInfo, err
		}
		if healthcheckStatus == compStatusHealthCheckDone {
			break HealthCheckLoop
		}
	}
	return healthStatus, healthInfo, nil
}

func printTrackingDeployStatus(c common.Args, ioStreams cmdutil.IOStreams, appName string, env *types.EnvMeta) (CompStatus, error) {
	sDeploy := newTrackingSpinnerWithDelay("Checking Status ...", trackingInterval)
	sDeploy.Start()
	defer sDeploy.Stop()
TrackDeployLoop:
	for {
		time.Sleep(trackingInterval)
		deployStatus, failMsg, err := TrackDeployStatus(c, appName, env)
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
func TrackDeployStatus(c common.Args, appName string, env *types.EnvMeta) (CompStatus, string, error) {
	appObj, err := appfile.LoadApplication(env.Namespace, appName, c)
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

// trackHealthCheckingStatus will check health status from health scope
func trackHealthCheckingStatus(ctx context.Context, c client.Client, compName, appName string, env *types.EnvMeta) (CompStatus, HealthStatus, string, error) {
	app, err := loadRemoteApplication(c, env.Namespace, appName)
	if err != nil {
		return compStatusUnknown, HealthStatusNotDiagnosed, "", err
	}

	if len(app.Status.Conditions) < 1 {
		// still reconciling
		return compStatusUnknown, HealthStatusUnknown, "", nil
	}
	// check whether referenced a HealthScope
	var healthScopeName string
	for _, v := range app.Spec.Components {
		if len(v.Scopes) > 0 {
			healthScopeName = v.Scopes[api.DefaultHealthScopeKey]
		}
	}
	var healthStatus HealthStatus
	if healthScopeName != "" {
		var healthScope v1alpha2.HealthScope
		if err = c.Get(ctx, client.ObjectKey{Namespace: env.Namespace, Name: healthScopeName}, &healthScope); err != nil {
			return compStatusUnknown, HealthStatusUnknown, "", err
		}
		var wlhc *v1alpha2.WorkloadHealthCondition
		for _, v := range healthScope.Status.WorkloadHealthConditions {
			if v.ComponentName == compName {
				wlhc = v
			}
		}
		if wlhc == nil {
			cTime := app.GetCreationTimestamp()
			if time.Since(cTime.Time) <= deployTimeout {
				return compStatusHealthChecking, HealthStatusUnknown, "", nil
			}
			if len(healthScope.Spec.AppRefs) == 0 && len(healthScope.Spec.WorkloadReferences) == 0 {
				return compStatusHealthCheckDone, HealthStatusHealthy, "no workload or app found in health scope", nil
			}
			return compStatusUnknown, HealthStatusUnknown, "", fmt.Errorf("cannot get health condition from the health scope: %s", healthScope.Name)
		}
		healthStatus = wlhc.HealthStatus
		if healthStatus == HealthStatusHealthy {
			return compStatusHealthCheckDone, healthStatus, wlhc.Diagnosis, nil
		}
		if healthStatus == HealthStatusUnhealthy {
			cTime := app.GetCreationTimestamp()
			if time.Since(cTime.Time) <= healthCheckBufferTime {
				return compStatusHealthChecking, HealthStatusUnknown, "", nil
			}
			return compStatusHealthCheckDone, healthStatus, wlhc.Diagnosis, nil
		}
	}
	return compStatusHealthCheckDone, HealthStatusNotDiagnosed, "", nil
}

func getWorkloadStatusFromApp(app *v1beta1.Application, compName string) (commontypes.ApplicationComponentStatus, bool) {
	foundWlStatus := false
	wlStatus := commontypes.ApplicationComponentStatus{}
	if app == nil {
		return wlStatus, foundWlStatus
	}
	for _, v := range app.Status.Services {
		if v.Name == compName {
			wlStatus = v
			foundWlStatus = true
			break
		}
	}
	return wlStatus, foundWlStatus
}

func getHealthStatusColor(s bool) *color.Color {
	if s {
		return green
	}
	return yellow
}
