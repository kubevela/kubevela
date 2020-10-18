package routes

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/api/v1alpha1"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func NeedDiscovery(routeTrait *v1alpha1.Route) bool {
	if len(routeTrait.Spec.Rules) == 0 {
		return true
	}
	for _, rule := range routeTrait.Spec.Rules {
		if rule.Backend == nil {
			return true
		}
		if rule.Backend.BackendService == nil {
			return true
		}
		if rule.Backend.BackendService.ServiceName == "" {
			return true
		}
	}
	return false
}

func GetPodSpecPath(workloadDef *v1alpha2.WorkloadDefinition) (string, bool) {
	if workloadDef.Spec.PodSpecPath != "" {
		return workloadDef.Spec.PodSpecPath, true
	}
	if workloadDef.Labels == nil {
		return "", false
	}
	podSpecable, ok := workloadDef.Labels[types.LabelPodSpecable]
	if !ok {
		return "", false
	}
	ok, _ = strconv.ParseBool(podSpecable)
	return "", ok
}

func discoveryFromPodSpec(w *unstructured.Unstructured, fieldPath string) ([]intstr.IntOrString, error) {
	paved := fieldpath.Pave(w.Object)
	obj, err := paved.GetValue(fieldPath)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("discovery podSpec from %s in workload %v err %v", fieldPath, w.GetName(), err)
	}
	var spec corev1.PodSpec
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

// discoveryFromPodTemplate not only discovery port, will also use labels in podTemplate
func discoveryFromPodTemplate(w *unstructured.Unstructured, fields ...string) ([]intstr.IntOrString, map[string]string, error) {
	obj, found, _ := unstructured.NestedMap(w.Object, fields...)
	if !found {
		return nil, nil, fmt.Errorf("not have spec.template in workload %v", w.GetName())
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, nil, fmt.Errorf("workload %v convert object err %v", w.GetName(), err)
	}
	var spec corev1.PodTemplateSpec
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

func getContainerPorts(cs []corev1.Container) []intstr.IntOrString {
	var ports []intstr.IntOrString
	//TODO(wonderflow): exclude some sidecars
	for _, container := range cs {
		for _, port := range container.Ports {
			ports = append(ports, intstr.FromInt(int(port.ContainerPort)))
		}
	}
	return ports
}

// MatchService try check if the service matches the rules
func MatchService(targetPort intstr.IntOrString, rule v1alpha1.Rule) bool {
	// the rule is nil, continue
	if rule.Backend == nil || rule.Backend.BackendService == nil || rule.Backend.BackendService.Port.IntValue() == 0 {
		return true
	}
	if rule.Backend.BackendService.ServiceName != "" {
		return false
	}
	// the rule is not null, if any port matches, we regard them are all match
	if targetPort == rule.Backend.BackendService.Port {
		return true
	}
	// port is not matched, mark it not match
	return false
}

func FillRouteTraitWithService(service *corev1.Service, routeTrait *v1alpha1.Route) {
	if len(routeTrait.Spec.Rules) == 0 {
		routeTrait.Spec.Rules = []v1alpha1.Rule{{Name: "auto-created"}}
	}
	for idx, rule := range routeTrait.Spec.Rules {
		// If backendService.port not specified, will always use the service found and it's first port as backendService.
		for _, servicePort := range service.Spec.Ports {
			//We use targetPort rather than port to match with the rule, because if serviceName not specified,
			//Users will only know containerPort(which is targetPort)
			if MatchService(servicePort.TargetPort, rule) {
				ref := &v1alpha1.BackendServiceRef{
					//Use port of service rather than targetPort, it will be used in ingress pointing to the service
					Port:        intstr.FromInt(int(servicePort.Port)),
					ServiceName: service.Name,
				}
				if rule.Backend == nil {
					rule.Backend = &v1alpha1.Backend{BackendService: ref}
				} else {
					rule.Backend.BackendService = ref
				}
				routeTrait.Spec.Rules[idx] = rule
				break
			}
		}
	}
}

func filterLabels(labels map[string]string) map[string]string {
	newLabel := make(map[string]string)
	for k, v := range labels {
		if k == oam.LabelOAMResourceType || k == oam.WorkloadTypeLabel {
			continue
		}
		newLabel[k] = v
	}
	return newLabel
}
