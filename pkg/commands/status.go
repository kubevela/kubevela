package commands

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/api"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	oam2 "github.com/oam-dev/kubevela/pkg/serverlib"
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

var (
	kindHealthScope = reflect.TypeOf(v1alpha2.HealthScope{}).Name()
)

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
func NewAppStatusCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
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
			env, err := GetEnv(cmd)
			if err != nil {
				ioStreams.Errorf("Error: failed to get Env: %s", err)
				return err
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			return printAppStatus(ctx, newClient, ioStreams, appName, env, cmd)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.Flags().StringP("svc", "s", "", "service name")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printAppStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, appName string, env *types.EnvMeta, cmd *cobra.Command) error {
	app, err := appfile.LoadApplication(env.Name, appName)
	if err != nil {
		return err
	}
	namespace := env.Name

	targetServices, err := oam2.GetServicesWhenDescribingApplication(cmd, app)
	if err != nil {
		return err
	}

	cmd.Printf("About:\n\n")
	table := newUITable()
	table.AddRow("  Name:", appName)
	table.AddRow("  Namespace:", namespace)
	table.AddRow("  Created at:", app.CreateTime.String())
	table.AddRow("  Updated at:", app.UpdateTime.String())
	cmd.Printf("%s\n\n", table.String())

	cmd.Printf("Services:\n\n")

	for _, svcName := range targetServices {
		if err := printComponentStatus(ctx, c, ioStreams, svcName, appName, env); err != nil {
			return err
		}
	}

	return nil
}

func printComponentStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, compName, appName string, env *types.EnvMeta) error {
	app, appConfig, err := getAppConfig(ctx, c, compName, appName, env)
	if err != nil {
		return err
	}
	if app == nil || appConfig == nil {
		return errors.New(ErrNotLoadAppConfig)
	}
	svc, ok := app.Services[compName]
	if !ok {
		return fmt.Errorf(ErrServiceNotFound, compName)
	}
	workloadType := svc.GetType()

	healthStatus, healthInfo, err := healthCheckLoop(ctx, c, compName, appName, env)
	if err != nil {
		ioStreams.Info(healthInfo)
		return err
	}
	ioStreams.Infof(white.Sprintf("  - Name: %s\n", compName))
	ioStreams.Infof("    Type: %s\n", workloadType)

	healthColor := getHealthStatusColor(healthStatus)
	healthInfo = strings.ReplaceAll(healthInfo, "\n", "\n\t") // format healthInfo output
	ioStreams.Infof("    %s %s\n", healthColor.Sprint(healthStatus), healthColor.Sprint(healthInfo))

	// workload Must found
	ioStreams.Infof("    Traits:\n")
	workloadStatus, _ := getWorkloadStatusFromAppConfig(appConfig, compName)
	for _, tr := range workloadStatus.Traits {
		traitType, traitInfo, err := traitCheckLoop(ctx, c, tr.Reference, compName, appConfig, app, 60*time.Second)
		if err != nil {
			ioStreams.Infof("      - %s%s: %s, err: %v", emojiFail, white.Sprint(traitType), traitInfo, err)
			continue
		}
		ioStreams.Infof("      - %s%s: %s", emojiSucceed, white.Sprint(traitType), traitInfo)
	}
	ioStreams.Info("")
	ioStreams.Infof("    Last Deployment:\n")
	ioStreams.Infof("      Created at: %v\n", appConfig.CreationTimestamp)
	ioStreams.Infof("      Updated at: %v\n", app.UpdateTime.Format(time.RFC3339))
	return nil
}

func traitCheckLoop(ctx context.Context, c client.Client, reference runtimev1alpha1.TypedReference, compName string, appConfig *v1alpha2.ApplicationConfiguration, app *api.Application, timeout time.Duration) (string, string, error) {
	tr, err := oam2.GetUnstructured(ctx, c, appConfig.Namespace, reference)
	if err != nil {
		return "", "", err
	}
	traitType, ok := tr.GetLabels()[oam.TraitTypeLabel]
	if !ok {
		message, err := oam2.GetStatusFromObject(tr)
		return traitType, message, err
	}

	checker := oam2.GetChecker(traitType, c)

	// Health Check Loop For Trait
	var message string
	sHealthCheck := newTrackingSpinner(fmt.Sprintf("Checking %s status ...", traitType))
	sHealthCheck.Start()
	defer sHealthCheck.Stop()
CheckLoop:
	for {
		time.Sleep(trackingInterval)
		var check oam2.CheckStatus
		check, message, err = checker.Check(ctx, reference, compName, appConfig, app)
		if err != nil {
			message = red.Sprintf("%s check failed!", traitType)
			return traitType, message, err
		}
		if check == oam2.StatusDone {
			break CheckLoop
		}
		if time.Since(tr.GetCreationTimestamp().Time) >= timeout {
			return traitType, fmt.Sprintf("Checking timeout: %s", message), nil
		}
	}
	return traitType, message, nil
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

func tryGetWorkloadStatus(ctx context.Context, c client.Client, ns string, wlRef runtimev1alpha1.TypedReference) (string, error) {
	workload, err := oam2.GetUnstructured(ctx, c, ns, wlRef)
	if err != nil {
		return "", err
	}
	return oam2.GetStatusFromObject(workload)
}

func printTrackingDeployStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, compName, appName string, env *types.EnvMeta) (CompStatus, error) {
	sDeploy := newTrackingSpinnerWithDelay("Checking Status ...", trackingInterval)
	sDeploy.Start()
	defer sDeploy.Stop()
TrackDeployLoop:
	for {
		time.Sleep(trackingInterval)
		deployStatus, failMsg, err := TrackDeployStatus(ctx, c, compName, appName, env)
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
func TrackDeployStatus(ctx context.Context, c client.Client, compName, appName string, env *types.EnvMeta) (CompStatus, string, error) {
	app, appObj, err := getApp(ctx, c, compName, appName, env)
	if err != nil {
		return compStatusUnknown, "", err
	}
	if app == nil || appObj == nil {
		return compStatusUnknown, "", errors.New(ErrNotLoadAppConfig)
	}
	condition := appObj.Status.Conditions
	if len(condition) < 1 {
		return compStatusDeploying, "", nil
	}

	// If condition is true, we can regard appConfig is deployed successfully
	if appObj.Status.Phase == v1alpha2.ApplicationRunning {
		return compStatusDeployed, "", nil
	}

	// if not found workload status in AppConfig
	// then use age to check whether the workload controller is running
	if time.Since(appObj.GetCreationTimestamp().Time) > deployTimeout {
		return compStatusDeployFail, condition[0].Message, nil
	}
	return compStatusDeploying, "", nil
}

func trackHealthCheckingStatus(ctx context.Context, c client.Client, compName, appName string, env *types.EnvMeta) (CompStatus, HealthStatus, string, error) {
	app, appConfig, err := getAppConfig(ctx, c, compName, appName, env)
	if err != nil {
		return compStatusUnknown, HealthStatusNotDiagnosed, "", err
	}
	if app == nil || appConfig == nil {
		return compStatusUnknown, HealthStatusNotDiagnosed, "", errors.New(ErrNotLoadAppConfig)
	}

	wlStatus, foundWlStatus := getWorkloadStatusFromAppConfig(appConfig, compName)
	// make sure component already initilized
	if !foundWlStatus {
		if len(appConfig.Status.Conditions) < 1 {
			// still reconciling
			return compStatusUnknown, HealthStatusUnknown, "", nil
		}
		appConfigConditionMsg := appConfig.Status.GetCondition(runtimev1alpha1.TypeSynced).Message
		return compStatusUnknown, HealthStatusUnknown, "", fmt.Errorf(ErrFmtNotInitialized, appConfigConditionMsg)
	}
	// check whether referenced a HealthScope
	var healthScopeName string
	for _, v := range wlStatus.Scopes {
		if v.Reference.Kind == kindHealthScope {
			healthScopeName = v.Reference.Name
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
			return compStatusUnknown, HealthStatusUnknown, "", fmt.Errorf("cannot get health condition from the health scope: %s", healthScope.Name)
		}
		healthStatus = wlhc.HealthStatus
		if healthStatus == HealthStatusHealthy {
			return compStatusHealthCheckDone, healthStatus, wlhc.Diagnosis, nil
		}
		if healthStatus == HealthStatusUnhealthy {
			cTime := appConfig.GetCreationTimestamp()
			if time.Since(cTime.Time) <= healthCheckBufferTime {
				return compStatusHealthChecking, HealthStatusUnknown, "", nil
			}
			return compStatusHealthCheckDone, healthStatus, wlhc.Diagnosis, nil
		}
	}
	// No health scope specified or health status is unknown , try get status from workload
	statusInfo, err := tryGetWorkloadStatus(ctx, c, env.Namespace, wlStatus.Reference)
	if err != nil {
		return compStatusUnknown, HealthStatusUnknown, "", err
	}
	return compStatusHealthCheckDone, HealthStatusNotDiagnosed, statusInfo, nil
}

func getAppConfig(ctx context.Context, c client.Client, compName, appName string, env *types.EnvMeta) (*api.Application, *v1alpha2.ApplicationConfiguration, error) {
	var app *api.Application
	var err error
	if appName != "" {
		app, err = appfile.LoadApplication(env.Name, appName)
	} else {
		app, err = appfile.MatchAppByComp(env.Name, compName)
	}
	if err != nil {
		return nil, nil, err
	}

	appConfig, err := appfile.GetAppConfig(ctx, c, app, env)
	if err != nil {
		return nil, nil, err
	}
	return app, appConfig, nil
}

func getApp(ctx context.Context, c client.Client, compName, appName string, env *types.EnvMeta) (*api.Application, *v1alpha2.Application, error) {
	var app *api.Application
	var err error
	if appName != "" {
		app, err = appfile.LoadApplication(env.Name, appName)
	} else {
		app, err = appfile.MatchAppByComp(env.Name, compName)
	}
	if err != nil {
		return nil, nil, err
	}

	appObj, err := appfile.GetApplication(ctx, c, app, env)
	if err != nil {
		return nil, nil, err
	}
	return app, appObj, nil
}

func getWorkloadStatusFromAppConfig(appConfig *v1alpha2.ApplicationConfiguration, compName string) (v1alpha2.WorkloadStatus, bool) {
	foundWlStatus := false
	wlStatus := v1alpha2.WorkloadStatus{}
	if appConfig == nil {
		return wlStatus, foundWlStatus
	}
	for _, v := range appConfig.Status.Workloads {
		if v.ComponentName == compName {
			wlStatus = v
			foundWlStatus = true
			break
		}
	}
	return wlStatus, foundWlStatus
}

func getHealthStatusColor(s HealthStatus) *color.Color {
	var c *color.Color
	switch s {
	case HealthStatusHealthy:
		c = green
	case HealthStatusUnknown, HealthStatusNotDiagnosed:
		c = yellow
	default:
		c = red
	}
	return c
}
