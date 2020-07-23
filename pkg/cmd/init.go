package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"k8s.io/apimachinery/pkg/runtime"

	oamv1 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"

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

const initDesc = `
This command installs oam-kubernetes-runtime  onto your Kubernetes Cluster.
As with the rest of the RudrX commands, 'rudrx init' discovers Kubernetes clusters
by reading $KUBECONFIG (default '~/.kube/config') and using the default context.
When installing oam-kubernetes-runtime, 'rudrx init' will attempt to install the latest released
version. 
`

type initCmd struct {
	namespace string
	out       io.Writer
	client    client.Client
	config    *rest.Config
}

const (
	DefaultOAMNS          = "oam-system"
	DefaultOAMReleaseName = "core-runtime"
	DefaultOAMChartName   = "crossplane-master/oam-kubernetes-runtime"
	DefaultOAMRepoName    = "crossplane-master"
	DefaultOAMRepoUrl     = "https://charts.crossplane.io/master"
	DefaultOAMVersion     = ">0.0.0-0"
)

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
)

func NewInitCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams) *cobra.Command {

	i := &initCmd{out: ioStreams.Out}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize RudrX on both client and server",
		Long:  initDesc,
		RunE: func(cmd *cobra.Command, args []string) error {
			i.client = c
			i.namespace = DefaultOAMNS
			return i.run(ioStreams)
		},
	}

	return cmd
}

func (i *initCmd) run(ioStreams cmdutil.IOStreams) error {

	if err := cmdutil.GetKubeClient(); err != nil {
		return fmt.Errorf("could not get kubernetes client: %s", err)
	}

	if !cmdutil.IsNamespaceExist(i.client, DefaultOAMNS) {
		if err := cmdutil.NewNamespace(i.client, DefaultOAMNS); err != nil {
			return err
		}
	}

	if i.IsOamRuntimeExist() {
		fmt.Println("Successfully initialized.")
		return nil
	}

	if err := InstallOamRuntime(ioStreams); err != nil {
		return err
	}

	return nil
}

func (i *initCmd) IsOamRuntimeExist() bool {

	for _, object := range defaultObject {
		if err := cmdutil.IsCoreCRDExist(i.client, context.Background(), object.(runtime.Object)); err != nil {
			return false
		}
	}
	return true
}

func InstallOamRuntime(ioStreams cmdutil.IOStreams) error {

	err := AddHelmRepository(DefaultOAMRepoName, DefaultOAMRepoUrl,
		"", "", "", "", "", false, ioStreams.Out)
	if err != nil {
		return err
	}

	chartClient, err := NewHelmInstall()
	if err != nil {
		return err
	}

	chartRequested, err := GetChart(chartClient, DefaultOAMChartName)
	if err != nil {
		return err
	}

	release, err := chartClient.Run(chartRequested, nil)
	if err != nil {
		return err
	}

	fmt.Println("Successfully installed oam-kubernetes-runtime release: ", release.Name)
	return nil
}

func NewHelmInstall() (*action.Install, error) {
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(
		kube.GetConfig(cmdutil.GetKubeConfig(), "", DefaultOAMNS),
		DefaultOAMNS,
		os.Getenv("HELM_DRIVER"),
		func(format string, v ...interface{}) {
			fmt.Sprintf(format, v)
		},
	); err != nil {
		return nil, err
	}

	client := action.NewInstall(actionConfig)
	client.Namespace = DefaultOAMNS
	client.ReleaseName = DefaultOAMReleaseName
	client.Version = DefaultOAMVersion

	return client, nil
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
