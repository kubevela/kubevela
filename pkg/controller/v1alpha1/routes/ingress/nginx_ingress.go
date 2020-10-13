package ingress

import (
	"fmt"
	"reflect"
	"strconv"

	standardv1alpha1 "github.com/oam-dev/kubevela/api/v1alpha1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

type Nginx struct{}

var _ RouteIngress = &Nginx{}

func (*Nginx) Construct(routeTrait *standardv1alpha1.Route) []*v1beta1.Ingress {

	var ingresses []*v1beta1.Ingress
	for idx, rule := range routeTrait.Spec.Rules {
		name := rule.Name
		if name == "" {
			name = strconv.Itoa(idx)
		}
		backend := rule.Backend
		if backend == nil || backend.BackendService == nil {
			continue
		}

		var annotations = make(map[string]string)

		annotations["kubernetes.io/ingress.class"] = TypeNginx

		// SSL
		var issuerAnn = "cert-manager.io/issuer"
		if routeTrait.Spec.TLS.Type == standardv1alpha1.ClusterIssuer {
			issuerAnn = "cert-manager.io/cluster-issuer"
		}
		annotations[issuerAnn] = routeTrait.Spec.TLS.IssuerName

		// Rewrite
		if rule.RewriteTarget != "" {
			annotations["ingress.kubernetes.io/rewrite-target"] = rule.RewriteTarget
		}

		// Custom headers
		var headerSnippet string
		for k, v := range rule.CustomHeaders {
			headerSnippet += fmt.Sprintf("more_set_headers \"%s: %s\";\n", k, v)
		}
		if headerSnippet != "" {
			annotations["nginx.ingress.kubernetes.io/configuration-snippet"] = headerSnippet
		}

		//Send timeout
		if backend.SendTimeout != 0 {
			annotations["nginx.ingress.kubernetes.io/proxy-send-timeout"] = strconv.Itoa(backend.SendTimeout)
		}

		//Read timeout
		if backend.ReadTimeout != 0 {
			annotations["nginx.ingress.kubernetes.io/proxy‑read‑timeout"] = strconv.Itoa(backend.ReadTimeout)
		}

		ingress := &v1beta1.Ingress{
			TypeMeta: metav1.TypeMeta{
				Kind:       reflect.TypeOf(v1beta1.Ingress{}).Name(),
				APIVersion: v1beta1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        routeTrait.Name + "-" + name,
				Namespace:   routeTrait.Namespace,
				Annotations: annotations,
				Labels:      routeTrait.GetLabels(),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         routeTrait.GetObjectKind().GroupVersionKind().GroupVersion().String(),
						Kind:               routeTrait.GetObjectKind().GroupVersionKind().Kind,
						UID:                routeTrait.GetUID(),
						Name:               routeTrait.GetName(),
						Controller:         pointer.BoolPtr(true),
						BlockOwnerDeletion: pointer.BoolPtr(true),
					},
				},
			},
		}
		ingress.Spec.TLS = []v1beta1.IngressTLS{
			{
				Hosts:      []string{routeTrait.Spec.Host},
				SecretName: routeTrait.Name + "-" + name + "-cert",
			},
		}
		if rule.DefaultBackend != nil {
			ingress.Spec.Backend = &v1beta1.IngressBackend{
				Resource: &v1.TypedLocalObjectReference{
					APIGroup: &rule.DefaultBackend.APIVersion,
					Kind:     rule.DefaultBackend.Kind,
					Name:     rule.DefaultBackend.Name,
				},
			}
		}
		ingress.Spec.Rules = []v1beta1.IngressRule{
			{
				Host: routeTrait.Spec.Host,
				IngressRuleValue: v1beta1.IngressRuleValue{HTTP: &v1beta1.HTTPIngressRuleValue{
					Paths: []v1beta1.HTTPIngressPath{
						{
							Path: rule.Path,
							Backend: v1beta1.IngressBackend{
								ServiceName: backend.BackendService.ServiceName,
								ServicePort: backend.BackendService.Port,
							},
						},
					},
				}},
			},
		}
		ingresses = append(ingresses, ingress)
	}
	return ingresses
}
