package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloud-native-application/rudrx/pkg/builtin"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	"helm.sh/helm/v3/pkg/release"

	"github.com/cloud-native-application/rudrx/api/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/apimachinery/pkg/runtime"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	oamv1 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"

	"github.com/cloud-native-application/rudrx/pkg/plugins"
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
		"deployment": builtin.Deployment,
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
	ioStreams.Info("- Install OAM runtime:")
	if !cmdutil.IsNamespaceExist(i.client, types.DefaultOAMNS) {
		if err := cmdutil.NewNamespace(i.client, types.DefaultOAMNS); err != nil {
			return err
		}
	}

	if i.IsOamRuntimeExist() {
		i.ioStreams.Info("Vela system along with OAM runtime already exist.")
		return nil
	}

	if err := InstallOamRuntime(ioStreams, i.version); err != nil {
		return err
	}

	ioStreams.Info("- Apply builtin capabilities:")
	if err := GenNativeResourceDefinition(i.client); err != nil {
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
	return HelmInstall(ioStreams, types.DefaultOAMRepoName, types.DefaultOAMRepoUrl, types.DefaultOAMRuntimeChartName, version, types.DefaultOAMReleaseName)
}

func HelmInstall(ioStreams cmdutil.IOStreams, repoName, repoUrl, chartName, version, releaseName string) error {
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
	release, err := chartClient.Run(chartRequested, nil)
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
	client.Version = version
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
	for name, reference := range workloadResource {
		workload := NewWorkloadDefinition(name, reference)
		err := c.Get(context.Background(), client.ObjectKey{Name: name}, workload)
		if kubeerrors.IsNotFound(err) {
			if err := c.Create(context.Background(), workload); err != nil {
				return fmt.Errorf("create workload definition %s hit an issue: %v", reference, err)
			}
		} else if err != nil {
			return fmt.Errorf("get workload definition hit an issue: %v", err)
		}
	}

	return nil

}

func NewWorkloadDefinition(name, reference string) *oamv1.WorkloadDefinition {
	return &oamv1.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: oamv1.WorkloadDefinitionSpec{
			Reference: oamv1.DefinitionReference{Name: reference},
		},
	}
}

func GetTraitAliasByTraitDefinition(traitDefinition oamv1.TraitDefinition) (string, error) {
	velaApplicationFolder := filepath.Join("~/.vela", "applications")
	system.CreateIfNotExist(velaApplicationFolder)
	d, _ := ioutil.TempDir(velaApplicationFolder, "cue")
	defer os.RemoveAll(d)
	template, err := plugins.HandleTemplate(traitDefinition.Spec.Extension, traitDefinition.Name, d)
	if err != nil {
		return "", nil
	}
	return template.Name, nil
}

func GetTraitAliasByKind(ctx context.Context, c client.Client, traitKind string) string {
	var traitAlias string
	t, err := GetTraitDefinitionByKind(ctx, c, traitKind)
	if err != nil {
		return traitKind
	}

	if traitAlias, err = GetTraitAliasByTraitDefinition(t); err != nil {
		return traitKind
	}

	return traitAlias
}
func GetTraitDefinitionByKind(ctx context.Context, c client.Client, traitKind string) (oamv1.TraitDefinition, error) {
	var traitDefinitionList oamv1.TraitDefinitionList
	var traitDefinition oamv1.TraitDefinition
	if err := c.List(ctx, &traitDefinitionList); err != nil {
		return traitDefinition, err
	}
	for _, t := range traitDefinitionList.Items {
		if t.Annotations["oam.appengine.info/kind"] == traitKind {
			return t, nil
		}
	}
	return traitDefinition, errors.New(fmt.Sprintf("Could not find TraitDefinition by kind %s", traitKind))
}

func GetTraitAliasByComponentTraitList(ctx context.Context, c client.Client, componentTraitList []corev1alpha2.ComponentTrait) []string {
	var traitAlias []string
	for _, t := range componentTraitList {
		_, _, kind := GetGVKFromRawExtension(t.Trait)
		alias := GetTraitAliasByKind(ctx, c, kind)
		traitAlias = append(traitAlias, alias)
	}
	return traitAlias
}

func GetGVKFromRawExtension(extension runtime.RawExtension) (string, string, string) {
	if extension.Object != nil {
		gvk := extension.Object.GetObjectKind().GroupVersionKind()
		return gvk.Group, gvk.Version, gvk.Kind
	}
	var data map[string]interface{}
	// leverage Admission Controller to do the check
	_ = json.Unmarshal(extension.Raw, &data)
	obj := unstructured.Unstructured{Object: data}
	gvk := obj.GroupVersionKind()
	return gvk.Group, gvk.Version, gvk.Kind
}
