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

package bcode

// ErrListHelmChart is the error of cannot list helm chart from given repoURL
var ErrListHelmChart = NewBcode(200, 13001, "cannot list helm charts from given repoURL")

// ErrListHelmVersions is the error of cannot list helm chart versions given given repoUrl
var ErrListHelmVersions = NewBcode(200, 13002, "cannot list helm versions from given repoURL")

// ErrGetChartValues is the error of cannot get values of the chart
var ErrGetChartValues = NewBcode(200, 13003, "cannot get the values info of the chart")

// ErrChartNotExist is the error of the chart not exist
var ErrChartNotExist = NewBcode(200, 13004, "this chart not exist in the repository")

// ErrSkipCacheParameter means the skip cache parameter miss config
var ErrSkipCacheParameter = NewBcode(400, 13005, "skip cache parameter miss config, the value only can be true or false")

// ErrRepoBasicAuth means extract repo auth info from secret error
var ErrRepoBasicAuth = NewBcode(400, 13006, "extract repo auth info from secret error")
