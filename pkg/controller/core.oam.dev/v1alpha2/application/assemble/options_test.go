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
	"io/ioutil"
	"reflect"

	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/openkruise/kruise-api/apps/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test WorkloadOption", func() {
	var (
		compName    = "test-comp"
		compRevName = "test-comp-v1"

		appRev *v1beta1.ApplicationRevision
	)

	BeforeEach(func() {
		appRev = &v1beta1.ApplicationRevision{}
		b, err := ioutil.ReadFile("./testdata/apprevision.yaml")
		Expect(err).Should(BeNil())
		err = yaml.Unmarshal(b, appRev)
		Expect(err).Should(BeNil())
	})

	It("test NameNonInplaceUpgradableWorkload WorkloadOption", func() {
		By("Add NameNonInplaceUpgradableWorkload workload option")
		ao := NewAppManifests(appRev).WithWorkloadOption(NameNonInplaceUpgradableWorkload())
		workloads, _, _, err := ao.GroupAssembledManifests()
		Expect(err).Should(BeNil())
		Expect(len(workloads)).Should(Equal(1))

		By("Verify workload name is set as component revision name")
		wl := workloads[compName]
		Expect(wl.GetName()).Should(Equal(compRevName))
	})

	Context("test PrepareWorkloadForRollout WorkloadOption", func() {
		It("test rollout OpenKruise CloneSet", func() {
			By("Use openkruise CloneSet as workload")
			cs := v1alpha1.CloneSet{}
			cs.SetGroupVersionKind(v1alpha1.SchemeGroupVersion.WithKind(reflect.TypeOf(v1alpha1.CloneSet{}).Name()))
			comp := v1alpha2.Component{}
			comp.SetName(compName)
			comp.Spec.Workload = util.Object2RawExtension(cs)
			Expect(len(appRev.Spec.Components) > 0).Should(BeTrue())
			appRev.Spec.Components[0] = common.RawComponent{
				Raw: util.Object2RawExtension(comp),
			}

			By("Add PrepareWorkloadForRollout WorkloadOption")
			ao := NewAppManifests(appRev).WithWorkloadOption(PrepareWorkloadForRollout())
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
			sts := v1alpha1.StatefulSet{}
			sts.SetGroupVersionKind(v1alpha1.SchemeGroupVersion.WithKind(reflect.TypeOf(v1alpha1.StatefulSet{}).Name()))
			comp := v1alpha2.Component{}
			comp.SetName(compName)
			comp.Spec.Workload = util.Object2RawExtension(sts)
			Expect(len(appRev.Spec.Components) > 0).Should(BeTrue())
			appRev.Spec.Components[0] = common.RawComponent{
				Raw: util.Object2RawExtension(comp),
			}

			By("Add PrepareWorkloadForRollout WorkloadOption")
			ao := NewAppManifests(appRev).WithWorkloadOption(PrepareWorkloadForRollout())
			workloads, _, _, err := ao.GroupAssembledManifests()
			Expect(err).Should(BeNil())
			Expect(len(workloads)).Should(Equal(1))

			By("Verify workload name is set as component name")
			wl := workloads[compName]
			Expect(wl.GetName()).Should(Equal(compName))
			By("Verify workload is paused")
			assembledCS := &v1alpha1.StatefulSet{}
			runtime.DefaultUnstructuredConverter.FromUnstructured(wl.Object, assembledCS)
			Expect(assembledCS.Spec.UpdateStrategy.RollingUpdate.Paused).Should(BeTrue())
		})

		It("test rollout Deployment", func() {
			By("Add PrepareWorkloadForRollout WorkloadOption")
			ao := NewAppManifests(appRev).WithWorkloadOption(PrepareWorkloadForRollout())
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
		releaseRaw, _ := release.MarshalJSON()

		rlsWithoutChart := release.DeepCopy()
		unstructured.SetNestedMap(rlsWithoutChart.Object, nil, "spec", "chart")
		rlsWithoutChartRaw, _ := rlsWithoutChart.MarshalJSON()

		wl := &unstructured.Unstructured{}
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
			helm           *common.Helm
			workloadInComp *unstructured.Unstructured
			wantWorkload   *unstructured.Unstructured
			wantErr        error
		}

		DescribeTable("Test cases for DiscoveryHelmBasedWorkload", func(tc SubCase) {
			By(tc.reason)
			comp := &v1alpha2.Component{}
			if tc.workloadInComp != nil {
				wlRaw, _ := tc.workloadInComp.MarshalJSON()
				comp.Spec.Workload = runtime.RawExtension{Raw: wlRaw}
			}
			comp.Spec.Helm = tc.helm
			assembledWorkload := &unstructured.Unstructured{}
			assembledWorkload.SetNamespace(ns)
			err := discoverHelmModuleWorkload(context.Background(), tc.c, assembledWorkload, comp)

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
				reason: "An error should occur because cannot get release",
				helm: &common.Helm{
					Release: runtime.RawExtension{Raw: []byte("boom")},
				},
				wantErr: errors.Wrap(errors.New("invalid character 'b' looking for beginning of value"),
					"cannot get helm release from component"),
			}),
			Entry("CannotGetChartFromRelease", SubCase{
				reason: "An error should occur because cannot get chart info",
				helm: &common.Helm{
					Release: runtime.RawExtension{Raw: rlsWithoutChartRaw},
				},
				wantErr: errors.New("cannot get helm chart name"),
			}),
			Entry("CannotGetWorkload", SubCase{
				reason: "An error should occur because cannot get workload from k8s cluster",
				helm: &common.Helm{
					Release: runtime.RawExtension{Raw: releaseRaw},
				},
				workloadInComp: &unstructured.Unstructured{},
				c:              &test.MockClient{MockGet: test.NewMockGetFn(errors.New("boom"))},
				wantErr:        errors.New("boom"),
			}),
			Entry("GetNotMatchedWorkload", SubCase{
				reason: "An error should occur because the found workload is not managed by Helm",
				helm: &common.Helm{
					Release: runtime.RawExtension{Raw: releaseRaw},
				},
				workloadInComp: &unstructured.Unstructured{},
				c: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
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
				c: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
					o, _ := obj.(*unstructured.Unstructured)
					*o = *wl.DeepCopy()
					return nil
				})},
				workloadInComp: wl.DeepCopy(),
				helm: &common.Helm{
					Release: runtime.RawExtension{Raw: releaseRaw},
				},
				wantWorkload: func() *unstructured.Unstructured {
					r := &unstructured.Unstructured{}
					r.SetNamespace(ns)
					r.SetName("test-rls-test-chart")
					return r
				}(),
				wantErr: nil,
			}),
		)
	})
})
