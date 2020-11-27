package ingress

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	standardv1alpha1 "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	certmanager "github.com/wonderflow/cert-manager-api/pkg/apis/certmanager/v1"
	cmmeta "github.com/wonderflow/cert-manager-api/pkg/apis/meta/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Nginx is nginx ingress implementation
type Nginx struct {
	Client client.Client
}

var _ RouteIngress = &Nginx{}

// CheckStatus will check status of the ingress
func (n *Nginx) CheckStatus(routeTrait *standardv1alpha1.Route) (string, []runtimev1alpha1.Condition) {
	ctx := context.Background()
	// check issuer
	if routeTrait.Spec.TLS != nil && routeTrait.Spec.TLS.Type != standardv1alpha1.ClusterIssuer {
		tls := routeTrait.Spec.TLS
		var issuer certmanager.Issuer
		err := n.Client.Get(ctx, types.NamespacedName{Namespace: routeTrait.Namespace, Name: tls.IssuerName}, &issuer)
		if err != nil || len(issuer.Status.Conditions) < 1 {
			var message string
			if err == nil {
				message = fmt.Sprintf("issuer '%v' is pending to be resolved by controller", tls.IssuerName)
			} else {
				message = err.Error()
			}
			return StatusSynced, []runtimev1alpha1.Condition{{Type: runtimev1alpha1.TypeSynced,
				Status: v1.ConditionFalse, LastTransitionTime: metav1.Now(), Reason: runtimev1alpha1.ReasonUnavailable,
				Message: message}}
		}
		// TODO(wonderflow): handle more than one condition case
		condition := issuer.Status.Conditions[0]
		if condition.Status != cmmeta.ConditionTrue {
			return StatusSynced, []runtimev1alpha1.Condition{{Type: runtimev1alpha1.TypeSynced,
				Status: v1.ConditionFalse, LastTransitionTime: metav1.Now(), Reason: runtimev1alpha1.ConditionReason(condition.Reason),
				Message: condition.Message}}
		}
	}
	// check ingress
	ingresses := n.Construct(routeTrait)
	for _, in := range ingresses {

		// Check Certificate
		if routeTrait.Spec.TLS != nil {
			var cert certmanager.Certificate
			// check cert
			err := n.Client.Get(ctx, types.NamespacedName{Namespace: routeTrait.Namespace, Name: in.Name + "-cert"}, &cert)
			if err != nil || len(cert.Status.Conditions) < 1 {
				var message string
				if err == nil {
					message = fmt.Sprintf("CertificateRequest %s is pending to be resolved by controller", in.Name)
				} else {
					message = err.Error()
				}
				return StatusSynced, []runtimev1alpha1.Condition{{Type: runtimev1alpha1.TypeSynced,
					Status: v1.ConditionFalse, LastTransitionTime: metav1.Now(), Reason: runtimev1alpha1.ReasonUnavailable,
					Message: message}}
			}
			// TODO(wonderflow): handle more than one condition case
			certcondition := cert.Status.Conditions[0]
			if certcondition.Status != cmmeta.ConditionTrue || certcondition.Type != certmanager.CertificateConditionReady {
				return StatusSynced, []runtimev1alpha1.Condition{{Type: runtimev1alpha1.TypeSynced,
					Status: v1.ConditionFalse, LastTransitionTime: metav1.Now(), Reason: runtimev1alpha1.ConditionReason(certcondition.Reason),
					Message: certcondition.Message}}
			}
		}

		// Check Ingress
		var ingress v1beta1.Ingress
		if err := n.Client.Get(ctx, types.NamespacedName{Namespace: in.Namespace, Name: in.Name}, &ingress); err != nil {
			return StatusSynced, []runtimev1alpha1.Condition{{Type: runtimev1alpha1.TypeSynced,
				Status: v1.ConditionFalse, LastTransitionTime: metav1.Now(), Reason: runtimev1alpha1.ReasonUnavailable,
				Message: err.Error()}}
		}
		ingressvalue := ingress.Status.LoadBalancer.Ingress
		if len(ingressvalue) < 1 || (ingressvalue[0].IP == "" && ingressvalue[0].Hostname == "") {
			return StatusSynced, []runtimev1alpha1.Condition{{Type: runtimev1alpha1.TypeSynced,
				Status: v1.ConditionFalse, LastTransitionTime: metav1.Now(), Reason: runtimev1alpha1.ReasonCreating,
				Message: fmt.Sprintf("IP/Hostname of %s ingress is generating", in.Name)}}
		}
	}
	return StatusReady, []runtimev1alpha1.Condition{{Type: runtimev1alpha1.TypeReady, Status: v1.ConditionTrue,
		Reason: runtimev1alpha1.ReasonAvailable, LastTransitionTime: metav1.Now()}}
}

// Construct will construct ingress from route
func (*Nginx) Construct(routeTrait *standardv1alpha1.Route) []*v1beta1.Ingress {

	// Don't create ingress if no host set, this is used for local K8s cluster demo and the route trait will create K8s service only.
	if routeTrait.Spec.Host == "" || strings.Contains(routeTrait.Spec.Host, "localhost") || strings.Contains(routeTrait.Spec.Host, "127.0.0.1") {
		return nil
	}
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
		if routeTrait.Spec.TLS != nil {
			var issuerAnn = "cert-manager.io/issuer"
			if routeTrait.Spec.TLS.Type == standardv1alpha1.ClusterIssuer {
				issuerAnn = "cert-manager.io/cluster-issuer"
			}
			annotations[issuerAnn] = routeTrait.Spec.TLS.IssuerName
		}
		// Rewrite
		if rule.RewriteTarget != "" {
			annotations["nginx.ingress.kubernetes.io/rewrite-target"] = rule.RewriteTarget
		}

		// Custom headers
		var headerSnippet string
		for k, v := range rule.CustomHeaders {
			headerSnippet += fmt.Sprintf("more_set_headers \"%s: %s\";\n", k, v)
		}
		if headerSnippet != "" {
			annotations["nginx.ingress.kubernetes.io/configuration-snippet"] = headerSnippet
		}

		// Send timeout
		if backend.SendTimeout != 0 {
			annotations["nginx.ingress.kubernetes.io/proxy-send-timeout"] = strconv.Itoa(backend.SendTimeout)
		}

		// Read timeout
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
		if routeTrait.Spec.TLS != nil {
			ingress.Spec.TLS = []v1beta1.IngressTLS{
				{
					Hosts:      []string{routeTrait.Spec.Host},
					SecretName: routeTrait.Name + "-" + name + "-cert",
				},
			}
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
