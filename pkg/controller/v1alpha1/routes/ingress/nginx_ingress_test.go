package ingress

import (
	"strconv"
	"testing"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	standardv1alpha1 "github.com/oam-dev/kubevela/api/v1alpha1"
)

func TestConstruct(t *testing.T) {
	tests := map[string]struct {
		routeTrait *standardv1alpha1.Route
		exp        []*v1beta1.Ingress
	}{
		"normal case": {
			routeTrait: &standardv1alpha1.Route{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Route",
					APIVersion: "standard.oam.dev/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "trait-test",
				},
				Spec: standardv1alpha1.RouteSpec{
					Host: "test.abc",
					TLS: &standardv1alpha1.TLS{
						IssuerName: "test-issuer",
						Type:       "Issuer",
					},
					Rules: []standardv1alpha1.Rule{
						{
							Name:    "myrule1",
							Backend: &standardv1alpha1.Backend{BackendService: &standardv1alpha1.BackendServiceRef{ServiceName: "test", Port: intstr.FromInt(3030)}},
							DefaultBackend: &v1alpha1.TypedReference{
								APIVersion: "k8s.example.com/v1",
								Kind:       "StorageBucket",
								Name:       "static-assets",
							},
						},
					},
				},
			},
			exp: []*v1beta1.Ingress{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Ingress",
						APIVersion: "networking.k8s.io/v1beta1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "trait-test-myrule1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
							"cert-manager.io/issuer":      "test-issuer",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "standard.oam.dev/v1alpha1",
								Kind:               "Route",
								Name:               "trait-test",
								Controller:         pointer.BoolPtr(true),
								BlockOwnerDeletion: pointer.BoolPtr(true),
							},
						},
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{
							{
								Hosts:      []string{"test.abc"},
								SecretName: "trait-test-myrule1-cert",
							},
						},
						Backend: &v1beta1.IngressBackend{Resource: &v1.TypedLocalObjectReference{
							APIGroup: pointer.StringPtr("k8s.example.com/v1"),
							Kind:     "StorageBucket",
							Name:     "static-assets",
						}},
						Rules: []v1beta1.IngressRule{
							{
								Host: "test.abc",
								IngressRuleValue: v1beta1.IngressRuleValue{HTTP: &v1beta1.HTTPIngressRuleValue{Paths: []v1beta1.HTTPIngressPath{
									{
										Path:    "",
										Backend: v1beta1.IngressBackend{ServiceName: "test", ServicePort: intstr.FromInt(3030)},
									},
								}}},
							},
						},
					},
				},
			},
		},
	}
	for message, ti := range tests {
		nginx := &Nginx{}
		got := nginx.Construct(ti.routeTrait)
		assert.Equal(t, len(ti.exp), len(got))
		for idx := range ti.exp {
			assert.Equal(t, ti.exp[idx], got[idx], message+" index "+strconv.Itoa(idx))
		}
	}
}
