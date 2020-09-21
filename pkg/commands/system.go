package commands

import (
	"context"
	"fmt"
	"io"
	"strings"

	oamv1 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/ghodss/yaml"
	"github.com/openservicemesh/osm/pkg/cli"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/builtin/traitdefinition"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
)

type initCmd struct {
	namespace string
	ioStreams cmdutil.IOStreams
	client    client.Client
	chartPath string
}

type infoCmd struct {
	out io.Writer
}

var (
	defaultObject = []interface{}{
		&oamv1.WorkloadDefinition{},
		&oamv1.ApplicationConfiguration{},
		&oamv1.Component{},
		&oamv1.TraitDefinition{},
		&oamv1.ContainerizedWorkload{},
		&oamv1.HealthScope{},
		&oamv1.ManualScalerTrait{},
		&oamv1.ScopeDefinition{},
	}

	workloadResource = map[string]string{}

	traitResource = map[string]string{
		"manualscalertraits.core.oam.dev":    traitdefinition.ManualScaler,
		"simplerollouttraits.extend.oam.dev": traitdefinition.SimpleRollout,
	}
)

func SystemCommandGroup(c types.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "system management utilities",
		Long:  "system management utilities",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.AddCommand(NewAdminInfoCommand(ioStream), NewRefreshCommand(c, ioStream))
	return cmd
}

func NewAdminInfoCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	i := &infoCmd{out: ioStreams.Out}

	cmd := &cobra.Command{
		Use:   "info",
		Short: "show vela client and cluster chartPath",
		Long:  "show vela client and cluster chartPath",
		RunE: func(cmd *cobra.Command, args []string) error {
			return i.run(ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	return cmd
}

func (i *infoCmd) run(ioStreams cmdutil.IOStreams) error {
	clusterVersion, err := GetOAMReleaseVersion()
	if err != nil {
		return fmt.Errorf("fail to get cluster chartPath: %s", err)
	}
	ioStreams.Info("Versions:")
	ioStreams.Infof("oam-kubernetes-runtime: %s \n", clusterVersion)
	// TODO(wonderflow): we should print all helm charts installed by vela, including plugins

	return nil
}

func NewInstallCommand(c types.Args, chartSource string, ioStreams cmdutil.IOStreams) *cobra.Command {
	i := &initCmd{ioStreams: ioStreams}
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Initialize vela on both client and server",
		Long:  "Install OAM runtime and vela builtin capabilities.",
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			i.client = newClient
			i.namespace = types.DefaultOAMNS
			return i.run(ioStreams, chartSource)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}

	flag := cmd.Flags()
	flag.StringVarP(&i.chartPath, "vela-chart-path", "p", "", "path to vela core chart to override default chart")

	return cmd
}

func (i *initCmd) run(ioStreams cmdutil.IOStreams, chartSource string) error {
	ioStreams.Info("- Installing Vela Core Chart:")
	if !cmdutil.IsNamespaceExist(i.client, types.DefaultOAMNS) {
		if err := cmdutil.NewNamespace(i.client, types.DefaultOAMNS); err != nil {
			return err
		}
	}

	if i.IsOamRuntimeExist() {
		i.ioStreams.Info("Vela system along with OAM runtime already exist.")
	} else {
		if err := InstallOamRuntime(i.chartPath, chartSource); err != nil {
			return err
		}
	}

	ioStreams.Info("- Installing builtin capabilities:")
	if err := GenNativeResourceDefinition(i.client); err != nil {
		return err
	}
	ioStreams.Info()
	if err := RefreshDefinitions(context.Background(), i.client, ioStreams); err != nil {
		return err
	}
	ioStreams.Info("- Finished.")
	return nil
}

func (i *initCmd) IsOamRuntimeExist() bool {
	for _, object := range defaultObject {
		if err := cmdutil.IsCoreCRDExist(context.Background(), i.client, object.(runtime.Object)); err != nil {
			return false
		}
	}
	return oam.IsHelmReleaseRunning(types.DefaultOAMReleaseName, types.DefaultOAMRuntimeChartName, i.ioStreams)
}

func InstallOamRuntime(chartPath, chartSource string) error {
	var err error
	var chartRequested *chart.Chart
	if chartPath != "" {
		chartRequested, err = loader.Load(chartPath)
	} else {
		chartRequested, err = cli.LoadChart(chartSource)
	}
	if err != nil {
		return fmt.Errorf("error loading chart for installation: %s", err)
	}
	installClient, err := oam.NewHelmInstall("", "", types.DefaultOAMReleaseName)
	if err != nil {
		return fmt.Errorf("error create helm install client: %s", err)
	}
	//TODO(wonderflow) values here could give more arguments in command line
	if _, err = installClient.Run(chartRequested, nil); err != nil {
		return err
	}
	return nil
}

func GetOAMReleaseVersion() (string, error) {
	results, err := oam.GetHelmRelease()
	if err != nil {
		return "", err
	}

	for _, result := range results {
		if result.Chart.ChartFullPath() == types.DefaultOAMRuntimeChartName {
			return result.Chart.AppVersion(), nil
		}
	}
	return "", errors.New("oam-kubernetes-runtime not found in your kubernetes cluster, try `vela install` to install")
}

func GenNativeResourceDefinition(c client.Client) error {
	var capabilities []string
	ctx := context.Background()
	for name, manifest := range workloadResource {
		wd := NewWorkloadDefinition(manifest)
		capabilities = append(capabilities, name)
		nwd := &oamv1.WorkloadDefinition{}
		err := c.Get(ctx, client.ObjectKey{Name: name}, nwd)
		if err != nil && kubeerrors.IsNotFound(err) {
			if err := c.Create(context.Background(), &wd); err != nil {
				return fmt.Errorf("create workload definition %s hit an issue: %v", name, err)
			}
			continue
		}
		wd.ResourceVersion = nwd.ResourceVersion
		if err := c.Update(ctx, &wd); err != nil {
			return fmt.Errorf("update workload definition %s err %v", wd.Name, err)
		}
	}

	for name, manifest := range traitResource {
		td := NewTraitDefinition(manifest)
		capabilities = append(capabilities, name)
		ntd := &oamv1.TraitDefinition{}
		err := c.Get(context.Background(), client.ObjectKey{Name: name}, ntd)
		if err != nil && kubeerrors.IsNotFound(err) {
			if err := c.Create(context.Background(), &td); err != nil {
				return fmt.Errorf("create trait definition %s hit an issue: %v", name, err)
			}
			continue
		}
		td.ResourceVersion = ntd.ResourceVersion
		if err := c.Update(ctx, &td); err != nil {
			return fmt.Errorf("update trait definition %s err %v", td.Name, err)
		}
	}

	fmt.Printf("Successful applied %d kinds of Workloads and Traits: %s.", len(capabilities), strings.Join(capabilities, ","))
	return nil
}

func NewWorkloadDefinition(manifest string) oamv1.WorkloadDefinition {
	var workloadDefinition oamv1.WorkloadDefinition
	// We have tests to make sure built-in resource can always unmarshal succeed
	_ = yaml.Unmarshal([]byte(manifest), &workloadDefinition)
	return workloadDefinition
}

func NewTraitDefinition(manifest string) oamv1.TraitDefinition {
	var traitDefinition oamv1.TraitDefinition
	// We have tests to make sure built-in resource can always unmarshal succeed
	_ = yaml.Unmarshal([]byte(manifest), &traitDefinition)
	return traitDefinition
}
