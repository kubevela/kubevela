package commands

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/fatih/color"
	"github.com/gosuri/uitable"
	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	oam2 "github.com/oam-dev/kubevela/pkg/oam"
)

// HealthStatus represents health status strings.
type HealthStatus = v1alpha2.HealthStatus

const (
	// HealthStatusNotDiagnosed means there's no health scope refered or unknown health status returned
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

const (
	// nolint
	compStatusInitializing CompStatus = iota
	// nolint
	compStatusInitFail
	// nolint
	compStatusInitialized
	compStatusDeploying
	compStatusDeployFail
	compStatusDeployed
	compStatusHealthChecking
	compStatusHealthCheckDone
	compStatusUnknown
)

const (
	ErrNotLoadAppConfig  = "cannot load the application"
	ErrFmtNotInitialized = "service: %s not ready"
	ErrServiceNotFound   = "service %s not found in app"
)

var (
	red    = color.New(color.FgRed)
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	white  = color.New(color.Bold, color.FgWhite)
)

var (
	emojiSucceed = emoji.Sprint(":check_mark_button:")
	emojiFail    = emoji.Sprint(":cross_mark:")
	// nolint
	emojiTimeout   = emoji.Sprint(":heavy_exclamation_mark:")
	emojiLightBulb = emoji.Sprint(":light_bulb:")
	// nolint
	emojiWait = emoji.Sprint(":hourglass:")
)

const (
	trackingInterval time.Duration = 1 * time.Second
	// nolint
	initTimeout           time.Duration = 30 * time.Second
	deployTimeout         time.Duration = 10 * time.Second
	healthCheckBufferTime time.Duration = 120 * time.Second
)

func NewAppStatusCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "status <APPLICATION-NAME>",
		Short:   "get status of an application",
		Long:    "get status of an application, including workloads and traits of each service.",
		Example: `vela status <APPLICATION-NAME>`,
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
	app, err := application.Load(env.Name, appName)
	if err != nil {
		return err
	}
	namespace := env.Name

	targetServices, err := oam2.GetServicesWhenDescribingApplication(cmd, app)
	if err != nil {
		return err
	}

	cmd.Printf("About:\n\n")
	table := uitable.New()
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
	app, appConfig, err := getApp(ctx, c, compName, appName, env)
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

func traitCheckLoop(ctx context.Context, c client.Client, reference runtimev1alpha1.TypedReference, compName string, appConfig *v1alpha2.ApplicationConfiguration, app *application.Application, timeout time.Duration) (string, string, error) {
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
	sDeploy := newTrackingSpinner("Checking Status ...")
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
		}
	}
	return compStatusDeployed, nil
}

// TrackDeployStatus will only check AppConfig is deployed successfully,
func TrackDeployStatus(ctx context.Context, c client.Client, compName, appName string, env *types.EnvMeta) (CompStatus, string, error) {
	app, appConfig, err := getApp(ctx, c, compName, appName, env)
	if err != nil {
		return compStatusUnknown, "", err
	}
	if app == nil || appConfig == nil {
		return compStatusUnknown, "", errors.New(ErrNotLoadAppConfig)
	}
	condition := appConfig.Status.Conditions
	if len(condition) < 1 {
		return compStatusDeploying, "", nil
	}

	// If condition is true, we can regard appConfig is deployed successfully
	if condition[0].Status == corev1.ConditionTrue {
		return compStatusDeployed, "", nil
	}

	// if not found workload status in AppConfig
	// then use age to check whether the workload controller is running
	if time.Since(appConfig.GetCreationTimestamp().Time) > deployTimeout {
		return compStatusDeployFail, condition[0].Message, nil
	}
	return compStatusDeploying, "", nil
}

func trackHealthCheckingStatus(ctx context.Context, c client.Client, compName, appName string, env *types.EnvMeta) (CompStatus, HealthStatus, string, error) {
	app, appConfig, err := getApp(ctx, c, compName, appName, env)
	if err != nil {
		return compStatusUnknown, HealthStatusNotDiagnosed, "", err
	}
	if app == nil || appConfig == nil {
		return compStatusUnknown, HealthStatusNotDiagnosed, "", errors.New(ErrNotLoadAppConfig)
	}

	wlStatus, foundWlStatus := getWorkloadStatusFromAppConfig(appConfig, compName)
	// make sure component already initilized
	if !foundWlStatus {
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

func getApp(ctx context.Context, c client.Client, compName, appName string, env *types.EnvMeta) (*application.Application, *v1alpha2.ApplicationConfiguration, error) {
	var app *application.Application
	var err error
	if appName != "" {
		app, err = application.Load(env.Name, appName)
	} else {
		app, err = application.MatchAppByComp(env.Name, compName)
	}
	if err != nil {
		return nil, nil, err
	}

	appConfig := &v1alpha2.ApplicationConfiguration{}
	if err = c.Get(ctx, client.ObjectKey{Namespace: env.Namespace, Name: app.Name}, appConfig); err != nil {
		return nil, nil, err
	}
	return app, appConfig, nil
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

func newTrackingSpinner(suffix string) *spinner.Spinner {
	suffixColor := color.New(color.Bold, color.FgGreen)
	return spinner.New(
		spinner.CharSets[14],
		100*time.Millisecond,
		spinner.WithColor("green"),
		spinner.WithHiddenCursor(true),
		spinner.WithSuffix(suffixColor.Sprintf(" %s", suffix)))
}

func applySpinnerNewSuffix(s *spinner.Spinner, suffix string) {
	suffixColor := color.New(color.Bold, color.FgGreen)
	s.Suffix = suffixColor.Sprintf(" %s", suffix)
}

func getHealthStatusColor(s HealthStatus) *color.Color {
	var c *color.Color
	switch s {
	case HealthStatusHealthy:
		c = green
	case HealthStatusUnhealthy:
		c = red
	case HealthStatusUnknown:
		c = yellow
	case HealthStatusNotDiagnosed:
		c = yellow
	default:
		c = red
	}
	return c
}
