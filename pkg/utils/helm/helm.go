/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package helm

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	v1 "k8s.io/api/core/v1"
	types2 "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// VelaDebugLog defines an ENV to set vela helm install log to be debug
const (
	VelaDebugLog       = "VELA_DEBUG"
	userNameSecKey     = "username"
	userPasswordSecKey = "password"
	caFileSecKey       = "caFile"
	keyFileKey         = "keyFile"
	certFileKey        = "certFile"
)

var (
	settings = cli.New()
)

// Install will install helm chart
func Install(ioStreams cmdutil.IOStreams, repoName, repoURL, chartName, version, namespace, releaseName string,
	vals map[string]interface{}) error {

	if len(namespace) > 0 {
		args, err := common.InitBaseRestConfig()
		if err != nil {
			return err
		}
		kubeClient, err := args.GetClient()
		if err != nil {
			return err
		}
		exist, err := cmdutil.DoesNamespaceExist(kubeClient, namespace)
		if err != nil {
			return err
		}
		if !exist {
			if err = cmdutil.NewNamespace(kubeClient, namespace); err != nil {
				return fmt.Errorf("create namespace (%s) failed for chart %s: %w", namespace, chartName, err)
			}
		}
	}
	// check release running first before add repo and install chart
	if IsHelmReleaseRunning(releaseName, chartName, namespace, ioStreams) {
		return nil
	}

	if !IsHelmRepositoryExist(repoName, repoURL) {
		err := AddHelmRepository(repoName, repoURL,
			"", "", "", "", "", false, ioStreams.Out)
		if err != nil {
			return err
		}
	}

	chartClient, err := NewHelmInstall(version, namespace, releaseName)
	if err != nil {
		return err
	}
	chartRequested, err := GetChart(chartClient, repoName+"/"+chartName)
	if err != nil {
		return err
	}
	rel, err := chartClient.Run(chartRequested, vals)
	if err != nil {
		return err
	}
	ioStreams.Infof("Successfully installed chart (%s) with release name (%s)\n", chartName, rel.Name)
	return nil
}

// Uninstall will uninstall helm chart
func Uninstall(ioStreams cmdutil.IOStreams, chartName, namespace, releaseName string) error {
	if !IsHelmReleaseRunning(releaseName, chartName, namespace, ioStreams) {
		return nil
	}
	uninstall, err := NewHelmUninstall(namespace)
	if err != nil {
		return err
	}
	_, err = uninstall.Run(releaseName)
	if err != nil {
		return err
	}
	ioStreams.Infof("Successfully removed chart (%s) with release name (%s)\n", chartName, releaseName)
	return nil
}

// NewHelmInstall will create a install client for helm install
func NewHelmInstall(version, namespace, releaseName string) (*action.Install, error) {
	actionConfig := new(action.Configuration)
	if len(namespace) == 0 {
		namespace = types.DefaultKubeVelaNS
	}
	if err := actionConfig.Init(
		cmdutil.NewRestConfigGetter(namespace),
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
		client.Version = types.DefaultKubeVelaVersion
	}
	return client, nil
}

// NewHelmUninstall will create a helm uninstall client
func NewHelmUninstall(namespace string) (*action.Uninstall, error) {
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(
		cmdutil.NewRestConfigGetter(namespace),
		namespace,
		os.Getenv("HELM_DRIVER"),
		debug,
	); err != nil {
		return nil, err
	}
	return action.NewUninstall(actionConfig), nil
}

// IsHelmRepositoryExist will check help repo exists
func IsHelmRepositoryExist(name, url string) bool {
	repos := GetHelmRepositoryList()
	for _, repo := range repos {
		if repo.Name == name && repo.URL == url {
			return true
		}
	}
	return false
}

// GetHelmRepositoryList get the helm repo list from default setting
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

// AddHelmRepository add helm repo
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

// IsHelmReleaseRunning check helm release running
func IsHelmReleaseRunning(releaseName, chartName, ns string, streams cmdutil.IOStreams) bool {
	releases, err := GetHelmRelease(ns)
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

// GetHelmRelease will get helm release
func GetHelmRelease(ns string) ([]*release.Release, error) {
	actionConfig := new(action.Configuration)
	client := action.NewList(actionConfig)

	if err := actionConfig.Init(settings.RESTClientGetter(), ns, os.Getenv("HELM_DRIVER"), debug); err != nil {
		return nil, err
	}
	results, err := client.Run()
	if err != nil {
		return nil, err
	}

	return results, nil
}

// GetChart will locate chart
func GetChart(client *action.Install, name string) (*chart.Chart, error) {
	if os.Getenv(VelaDebugLog) != "" {
		settings.Debug = true
	}

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

// InstallHelmChart will install helm chart from types.Chart
func InstallHelmChart(ioStreams cmdutil.IOStreams, c types.Chart) error {
	return Install(ioStreams, c.Repo, c.URL, c.Name, c.Version, c.Namespace, c.Name, c.Values)
}

// SetHTTPOption will read username and password from secret return a httpOption that contain these info.
func SetHTTPOption(ctx context.Context, k8sClient client.Client, secretRef types2.NamespacedName) (*common.HTTPOption, error) {
	sec := v1.Secret{}
	err := k8sClient.Get(ctx, secretRef, &sec)
	if err != nil {
		return nil, err
	}
	opts := &common.HTTPOption{}
	if len(sec.Data[userNameSecKey]) != 0 && len(sec.Data[userPasswordSecKey]) != 0 {
		opts.Username = string(sec.Data[userNameSecKey])
		opts.Password = string(sec.Data[userPasswordSecKey])
	}
	if len(sec.Data[caFileSecKey]) != 0 {
		opts.CaFile = string(sec.Data[caFileSecKey])
	}
	if len(sec.Data[certFileKey]) != 0 {
		opts.CertFile = string(sec.Data[certFileKey])
	}
	if len(sec.Data[keyFileKey]) != 0 {
		opts.KeyFile = string(sec.Data[keyFileKey])
	}
	return opts, nil
}
