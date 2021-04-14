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

package controllers_test

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("AppConfig renders workloads", func() {
	var (
		namespace      = "appconfig-render-test"
		cwName         = "test-cw"
		compName       = "test-component"
		wdName         = "containerizedworkloads.core.oam.dev"
		containerName  = "test-container"
		containerImage = "notarealimage"
		acName         = "test-ac"

		envVars = []string{
			"VAR_ONE",
			"VAR_TWO",
			"VAR_THREE",
		}

		paramVals = []string{
			"replace-one",
			"replace-two",
			"replace-three",
		}
	)
	ctx := context.TODO()
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	BeforeEach(func() {
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: namespace}, &corev1.Namespace{})
		}, time.Second*120, time.Second*10).Should(&util.NotFoundMatcher{})
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	It("Test AppConfig controller renders workloads", func() {
		By("Create WorkloadDefinition")
		d := wd(wdNameAndDef(wdName))
		Expect(k8sClient.Create(ctx, d)).Should(Succeed())
		workload := cw(
			cwWithName(cwName),
			cwWithContainers([]v1alpha2.Container{
				{
					Name:  containerName,
					Image: containerImage,
					Environment: []v1alpha2.ContainerEnvVar{
						{
							Name: envVars[0],
						},
						{
							Name: envVars[1],
						},
						{
							Name: envVars[2],
						},
					},
				},
			}),
		)

		rawWorkload := runtime.RawExtension{Object: workload}

		By("Create Component")
		co := comp(
			compWithName(compName),
			compWithNamespace(namespace),
			compWithWorkload(rawWorkload),
			compWithParams([]v1alpha2.ComponentParameter{
				{
					Name:       envVars[0],
					FieldPaths: []string{"spec.containers[0].env[0].value"},
				},
				{
					Name:       envVars[1],
					FieldPaths: []string{"spec.containers[0].env[1].value"},
				},
				{
					Name:       envVars[2],
					FieldPaths: []string{"spec.containers[0].env[2].value"},
				},
			}))
		Expect(k8sClient.Create(ctx, co)).Should(Succeed())
		verifyComponentCreated("AC render 0", namespace, compName)

		By("Create ApplicationConfiguration")
		ac := ac(
			acWithName(acName),
			acWithNamspace(namespace),
			acWithComps([]v1alpha2.ApplicationConfigurationComponent{
				{
					ComponentName: compName,
					ParameterValues: []v1alpha2.ComponentParameterValue{
						{
							Name:  envVars[0],
							Value: intstr.FromString(paramVals[0]),
						},
						{
							Name:  envVars[1],
							Value: intstr.FromString(paramVals[1]),
						},
						{
							Name:  envVars[2],
							Value: intstr.FromString(paramVals[2]),
						},
					},
				},
			}))
		Expect(k8sClient.Create(ctx, ac)).Should(Succeed())

		By("Verify workloads are created")
		Eventually(func() bool {
			requestReconcileNow(ctx, ac)
			cw := &v1alpha2.ContainerizedWorkload{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: cwName, Namespace: namespace}, cw); err != nil {
				return false
			}
			if len(cw.Spec.Containers) != 1 {
				return false
			}
			for i, e := range cw.Spec.Containers[0].Environment {
				if e.Name != envVars[i] {
					return false
				}
				if e.Value != nil && *e.Value != paramVals[i] {
					return false
				}
			}
			return true
		}, time.Second*10, time.Second*2).Should(BeTrue())
	})
})

type wdModifier func(*v1alpha2.WorkloadDefinition)

func wdNameAndDef(n string) wdModifier {
	return func(wd *v1alpha2.WorkloadDefinition) {
		wd.ObjectMeta.Name = n
		wd.ObjectMeta.Namespace = "appconfig-render-test"
		wd.Spec.Reference = common.DefinitionReference{
			Name: n,
		}
	}
}

func wd(m ...wdModifier) *v1alpha2.WorkloadDefinition {
	w := &v1alpha2.WorkloadDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.WorkloadDefinitionKind,
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
	}

	for _, fn := range m {
		fn(w)
	}
	return w
}

type compModifier func(*v1alpha2.Component)

func compWithName(n string) compModifier {
	return func(c *v1alpha2.Component) {
		c.Name = n
	}
}

func compWithNamespace(n string) compModifier {
	return func(c *v1alpha2.Component) {
		c.Namespace = n
	}
}

func compWithWorkload(w runtime.RawExtension) compModifier {
	return func(c *v1alpha2.Component) {
		c.Spec.Workload = w
	}
}

func compWithParams(p []v1alpha2.ComponentParameter) compModifier {
	return func(c *v1alpha2.Component) {
		c.Spec.Parameters = p
	}
}

func comp(m ...compModifier) *v1alpha2.Component {
	c := &v1alpha2.Component{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.ComponentKind,
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
	}

	for _, fn := range m {
		fn(c)
	}
	return c
}

type acModifier func(*v1alpha2.ApplicationConfiguration)

func acWithName(n string) acModifier {
	return func(a *v1alpha2.ApplicationConfiguration) {
		a.Name = n
	}
}

func acWithNamspace(n string) acModifier {
	return func(a *v1alpha2.ApplicationConfiguration) {
		a.Namespace = n
	}
}

func acWithComps(c []v1alpha2.ApplicationConfigurationComponent) acModifier {
	return func(a *v1alpha2.ApplicationConfiguration) {
		a.Spec.Components = c
	}
}

func ac(m ...acModifier) *v1alpha2.ApplicationConfiguration {
	a := &v1alpha2.ApplicationConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.ApplicationConfigurationKind,
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
	}

	for _, fn := range m {
		fn(a)
	}
	return a
}

type cwModifier func(*v1alpha2.ContainerizedWorkload)

func cwWithName(n string) cwModifier {
	return func(cw *v1alpha2.ContainerizedWorkload) {
		cw.Name = n
	}
}

func cwWithContainers(c []v1alpha2.Container) cwModifier {
	return func(cw *v1alpha2.ContainerizedWorkload) {
		cw.Spec.Containers = c
	}
}

func cw(m ...cwModifier) *v1alpha2.ContainerizedWorkload {
	cw := &v1alpha2.ContainerizedWorkload{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.ContainerizedWorkloadKind,
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
	}

	for _, fn := range m {
		fn(cw)
	}
	return cw
}
