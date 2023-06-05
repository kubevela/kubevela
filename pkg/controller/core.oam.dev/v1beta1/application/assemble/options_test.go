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

package assemble

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
)

var _ = Describe("Test WorkloadOption", func() {
	var (
		appRev *v1beta1.ApplicationRevision
	)

	BeforeEach(func() {
		appRev = &v1beta1.ApplicationRevision{}
		b, err := os.ReadFile("./testdata/apprevision.yaml")
		Expect(err).Should(BeNil())
		err = yaml.Unmarshal(b, appRev)
		Expect(err).Should(BeNil())
	})

	Describe("test DiscoveryHelmBasedWorkload", func() {
		ns := "test-ns"
		releaseName := "test-rls"
		chartName := "test-chart"
		release := &unstructured.Unstructured{}
		release.SetGroupVersionKind(helmapi.HelmReleaseGVK)
		release.SetName(releaseName)
		unstructured.SetNestedMap(release.Object, map[string]interface{}{
			"chart": map[string]interface{}{
				"spec": map[string]interface{}{
					"chart":   chartName,
					"version": "1.0.0",
				},
			},
		}, "spec")

		rlsWithoutChart := release.DeepCopy()
		unstructured.SetNestedMap(rlsWithoutChart.Object, nil, "spec", "chart")

		wl := &unstructured.Unstructured{}
		wl.SetAPIVersion("apps/v1")
		wl.SetKind("Deployment")
		wl.SetLabels(map[string]string{
			"app.kubernetes.io/managed-by": "Helm",
		})
		wl.SetAnnotations(map[string]string{
			"meta.helm.sh/release-name":      releaseName,
			"meta.helm.sh/release-namespace": ns,
		})
		type SubCase struct {
			reason         string
			c              client.Reader
			helm           *unstructured.Unstructured
			workloadInComp *unstructured.Unstructured
			wantWorkload   *unstructured.Unstructured
			wantErr        error
		}

		DescribeTable("Test cases for DiscoveryHelmBasedWorkload", func(tc SubCase) {
			By(tc.reason)
			assembledWorkload := &unstructured.Unstructured{}
			assembledWorkload.SetAPIVersion("apps/v1")
			assembledWorkload.SetKind("Deployment")
			assembledWorkload.SetNamespace(ns)
			err := discoverHelmModuleWorkload(context.Background(), tc.c, assembledWorkload, nil, []*unstructured.Unstructured{tc.helm})

			By("Verify error")
			diff := cmp.Diff(tc.wantErr, err, test.EquateErrors())
			Expect(diff).Should(BeEmpty())

			if tc.wantErr == nil {
				By("Verify found workload")
				diff = cmp.Diff(tc.wantWorkload, assembledWorkload)
				Expect(diff).Should(BeEmpty())
			}
		},
			Entry("CannotGetReleaseFromComp", SubCase{
				reason:  "An error should occur because cannot get release",
				helm:    &unstructured.Unstructured{},
				wantErr: errors.New("cannot get helm release"),
			}),
			Entry("CannotGetChartFromRelease", SubCase{
				reason:  "An error should occur because cannot get chart info",
				helm:    rlsWithoutChart.DeepCopy(),
				wantErr: errors.New("cannot get helm chart name"),
			}),
			Entry("CannotGetWorkload", SubCase{
				reason:         "An error should occur because cannot get workload from k8s cluster",
				helm:           release.DeepCopy(),
				workloadInComp: &unstructured.Unstructured{},
				c:              &test.MockClient{MockGet: test.NewMockGetFn(errors.New("boom"))},
				wantErr:        errors.New("boom"),
			}),
			Entry("GetNotMatchedWorkload", SubCase{
				reason:         "An error should occur because the found workload is not managed by Helm",
				helm:           release.DeepCopy(),
				workloadInComp: &unstructured.Unstructured{},
				c: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					o, _ := obj.(*unstructured.Unstructured)
					*o = unstructured.Unstructured{}
					o.SetLabels(map[string]string{
						"app.kubernetes.io/managed-by": "non-helm",
					})
					return nil
				})},
				wantErr: fmt.Errorf("the workload is found but not match with helm info(meta.helm.sh/release-name: %s,"+
					" meta.helm.sh/namespace: %s, app.kubernetes.io/managed-by: Helm)", "test-rls", "test-ns"),
			}),
			Entry("DiscoverSuccessfully", SubCase{
				reason: "No error should occur and the workload should be found",
				c: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					o, _ := obj.(*unstructured.Unstructured)
					*o = *wl.DeepCopy()
					return nil
				})},
				workloadInComp: wl.DeepCopy(),
				helm:           release.DeepCopy(),
				wantWorkload: func() *unstructured.Unstructured {
					r := &unstructured.Unstructured{}
					r.SetAPIVersion("apps/v1")
					r.SetKind("Deployment")
					r.SetNamespace(ns)
					r.SetName("test-rls-test-chart")
					return r
				}(),
				wantErr: nil,
			}),
		)
	})
})
