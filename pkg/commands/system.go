package commands

import (
	"context"
	"fmt"
	"io"

	"github.com/openservicemesh/osm/pkg/cli"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/strvals"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
)

type initCmd struct {
	namespace string
	ioStreams cmdutil.IOStreams
	client    client.Client
	chartPath string
	chartArgs chartArgs
}

type chartArgs struct {
	imageRepo       string
	imageTag        string
	imagePullPolicy string
}

type infoCmd struct {
	out io.Writer
}

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
		return fmt.Errorf("fail to get cluster chartPath: %v", err)
	}
	ioStreams.Info("Versions:")
	ioStreams.Infof("oam-kubernetes-runtime: %s \n", clusterVersion)
	// TODO(wonderflow): we should print all helm charts installed by vela, including plugins

	return nil
}

func NewInstallCommand(c types.Args, chartContent string, ioStreams cmdutil.IOStreams) *cobra.Command {
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
			return i.run(ioStreams, chartContent)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}

	flag := cmd.Flags()
	flag.StringVarP(&i.chartPath, "vela-chart-path", "p", "", "path to vela core chart to override default chart")
	flag.StringVarP(&i.chartArgs.imagePullPolicy, "image-pull-policy", "", "Always", "vela core image pull policy, this will align to chart value image.pullPolicy")
	flag.StringVarP(&i.chartArgs.imageRepo, "image-repo", "", "oamdev/vela-core", "vela core image repo, this will align to chart value image.repo")
	flag.StringVarP(&i.chartArgs.imageTag, "image-tag", "", "latest", "vela core image repo, this will align to chart value image.tag")

	return cmd
}

func (i *initCmd) run(ioStreams cmdutil.IOStreams, chartSource string) error {
	ioStreams.Info("- Installing Vela Core Chart:")
	exist, err := cmdutil.DoesNamespaceExist(i.client, types.DefaultOAMNS)
	if err != nil {
		return err
	}
	if !exist {
		if err := cmdutil.NewNamespace(i.client, types.DefaultOAMNS); err != nil {
			return err
		}
		ioStreams.Info("created namespace", types.DefaultOAMNS)
	}

	if oam.IsHelmReleaseRunning(types.DefaultOAMReleaseName, types.DefaultOAMRuntimeChartName, i.ioStreams) {
		i.ioStreams.Info("Vela system along with OAM runtime already exist.")
	} else {
		vals, err := i.resolveValues()
		if err != nil {
			i.ioStreams.Errorf("resolve values for vela-core chart err %v, will install with default values", err)
			vals = make(map[string]interface{})
		}
		if err := InstallOamRuntime(i.chartPath, chartSource, vals, ioStreams); err != nil {
			return err
		}
	}

	if err := RefreshDefinitions(context.Background(), i.client, ioStreams); err != nil {
		return err
	}
	ioStreams.Info("- Finished successfully.")
	return nil
}

func (i *initCmd) resolveValues() (map[string]interface{}, error) {
	finalValues := map[string]interface{}{}
	valuesConfig := []string{
		//TODO(wonderflow) values here could give more arguments in command line
		fmt.Sprintf("image.repository=%s", i.chartArgs.imageRepo),
		fmt.Sprintf("image.tag=%s", i.chartArgs.imageTag),
		fmt.Sprintf("image.pullPolicy=%s", i.chartArgs.imagePullPolicy),
	}
	for _, val := range valuesConfig {
		// parses Helm strvals line and merges into a map for the final overrides for values.yaml
		if err := strvals.ParseInto(val, finalValues); err != nil {
			return nil, err
		}
	}
	return finalValues, nil
}

func InstallOamRuntime(chartPath, chartSource string, vals map[string]interface{}, ioStreams cmdutil.IOStreams) error {
	var err error
	var chartRequested *chart.Chart
	if chartPath != "" {
		ioStreams.Infof("Use customized chart at: %s", chartPath)
		chartRequested, err = loader.Load(chartPath)
	} else {
		chartRequested, err = cli.LoadChart(chartSource)
		ioStreams.Infof("install chart %s, version %s, desc : %s, contains %d file\n",
			chartRequested.Metadata.Name, chartRequested.Metadata.Version, chartRequested.Metadata.Description,
			len(chartRequested.Raw))
	}
	if err != nil {
		return fmt.Errorf("error loading chart for installation: %s", err)
	}
	installClient, err := oam.NewHelmInstall("", types.DefaultOAMNS, types.DefaultOAMReleaseName)
	if err != nil {
		return fmt.Errorf("error create helm install client: %s", err)
	}
	release, err := installClient.Run(chartRequested, vals)
	if err != nil {
		ioStreams.Errorf("Failed to install the chart with error: %+v\n", err)
		return err
	}
	ioStreams.Infof("Successfully installed the chart, status: %s, last deployed time = %s\n",
		release.Info.Status,
		release.Info.LastDeployed.String())
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
