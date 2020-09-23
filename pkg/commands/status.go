package commands

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/fatih/color"
	"github.com/ghodss/yaml"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

// HealthStatus represents health status strings.
type HealthStatus = v1alpha2.HealthStatus

const (
	// StatusNotDiagnosed means there's no health check info returned from the scope.
	StatusNotDiagnosed HealthStatus = "NOT DIAGNOSED"
)

const (
	// StatusHealthy represents healthy status.
	StatusHealthy = v1alpha2.StatusHealthy
	// StatusUnhealthy represents unhealthy status.
	StatusUnhealthy = v1alpha2.StatusUnhealthy
	// StatusUnknown represents unknown status.
	StatusUnknown = v1alpha2.StatusUnknown
)

// WorkloadHealthCondition holds health status of any resource
type WorkloadHealthCondition = v1alpha2.WorkloadHealthCondition

// ScopeHealthCondition holds health condition of a scope
type ScopeHealthCondition = v1alpha2.ScopeHealthCondition

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
	kindHealthScope = reflect.TypeOf(v1alpha2.HealthScope{}).Name()
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
				HealthStatus: StatusNotDiagnosed,
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
	ioStreams.Infof("Showing status of Component %s deployed in Environment %s\n", compName, env.Name)
	var app *application.Application
	var err error
	if appName != "" {
		app, err = application.Load(env.Name, appName)
	} else {
		app, err = application.MatchAppByComp(env.Name, compName)
	}
	if err != nil {
		return err
	}

	var appConfig v1alpha2.ApplicationConfiguration
	if err = c.Get(ctx, client.ObjectKey{Namespace: env.Namespace, Name: app.Name}, &appConfig); err != nil {
		return err
	}

	var wlStatus v1alpha2.WorkloadStatus // very important
	for _, v := range appConfig.Status.Workloads {
		if v.ComponentName == compName {
			wlStatus = v
			break
		}
	}
	if wlStatus.ComponentName == "" {
		//TODO(roywang) cannot get workload instance
		//TODO(roywang) if appConfig reconcile condition is false, then output err msg
		ioStreams.Infof("\nComponent is not created yet.")
		return nil
	}

	var (
		healthColor  *color.Color
		healthStatus HealthStatus
		healthInfo   string
		workloadType string
	)

	workloadType = wlStatus.Reference.Kind

	// check whether referenced a HealthScope
	var healthScopeName string
	for _, v := range wlStatus.Scopes {
		if v.Reference.Kind == kindHealthScope {
			healthScopeName = v.Reference.Name
		}
	}

	if len(healthScopeName) == 0 {
		// no health scope referenced
		healthStatus = StatusNotDiagnosed
		statusInfo, err := getWorkloadStatus(ctx, c, env.Namespace, wlStatus.Reference)
		if err != nil {
			return err
		}
		// format output
		statusInfo = strings.ReplaceAll(statusInfo, "\n", "\n\t")
		healthInfo = fmt.Sprintf("%s \n\n\tWARN: The component is not in any HealthScope.  \n%s", healthStatus, statusInfo)
	} else {
		var healthScope v1alpha2.HealthScope
		if err = c.Get(ctx, client.ObjectKey{Namespace: env.Namespace, Name: healthScopeName}, &healthScope); err != nil {
			return err
		}
		var wlhc *v1alpha2.WorkloadHealthCondition
		for _, v := range healthScope.Status.WorkloadHealthConditions {
			if v.ComponentName == compName {
				wlhc = v
			}
		}
		healthStatus = wlhc.HealthStatus
		if healthStatus == StatusUnknown {
			healthStatus = StatusNotDiagnosed
			statusInfo, err := getWorkloadStatus(ctx, c, env.Namespace, wlStatus.Reference)
			if err != nil {
				return err
			}
			// format output
			statusInfo = strings.ReplaceAll(statusInfo, "\n", "\n\t")
			healthInfo = fmt.Sprintf("%s \n\n\tWARN: The component type is unknown to HealthScope. \n\t%s", healthStatus, statusInfo)
		} else {
			healthInfo = fmt.Sprintf("%s %s", healthStatus, wlhc.Diagnosis)
		}
	}

	ioStreams.Infof(white.Sprint("Component Status:\n"))
	healthColor = getHealthStatusColor(healthStatus)

	ioStreams.Infof("\tName: %s  %s(type) %s \n", compName, workloadType, healthColor.Sprint(healthInfo))

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
	case StatusHealthy:
		c = green
	case StatusUnhealthy:
		c = red
	case StatusUnknown:
		c = yellow
	case StatusNotDiagnosed:
		c = yellow
	default:
		c = red
	}
	return c
}

func getWorkloadStatus(ctx context.Context, c client.Client, ns string, wlRef runtimev1alpha1.TypedReference) (string, error) {
	wlUnstruct := unstructured.Unstructured{}
	wlUnstruct.SetGroupVersionKind(wlRef.GroupVersionKind())
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: wlRef.Name},
		&wlUnstruct); err != nil {
		return "", err
	}
	statusData, foundStatus, _ := unstructured.NestedMap(wlUnstruct.Object, "status")
	if foundStatus {
		statusYaml, err := yaml.Marshal(statusData)
		if err != nil {
			return "", err
		}
		return string(statusYaml), nil
	}
	return red.Sprintf("Error: Cannot get status info. \nPlease check the controller of workload: %s.", wlRef.GroupVersionKind().String()), nil
}
