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

	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"

	"github.com/oam-dev/kubevela/pkg/utils/helm"
)

const (
	versionPatten = "repoUrl: %s, chart: %s"
	valuePatten   = "repoUrl: %s, chart: %s, version: %s"
)

// NewHelmUsecase return a helmHandler
func NewHelmUsecase() HelmHandler {
	return defaultHelmHandler{
		repoCache:    map[string]*utils.MemoryCache{},
		versionCache: map[string]*utils.MemoryCache{},
		valuesCache:  map[string]*utils.MemoryCache{},
	}
}

// HelmHandler responsible handle helm related interface
type HelmHandler interface {
	ListChartNames(ctx context.Context, url string) ([]string, error)
	ListChartVersions(ctx context.Context, url string, chartName string) ([]string, error)
	GetChartValues(ctx context.Context, url string, chartName string, version string) (map[string]interface{}, error)
}

type defaultHelmHandler struct {
	repoCache    map[string]*utils.MemoryCache
	versionCache map[string]*utils.MemoryCache
	valuesCache  map[string]*utils.MemoryCache
}

func (d defaultHelmHandler) ListChartNames(ctx context.Context, url string) ([]string, error) {
	if m, ok := d.repoCache[url]; ok && !m.IsExpired() {
		return m.GetData().([]string), nil
	}
	helper := &helm.Helper{}
	charts, err := helper.ListChartsFromRepo(url)
	if err != nil {
		return nil, err
	}
	d.repoCache[url] = utils.NewMemoryCache(charts, 10*time.Minute)
	return charts, nil
}

func (d defaultHelmHandler) ListChartVersions(ctx context.Context, url string, chartName string) ([]string, error) {
	if m, ok := d.versionCache[fmt.Sprintf(versionPatten, url, chartName)]; ok && !m.IsExpired() {
		return m.GetData().([]string), nil
	}
	helper := &helm.Helper{}
	chartVersions, err := helper.ListVersions(url, chartName)
	if err != nil {
		return nil, err
	}
	res := make([]string, len(chartVersions))
	j := 0
	for _, v := range chartVersions {
		res[j] = v.Version
		j++
	}
	// helm version updating is more frequent， so set 1min expired time
	d.versionCache[fmt.Sprintf(versionPatten, url, chartName)] = utils.NewMemoryCache(res, 1*time.Minute)
	return res, nil
}

func (d defaultHelmHandler) GetChartValues(ctx context.Context, url string, chartName string, version string) (map[string]interface{}, error) {
	if m, ok := d.valuesCache[fmt.Sprintf(valuePatten, url, chartName, version)]; ok && !m.IsExpired() {
		return m.GetData().(map[string]interface{}), nil
	}
	helper := &helm.Helper{}
	v, err := helper.GetValuesFromChart(url, chartName, version)
	if err != nil {
		return nil, err
	}
	res := make(map[string]interface{}, len(v))
	flattenKey("", v, res)
	// modify charts value is very low frequency operation, so 10 min is enough
	d.valuesCache[fmt.Sprintf(valuePatten, url, chartName, version)] = utils.NewMemoryCache(res, 10*time.Minute)
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
