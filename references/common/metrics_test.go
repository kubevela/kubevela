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

package common

import (
	"testing"
	"time"

	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func TestToPercentageStr(t *testing.T) {
	var v1, v2 int64
	v1, v2 = 10, 100
	assert.Equal(t, ToPercentageStr(v1, v2), "10%")
	v1, v2 = 10, 0
	assert.Equal(t, ToPercentageStr(v1, v2), "N/A")
}

func TestGetPodResourceSpecAndUsage(t *testing.T) {
	testEnv := &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.Bool(false),
	}
	cfg, err := testEnv.Start()
	assert.NoError(t, err)

	k8sClient, err := client.New(cfg, client.Options{Scheme: common.Scheme})
	assert.NoError(t, err)

	quantityLimitsCPU, _ := resource.ParseQuantity("10m")
	quantityLimitsMemory, _ := resource.ParseQuantity("10Mi")
	quantityRequestsCPU, _ := resource.ParseQuantity("100m")
	quantityRequestsMemory, _ := resource.ParseQuantity("50Mi")
	quantityUsageCPU, _ := resource.ParseQuantity("8m")
	quantityUsageMemory, _ := resource.ParseQuantity("20Mi")

	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: map[v1.ResourceName]resource.Quantity{"memory": quantityRequestsMemory, "cpu": quantityRequestsCPU},
						Limits:   map[v1.ResourceName]resource.Quantity{"memory": quantityLimitsMemory, "cpu": quantityLimitsCPU},
					},
				},
			},
		},
	}
	podMetric := &v1beta1.PodMetrics{
		Containers: []v1beta1.ContainerMetrics{
			{
				Name: "",
				Usage: map[v1.ResourceName]resource.Quantity{
					"memory": quantityUsageMemory, "cpu": quantityUsageCPU,
				},
			},
		},
	}

	spec, usage := GetPodResourceSpecAndUsage(k8sClient, pod, podMetric)
	assert.Equal(t, usage.CPU, int64(8))
	assert.Equal(t, usage.Mem, int64(20971520))
	assert.Equal(t, spec.Lcpu, int64(10))
	assert.Equal(t, spec.Lmem, int64(10485760))
	assert.Equal(t, spec.Rcpu, int64(100))
	assert.Equal(t, spec.Rmem, int64(52428800))
}
