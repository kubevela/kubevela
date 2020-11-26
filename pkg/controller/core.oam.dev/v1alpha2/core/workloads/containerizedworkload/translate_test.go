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

package containerizedworkload

import (
	"context"
	"errors"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/mock"
)

var (
	workloadName      = "test-workload"
	workloadNamespace = "test-namespace"
	workloadUID       = "a-very-unique-identifier"

	containerName = "test-container"
	portName      = "test-port"
)

type deploymentModifier func(*appsv1.Deployment)

func dmWithLabel(label map[string]string) deploymentModifier {
	return func(cw *appsv1.Deployment) {
		cw.Labels = label
		cw.Spec.Template.Labels = label
	}
}

func dmWithAnnotation(annotation map[string]string) deploymentModifier {
	return func(cw *appsv1.Deployment) {
		cw.Annotations = annotation
	}
}

func dmWithOS(os string) deploymentModifier {
	return func(d *appsv1.Deployment) {
		if d.Spec.Template.Spec.NodeSelector == nil {
			d.Spec.Template.Spec.NodeSelector = map[string]string{}
		}
		d.Spec.Template.Spec.NodeSelector["beta.kubernetes.io/os"] = os
	}
}

func dmWithContainerPorts(ports ...int32) deploymentModifier {
	return func(d *appsv1.Deployment) {
		p := []corev1.ContainerPort{}
		for _, port := range ports {
			p = append(p, corev1.ContainerPort{
				Name:          portName,
				ContainerPort: port,
			})
		}
		d.Spec.Template.Spec.Containers = append(d.Spec.Template.Spec.Containers, corev1.Container{
			Name:  containerName,
			Ports: p,
		})
	}
}

func deployment(mod ...deploymentModifier) *appsv1.Deployment {
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       deploymentKind,
			APIVersion: deploymentAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadName,
			Namespace: workloadNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelKey: workloadUID,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelKey: workloadUID,
					},
				},
			},
		},
	}

	for _, m := range mod {
		m(d)
	}

	return d
}

type cwModifier func(*v1alpha2.ContainerizedWorkload)

func cwWithLabel(label map[string]string) cwModifier {
	return func(cw *v1alpha2.ContainerizedWorkload) {
		cw.Labels = label
	}
}

func cwWithAnnotation(annotation map[string]string) cwModifier {
	return func(cw *v1alpha2.ContainerizedWorkload) {
		cw.Annotations = annotation
	}
}

func cwWithOS(os string) cwModifier {
	return func(cw *v1alpha2.ContainerizedWorkload) {
		oamOS := v1alpha2.OperatingSystem(os)
		cw.Spec.OperatingSystem = &oamOS
	}
}

func cwWithContainer(c v1alpha2.Container) cwModifier {
	return func(cw *v1alpha2.ContainerizedWorkload) {
		cw.Spec.Containers = append(cw.Spec.Containers, c)
	}
}

func dmWithContainer(c corev1.Container) deploymentModifier {
	return func(d *appsv1.Deployment) {
		d.Spec.Template.Spec.Containers = append(d.Spec.Template.Spec.Containers, c)
	}
}

func containerizedWorkload(mod ...cwModifier) *v1alpha2.ContainerizedWorkload {
	cw := &v1alpha2.ContainerizedWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadName,
			Namespace: workloadNamespace,
			UID:       types.UID(workloadUID),
		},
	}

	for _, m := range mod {
		m(cw)
	}

	return cw
}

func TestContainerizedWorkloadTranslator(t *testing.T) {

	envVarSecretVal := "nicesecretvalue"
	cwLabel := map[string]string{
		"oam.dev/enabled": "true",
	}
	dmLabel := cwLabel
	dmLabel[labelKey] = workloadUID
	cwAnnotation := map[string]string{
		"dapr.io/enabled": "true",
	}
	dmAnnotation := cwAnnotation
	type args struct {
		w oam.Workload
	}

	type want struct {
		result []oam.Object
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ErrorWorkloadNotContainerizedWorkload": {
			reason: "Workload passed to translator that is not ContainerizedWorkload should return error.",
			args: args{
				w: &mock.Workload{},
			},
			want: want{err: errors.New(errNotContainerizedWorkload)},
		},
		"SuccessfulEmpty": {
			reason: "A ContainerizedWorkload should be successfully translated into a deployment.",
			args: args{
				w: containerizedWorkload(),
			},
			want: want{result: []oam.Object{deployment()}},
		},
		"SuccessfulWithLabelAndAnnotation": {
			reason: "A ContainerizedWorkload with label and annotation should be successfully pass onto a deployment.",
			args: args{
				w: containerizedWorkload(cwWithLabel(cwLabel), cwWithAnnotation(cwAnnotation)),
			},
			want: want{result: []oam.Object{deployment(dmWithLabel(cwLabel), dmWithAnnotation(dmAnnotation))}},
		},
		"SuccessfulOS": {
			reason: "A ContainerizedWorkload should be successfully translateddinto a deployment.",
			args: args{
				w: containerizedWorkload(cwWithOS("test")),
			},
			want: want{result: []oam.Object{deployment(dmWithOS("test"))}},
		},
		"SuccessfulContainers": {
			reason: "A ContainerizedWorkload should be successfully translated into a deployment.",
			args: args{
				w: containerizedWorkload(cwWithContainer(v1alpha2.Container{
					Name:      "cool-container",
					Image:     "cool/image:latest",
					Command:   []string{"run"},
					Arguments: []string{"--coolflag"},
					Ports: []v1alpha2.ContainerPort{
						{
							Name: "cool-port",
							Port: 8080,
						},
					},
					Resources: &v1alpha2.ContainerResources{
						Volumes: []v1alpha2.VolumeResource{
							{
								Name:      "cool-volume",
								MountPath: "/my/cool/path",
							},
						},
					},
					Environment: []v1alpha2.ContainerEnvVar{
						{
							Name: "COOL_SECRET",
							FromSecret: &v1alpha2.SecretKeySelector{
								Name: "cool-secret",
								Key:  "secretdata",
							},
						},
						{
							Name:  "NICE_SECRET",
							Value: &envVarSecretVal,
						},
						// If both Value and FromSecret are defined, we use Value
						{
							Name:  "USE_VAL_SECRET",
							Value: &envVarSecretVal,
							FromSecret: &v1alpha2.SecretKeySelector{
								Name: "cool-secret",
								Key:  "secretdata",
							},
						},
						// If neither Value or FromSecret is define, we skip
						{
							Name: "USE_VAL_SECRET",
						},
					},
				})),
			},
			want: want{result: []oam.Object{deployment(dmWithContainer(corev1.Container{
				Name:    "cool-container",
				Image:   "cool/image:latest",
				Command: []string{"run"},
				Args:    []string{"--coolflag"},
				Ports: []corev1.ContainerPort{
					{
						Name:          "cool-port",
						ContainerPort: 8080,
					},
				},
				// CPU and Memory get initialized because we set them if any
				// part of OAM Container.Resources is present. They are not
				// pointer values, so we cannot tell if they were omitted or
				// explicitly set to zero-value.
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"cpu":    {},
						"memory": {},
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "cool-volume",
						MountPath: "/my/cool/path",
					},
				},
				Env: []corev1.EnvVar{
					{
						Name: "COOL_SECRET",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								Key: "secretdata",
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "cool-secret",
								},
							},
						},
					},
					{
						Name:  "NICE_SECRET",
						Value: envVarSecretVal,
					},
					{
						Name:  "USE_VAL_SECRET",
						Value: envVarSecretVal,
					},
				},
			}))}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := TranslateContainerWorkload(context.Background(), tc.args.w)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want error, +got error:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.result, r); diff != "" {
				t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

type serviceModifier func(*corev1.Service)

func sWithContainerPort(target int) serviceModifier {
	return func(s *corev1.Service) {
		s.Spec.Ports = append(s.Spec.Ports, corev1.ServicePort{
			Name:       workloadName,
			Port:       int32(target),
			TargetPort: intstr.FromInt(target),
		})
	}
}

func service(mod ...serviceModifier) *corev1.Service {
	s := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       serviceKind,
			APIVersion: serviceAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadName,
			Namespace: workloadNamespace,
			Labels: map[string]string{
				labelKey: workloadUID,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				labelKey: workloadUID,
			},
			Type: corev1.ServiceTypeLoadBalancer,
		},
	}

	for _, m := range mod {
		m(s)
	}

	return s
}

func TestServiceInjector(t *testing.T) {
	type args struct {
		w oam.Workload
		o []oam.Object
	}

	type want struct {
		result []oam.Object
		err    error
	}

	invalidDeployment := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1alpha1",
		}}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilObject": {
			reason: "Nil object should immediately return nil.",
			args: args{
				w: &mock.Workload{},
			},
			want: want{},
		},
		"InvalidObject": {
			reason: "invalid object should immediately return nil.",
			args: args{
				w: &mock.Workload{},
				o: []oam.Object{
					invalidDeployment,
				},
			},
			want: want{
				result: []oam.Object{
					invalidDeployment,
				},
				err: nil,
			},
		},
		"SuccessfulInjectService_1D_1C_1P": {
			reason: "A Deployment with a port(s) should have a Service injected for first defined port.",
			args: args{
				w: &mock.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      workloadName,
						Namespace: workloadNamespace,
						UID:       types.UID(workloadUID),
					},
				},
				o: []oam.Object{deployment(dmWithContainerPorts(3000))},
			},
			want: want{result: []oam.Object{
				deployment(dmWithContainerPorts(3000)),
				service(sWithContainerPort(3000)),
			}},
		},
		"SuccessfulInjectService_1D_1C_2P": {
			reason: "A Deployment with a port(s) should have a Service injected for first defined port on the first container.",
			args: args{
				w: &mock.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      workloadName,
						Namespace: workloadNamespace,
						UID:       types.UID(workloadUID),
					},
				},
				o: []oam.Object{deployment(dmWithContainerPorts(3000, 3001))},
			},
			want: want{result: []oam.Object{
				deployment(dmWithContainerPorts(3000, 3001)),
				service(sWithContainerPort(3000)),
			}},
		},
		"SuccessfulInjectService_2D_1C_1P": {
			reason: "The first Deployment with a port(s) should have a Service injected for first defined port on the first container.",
			args: args{
				w: &mock.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      workloadName,
						Namespace: workloadNamespace,
						UID:       types.UID(workloadUID),
					},
				},
				o: []oam.Object{
					deployment(dmWithContainerPorts(4000)),
					deployment(dmWithContainerPorts(3000)),
				},
			},
			want: want{result: []oam.Object{
				deployment(dmWithContainerPorts(4000)),
				deployment(dmWithContainerPorts(3000)),
				service(sWithContainerPort(4000)),
			}},
		},
		"SuccessfulInjectService_2D_2C_2P": {
			reason: "The first Deployment with a port(s) should have a Service injected for first defined port on the first container.",
			args: args{
				w: &mock.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      workloadName,
						Namespace: workloadNamespace,
						UID:       types.UID(workloadUID),
					},
				},
				o: []oam.Object{
					deployment(dmWithContainerPorts(3000, 3001), dmWithContainerPorts(4000, 4001)),
					deployment(dmWithContainerPorts(5000, 5001), dmWithContainerPorts(6000, 6001)),
				},
			},
			want: want{result: []oam.Object{
				deployment(dmWithContainerPorts(3000, 3001), dmWithContainerPorts(4000, 4001)),
				deployment(dmWithContainerPorts(5000, 5001), dmWithContainerPorts(6000, 6001)),
				service(sWithContainerPort(3000)),
			}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := ServiceInjector(context.Background(), tc.args.w, tc.args.o)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nReason: %s\nServiceInjector(...): -want error, +got error:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.result, r); diff != "" {
				t.Errorf("\nReason: %s\nServiceInjector(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
