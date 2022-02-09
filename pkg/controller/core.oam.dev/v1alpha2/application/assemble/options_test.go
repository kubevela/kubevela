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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/openkruise/kruise-api/apps/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/dependency/kruiseapi"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Test WorkloadOption", func() {
	var (
		compName = "test-comp"
		appRev   *v1beta1.ApplicationRevision
	)

	BeforeEach(func() {
		appRev = &v1beta1.ApplicationRevision{}
		b, err := os.ReadFile("./testdata/apprevision.yaml")
		Expect(err).Should(BeNil())
		err = yaml.Unmarshal(b, appRev)
		Expect(err).Should(BeNil())
	})

	Context("test PrepareWorkloadForRollout WorkloadOption", func() {
		It("test rollout OpenKruise CloneSet", func() {
			By("Use openkruise CloneSet as workload")
			cs := &unstructured.Unstructured{}
			cs.SetGroupVersionKind(v1alpha1.SchemeGroupVersion.WithKind(kruiseapi.CloneSet))
			cs.SetLabels(map[string]string{oam.LabelAppComponent: compName})
			comp := types.ComponentManifest{
				Name:             compName,
				StandardWorkload: cs,
			}
			By("Add PrepareWorkloadForRollout WorkloadOption")
			ao := NewAppManifests(appRev, appParser).WithWorkloadOption(PrepareWorkloadForRollout(compName))
			ao.componentManifests = []*types.ComponentManifest{&comp}
			workloads, _, _, err := ao.GroupAssembledManifests()
			Expect(err).Should(BeNil())
			Expect(len(workloads)).Should(Equal(1))

			By("Verify workload name is set as component name")
			wl := workloads[compName]
			Expect(wl.GetName()).Should(Equal(compName))
			By("Verify workload is paused")
			assembledCS := &v1alpha1.CloneSet{}
			runtime.DefaultUnstructuredConverter.FromUnstructured(wl.Object, assembledCS)
			Expect(assembledCS.Spec.UpdateStrategy.Paused).Should(BeTrue())
		})

		It("test rollout OpenKruise StatefulSet", func() {
			By("Use openkruise CloneSet as workload")
			sts := &unstructured.Unstructured{}
			sts.SetGroupVersionKind(v1alpha1.SchemeGroupVersion.WithKind(kruiseapi.StatefulSet))
			sts.SetLabels(map[string]string{oam.LabelAppComponent: compName})
			comp := types.ComponentManifest{
				Name:             compName,
				StandardWorkload: sts,
			}
			By("Add PrepareWorkloadForRollout WorkloadOption")
			ao := NewAppManifests(appRev, appParser).WithWorkloadOption(PrepareWorkloadForRollout(compName))
			ao.componentManifests = []*types.ComponentManifest{&comp}
			workloads, _, _, err := ao.GroupAssembledManifests()
			Expect(err).Should(BeNil())
			Expect(len(workloads)).Should(Equal(1))

			By("Verify workload name is set as component name")
			wl := workloads[compName]
			Expect(wl.GetName()).Should(Equal(compName))
			By("Verify workload is paused")
			assembledCS := &v1alpha1.StatefulSet{}
			runtime.DefaultUnstructuredConverter.FromUnstructured(wl.Object, assembledCS)
			fmt.Println(assembledCS.Spec.UpdateStrategy)
			Expect(assembledCS.Spec.UpdateStrategy.RollingUpdate.Paused).Should(BeTrue())
		})

		It("test rollout Deployment", func() {
			By("Add PrepareWorkloadForRollout WorkloadOption")
			ao := NewAppManifests(appRev, appParser).WithWorkloadOption(PrepareWorkloadForRollout(compName))
			workloads, _, _, err := ao.GroupAssembledManifests()
			Expect(err).Should(BeNil())
			Expect(len(workloads)).Should(Equal(1))

			By("Verify workload name is set as component name")
			wl := workloads[compName]
			Expect(wl.GetName()).Should(Equal(compName))
			By("Verify workload is paused")
			assembledDeploy := &appsv1.Deployment{}
			runtime.DefaultUnstructuredConverter.FromUnstructured(wl.Object, assembledDeploy)
			Expect(assembledDeploy.Spec.Paused).Should(BeTrue())
		})

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
