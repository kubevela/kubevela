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

package usecase

import (
	"context"
	"strconv"

	"github.com/oam-dev/kubevela/pkg/utils/config"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"

	corev1 "k8s.io/api/core/v1"
	types2 "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"helm.sh/helm/v3/pkg/repo"
)

// NewHelmUsecase return a helmHandler
func NewHelmUsecase() HelmHandler {
	c, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kube client failure %s", err.Error())
	}
	return defaultHelmHandler{
		helper:    helm.NewHelperWithCache(),
		k8sClient: c,
	}
}

// HelmHandler responsible handle helm related interface
type HelmHandler interface {
	ListChartNames(ctx context.Context, url string, secretName string, skipCache bool) ([]string, error)
	ListChartVersions(ctx context.Context, url string, chartName string, secretName string, skipCache bool) (repo.ChartVersions, error)
	GetChartValues(ctx context.Context, url string, chartName string, version string, secretName string, skipCache bool) (map[string]interface{}, error)
	ListChartRepo(ctx context.Context, projectName string) (*v1.ChartRepoResponseList, error)
}

type defaultHelmHandler struct {
	helper    *helm.Helper
	k8sClient client.Client
}

func (d defaultHelmHandler) ListChartNames(ctx context.Context, repoURL string, secretName string, skipCache bool) ([]string, error) {
	if !utils.IsValidURL(repoURL) {
		return nil, bcode.ErrRepoInvalidURL
	}
	var opts *common.HTTPOption
	var err error
	if len(secretName) != 0 {
		opts, err = helm.SetBasicAuthInfo(ctx, d.k8sClient, types2.NamespacedName{Namespace: types.DefaultKubeVelaNS, Name: secretName})
		if err != nil {
			return nil, bcode.ErrRepoBasicAuth
		}
	}
	charts, err := d.helper.ListChartsFromRepo(repoURL, skipCache, opts)
	if err != nil {
		log.Logger.Errorf("cannot fetch charts repo: %s, error: %s", utils.Sanitize(repoURL), err.Error())
		return nil, bcode.ErrListHelmChart
	}
	return charts, nil
}

func (d defaultHelmHandler) ListChartVersions(ctx context.Context, repoURL string, chartName string, secretName string, skipCache bool) (repo.ChartVersions, error) {
	if !utils.IsValidURL(repoURL) {
		return nil, bcode.ErrRepoInvalidURL
	}
	var opts *common.HTTPOption
	var err error
	if len(secretName) != 0 {
		opts, err = helm.SetBasicAuthInfo(ctx, d.k8sClient, types2.NamespacedName{Namespace: types.DefaultKubeVelaNS, Name: secretName})
		if err != nil {
			return nil, bcode.ErrRepoBasicAuth
		}
	}
	chartVersions, err := d.helper.ListVersions(repoURL, chartName, skipCache, opts)
	if err != nil {
		log.Logger.Errorf("cannot fetch chart versions repo: %s, chart: %s error: %s", utils.Sanitize(repoURL), utils.Sanitize(chartName), err.Error())
		return nil, bcode.ErrListHelmVersions
	}
	if len(chartVersions) == 0 {
		log.Logger.Errorf("cannot fetch chart versions repo: %s, chart: %s", utils.Sanitize(repoURL), utils.Sanitize(chartName))
		return nil, bcode.ErrChartNotExist
	}
	return chartVersions, nil
}

func (d defaultHelmHandler) GetChartValues(ctx context.Context, repoURL string, chartName string, version string, secretName string, skipCache bool) (map[string]interface{}, error) {
	if !utils.IsValidURL(repoURL) {
		return nil, bcode.ErrRepoInvalidURL
	}
	var opts *common.HTTPOption
	var err error
	if len(secretName) != 0 {
		opts, err = helm.SetBasicAuthInfo(ctx, d.k8sClient, types2.NamespacedName{Namespace: types.DefaultKubeVelaNS, Name: secretName})
		if err != nil {
			return nil, bcode.ErrRepoBasicAuth
		}
	}
	v, err := d.helper.GetValuesFromChart(repoURL, chartName, version, skipCache, opts)
	if err != nil {
		log.Logger.Errorf("cannot fetch chart values repo: %s, chart: %s, version: %s, error: %s", utils.Sanitize(repoURL), utils.Sanitize(chartName), utils.Sanitize(version), err.Error())
		return nil, bcode.ErrGetChartValues
	}
	res := make(map[string]interface{}, len(v))
	flattenKey("", v, res)
	return res, nil
}

func (d defaultHelmHandler) ListChartRepo(ctx context.Context, projectName string) (*v1.ChartRepoResponseList, error) {
	var res []*v1.ChartRepoResponse
	var err error

	projectSecrets := corev1.SecretList{}
	opts := []client.ListOption{
		client.MatchingLabels{oam.LabelConfigType: "config-helm-repository"},
		client.InNamespace(types.DefaultKubeVelaNS),
	}
	err = d.k8sClient.List(ctx, &projectSecrets, opts...)
	if err != nil {
		return nil, err
	}

	for _, item := range projectSecrets.Items {
		if config.ProjectMatched(item.DeepCopy(), projectName) {
			res = append(res, &v1.ChartRepoResponse{URL: string(item.Data["url"]), SecretName: item.Name})
		}
	}

	return &v1.ChartRepoResponseList{ChartRepoResponse: res}, nil
}

// this func will flatten a nested map, the key will flatten with separator "." and the value's type will be keep
// src is the map you want to flatten the output will be set in dest map
// eg : src is  {a:{b:{c:true}}} , the dest is {a.b.c:true}
func flattenKey(prefix string, src map[string]interface{}, dest map[string]interface{}) {
	if len(prefix) > 0 {
		prefix += "."
	}
	for k, v := range src {
		switch child := v.(type) {
		case map[string]interface{}:
			flattenKey(prefix+k, child, dest)
		case []interface{}:
			for i := 0; i < len(child); i++ {
				dest[prefix+k+"."+strconv.Itoa(i)] = child[i]
			}
		default:
			dest[prefix+k] = v
		}
	}
}
