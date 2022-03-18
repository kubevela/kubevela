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

	"helm.sh/helm/v3/pkg/repo"

	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
)

// NewHelmUsecase return a helmHandler
func NewHelmUsecase() HelmHandler {
	return defaultHelmHandler{
		helper: helm.NewHelperWithCache(),
	}
}

// HelmHandler responsible handle helm related interface
type HelmHandler interface {
	ListChartNames(ctx context.Context, url string, skipCache bool) ([]string, error)
	ListChartVersions(ctx context.Context, url string, chartName string, skipCache bool) (repo.ChartVersions, error)
	GetChartValues(ctx context.Context, url string, chartName string, version string, skipCache bool) (map[string]interface{}, error)
}

type defaultHelmHandler struct {
	helper *helm.Helper
}

func (d defaultHelmHandler) ListChartNames(ctx context.Context, url string, skipCache bool) ([]string, error) {
	charts, err := d.helper.ListChartsFromRepo(url, skipCache)
	if err != nil {
		return nil, bcode.ErrListHelmChart
	}
	return charts, nil
}

func (d defaultHelmHandler) ListChartVersions(ctx context.Context, url string, chartName string, skipCache bool) (repo.ChartVersions, error) {
	chartVersions, err := d.helper.ListVersions(url, chartName, skipCache)
	if err != nil {
		return nil, bcode.ErrListHelmVersions
	}
	if len(chartVersions) == 0 {
		return nil, bcode.ErrChartNotExist
	}
	return chartVersions, nil
}

func (d defaultHelmHandler) GetChartValues(ctx context.Context, url string, chartName string, version string, skipCache bool) (map[string]interface{}, error) {
	v, err := d.helper.GetValuesFromChart(url, chartName, version, skipCache)
	if err != nil {
		return nil, bcode.ErrGetChartValues
	}
	res := make(map[string]interface{}, len(v))
	flattenKey("", v, res)
	return res, nil
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
