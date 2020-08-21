package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/cloud-native-application/rudrx/pkg/builtin/traitdefinition"

	"github.com/cloud-native-application/rudrx/pkg/builtin/workloaddefinition"

	"github.com/ghodss/yaml"

	"helm.sh/helm/v3/pkg/release"

	"github.com/cloud-native-application/rudrx/api/types"

	"k8s.io/apimachinery/pkg/runtime"

	oamv1 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/kube"
)

var (
	settings = cli.New()
)

type initCmd struct {
	namespace string
	ioStreams cmdutil.IOStreams
	client    client.Client
	version   string
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

	workloadResource = map[string]string{
		"deployments.apps":                    workloaddefinition.Deployment,
		"containerizedworkloads.core.oam.dev": workloaddefinition.ContainerizedWorkload,
	}

	traitResource = map[string]string{
		"manualscalertraits.core.oam.dev":    traitdefinition.ManualScaler,
		"simplerollouttraits.extend.oam.dev": traitdefinition.SimpleRollout,
	}
)

func SystemCommandGroup(parentCmd *cobra.Command, c types.Args, ioStream cmdutil.IOStreams) {
	parentCmd.AddCommand(NewAdminInitCommand(c, ioStream),
		NewAdminInfoCommand(ioStream),
	)
}

func NewAdminInfoCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	i := &infoCmd{out: ioStreams.Out}

	cmd := &cobra.Command{
		Use:   "system:info",
		Short: "show vela client and cluster version",
		Long:  "show vela client and cluster version",
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
		ioStreams.Errorf("fail to get cluster version, err: %v \n", err)
		return err
	}
	ioStreams.Info("Versions:")
	ioStreams.Infof("oam-kubernetes-runtime: %s \n", clusterVersion)
	// TODO(wonderflow): we should print all helm charts installed by vela, including plugins

	return nil
}

func NewAdminInitCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	i := &initCmd{ioStreams: ioStreams}
	cmd := &cobra.Command{
		Use:   "system:init",
		Short: "Initialize vela on both client and server",
		Long:  "Install OAM runtime and vela builtin capabilities.",
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			i.client = newClient
			i.namespace = types.DefaultOAMNS
			return i.run(ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}

	flag := cmd.Flags()
	flag.StringVarP(&i.version, "version", "v", "", "Override chart version")

	return cmd
}

func (i *initCmd) run(ioStreams cmdutil.IOStreams) error {
	ioStreams.Info("- Installing OAM Kubernetes Runtime:")
	if !cmdutil.IsNamespaceExist(i.client, types.DefaultOAMNS) {
		if err := cmdutil.NewNamespace(i.client, types.DefaultOAMNS); err != nil {
			return err
		}
	}

	if i.IsOamRuntimeExist() {
		i.ioStreams.Info("Vela system along with OAM runtime already exist.")
	}

	if err := InstallOamRuntime(ioStreams, i.version); err != nil {
		return err
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
		if err := cmdutil.IsCoreCRDExist(i.client, context.Background(), object.(runtime.Object)); err != nil {
			return false
		}
	}
	return IsHelmReleaseRunning(types.DefaultOAMReleaseName, types.DefaultOAMRuntimeChartName, i.ioStreams)
}

func IsHelmReleaseRunning(releaseName, chartName string, streams cmdutil.IOStreams) bool {
	releases, err := GetHelmRelease()
	if err != nil {
		streams.Error("get helm release err", err)
		return false
	}
	for _, r := range releases {
		if strings.Contains(r.Chart.ChartFullPath(), chartName) && r.Name == releaseName {
			return true
		}
	}
	return false
}

func InstallOamRuntime(ioStreams cmdutil.IOStreams, version string) error {
	return HelmInstall(ioStreams, types.DefaultOAMRepoName, types.DefaultOAMRepoUrl, types.DefaultOAMRuntimeChartName, version, types.DefaultOAMReleaseName, nil)
}

func HelmInstall(ioStreams cmdutil.IOStreams, repoName, repoUrl, chartName, version, releaseName string, vals map[string]interface{}) error {
	if !IsHelmRepositoryExist(repoName, repoUrl) {
		err := AddHelmRepository(repoName, repoUrl,
			"", "", "", "", "", false, ioStreams.Out)
		if err != nil {
			return err
		}
	}
	if IsHelmReleaseRunning(releaseName, chartName, ioStreams) {
		return nil
	}

	chartClient, err := NewHelmInstall(version, releaseName, ioStreams)
	if err != nil {
		return err
	}
	chartRequested, err := GetChart(chartClient, repoName+"/"+chartName)
	if err != nil {
		return err
	}
	release, err := chartClient.Run(chartRequested, vals)
	if err != nil {
		return err
	}
	ioStreams.Infof("Successfully installed %s as release name %s\n", chartName, release.Name)
	return nil
}

func HelmUninstall(ioStreams cmdutil.IOStreams, chartName, releaseName string) error {
	if !IsHelmReleaseRunning(releaseName, chartName, ioStreams) {
		return nil
	}
	uninstall, err := NewHelmUninstall()
	if err != nil {
		return err
	}
	_, err = uninstall.Run(releaseName)
	if err != nil {
		return err
	}
	ioStreams.Infof("Successfully removed %s with release name %s\n", chartName, releaseName)
	return nil
}

func NewHelmInstall(version, releaseName string, ioStreams cmdutil.IOStreams) (*action.Install, error) {
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		kube.GetConfig(cmdutil.GetKubeConfig(), "", types.DefaultOAMNS),
		types.DefaultOAMNS,
		os.Getenv("HELM_DRIVER"),
		debug,
	); err != nil {
		return nil, err
	}

	client := action.NewInstall(actionConfig)
	client.ReleaseName = releaseName
	// MUST set here, client didn't use namespace from configuration
	client.Namespace = types.DefaultOAMNS

	if len(version) > 0 {
		client.Version = version
	} else {
		client.Version = types.DefaultOAMVersion
	}
	return client, nil
}

func NewHelmUninstall() (*action.Uninstall, error) {
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(
		kube.GetConfig(cmdutil.GetKubeConfig(), "", types.DefaultOAMNS),
		types.DefaultOAMNS,
		os.Getenv("HELM_DRIVER"),
		debug,
	); err != nil {
		return nil, err
	}
	return action.NewUninstall(actionConfig), nil
}

func debug(format string, v ...interface{}) {
	if settings.Debug {
		format = fmt.Sprintf("[debug] %s\n", format)
		log.Output(2, fmt.Sprintf(format, v...))
	}
}

func GetChart(client *action.Install, name string) (*chart.Chart, error) {
	settings.Debug = true

	chartPath, err := client.ChartPathOptions.LocateChart(name, settings)
	if err != nil {
		return nil, err
	}

	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return nil, err
	}
	return chartRequested, nil
}

func AddHelmRepository(name, url, username, password, certFile, keyFile, caFile string, insecureSkipTLSverify bool, out io.Writer) error {
	var f repo.File
	c := repo.Entry{
		Name:                  name,
		URL:                   url,
		Username:              username,
		Password:              password,
		CertFile:              certFile,
		KeyFile:               keyFile,
		CAFile:                caFile,
		InsecureSkipTLSverify: insecureSkipTLSverify,
	}

	r, err := repo.NewChartRepository(&c, getter.All(settings))
	if err != nil {
		return err
	}

	if _, err := r.DownloadIndexFile(); err != nil {
		return errors.Wrapf(err, "looks like %q is not a valid chart repository or cannot be reached", url)
	}

	f.Update(&c)

	if err := f.WriteFile(settings.RepositoryConfig, 0644); err != nil {
		return err
	}
	fmt.Fprintf(out, "%q has been added to your repositories\n", name)
	return nil
}

func IsHelmRepositoryExist(name, url string) bool {
	repos := GetHelmRepositoryList()
	for _, repo := range repos {
		if repo.Name == name && repo.URL == url {
			return true
		}
	}
	return false
}

func GetHelmRepositoryList() []*repo.Entry {
	f, err := repo.LoadFile(settings.RepositoryConfig)
	if err == nil && len(f.Repositories) > 0 {
		return filterRepos(f.Repositories)
	}
	return nil
}

func GetHelmRelease() ([]*release.Release, error) {
	actionConfig := new(action.Configuration)
	client := action.NewList(actionConfig)

	if err := actionConfig.Init(settings.RESTClientGetter(), types.DefaultOAMNS, os.Getenv("HELM_DRIVER"), debug); err != nil {
		return nil, err
	}
	results, err := client.Run()
	if err != nil {
		return nil, err
	}

	return results, nil
}

func GetOAMReleaseVersion() (string, error) {
	results, err := GetHelmRelease()
	if err != nil {
		return "", err
	}

	for _, result := range results {
		if result.Chart.ChartFullPath() == types.DefaultOAMRuntimeChartName {
			return result.Chart.AppVersion(), nil
		}
	}
	return "", errors.New("oam-kubernetes-runtime not found in your kubernetes cluster, try `vela system:init` to install.")
}

func filterRepos(repos []*repo.Entry) []*repo.Entry {
	filteredRepos := make([]*repo.Entry, 0)
	for _, repo := range repos {
		filteredRepos = append(filteredRepos, repo)
	}
	return filteredRepos
}

func GenNativeResourceDefinition(c client.Client) error {
	var capabilities []string
	for name, manifest := range workloadResource {
		workloadDefinition, err := NewWorkloadDefinition(manifest)
		if err != nil {
			continue
		}
		err = c.Get(context.Background(), client.ObjectKey{Name: name}, &workloadDefinition)
		if kubeerrors.IsNotFound(err) {
			if err := c.Create(context.Background(), &workloadDefinition); err != nil {
				return fmt.Errorf("create workload definition %s hit an issue: %v", name, err)
			}
		} else if err != nil {
			return fmt.Errorf("get workload definition hit an issue: %v", err)
		}
		capabilities = append(capabilities, name)
	}

	for name, manifest := range traitResource {
		traitDefinition, err := NewTraitDefinition(manifest)
		if err != nil {
			fmt.Printf("creating local definition %s err %v", name, err)
			continue
		}
		err = c.Get(context.Background(), client.ObjectKey{Name: name}, &traitDefinition)
		if kubeerrors.IsNotFound(err) {
			if err := c.Create(context.Background(), &traitDefinition); err != nil {
				return fmt.Errorf("create workload definition %s hit an issue: %v", name, err)
			}
		} else if err != nil {
			return fmt.Errorf("get workload definition hit an issue: %v", err)
		}
		capabilities = append(capabilities, name)
	}

	fmt.Printf("Successful applied %d kinds of Workloads and Traits: %s.", len(capabilities), strings.Join(capabilities, ","))
	return nil
}

func NewWorkloadDefinition(manifest string) (oamv1.WorkloadDefinition, error) {
	var workloadDefinition oamv1.WorkloadDefinition
	err := yaml.Unmarshal([]byte(manifest), &workloadDefinition)
	return workloadDefinition, err
}

func NewTraitDefinition(manifest string) (oamv1.TraitDefinition, error) {
	var traitDefinition oamv1.TraitDefinition
	err := yaml.Unmarshal([]byte(manifest), &traitDefinition)
	return traitDefinition, err
}
