package oam

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

var (
	settings = cli.New()
)

func HelmInstall(ioStreams cmdutil.IOStreams, repoName, repoURL, chartName, version, namespace, releaseName string,
	vals map[string]interface{}) error {
	if !IsHelmRepositoryExist(repoName, repoURL) {
		err := AddHelmRepository(repoName, repoURL,
			"", "", "", "", "", false, ioStreams.Out)
		if err != nil {
			return err
		}
	}
	if IsHelmReleaseRunning(releaseName, chartName, ioStreams) {
		return nil
	}

	chartClient, err := NewHelmInstall(version, namespace, releaseName)
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

func NewHelmInstall(version, namespace, releaseName string) (*action.Install, error) {
	actionConfig := new(action.Configuration)
	if len(namespace) == 0 {
		namespace = types.DefaultOAMNS
	}
	if err := actionConfig.Init(
		kube.GetConfig(cmdutil.GetKubeConfig(), "", types.DefaultOAMNS),
		namespace,
		os.Getenv("HELM_DRIVER"),
		debug,
	); err != nil {
		return nil, err
	}

	client := action.NewInstall(actionConfig)
	client.ReleaseName = releaseName
	// MUST set here, client didn't use namespace from configuration
	client.Namespace = namespace

	if len(version) > 0 {
		client.Version = version
	} else {
		client.Version = types.DefaultOAMVersion
	}
	return client, nil
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
		return f.Repositories
	}
	return nil
}
func debug(format string, v ...interface{}) {
	if settings.Debug {
		format = fmt.Sprintf("[debug] %s\n", format)
		_ = log.Output(2, fmt.Sprintf(format, v...))
	}
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
