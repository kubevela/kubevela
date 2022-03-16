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
	"fmt"
	"strconv"
	"time"

	"helm.sh/helm/v3/pkg/repo"

	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"

	"github.com/oam-dev/kubevela/pkg/utils/helm"
)

const (
	chartPatten   = "repoUrl: %s"
	versionPatten = "repoUrl: %s, chart: %s"
	valuePatten   = "repoUrl: %s, chart: %s, version: %s"
)

// NewHelmUsecase return a helmHandler
func NewHelmUsecase() HelmHandler {
	return defaultHelmHandler{
		cache: utils.NewMemoryCacheStore(context.Background()),
	}
}

// HelmHandler responsible handle helm related interface
type HelmHandler interface {
	ListChartNames(ctx context.Context, url string) ([]string, error)
	ListChartVersions(ctx context.Context, url string, chartName string) (repo.ChartVersions, error)
	GetChartValues(ctx context.Context, url string, chartName string, version string) (map[string]interface{}, error)
}

type defaultHelmHandler struct {
	cache *utils.MemoryCacheStore
}

func (d defaultHelmHandler) ListChartNames(ctx context.Context, url string) ([]string, error) {
	if m := d.cache.Get(fmt.Sprintf(chartPatten, url)); m != nil {
		return m.([]string), nil
	}
	helper := &helm.Helper{}
	charts, err := helper.ListChartsFromRepo(url)
	if err != nil {
		return nil, err
	}
	d.cache.Put(fmt.Sprintf(chartPatten, url), charts, 3*time.Minute)
	return charts, nil
}

func (d defaultHelmHandler) ListChartVersions(ctx context.Context, url string, chartName string) (repo.ChartVersions, error) {
	if m := d.cache.Get(fmt.Sprintf(versionPatten, url, chartName)); m != nil {
		return m.(repo.ChartVersions), nil
	}
	helper := &helm.Helper{}
	chartVersions, err := helper.ListVersions(url, chartName)
	if err != nil {
		return nil, err
	}
	d.cache.Put(fmt.Sprintf(versionPatten, url, chartName), chartVersions, 3*time.Minute)
	return chartVersions, nil
}

func (d defaultHelmHandler) GetChartValues(ctx context.Context, url string, chartName string, version string) (map[string]interface{}, error) {
	if m := d.cache.Get(fmt.Sprintf(valuePatten, url, chartName, version)); m != nil {
		return m.(map[string]interface{}), nil
	}
	helper := &helm.Helper{}
	v, err := helper.GetValuesFromChart(url, chartName, version)
	if err != nil {
		return nil, err
	}
	res := make(map[string]interface{}, len(v))
	flattenKey("", v, res)
	d.cache.Put(fmt.Sprintf(valuePatten, url, chartName, version), res, 3*time.Minute)
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
