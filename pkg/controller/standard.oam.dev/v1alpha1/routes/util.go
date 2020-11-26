package routes

import (
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NeedDiscovery checks the routeTrait Spec if it's needed to automatically discover
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

// FillRouteTraitWithService will use existing Service or created Service to fill the spec
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
