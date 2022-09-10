/*
Copyright 2022 The KubeVela Authors.

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

package utils

import (
	"context"
	"math"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

const (
	// NA Not available.
	NA = "N/A"
)

// Metric including requests and limits metrics
type Metric struct {
	CPU, Mem   int64
	Lcpu, Lmem int64
}

// GatherPodMX return the usage metrics of a pod and specified metric including requests and limits metrics
func GatherPodMX(pod *v1.Pod, mx *v1beta1.PodMetrics) (c, r Metric) {
	rcpu, rmem := podRequests(pod.Spec)
	lcpu, lmem := podLimits(pod.Spec)
	r.CPU, r.Lcpu, r.Mem, r.Lmem = rcpu.MilliValue(), lcpu.MilliValue(), rmem.Value(), lmem.Value()

	if mx != nil {
		ccpu, cmem := podUsage(mx)
		c.CPU, c.Mem = ccpu.MilliValue(), cmem.Value()
	}
	return
}

func podUsage(metrics *v1beta1.PodMetrics) (*resource.Quantity, *resource.Quantity) {
	cpu, mem := new(resource.Quantity), new(resource.Quantity)
	for _, co := range metrics.Containers {
		usage := co.Usage

		if len(usage) == 0 {
			continue
		}
		if usage.Cpu() != nil {
			cpu.Add(*usage.Cpu())
		}
		if co.Usage.Memory() != nil {
			mem.Add(*usage.Memory())
		}
	}
	return cpu, mem
}

func podLimits(spec v1.PodSpec) (*resource.Quantity, *resource.Quantity) {
	cpu, mem := new(resource.Quantity), new(resource.Quantity)
	for _, co := range spec.Containers {
		limits := co.Resources.Limits
		if len(limits) == 0 {
			continue
		}
		if limits.Cpu() != nil {
			cpu.Add(*limits.Cpu())
		}
		if limits.Memory() != nil {
			mem.Add(*limits.Memory())
		}
	}
	return cpu, mem
}

func podRequests(spec v1.PodSpec) (*resource.Quantity, *resource.Quantity) {
	cpu, mem := new(resource.Quantity), new(resource.Quantity)
	for _, co := range spec.Containers {
		req := co.Resources.Requests
		if len(req) == 0 {
			continue
		}
		if req.Cpu() != nil {
			cpu.Add(*req.Cpu())
		}
		if req.Memory() != nil {
			mem.Add(*req.Memory())
		}
	}
	return cpu, mem
}

// PodMetric return the pod metric
func PodMetric(cfg *rest.Config, name, namespace string) (*v1beta1.PodMetrics, error) {
	ctx := context.Background()
	c, err := metrics.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	metric, err := c.MetricsV1beta1().PodMetricses(namespace).Get(ctx, name, apiv1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return metric, nil
}

// ToPercentage computes percentage as string otherwise n/aa.
func ToPercentage(v1, v2 int64) int {
	if v2 == 0 {
		return 0
	}
	return int(math.Floor((float64(v1) / float64(v2)) * 100))
}

// ToPercentageStr computes percentage, but if v2 is 0, it will return NAValue instead of 0.
func ToPercentageStr(v1, v2 int64) string {
	if v2 == 0 {
		return NA
	}
	return strconv.Itoa(ToPercentage(v1, v2)) + "%"
}
