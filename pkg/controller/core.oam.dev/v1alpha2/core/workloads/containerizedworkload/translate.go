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
	"fmt"
	"hash/fnv"
	"path"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

var (
	deploymentKind       = reflect.TypeOf(appsv1.Deployment{}).Name()
	deploymentAPIVersion = appsv1.SchemeGroupVersion.String()
	serviceKind          = reflect.TypeOf(corev1.Service{}).Name()
	serviceAPIVersion    = corev1.SchemeGroupVersion.String()
	configMapKind        = reflect.TypeOf(corev1.ConfigMap{}).Name()
	configMapAPIVersion  = corev1.SchemeGroupVersion.String()
)

// Reconcile error strings.
const (
	labelKey = "containerizedworkload.oam.crossplane.io"

	errNotContainerizedWorkload = "object is not a containerized workload"
)

// TranslateContainerWorkload translates a ContainerizedWorkload into a Deployment.
// nolint:gocyclo
func TranslateContainerWorkload(ctx context.Context, w oam.Workload) ([]oam.Object, error) {
	cw, ok := w.(*v1alpha2.ContainerizedWorkload)
	if !ok {
		return nil, errors.New(errNotContainerizedWorkload)
	}

	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       deploymentKind,
			APIVersion: deploymentAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cw.GetName(),
			Namespace: w.GetNamespace(),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelKey: string(cw.GetUID()),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelKey: string(cw.GetUID()),
					},
				},
			},
		},
	}

	if cw.Spec.OperatingSystem != nil {
		if d.Spec.Template.Spec.NodeSelector == nil {
			d.Spec.Template.Spec.NodeSelector = map[string]string{}
		}
		d.Spec.Template.Spec.NodeSelector["beta.kubernetes.io/os"] = string(*cw.Spec.OperatingSystem)
	}

	if cw.Spec.CPUArchitecture != nil {
		if d.Spec.Template.Spec.NodeSelector == nil {
			d.Spec.Template.Spec.NodeSelector = map[string]string{}
		}
		d.Spec.Template.Spec.NodeSelector["kubernetes.io/arch"] = string(*cw.Spec.CPUArchitecture)
	}

	for _, container := range cw.Spec.Containers {
		if container.ImagePullSecret != nil {
			d.Spec.Template.Spec.ImagePullSecrets = append(d.Spec.Template.Spec.ImagePullSecrets, corev1.LocalObjectReference{
				Name: *container.ImagePullSecret,
			})
		}
		kubernetesContainer := corev1.Container{
			Name:    container.Name,
			Image:   container.Image,
			Command: container.Command,
			Args:    container.Arguments,
		}

		if container.Resources != nil {
			kubernetesContainer.Resources = corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    container.Resources.CPU.Required,
					corev1.ResourceMemory: container.Resources.Memory.Required,
				},
			}
			for _, v := range container.Resources.Volumes {
				mount := corev1.VolumeMount{
					Name:      v.Name,
					MountPath: v.MountPath,
				}
				if v.AccessMode != nil && *v.AccessMode == v1alpha2.VolumeAccessModeRO {
					mount.ReadOnly = true
				}
				kubernetesContainer.VolumeMounts = append(kubernetesContainer.VolumeMounts, mount)

			}
		}

		for _, p := range container.Ports {
			port := corev1.ContainerPort{
				Name:          p.Name,
				ContainerPort: p.Port,
			}
			if p.Protocol != nil {
				port.Protocol = corev1.Protocol(*p.Protocol)
			}
			kubernetesContainer.Ports = append(kubernetesContainer.Ports, port)
		}

		for _, e := range container.Environment {
			if e.Value != nil {
				kubernetesContainer.Env = append(kubernetesContainer.Env, corev1.EnvVar{
					Name:  e.Name,
					Value: *e.Value,
				})
				continue
			}
			if e.FromSecret != nil {
				kubernetesContainer.Env = append(kubernetesContainer.Env, corev1.EnvVar{
					Name: e.Name,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							Key: e.FromSecret.Key,
							LocalObjectReference: corev1.LocalObjectReference{
								Name: e.FromSecret.Name,
							},
						},
					},
				})
			}
		}

		if container.LivenessProbe != nil {
			kubernetesContainer.LivenessProbe = &corev1.Probe{}
			if container.LivenessProbe.InitialDelaySeconds != nil {
				kubernetesContainer.LivenessProbe.InitialDelaySeconds = *container.LivenessProbe.InitialDelaySeconds
			}
			if container.LivenessProbe.TimeoutSeconds != nil {
				kubernetesContainer.LivenessProbe.TimeoutSeconds = *container.LivenessProbe.TimeoutSeconds
			}
			if container.LivenessProbe.PeriodSeconds != nil {
				kubernetesContainer.LivenessProbe.PeriodSeconds = *container.LivenessProbe.PeriodSeconds
			}
			if container.LivenessProbe.SuccessThreshold != nil {
				kubernetesContainer.LivenessProbe.SuccessThreshold = *container.LivenessProbe.SuccessThreshold
			}
			if container.LivenessProbe.FailureThreshold != nil {
				kubernetesContainer.LivenessProbe.FailureThreshold = *container.LivenessProbe.FailureThreshold
			}

			// NOTE(hasheddan): Kubernetes specifies that only one type of
			// handler should be provided. OAM does not impose that same
			// restriction. We optimistically check all and set whatever is
			// provided.
			if container.LivenessProbe.HTTPGet != nil {
				kubernetesContainer.LivenessProbe.Handler.HTTPGet = &corev1.HTTPGetAction{
					Path: container.LivenessProbe.HTTPGet.Path,
					Port: intstr.IntOrString{IntVal: container.LivenessProbe.HTTPGet.Port},
				}

				for _, h := range container.LivenessProbe.HTTPGet.HTTPHeaders {
					kubernetesContainer.LivenessProbe.Handler.HTTPGet.HTTPHeaders = append(kubernetesContainer.LivenessProbe.Handler.HTTPGet.HTTPHeaders, corev1.HTTPHeader{
						Name:  h.Name,
						Value: h.Value,
					})
				}
			}
			if container.LivenessProbe.Exec != nil {
				kubernetesContainer.LivenessProbe.Exec = &corev1.ExecAction{
					Command: container.LivenessProbe.Exec.Command,
				}
			}
			if container.LivenessProbe.TCPSocket != nil {
				kubernetesContainer.LivenessProbe.TCPSocket = &corev1.TCPSocketAction{
					Port: intstr.IntOrString{IntVal: container.LivenessProbe.TCPSocket.Port},
				}
			}
		}

		if container.ReadinessProbe != nil {
			kubernetesContainer.ReadinessProbe = &corev1.Probe{}
			if container.ReadinessProbe.InitialDelaySeconds != nil {
				kubernetesContainer.ReadinessProbe.InitialDelaySeconds = *container.ReadinessProbe.InitialDelaySeconds
			}
			if container.ReadinessProbe.TimeoutSeconds != nil {
				kubernetesContainer.ReadinessProbe.TimeoutSeconds = *container.ReadinessProbe.TimeoutSeconds
			}
			if container.ReadinessProbe.PeriodSeconds != nil {
				kubernetesContainer.ReadinessProbe.PeriodSeconds = *container.ReadinessProbe.PeriodSeconds
			}
			if container.ReadinessProbe.SuccessThreshold != nil {
				kubernetesContainer.ReadinessProbe.SuccessThreshold = *container.ReadinessProbe.SuccessThreshold
			}
			if container.ReadinessProbe.FailureThreshold != nil {
				kubernetesContainer.ReadinessProbe.FailureThreshold = *container.ReadinessProbe.FailureThreshold
			}

			// NOTE(hasheddan): Kubernetes specifies that only one type of
			// handler should be provided. OAM does not impose that same
			// restriction. We optimistically check all and set whatever is
			// provided.
			if container.ReadinessProbe.HTTPGet != nil {
				kubernetesContainer.ReadinessProbe.Handler.HTTPGet = &corev1.HTTPGetAction{
					Path: container.ReadinessProbe.HTTPGet.Path,
					Port: intstr.IntOrString{IntVal: container.ReadinessProbe.HTTPGet.Port},
				}

				for _, h := range container.ReadinessProbe.HTTPGet.HTTPHeaders {
					kubernetesContainer.ReadinessProbe.Handler.HTTPGet.HTTPHeaders = append(kubernetesContainer.ReadinessProbe.Handler.HTTPGet.HTTPHeaders, corev1.HTTPHeader{
						Name:  h.Name,
						Value: h.Value,
					})
				}
			}
			if container.ReadinessProbe.Exec != nil {
				kubernetesContainer.ReadinessProbe.Exec = &corev1.ExecAction{
					Command: container.ReadinessProbe.Exec.Command,
				}
			}
			if container.ReadinessProbe.TCPSocket != nil {
				kubernetesContainer.ReadinessProbe.TCPSocket = &corev1.TCPSocketAction{
					Port: intstr.IntOrString{IntVal: container.ReadinessProbe.TCPSocket.Port},
				}
			}
		}

		for _, c := range container.ConfigFiles {
			v, vm := translateConfigFileToVolume(c, w.GetName(), container.Name)
			kubernetesContainer.VolumeMounts = append(kubernetesContainer.VolumeMounts, vm)
			d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes, v)
		}

		d.Spec.Template.Spec.Containers = append(d.Spec.Template.Spec.Containers, kubernetesContainer)
	}

	// pass through label and annotation from the workload to the deployment
	util.PassLabelAndAnnotation(w, d)
	// pass through label from the workload to the pod template
	util.PassLabel(w, &d.Spec.Template)

	return []oam.Object{d}, nil
}

func translateConfigFileToVolume(cf v1alpha2.ContainerConfigFile, wlName, containerName string) (v corev1.Volume, vm corev1.VolumeMount) {
	mountPath, _ := path.Split(cf.Path)
	// translate into ConfigMap Volume
	if cf.Value != nil {
		name, _ := generateConfigMapName(cf, wlName, containerName)
		v = corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: name},
				},
			},
		}
		vm = corev1.VolumeMount{
			MountPath: mountPath,
			Name:      name,
		}
		return v, vm
	}

	// translate into Secret Volume
	secretName := cf.FromSecret.Name
	itemKey := cf.FromSecret.Key
	v = corev1.Volume{
		Name: secretName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretName,
				Items: []corev1.KeyToPath{
					{
						Key: itemKey,
						// OAM v1alpha2 SecretKeySelector doen't provide Path field
						// just use itemKey as relative Path
						Path: itemKey,
					},
				},
			},
		},
	}
	vm = corev1.VolumeMount{
		MountPath: cf.Path,
		Name:      secretName,
	}
	return v, vm
}

func generateConfigMapName(cf v1alpha2.ContainerConfigFile, wlName, containerName string) (string, error) {
	h := fnv.New32a()
	_, err := h.Write([]byte(cf.Path))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%d", wlName, containerName, h.Sum32()), nil
}

// TranslateConfigMaps translate non-secret ContainerConfigFile into ConfigMaps
func TranslateConfigMaps(ctx context.Context, w oam.Object) ([]*corev1.ConfigMap, error) {
	cw, ok := w.(*v1alpha2.ContainerizedWorkload)
	if !ok {
		return nil, errors.New(errNotContainerizedWorkload)
	}

	newConfigMaps := []*corev1.ConfigMap{}
	for _, c := range cw.Spec.Containers {
		for _, cf := range c.ConfigFiles {
			if cf.Value == nil {
				continue
			}
			_, key := path.Split(cf.Path)
			cmName, err := generateConfigMapName(cf, cw.GetName(), c.Name)
			if err != nil {
				return nil, err
			}
			cm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       configMapKind,
					APIVersion: configMapAPIVersion,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: cw.GetNamespace(),
				},
				Data: map[string]string{
					key: *cf.Value,
				},
			}
			// pass through label and annotation from the workload to the configmap
			util.PassLabelAndAnnotation(w, cm)
			newConfigMaps = append(newConfigMaps, cm)
		}
	}
	return newConfigMaps, nil
}

// ServiceInjector adds a Service object for the first Port on the first
// Container for the first Deployment observed in a workload translation.
func ServiceInjector(ctx context.Context, w oam.Workload, objs []oam.Object) ([]oam.Object, error) {
	if objs == nil {
		return nil, nil
	}

	for _, o := range objs {
		d, ok := o.(*appsv1.Deployment)
		if !ok {
			continue
		}

		// We don't add a Service if there are no containers for the Deployment.
		// This should never happen in practice.
		if len(d.Spec.Template.Spec.Containers) < 1 {
			continue
		}

		s := &corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       serviceKind,
				APIVersion: serviceAPIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      d.GetName(),
				Namespace: d.GetNamespace(),
				Labels: map[string]string{
					labelKey: string(w.GetUID()),
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: d.Spec.Selector.MatchLabels,
				Ports:    []corev1.ServicePort{},
				Type:     corev1.ServiceTypeLoadBalancer,
			},
		}

		// We only add a single Service for the Deployment, even if multiple
		// ports or no ports are defined on the first container. This is to
		// exclude the need for implementing garbage collection in the
		// short-term in the case that ports are modified after creation.
		if len(d.Spec.Template.Spec.Containers[0].Ports) > 0 {
			s.Spec.Ports = []corev1.ServicePort{
				{
					Name:       d.GetName(),
					Port:       d.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort,
					TargetPort: intstr.FromInt(int(d.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort)),
				},
			}
		}
		objs = append(objs, s)
		break
	}
	return objs, nil
}
