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
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	appsv1 "k8s.io/api/apps/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	k8scmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/yaml"

	utils2 "github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	repoPatten   = " repoUrl: %s"
	valuesPatten = "repoUrl: %s, chart: %s, version: %s"
)

// Helper provides helper functions for common Helm operations
type Helper struct {
	cache *utils2.MemoryCacheStore
}

// NewHelper creates a Helper
func NewHelper() *Helper {
	return &Helper{}
}

// NewHelperWithCache creates a Helper with cache usually used by apiserver
func NewHelperWithCache() *Helper {
	return &Helper{
		cache: utils2.NewMemoryCacheStore(context.Background()),
	}
}

// LoadCharts load helm chart from local or remote
func (h *Helper) LoadCharts(chartRepoURL string, opts *common.HTTPOption) (*chart.Chart, error) {
	var err error
	var chart *chart.Chart
	if utils.IsValidURL(chartRepoURL) {
		chartBytes, err := common.HTTPGetWithOption(context.Background(), chartRepoURL, opts)
		if err != nil {
			return nil, errors.New("error retrieving Helm Chart at " + chartRepoURL + ": " + err.Error())
		}
		ch, err := loader.LoadArchive(bytes.NewReader(chartBytes))
		if err != nil {
			return nil, errors.New("error retrieving Helm Chart at " + chartRepoURL + ": " + err.Error())
		}
		return ch, err
	}
	chart, err = loader.Load(chartRepoURL)
	if err != nil {
		return nil, err
	}
	return chart, nil
}

// UpgradeChartOptions options for upgrade chart
type UpgradeChartOptions struct {
	Config      *rest.Config
	Detail      bool
	Logging     cmdutil.IOStreams
	Wait        bool
	ReuseValues bool
}

// UpgradeChart install or upgrade helm chart
func (h *Helper) UpgradeChart(ch *chart.Chart, releaseName, namespace string, values map[string]interface{}, config UpgradeChartOptions) (*release.Release, error) {
	if ch == nil || len(ch.Templates) == 0 {
		return nil, fmt.Errorf("empty chart provided for %s", releaseName)
	}
	config.Logging.Infof("Start upgrading Helm Chart %s in namespace %s\n", releaseName, namespace)

	cfg, err := newActionConfig(config.Config, namespace, config.Detail, config.Logging)
	if err != nil {
		return nil, err
	}
	histClient := action.NewHistory(cfg)
	var newRelease *release.Release
	timeoutInMinutes := 18
	releases, err := histClient.Run(releaseName)
	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			// fresh install
			install := action.NewInstall(cfg)
			install.Namespace = namespace
			install.ReleaseName = releaseName
			install.Wait = config.Wait
			install.Timeout = time.Duration(timeoutInMinutes) * time.Minute
			newRelease, err = install.Run(ch, values)
		} else {
			return nil, fmt.Errorf("could not retrieve history of releases associated to %s: %w", releaseName, err)
		}
	} else {
		config.Logging.Infof("Found existing installation, overwriting...")

		// check if the previous installation is still pending (e.g., waiting to complete)
		for _, r := range releases {
			if r.Info.Status == release.StatusPendingInstall || r.Info.Status == release.StatusPendingUpgrade ||
				r.Info.Status == release.StatusPendingRollback {
				return nil, fmt.Errorf("previous installation (e.g., using vela install or helm upgrade) is still in progress. Please try again in %d minutes", timeoutInMinutes)
			}
		}

		// overwrite existing installation
		install := action.NewUpgrade(cfg)
		install.Namespace = namespace
		install.Wait = config.Wait
		install.Timeout = time.Duration(timeoutInMinutes) * time.Minute
		install.ReuseValues = config.ReuseValues
		newRelease, err = install.Run(releaseName, ch, values)
	}
	// check if install/upgrade worked
	if err != nil {
		return nil, fmt.Errorf("error when installing/upgrading Helm Chart %s in namespace %s: %w",
			releaseName, namespace, err)
	}
	if newRelease == nil {
		return nil, fmt.Errorf("failed to install release %s", releaseName)
	}
	return newRelease, nil
}

// UninstallRelease uninstalls the provided release
func (h *Helper) UninstallRelease(releaseName, namespace string, config *rest.Config, showDetail bool, logging cmdutil.IOStreams) error {
	cfg, err := newActionConfig(config, namespace, showDetail, logging)
	if err != nil {
		return err
	}

	iCli := action.NewUninstall(cfg)
	_, err = iCli.Run(releaseName)

	if err != nil {
		return fmt.Errorf("error when uninstalling Helm release %s in namespace %s: %w",
			releaseName, namespace, err)
	}
	return nil
}

// ListVersions list available versions from repo
func (h *Helper) ListVersions(repoURL string, chartName string, skipCache bool, opts *common.HTTPOption) (repo.ChartVersions, error) {
	i, err := h.GetIndexInfo(repoURL, skipCache, opts)
	if err != nil {
		return nil, err
	}
	return i.Entries[chartName], nil
}

// GetIndexInfo get index.yaml form given repo url
func (h *Helper) GetIndexInfo(repoURL string, skipCache bool, opts *common.HTTPOption) (*repo.IndexFile, error) {
	if h.cache != nil && !skipCache {
		if i := h.cache.Get(fmt.Sprintf(repoPatten, repoURL)); i != nil {
			return i.(*repo.IndexFile), nil
		}
	}
	var body []byte
	if utils.IsValidURL(repoURL) {
		parsedURL, err := url.Parse(repoURL)
		if err != nil {
			return nil, err
		}
		parsedURL.RawPath = path.Join(parsedURL.RawPath, "index.yaml")
		parsedURL.Path = path.Join(parsedURL.Path, "index.yaml")
		indexURL := parsedURL.String()
		body, err = common.HTTPGetWithOption(context.Background(), indexURL, opts)
		if err != nil {
			return nil, fmt.Errorf("download index file from %s failure %w", repoURL, err)
		}
	} else {
		var err error
		body, err = ioutil.ReadFile(path.Join(filepath.Clean(repoURL), "index.yaml"))
		if err != nil {
			return nil, fmt.Errorf("read index file from %s failure %w", repoURL, err)
		}
	}
	i := &repo.IndexFile{}
	if err := yaml.UnmarshalStrict(body, i); err != nil {
		return nil, fmt.Errorf("parse index file from %s failure %w", repoURL, err)
	}

	if h.cache != nil {
		h.cache.Put(fmt.Sprintf(repoPatten, repoURL), i, calculateCacheTimeFromIndex(len(i.Entries)))
	}
	return i, nil
}

// GetDeploymentsFromManifest get deployment from helm manifest
func GetDeploymentsFromManifest(helmManifest string) []*appsv1.Deployment {
	deployments := []*appsv1.Deployment{}
	dec := kyaml.NewYAMLToJSONDecoder(strings.NewReader(helmManifest))
	for {
		var deployment appsv1.Deployment
		err := dec.Decode(&deployment)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			continue
		}
		if strings.EqualFold(deployment.Kind, "deployment") {
			deployments = append(deployments, &deployment)
		}
	}
	return deployments
}

// GetCRDFromChart get crd from helm chart
func GetCRDFromChart(chart *chart.Chart) []*crdv1.CustomResourceDefinition {
	crds := []*crdv1.CustomResourceDefinition{}
	for _, crdFile := range chart.CRDs() {
		var crd crdv1.CustomResourceDefinition
		err := kyaml.Unmarshal(crdFile.Data, &crd)
		if err != nil {
			continue
		}
		crds = append(crds, &crd)
	}
	return crds
}

func newActionConfig(config *rest.Config, namespace string, showDetail bool, logging cmdutil.IOStreams) (*action.Configuration, error) {
	restClientGetter := cmdutil.NewRestConfigGetterByConfig(config, namespace)
	log := func(format string, a ...interface{}) {
		if showDetail {
			logging.Infof(format+"\n", a...)
		}
	}
	kubeClient := &kube.Client{
		Factory: k8scmdutil.NewFactory(restClientGetter),
		Log:     log,
	}
	client, err := kubeClient.Factory.KubernetesClientSet()
	if err != nil {
		return nil, err
	}
	s := driver.NewSecrets(client.CoreV1().Secrets(namespace))
	s.Log = log
	return &action.Configuration{
		RESTClientGetter: restClientGetter,
		Releases:         storage.Init(s),
		KubeClient:       kubeClient,
		Log:              log,
	}, nil
}

// ListChartsFromRepo list available helm charts in a repo
func (h *Helper) ListChartsFromRepo(repoURL string, skipCache bool, opts *common.HTTPOption) ([]string, error) {
	i, err := h.GetIndexInfo(repoURL, skipCache, opts)
	if err != nil {
		return nil, err
	}
	res := make([]string, len(i.Entries))
	j := 0
	for s := range i.Entries {
		res[j] = s
		j++
	}
	return res, nil
}

// GetValuesFromChart will extract the parameter from a helm chart
func (h *Helper) GetValuesFromChart(repoURL string, chartName string, version string, skipCache bool, opts *common.HTTPOption) (map[string]interface{}, error) {
	if h.cache != nil && !skipCache {
		if v := h.cache.Get(fmt.Sprintf(valuesPatten, repoURL, chartName, version)); v != nil {
			return v.(map[string]interface{}), nil
		}
	}
	i, err := h.GetIndexInfo(repoURL, skipCache, opts)
	if err != nil {
		return nil, err
	}
	chartVersions, ok := i.Entries[chartName]
	if !ok {
		return nil, fmt.Errorf("cannot find chart %s in this repo", chartName)
	}
	var urls []string
	for _, chartVersion := range chartVersions {
		if chartVersion.Version == version {
			urls = chartVersion.URLs
		}
	}
	for _, u := range urls {
		c, err := h.LoadCharts(u, opts)
		if err != nil {
			continue
		}
		if h.cache != nil {
			h.cache.Put(fmt.Sprintf(valuesPatten, repoURL, chartName, version), c.Values, calculateCacheTimeFromIndex(len(i.Entries)))
		}
		return c.Values, nil
	}
	return nil, fmt.Errorf("cannot load chart from chart repo")
}

func calculateCacheTimeFromIndex(length int) time.Duration {
	cacheTime := 3 * time.Minute
	if length > 20 {
		// huge helm repo like https://charts.bitnami.com/bitnami have too many(106) charts, generally user cannot modify it.
		// need more cache time
		cacheTime = 1 * time.Hour
	}
	return cacheTime
}
