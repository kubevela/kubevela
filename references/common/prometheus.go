/*
Copyright 2021 The KubeVela Authors.

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

package common

import (
	"context"

	"github.com/oam-dev/kubevela/pkg/utils/util"

	monitoring "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InstallPrometheusInstance will install prometheus instance when the Capability is 'metrics'
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
		Image:              pointer.String("quay.io/prometheus/prometheus:v2.19.2"),
		NodeSelector:       map[string]string{"kubernetes.io/os": "linux"},
		Replicas:           pointer.Int32(1),
		ServiceAccountName: "kube-prometheus-stack-prometheus",
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser:    pointer.Int64(1000),
			RunAsNonRoot: pointer.Bool(true),
			FSGroup:      pointer.Int64(2000),
		},
		ServiceMonitorSelector: &v1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "oam", "controller": "metricsTrait"}},
		ServiceMonitorNamespaceSelector: &v1.LabelSelector{
			MatchLabels: util.OAMLabel,
		},
		Version: "v2.19.2",
	}
	return kubecli.Create(context.Background(), &promIns)
}
