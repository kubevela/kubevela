package dependency

import (
	"context"

	"github.com/oam-dev/kubevela/pkg/commands/util"

	monitoring "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InstallPrometheusInstance will install prometheus instance for vela-core
func InstallPrometheusInstance(kubecli client.Client) error {
	var promIns = monitoring.Prometheus{}
	err := kubecli.Get(context.Background(), types.NamespacedName{Namespace: "monitoring", Name: "oam"}, &promIns)
	if err == nil {
		return nil
	}
	promIns.Name = "oam"
	promIns.Namespace = "monitoring"
	promIns.SetLabels(map[string]string{"prometheus": "kubevela"})
	promIns.Spec = monitoring.PrometheusSpec{
		Image:              pointer.StringPtr("quay.io/prometheus/prometheus:v2.19.2"),
		NodeSelector:       map[string]string{"kubernetes.io/os": "linux"},
		Replicas:           pointer.Int32Ptr(1),
		ServiceAccountName: "kube-prometheus-stack-prometheus",
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser:    pointer.Int64Ptr(1000),
			RunAsNonRoot: pointer.BoolPtr(true),
			FSGroup:      pointer.Int64Ptr(2000),
		},
		ServiceMonitorSelector: &v1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "oam", "controller": "metricsTrait"}},
		ServiceMonitorNamespaceSelector: &v1.LabelSelector{
			MatchLabels: util.OAMLabel,
		},
		Version: "v2.19.2",
	}
	return kubecli.Create(context.Background(), &promIns)
}
