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

package applicationcontext

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	core_oam_dev "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test ApplicationContext Controller", func() {
	ctx := context.Background()

	It("Applying ApplicationContext", func() {
		Context("appContext not found", func() {
			By("reconciling")
			req := reconcile.Request{NamespacedName: client.ObjectKey{Name: "not-existed-appContext", Namespace: "ns1"}}
			testutil.ReconcileRetry(&r, req)
		})

		Context("appContext rollingStatus is completed", func() {
			By("apply an ApplicationContext")
			var (
				appContextName  = "app1"
				appRevisionName = "app1-v1"
				componentName   = "comp1"
				ns              = "default"
				appContext      = v1alpha2.ApplicationContext{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "core.oam.dev/v1alpha2",
						Kind:       "ApplicationContext",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      appContextName,
						Namespace: ns,
					},
					Spec: v1alpha2.ApplicationContextSpec{
						ApplicationRevisionName: appRevisionName,
					},
				}
				req = reconcile.Request{NamespacedName: client.ObjectKey{Name: appContextName, Namespace: ns}}
			)
			Expect(k8sClient.Create(ctx, &appContext)).Should(Succeed())
			testutil.ReconcileRetry(&r, req)

			By("check the ApplicationContext")
			var got v1alpha2.ApplicationContext
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: appContextName}, &got)).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			By("create an component")
			workload := v1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      componentName,
					Namespace: ns,
				},
				Spec: v1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.oam.dev/component": componentName}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app.oam.dev/component": componentName}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Image: "nginx", Name: componentName}}},
					},
				},
			}

			comp := v1alpha2.Component{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Component",
					APIVersion: "core.oam.dev/v1alpha2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      componentName,
					Namespace: ns,
				},
				Spec: v1alpha2.ComponentSpec{
					Workload: util.Object2RawExtension(workload),
				},
			}
			Expect(k8sClient.Create(ctx, &comp)).Should(Succeed())

			By("create an application revision")
			app := v1alpha2.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "core.oam.dev/v1alpha2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      appContextName,
					Namespace: ns,
				},
				Spec: v1alpha2.ApplicationSpec{
					Components: []v1alpha2.ApplicationComponent{
						{
							Name:         componentName,
							WorkloadType: "worker",
							Settings:     runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","config":"myconfig"}`)},
						},
					},
				},
			}

			appConfig := v1alpha2.ApplicationConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "core.oam.dev/v1alpha2",
					Kind:       "ApplicationConfiguration",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      appContextName,
					Namespace: ns,
				},
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{{ComponentName: componentName}}},
			}

			appRevision := v1alpha2.ApplicationRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appRevisionName,
					Namespace: ns,
				},
				Spec: v1alpha2.ApplicationRevisionSpec{
					Application:              app,
					ApplicationConfiguration: util.Object2RawExtension(appConfig),
				},
			}
			Expect(k8sClient.Create(ctx, &appRevision)).Should(Succeed())
			testutil.ReconcileRetry(&r, req)
		})
	})

	It("Testing Setup", func() {
		Expect(Setup(mgr, core_oam_dev.Args{})).Should(BeNil())
	})
})
