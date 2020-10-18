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
	"github.com/fatih/color"
	"github.com/ghodss/yaml"
	"github.com/gosuri/uitable"
	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/duration"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
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
	compStatusInitializing CompStatus = iota
	compStatusInitFail
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
	ErrFmtNotInitialized = "oam-core-controller cannot initilize the component: %s"
)

const (
	firstElemPrefix = `├─`
	lastElemPrefix  = `└─`
	indent          = "  "
	pipe            = `│ `
)

var (
	gray   = color.New(color.FgHiBlack)
	red    = color.New(color.FgRed)
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	white  = color.New(color.Bold, color.FgWhite)
)

var (
	emojiSucceed   = emoji.Sprint(":check_mark_button:")
	emojiFail      = emoji.Sprint(":cross_mark:")
	emojiTimeout   = emoji.Sprint(":heavy_exclamation_mark:")
	emojiLightBulb = emoji.Sprint(":light_bulb:")
)

const (
	trackingInterval      time.Duration = 1 * time.Second
	initTimeout           time.Duration = 30 * time.Second
	deployTimeout         time.Duration = 30 * time.Second
	healthCheckBufferTime time.Duration = 120 * time.Second
)

func NewAppStatusCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "status <APPLICATION-NAME>",
		Short:   "get status of an application",
		Long:    "get status of an application, including workloads and traits of each components.",
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
			return printAppStatus(ctx, newClient, ioStreams, appName, env)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printAppStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, appName string, env *types.EnvMeta) error {
	app, err := application.Load(env.Name, appName)
	if err != nil {
		return err
	}
	namespace := env.Name
	tbl := uitable.New()
	tbl.Separator = "  "
	tbl.AddRow(
		white.Sprint("NAMESPCAE"),
		white.Sprint("NAME"),
		white.Sprint("INFO"))

	tbl.AddRow(
		namespace,
		fmt.Sprintf("%s/%s",
			"Application",
			appName))

	components := app.GetComponents()
	// get a map coantaining all workloads health condition
	wlConditionsMap, err := getWorkloadHealthConditions(ctx, c, app, namespace)
	if err != nil {
		return err
	}

	for cIndex, compName := range components {
		var cPrefix string
		switch cIndex {
		case len(components) - 1:
			cPrefix = lastElemPrefix
		default:
			cPrefix = firstElemPrefix
		}

		wlHealthCondition := wlConditionsMap[compName]
		wlHealthStatus := wlHealthCondition.HealthStatus
		healthColor := getHealthStatusColor(wlHealthStatus)

		// print component info
		tbl.AddRow("",
			fmt.Sprintf("%s%s/%s",
				gray.Sprint(printPrefix(cPrefix)),
				"Component",
				compName),
			healthColor.Sprintf("%s %s", wlHealthStatus, wlHealthCondition.Diagnosis))
	}
	ioStreams.Info(tbl)
	return nil
}

// map componentName <=> WorkloadHealthCondition
func getWorkloadHealthConditions(ctx context.Context, c client.Client, app *application.Application, ns string) (map[string]*WorkloadHealthCondition, error) {
	hs := &v1alpha2.HealthScope{}
	// only use default health scope
	hsName := application.FormatDefaultHealthScopeName(app.Name)
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: hsName}, hs); err != nil {
		return nil, err
	}
	wlConditions := hs.Status.WorkloadHealthConditions
	r := map[string]*WorkloadHealthCondition{}
	components := app.GetComponents()
	for _, compName := range components {
		for _, wlhc := range wlConditions {
			if wlhc.ComponentName == compName {
				r[compName] = wlhc
				break
			}
		}
		if r[compName] == nil {
			r[compName] = &WorkloadHealthCondition{
				HealthStatus: HealthStatusNotDiagnosed,
			}
		}
	}

	return r, nil
}

func NewCompStatusCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "status <COMPONENT-NAME>",
		Short:   "get status of a component",
		Long:    "get status of a component, including its workload and health status",
		Example: `vela comp status <COMPONENT-NAME>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength == 0 {
				ioStreams.Errorf("Hint: please specify a component")
				os.Exit(1)
			}
			compName := args[0]
			env, err := GetEnv(cmd)
			if err != nil {
				ioStreams.Errorf("Error: failed to get Env: %s", err)
				return err
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			appName, _ := cmd.Flags().GetString(App)
			return printComponentStatus(ctx, newClient, ioStreams, compName, appName, env)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printComponentStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, compName, appName string, env *types.EnvMeta) error {
	app, appConfig, err := getApp(ctx, c, compName, appName, env)
	if err != nil {
		return err
	}
	if app == nil || appConfig == nil {
		return errors.New(ErrNotLoadAppConfig)
	}

	wlStatus, foundWlStatus := getWorkloadStatusFromAppConfig(appConfig, compName)
	if !foundWlStatus {
		appConfigReconcileStatus := appConfig.Status.GetCondition(runtimev1alpha1.TypeSynced).Status
		switch appConfigReconcileStatus {
		case corev1.ConditionUnknown:
			ioStreams.Info("\nUnknown error occurs during component initialization. \nPlease check OAM controller ...")
		case corev1.ConditionTrue:
			ioStreams.Info("\nThe component is still under initialization, please try again later ...")
		case corev1.ConditionFalse:
			appConfigConditionMsg := appConfig.Status.GetCondition(runtimev1alpha1.TypeSynced).Message
			ioStreams.Info("\nError occurs in OAM runtime during component initialization.")
			ioStreams.Infof("\nOAM controller condition message: %s \n", appConfigConditionMsg)
		}
		return nil
	}

	var healthInfo string
	var healthStatus HealthStatus

	sHealthCheck := newTrackingSpinner("Checking health status ...")
	sHealthCheck.Start()

HealthCheckLoop:
	for {
		time.Sleep(trackingInterval)
		var healthcheckStatus CompStatus
		healthcheckStatus, healthStatus, healthInfo, err = trackHealthCheckingStatus(ctx, c, compName, appName, env)
		if err != nil {
			sHealthCheck.Stop()
			ioStreams.Info(red.Sprintf("Health checking failed!"))
			return err
		}
		if healthcheckStatus == compStatusHealthCheckDone {
			sHealthCheck.Stop()
			break HealthCheckLoop
		}
	}

	ioStreams.Infof("Showing status of Component %s deployed in Environment %s\n", compName, env.Name)
	ioStreams.Infof(white.Sprint("Component Status:\n"))
	workloadType := wlStatus.Reference.Kind
	healthColor := getHealthStatusColor(healthStatus)
	healthInfo = strings.ReplaceAll(healthInfo, "\n", "\n\t") // formart healthInfo output
	ioStreams.Infof("\tName: %s  %s(type) %s %s\n",
		compName, workloadType, healthColor.Sprint(healthStatus), healthColor.Sprint(healthInfo))

	traits, err := app.GetTraits(compName)
	if err != nil {
		return err
	}
	if len(traits) > 0 {
		// print tree structure of Traits
		tbl := uitable.New()
		tbl.Separator = "  "
		traitNames := []string{}
		for k := range traits {
			traitNames = append(traitNames, k)
		}
		for tIndex, tName := range traitNames {
			var tPrefix string
			switch tIndex {
			case len(traitNames) - 1:
				tPrefix = lastElemPrefix
			default:
				tPrefix = firstElemPrefix
			}
			tbl.AddRow(
				"\t",
				fmt.Sprintf("%s%s%s/%s",
					indent,
					gray.Sprint(printPrefix(tPrefix)),
					"Trait",
					tName))
		}
		ioStreams.Info("\tTraits")
		ioStreams.Info(tbl)
	}

	ioStreams.Infof(white.Sprint("\nLast Deployment:\n"))
	ioStreams.Infof("\tCreated at: %v\n", appConfig.CreationTimestamp)
	ioStreams.Infof("\tUpdated at: %v\n", app.UpdateTime.Format(time.RFC3339))
	return nil
}

func getWorkloadInstanceStatusAndCreationTime(ctx context.Context, c client.Client, ns string, wlRef runtimev1alpha1.TypedReference) (string, bool, metav1.Time, error) {
	wlUnstruct := unstructured.Unstructured{}
	wlUnstruct.SetGroupVersionKind(wlRef.GroupVersionKind())
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: wlRef.Name},
		&wlUnstruct); err != nil {
		return "", false, metav1.Time{}, err
	}
	ct := wlUnstruct.GetCreationTimestamp()

	statusData, foundStatus, _ := unstructured.NestedMap(wlUnstruct.Object, "status")
	if foundStatus {
		statusYaml, err := yaml.Marshal(statusData)
		if err != nil {
			return "", false, ct, err
		}
		return string(statusYaml), true, ct, nil
	}
	return "", false, ct, nil
}

func printTrackingInitStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, compName, appName string, env *types.EnvMeta) (CompStatus, error) {
	tInit := time.Now()
	sInit := newTrackingSpinner("Initializing ...")
	sInit.Start()
TrackInitLoop:
	for {
		time.Sleep(trackingInterval)
		if time.Since(tInit) > initTimeout {
			ioStreams.Info(red.Sprintf("\n%sInitialization Timeout After %s!",
				emojiTimeout, duration.HumanDuration(time.Since(tInit))))
			ioStreams.Info(red.Sprint("Please make sure oam-core-controller is installed."))
			sInit.Stop()
			return compStatusUnknown, nil
		}
		initStatus, failMsg, err := trackInitializeStatus(ctx, c, compName, appName, env)
		if err != nil {
			return compStatusUnknown, err
		}
		switch initStatus {
		case compStatusInitializing:
			continue
		case compStatusInitialized:
			ioStreams.Info(green.Sprintf("\n%sInitialization Succeed!", emojiSucceed))
			sInit.Stop()
			break TrackInitLoop
		case compStatusInitFail:
			ioStreams.Info(red.Sprintf("\n%sInitialization Failed!", emojiFail))
			ioStreams.Info(red.Sprintf("Reason: %s", failMsg))
			sInit.Stop()
			return compStatusInitFail, nil
		}
	}
	return compStatusInitialized, nil
}

func trackInitializeStatus(ctx context.Context, c client.Client, compName, appName string, env *types.EnvMeta) (CompStatus, string, error) {
	app, appConfig, err := getApp(ctx, c, compName, appName, env)
	if err != nil {
		return compStatusUnknown, "", err
	}
	if app == nil || appConfig == nil {
		return compStatusUnknown, "", errors.New(ErrNotLoadAppConfig)
	}
	_, foundWlStatus := getWorkloadStatusFromAppConfig(appConfig, compName)
	appConfigReconcileStatus := appConfig.Status.GetCondition(runtimev1alpha1.TypeSynced).Status
	switch appConfigReconcileStatus {
	case corev1.ConditionUnknown:
		return compStatusInitializing, "", nil
	case corev1.ConditionTrue:
		if foundWlStatus {
			return compStatusInitialized, "", nil
		}
		return compStatusInitializing, "", nil
	case corev1.ConditionFalse:
		appConfigConditionMsg := appConfig.Status.GetCondition(runtimev1alpha1.TypeSynced).Message
		return compStatusInitFail, appConfigConditionMsg, nil
	}
	return compStatusInitializing, "", nil
}

func printTrackingDeployStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, compName, appName string, env *types.EnvMeta) (CompStatus, error) {
	sDeploy := newTrackingSpinner("Deploying ...")
	sDeploy.Start()
TrackDeployLoop:
	for {
		time.Sleep(trackingInterval)
		deployStatus, failMsg, err := trackDeployStatus(ctx, c, compName, appName, env)
		if err != nil {
			return compStatusUnknown, err
		}
		switch deployStatus {
		case compStatusDeploying:
			continue
		case compStatusDeployed:
			ioStreams.Info(green.Sprintf("\n%sDeployment Succeed!", emojiSucceed))
			sDeploy.Stop()
			break TrackDeployLoop
		case compStatusDeployFail:
			ioStreams.Info(red.Sprintf("\n%sDeployment Failed!", emojiFail))
			ioStreams.Info(red.Sprintf("Reason: %s", failMsg))
			sDeploy.Stop()
			return compStatusDeployFail, nil
		}
	}
	return compStatusDeployed, nil
}

func trackDeployStatus(ctx context.Context, c client.Client, compName, appName string, env *types.EnvMeta) (CompStatus, string, error) {
	app, appConfig, err := getApp(ctx, c, compName, appName, env)
	if err != nil {
		return compStatusUnknown, "", err
	}
	if app == nil || appConfig == nil {
		return compStatusUnknown, "", errors.New(ErrNotLoadAppConfig)
	}

	wlStatus, foundWlStatus := getWorkloadStatusFromAppConfig(appConfig, compName)
	// make sure component already initilized
	if !foundWlStatus {
		appConfigConditionMsg := appConfig.Status.GetCondition(runtimev1alpha1.TypeSynced).Message
		return compStatusUnknown, "", fmt.Errorf(ErrFmtNotInitialized, appConfigConditionMsg)
	}
	wlRef := wlStatus.Reference

	//TODO(roywang) temporarily use status to judge workload controller is running
	// even not every workload has `status` field
	//TODO(roywang) check whether traits are ready
	_, foundStatus, ct, err := getWorkloadInstanceStatusAndCreationTime(ctx, c, env.Namespace, wlRef)
	if err != nil {
		return compStatusUnknown, "", err
	}
	if foundStatus {
		return compStatusDeployed, "", nil
	}

	// if not found workload status in AppConfig
	// then use age to check whether the worload controller is running
	if time.Since(ct.Time) > deployTimeout {
		return compStatusDeployFail, fmt.Sprintf("The controller of [%s] is not installed or running.",
			wlStatus.Reference.GroupVersionKind().String()), nil
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
	if len(healthScopeName) == 0 {
		// no health scope referenced
		statusInfo, _, _, err := getWorkloadInstanceStatusAndCreationTime(ctx, c, env.Namespace, wlStatus.Reference)
		if err != nil {
			return compStatusUnknown, HealthStatusUnknown, "", err
		}
		return compStatusHealthCheckDone, HealthStatusNotDiagnosed, statusInfo, nil
	}
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
	healthStatus := wlhc.HealthStatus
	if healthStatus == HealthStatusUnknown {
		healthStatus = HealthStatusNotDiagnosed
		statusInfo, _, _, err := getWorkloadInstanceStatusAndCreationTime(ctx, c, env.Namespace, wlStatus.Reference)
		if err != nil {
			return compStatusUnknown, HealthStatusUnknown, "", errors.Wrap(err, "WARN: The component type is unknown to HealthScope and cannot get status.")
		}
		healthInfo := fmt.Sprintf("WARN: The component type is unknown to HealthScope.\nYou may check component status with [%s/%s] status: \n%s",
			wlhc.TargetWorkload.Kind, wlhc.TargetWorkload.Name, statusInfo)
		return compStatusHealthCheckDone, healthStatus, healthInfo, nil
	}
	if healthStatus == HealthStatusUnhealthy {
		cTime := appConfig.GetCreationTimestamp()
		if time.Since(cTime.Time) <= healthCheckBufferTime {
			return compStatusHealthChecking, HealthStatusUnknown, "", nil
		}
	}
	return compStatusHealthCheckDone, healthStatus, wlhc.Diagnosis, nil
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

func printPrefix(p string) string {
	if strings.HasSuffix(p, firstElemPrefix) {
		p = strings.Replace(p, firstElemPrefix, pipe, strings.Count(p, firstElemPrefix)-1)
	} else {
		p = strings.ReplaceAll(p, firstElemPrefix, pipe)
	}

	if strings.HasSuffix(p, lastElemPrefix) {
		p = strings.Replace(p, lastElemPrefix, strings.Repeat(" ", len([]rune(lastElemPrefix))), strings.Count(p, lastElemPrefix)-1)
	} else {
		p = strings.ReplaceAll(p, lastElemPrefix, strings.Repeat(" ", len([]rune(lastElemPrefix))))
	}
	return p
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
