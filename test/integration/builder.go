// +build integration

/*
Copyright 2020 The Crossplane Authors.

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

package integration

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

type wdModifier func(*v1alpha2.WorkloadDefinition)

func wdNameAndDef(n string) wdModifier {
	return func(wd *v1alpha2.WorkloadDefinition) {
		wd.ObjectMeta.Name = n
		wd.Spec.Reference = v1alpha2.DefinitionReference{
			Name: n,
		}
	}
}

func wd(m ...wdModifier) *v1alpha2.WorkloadDefinition {
	w := &v1alpha2.WorkloadDefinition{
		TypeMeta: v1.TypeMeta{
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
		TypeMeta: v1.TypeMeta{
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
		TypeMeta: v1.TypeMeta{
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

func cwWithNamspace(n string) cwModifier {
	return func(cw *v1alpha2.ContainerizedWorkload) {
		cw.Namespace = n
	}
}

func cwWithContainers(c []v1alpha2.Container) cwModifier {
	return func(cw *v1alpha2.ContainerizedWorkload) {
		cw.Spec.Containers = c
	}
}

func cw(m ...cwModifier) *v1alpha2.ContainerizedWorkload {
	cw := &v1alpha2.ContainerizedWorkload{
		TypeMeta: v1.TypeMeta{
			Kind:       v1alpha2.ContainerizedWorkloadKind,
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
	}

	for _, fn := range m {
		fn(cw)
	}
	return cw
}
