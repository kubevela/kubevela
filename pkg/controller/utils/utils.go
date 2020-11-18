package utils

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const LabelPodSpecable = "workload.oam.dev/podspecable"

func GetPodSpecPath(workloadDef *v1alpha2.WorkloadDefinition) (string, bool) {
	if workloadDef.Spec.PodSpecPath != "" {
		return workloadDef.Spec.PodSpecPath, true
	}
	if workloadDef.Labels == nil {
		return "", false
	}
	podSpecable, ok := workloadDef.Labels[LabelPodSpecable]
	if !ok {
		return "", false
	}
	ok, _ = strconv.ParseBool(podSpecable)
	return "", ok
}

func DiscoveryFromPodSpec(w *unstructured.Unstructured, fieldPath string) ([]intstr.IntOrString, error) {
	paved := fieldpath.Pave(w.Object)
	obj, err := paved.GetValue(fieldPath)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("discovery podSpec from %s in workload %v err %v", fieldPath, w.GetName(), err)
	}
	var spec v1.PodSpec
	err = json.Unmarshal(data, &spec)
	if err != nil {
		return nil, fmt.Errorf("discovery podSpec from %s in workload %v err %v", fieldPath, w.GetName(), err)
	}
	ports := getContainerPorts(spec.Containers)
	if len(ports) == 0 {
		return nil, fmt.Errorf("no port found in podSpec %v", w.GetName())
	}
	return ports, nil
}

// DiscoveryFromPodTemplate not only discovery port, will also use labels in podTemplate
func DiscoveryFromPodTemplate(w *unstructured.Unstructured, fields ...string) ([]intstr.IntOrString, map[string]string, error) {
	obj, found, _ := unstructured.NestedMap(w.Object, fields...)
	if !found {
		return nil, nil, fmt.Errorf("not have spec.template in workload %v", w.GetName())
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, nil, fmt.Errorf("workload %v convert object err %v", w.GetName(), err)
	}
	var spec v1.PodTemplateSpec
	err = json.Unmarshal(data, &spec)
	if err != nil {
		return nil, nil, fmt.Errorf("workload %v convert object to PodTemplate err %v", w.GetName(), err)
	}
	ports := getContainerPorts(spec.Spec.Containers)
	if len(ports) == 0 {
		return nil, nil, fmt.Errorf("no port found in workload %v", w.GetName())
	}
	return ports, spec.Labels, nil
}

func getContainerPorts(cs []v1.Container) []intstr.IntOrString {
	var ports []intstr.IntOrString
	//TODO(wonderflow): exclude some sidecars
	for _, container := range cs {
		for _, port := range container.Ports {
			ports = append(ports, intstr.FromInt(int(port.ContainerPort)))
		}
	}
	return ports
}

// SelectOAMAppLabelsWithoutRevision will filter and return OAM app labels only, if no labels, return the original one.
func SelectOAMAppLabelsWithoutRevision(labels map[string]string) map[string]string {
	newLabel := make(map[string]string)
	for k, v := range labels {
		// Note: we don't include revision label by design
		// if we want to distinguish with different revisions, we should include it in other function.
		if k != oam.LabelAppName && k != oam.LabelAppComponent {
			continue
		}
		newLabel[k] = v
	}
	if len(newLabel) == 0 {
		return labels
	}
	return newLabel
}
