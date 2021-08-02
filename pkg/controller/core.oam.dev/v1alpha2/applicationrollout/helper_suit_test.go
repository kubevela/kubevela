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

package applicationrollout

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/pkg/oam"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
)

var _ = Describe("TestHandleReplicasFunc", func() {
	ctx := context.Background()

	It("Test exsit workload", func() {
		ns := v1.Namespace{}
		ns.SetName("test-namespace")
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		workloadName := "test-workload"
		workload := new(appsv1.Deployment)
		workload.SetName(workloadName)
		workload.SetNamespace("test-namespace")
		workload.SetLabels(map[string]string{
			oam.LabelAppComponent: workloadName,
		})
		workload.Spec = appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(5),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"test": "test",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"test": "test",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "test",
							Image: "nginx",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, workload)).Should(BeNil())
		checkworkload := unstructured.Unstructured{}
		checkworkload.SetAPIVersion("apps/v1")
		checkworkload.SetKind("Deployment")
		checkworkload.SetNamespace("test-namespace")
		checkworkload.SetName(workloadName)
		checkworkload.SetLabels(map[string]string{
			oam.LabelAppComponent: workloadName,
		})
		Eventually(func() error {
			handleFunc := HandleReplicas(ctx, workloadName, k8sClient)
			if err := handleFunc.ApplyToWorkload(&checkworkload, nil, nil); err != nil {
				return err
			}
			replicasFieldPath := "spec.replicas"
			wlpv := fieldpath.Pave(checkworkload.UnstructuredContent())
			replicas, err := wlpv.GetInteger(replicasFieldPath)
			if err != nil {
				return err
			}
			if replicas != 5 {
				return fmt.Errorf("replicas number not 5")
			}
			return nil
		}, 10*time.Second, 500*time.Millisecond).Should(BeNil())
	})

	It("Test not exist workload", func() {
		workloadName := "not-exist-workload"
		checkworkload := unstructured.Unstructured{}
		checkworkload.SetAPIVersion("apps/v1")
		checkworkload.SetKind("Deployment")
		checkworkload.SetNamespace("test-namespace")
		checkworkload.SetName(workloadName)
		checkworkload.SetLabels(map[string]string{
			oam.LabelAppComponent: workloadName,
		})
		Eventually(func() error {
			handleFunc := HandleReplicas(ctx, workloadName, k8sClient)
			if err := handleFunc.ApplyToWorkload(&checkworkload, nil, nil); err != nil {
				return err
			}
			replicasFieldPath := "spec.replicas"
			wlpv := fieldpath.Pave(checkworkload.UnstructuredContent())
			replicas, err := wlpv.GetInteger(replicasFieldPath)
			if err != nil {
				return err
			}
			if replicas != 0 {
				return fmt.Errorf("replicas number not 0")
			}
			return nil
		}, 10*time.Second, 500*time.Millisecond).Should(BeNil())
	})
})
