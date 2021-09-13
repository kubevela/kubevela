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
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	commonutil "github.com/oam-dev/kubevela/pkg/utils/common"
)

var (
	// DefaultIntervalDuration is the interval that flux controller reconcile HelmRelease and HelmRepository
	DefaultIntervalDuration = &metav1.Duration{Duration: 5 * time.Minute}
)

// ConstructHelmReleaseName will format helm release name in a fixed way
func ConstructHelmReleaseName(appName, compName string) string {
	return appName + "-" + compName
}

// RenderHelmReleaseAndHelmRepo constructs HelmRelease and HelmRepository in unstructured format
func RenderHelmReleaseAndHelmRepo(helmSpec *common.Helm, compName, appName, ns string, values map[string]interface{}) (*unstructured.Unstructured, *unstructured.Unstructured, error) {
	releaseSpec, repoSpec, err := decodeHelmSpec(helmSpec)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "Helm spec is invalid")
	}
	if releaseSpec.Interval == nil {
		releaseSpec.Interval = DefaultIntervalDuration
	}
	if repoSpec.Interval == nil {
		repoSpec.Interval = DefaultIntervalDuration
	}

	// construct unstructured HelmRepository object
	repoName := fmt.Sprintf("%s-%s", appName, compName)
	helmRepo := commonutil.GenerateUnstructuredObj(repoName, ns, helmapi.HelmRepositoryGVK)
	if err := commonutil.SetSpecObjIntoUnstructuredObj(repoSpec, helmRepo); err != nil {
		return nil, nil, errors.Wrap(err, "cannot set spec to HelmRepository")
	}

	// construct unstructured HelmRelease object
	rlsName := ConstructHelmReleaseName(appName, compName)
	helmRelease := commonutil.GenerateUnstructuredObj(rlsName, ns, helmapi.HelmReleaseGVK)

	// construct HelmRelease chart values
	chartValues := map[string]interface{}{}
	if releaseSpec.Values != nil {
		if err := json.Unmarshal(releaseSpec.Values.Raw, &chartValues); err != nil {
			return nil, nil, errors.Wrap(err, "cannot get chart values")
		}
	}
	for k, v := range values {
		// override values with settings from application
		chartValues[k] = v
	}
	if len(chartValues) > 0 {
		// avoid an empty map
		vJSON, err := json.Marshal(chartValues)
		if err != nil {
			return nil, nil, errors.Wrap(err, "cannot get chart values")
		}
		releaseSpec.Values = &apiextensionsv1.JSON{Raw: vJSON}
	}

	// reference HelmRepository by HelmRelease
	releaseSpec.Chart.Spec.SourceRef = helmapi.CrossNamespaceObjectReference{
		Kind:      helmapi.HelmRepositoryKind,
		Namespace: ns,
		Name:      repoName,
	}
	if err := commonutil.SetSpecObjIntoUnstructuredObj(releaseSpec, helmRelease); err != nil {
		return nil, nil, errors.Wrap(err, "cannot set spec to HelmRelease")
	}

	return helmRelease, helmRepo, nil
}

func decodeHelmSpec(h *common.Helm) (*helmapi.HelmReleaseSpec, *helmapi.HelmRepositorySpec, error) {
	releaseSpec := &helmapi.HelmReleaseSpec{}
	if err := json.Unmarshal(h.Release.Raw, releaseSpec); err != nil {
		return nil, nil, errors.Wrap(err, "Helm release spec is invalid")
	}
	repoSpec := &helmapi.HelmRepositorySpec{}
	if err := json.Unmarshal(h.Repository.Raw, repoSpec); err != nil {
		return nil, nil, errors.Wrap(err, "Helm repository spec is invalid")
	}
	return releaseSpec, repoSpec, nil
}
