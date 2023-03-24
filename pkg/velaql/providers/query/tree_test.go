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

package query

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/stretchr/testify/assert"
	v12 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	types3 "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
)

func TestPodStatus(t *testing.T) {
	succeedPod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "Pod"}, Status: v1.PodStatus{
		Phase: v1.PodSucceeded,
	}}

	runningReadyPod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "Pod"},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyAlways,
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodReady,
					Status: v1.ConditionTrue,
				},
			},
			Phase: v1.PodRunning,
		}}

	runningUnHealthyPod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "Pod"},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyAlways,
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					LastTerminationState: v1.ContainerState{
						Terminated: &v1.ContainerStateTerminated{
							ExitCode: 127,
						},
					},
				},
			},
			Phase: v1.PodRunning,
		}}

	runningProgressingPod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "Pod"},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyAlways,
		},
		Status: v1.PodStatus{
			Phase:  v1.PodRunning,
			Reason: "ContainerCreating",
		}}

	restartNeverPod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "Pod"},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		}}

	pendingPod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "Pod"},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
		}, Status: v1.PodStatus{Phase: v1.PodPending},
	}

	failedWithMessagePod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "Pod"},
		Status: v1.PodStatus{Phase: v1.PodFailed, Message: "some message"},
	}

	failedWithOOMKillPod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "Pod"},
		Status: v1.PodStatus{Phase: v1.PodFailed, ContainerStatuses: []v1.ContainerStatus{
			{
				State: v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{
						Reason: "OOMKilled",
					},
				},
			},
		}},
	}

	failedWithExistCodePod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "Pod"},
		Status: v1.PodStatus{Phase: v1.PodFailed, ContainerStatuses: []v1.ContainerStatus{
			{
				Name: "nginx",
				State: v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{
						ExitCode: 189,
					},
				},
			},
		}},
	}

	testCases := map[string]struct {
		intput v1.Pod
		result types.HealthStatus
	}{
		"succeedPod": {
			intput: succeedPod,
			result: types.HealthStatus{Status: types.HealthStatusHealthy},
		},
		"runningReadyPod": {
			intput: runningReadyPod,
			result: types.HealthStatus{Status: types.HealthStatusHealthy, Reason: "all containers are ready"},
		},
		"runningUnHealthyPod": {
			intput: runningUnHealthyPod,
			result: types.HealthStatus{Status: types.HealthStatusUnHealthy},
		},
		"runningProgressingPod": {
			intput: runningProgressingPod,
			result: types.HealthStatus{Status: types.HealthStatusProgressing, Reason: "ContainerCreating"},
		},
		"restartNeverPod": {
			intput: restartNeverPod,
			result: types.HealthStatus{Status: types.HealthStatusProgressing},
		},
		"pendingPod": {
			intput: pendingPod,
			result: types.HealthStatus{Status: types.HealthStatusProgressing},
		},
		"failedWithMessagePod": {
			intput: failedWithMessagePod,
			result: types.HealthStatus{Status: types.HealthStatusUnHealthy, Message: "some message"},
		},
		"failedWithOOMKillPod": {
			intput: failedWithOOMKillPod,
			result: types.HealthStatus{Status: types.HealthStatusUnHealthy, Message: "OOMKilled"},
		},
		"failedWithExistCodePod": {
			intput: failedWithExistCodePod,
			result: types.HealthStatus{Status: types.HealthStatusUnHealthy, Message: "container \"nginx\" failed with exit code 189"},
		},
	}
	for _, s := range testCases {
		pod := s.intput.DeepCopy()
		p, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
		assert.NoError(t, err)
		res, err := checkPodStatus(unstructured.Unstructured{Object: p})
		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, *res, s.result)
	}
}

func TestService2EndpointOption(t *testing.T) {
	labels := map[string]string{
		"service-name": "test",
		"uid":          "test-uid",
	}
	u := unstructured.Unstructured{}
	u.SetAPIVersion("v1")
	u.SetKind("Service")
	u.SetLabels(labels)
	l, err := service2EndpointListOption(u)
	assert.NoError(t, err)
	assert.Equal(t, "service-name=test,uid=test-uid", l.LabelSelector.String())
}

func TestConvertLabel2Selector(t *testing.T) {
	cronJob1 := `
apiVersion: batch/v1
kind: CronJob
metadata:
  name: cronjob1
  labels:
    app: cronjob1
spec:
  schedule: "* * * * *"
  jobTemplate:
    metadata:
      labels:
        app: cronJob1
    spec:
      template:
        spec:
          containers:
          - name: cronjob
            image: busybox
            command: ["/bin/sh","-c","date"]
          restartPolicy: Never 
`
	obj := unstructured.Unstructured{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode([]byte(cronJob1), nil, &obj)
	assert.NoError(t, err)
	workload1 := WorkloadUnstructured{obj}
	selector1, err := workload1.convertLabel2Selector("not", "exist")
	assert.Equal(t, selector1, labels.Everything())
	assert.NoError(t, err)

	selector2, err := workload1.convertLabel2Selector()
	assert.Equal(t, selector2, nil)
	assert.Error(t, err)

	_, _, err = dec.Decode([]byte(cronJob1), nil, &obj)
	assert.NoError(t, err)
	workload2 := WorkloadUnstructured{obj}
	selector3, err := workload2.convertLabel2Selector("apiVersion")
	assert.Equal(t, selector3, labels.Everything())
	assert.NoError(t, err)

	_, _, err = dec.Decode([]byte(cronJob1), nil, &obj)
	assert.NoError(t, err)
	workload3 := WorkloadUnstructured{obj}
	selector4, err := workload3.convertLabel2Selector("spec", "jobTemplate", "metadata", "labels")
	assert.Equal(t, selector4.String(), "app=cronJob1")
	assert.NoError(t, err)
}

func TestCronJobLabelListOption(t *testing.T) {
	cronJob := `
apiVersion: batch/v1
kind: CronJob
metadata:
  name: cronjob1
  labels:
    app: cronjob1
spec:
  schedule: "* * * * *"
  jobTemplate:
    metadata:
      labels:
        app: cronJob1
    spec:
      template:
        spec:
          containers:
          - name: cronjob
            image: busybox
            command: ["/bin/sh","-c","date"]
          restartPolicy: Never 
`

	// convert yaml to unstructured
	obj := unstructured.Unstructured{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode([]byte(cronJob), nil, &obj)
	assert.NoError(t, err)
	l, err := cronJobLabelListOption(obj)
	assert.NoError(t, err)
	assert.Equal(t, "app=cronJob1", l.LabelSelector.String())
}

func TestServiceStatus(t *testing.T) {
	lbHealthSvc := v1.Service{Spec: v1.ServiceSpec{Type: v1.ServiceTypeLoadBalancer}, Status: v1.ServiceStatus{
		LoadBalancer: v1.LoadBalancerStatus{
			Ingress: []v1.LoadBalancerIngress{
				{
					IP: "198.1.1.1",
				},
			},
		},
	}}
	lbProgressingSvc := v1.Service{Spec: v1.ServiceSpec{Type: v1.ServiceTypeLoadBalancer}, Status: v1.ServiceStatus{}}
	testCases := map[string]struct {
		input v1.Service
		res   types.HealthStatus
	}{
		"health": {
			input: lbHealthSvc,
			res:   types.HealthStatus{Status: types.HealthStatusHealthy},
		},
		"progressing": {
			input: lbProgressingSvc,
			res:   types.HealthStatus{Status: types.HealthStatusProgressing},
		},
	}
	for _, s := range testCases {
		svc := s.input.DeepCopy()
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&svc)
		assert.NoError(t, err)
		res, err := checkServiceStatus(unstructured.Unstructured{Object: u})
		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, s.res, *res)
	}
}

func TestPVCStatus(t *testing.T) {
	heathyPVC := v1.PersistentVolumeClaim{Status: v1.PersistentVolumeClaimStatus{Phase: v1.ClaimBound}}
	unHeathyPVC := v1.PersistentVolumeClaim{Status: v1.PersistentVolumeClaimStatus{Phase: v1.ClaimLost}}
	progressingPVC := v1.PersistentVolumeClaim{Status: v1.PersistentVolumeClaimStatus{Phase: v1.ClaimPending}}
	heathyUnkown := v1.PersistentVolumeClaim{Status: v1.PersistentVolumeClaimStatus{}}
	testCases := map[string]struct {
		input v1.PersistentVolumeClaim
		res   types.HealthStatus
	}{
		"health": {
			input: heathyPVC,
			res:   types.HealthStatus{Status: types.HealthStatusHealthy},
		},
		"progressing": {
			input: progressingPVC,
			res:   types.HealthStatus{Status: types.HealthStatusProgressing},
		},
		"unHealthy": {
			input: unHeathyPVC,
			res:   types.HealthStatus{Status: types.HealthStatusUnHealthy},
		},
		"unknown": {
			input: heathyUnkown,
			res:   types.HealthStatus{Status: types.HealthStatusUnKnown},
		},
	}
	for _, s := range testCases {
		pvc := s.input.DeepCopy()
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
		assert.NoError(t, err)
		res, err := checkPVCHealthStatus(unstructured.Unstructured{Object: u})
		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, s.res, *res)
	}
}

func TestGetReplicaSetCondition(t *testing.T) {
	rs := v12.ReplicaSet{Status: v12.ReplicaSetStatus{
		Conditions: []v12.ReplicaSetCondition{{
			Type: v12.ReplicaSetReplicaFailure,
		}},
	}}
	c := getAppsv1ReplicaSetCondition(rs.Status, v12.ReplicaSetReplicaFailure)
	assert.NotNil(t, c)
}

func TestReplicaSetStatus(t *testing.T) {
	unhealthyReplicaSet := v12.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 2,
		},
		Status: v12.ReplicaSetStatus{ObservedGeneration: 2,
			Conditions: []v12.ReplicaSetCondition{{
				Type:   v12.ReplicaSetReplicaFailure,
				Status: v1.ConditionTrue,
			}}},
	}
	progressingPodReplicaSet := v12.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 2,
		},
		Spec: v12.ReplicaSetSpec{Replicas: pointer.Int32(3)},
		Status: v12.ReplicaSetStatus{
			ObservedGeneration: 2,
			ReadyReplicas:      2,
			AvailableReplicas:  2,
		},
	}
	progressingRollingReplicaSet := v12.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 2,
		},
		Spec: v12.ReplicaSetSpec{Replicas: pointer.Int32(3)},
		Status: v12.ReplicaSetStatus{
			ObservedGeneration: 1,
		},
	}
	testCases := map[string]struct {
		input v12.ReplicaSet
		res   types.HealthStatus
	}{
		"unHealth": {
			input: unhealthyReplicaSet,
			res:   types.HealthStatus{Status: types.HealthStatusUnHealthy},
		},
		"progressing": {
			input: progressingPodReplicaSet,
			res:   types.HealthStatus{Status: types.HealthStatusProgressing, Message: "Waiting for rollout to finish: 2 out of 3 new replicas are available..."},
		},
		"rolling": {
			input: progressingRollingReplicaSet,
			res:   types.HealthStatus{Status: types.HealthStatusProgressing, Message: "Waiting for rollout to finish: observed replica set generation less then desired generation"},
		},
	}
	for _, s := range testCases {
		rs := s.input.DeepCopy()
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&rs)
		assert.NoError(t, err)
		res, err := checkReplicaSetStatus(unstructured.Unstructured{Object: u})
		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, s.res, *res)
	}
}

func TestHelmResourceStatus(t *testing.T) {
	tm := metav1.TypeMeta{APIVersion: "helm.toolkit.fluxcd.io/v2beta1", Kind: "HelmRelease"}
	healthHr := v2beta1.HelmRelease{TypeMeta: tm, Status: v2beta1.HelmReleaseStatus{Conditions: []metav1.Condition{
		{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
		},
	}}}
	unHealthyHr := v2beta1.HelmRelease{TypeMeta: tm, Status: v2beta1.HelmReleaseStatus{Conditions: []metav1.Condition{
		{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Message: "some reason",
		},
	}}}
	unKnowHealthyHr := v2beta1.HelmRelease{TypeMeta: tm, Status: v2beta1.HelmReleaseStatus{Conditions: []metav1.Condition{
		{
			Type:   "OtherType",
			Status: metav1.ConditionFalse,
		},
	}}}
	testCases := map[string]struct {
		hr  v2beta1.HelmRelease
		res *types.HealthStatus
	}{
		"healthHr": {
			hr:  healthHr,
			res: &types.HealthStatus{Status: types.HealthStatusHealthy},
		},
		"unHealthyHr": {
			hr:  unHealthyHr,
			res: &types.HealthStatus{Status: types.HealthStatusUnHealthy, Message: "some reason"},
		},
		"unKnowHealthyHr": {
			hr:  unKnowHealthyHr,
			res: &types.HealthStatus{Status: types.HealthStatusUnKnown},
		},
	}
	for _, s := range testCases {
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(s.hr.DeepCopy())
		assert.NoError(t, err)
		res, err := CheckResourceStatus(unstructured.Unstructured{Object: obj})
		assert.NoError(t, err)
		assert.Equal(t, res, s.res)
	}
}

func TestHelmRepoResourceStatus(t *testing.T) {
	tm := metav1.TypeMeta{APIVersion: "source.toolkit.fluxcd.io/v1beta2", Kind: "HelmRepository"}
	healthHr := v1beta2.HelmRepository{TypeMeta: tm, Status: v1beta2.HelmRepositoryStatus{Conditions: []metav1.Condition{
		{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
		},
	}}}
	unHealthyHr := v1beta2.HelmRepository{TypeMeta: tm, Status: v1beta2.HelmRepositoryStatus{Conditions: []metav1.Condition{
		{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Message: "some reason",
		},
	}}}
	unKnowHealthyHr := v1beta2.HelmRepository{TypeMeta: tm, Status: v1beta2.HelmRepositoryStatus{Conditions: []metav1.Condition{
		{
			Type:   "OtherType",
			Status: metav1.ConditionFalse,
		},
	}}}
	testCases := map[string]struct {
		hr  v1beta2.HelmRepository
		res *types.HealthStatus
	}{
		"healthHr": {
			hr:  healthHr,
			res: &types.HealthStatus{Status: types.HealthStatusHealthy},
		},
		"unHealthyHr": {
			hr:  unHealthyHr,
			res: &types.HealthStatus{Status: types.HealthStatusUnHealthy, Message: "some reason"},
		},
		"unKnowHealthyHr": {
			hr:  unKnowHealthyHr,
			res: &types.HealthStatus{Status: types.HealthStatusUnKnown},
		},
	}
	for _, s := range testCases {
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(s.hr.DeepCopy())
		assert.NoError(t, err)
		res, err := CheckResourceStatus(unstructured.Unstructured{Object: obj})
		assert.NoError(t, err)
		assert.Equal(t, res, s.res)
	}
}

func TestGenListOption(t *testing.T) {
	resLabel, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: map[string]string{"testKey": "testVal"}})
	assert.NoError(t, err)
	listOption := client.ListOptions{LabelSelector: resLabel, Namespace: "test"}

	deploy := v12.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}, Spec: v12.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"testKey": "testVal"}}}}
	du, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&deploy)
	assert.NoError(t, err)
	assert.NotNil(t, du)
	dls, err := defaultWorkloadLabelListOption(unstructured.Unstructured{Object: du})
	assert.NoError(t, err)
	assert.Equal(t, listOption, dls)

	rs := v12.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}, Spec: v12.ReplicaSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"testKey": "testVal"}}}}
	rsu, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&rs)
	assert.NoError(t, err)
	assert.NotNil(t, du)
	rsls, err := defaultWorkloadLabelListOption(unstructured.Unstructured{Object: rsu})
	assert.NoError(t, err)
	assert.Equal(t, listOption, rsls)

	sts := v12.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}, Spec: v12.StatefulSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"testKey": "testVal"}}}}
	stsu, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&sts)
	assert.NoError(t, err)
	assert.NotNil(t, stsu)
	stsls, err := defaultWorkloadLabelListOption(unstructured.Unstructured{Object: stsu})
	assert.NoError(t, err)
	assert.Equal(t, listOption, stsls)

	helmRelease := unstructured.Unstructured{}
	helmRelease.SetName("test-helm")
	helmRelease.SetNamespace("test-ns")
	hrll, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: map[string]string{"helm.toolkit.fluxcd.io/name": "test-helm", "helm.toolkit.fluxcd.io/namespace": "test-ns"}})
	assert.NoError(t, err)

	hrls, err := helmRelease2AnyListOption(helmRelease)
	assert.NoError(t, err)
	assert.Equal(t, hrls, client.ListOptions{LabelSelector: hrll})
}

func TestPodAdditionalInfo(t *testing.T) {
	typeMeta := metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}

	type testCase struct {
		pod v1.Pod
		res map[string]interface{}
	}

	case1 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}},
			Status: v1.PodStatus{
				InitContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Terminated: &v1.ContainerStateTerminated{
								ExitCode: 0,
							},
						},
					},
				},
				Reason: "NodeLost"},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "Unknown",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case2 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Status: v1.PodStatus{
				InitContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Terminated: &v1.ContainerStateTerminated{
								ExitCode: 127,
							},
						},
					},
				}},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "Init:ExitCode:127",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case3 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Status: v1.PodStatus{
				InitContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Terminated: &v1.ContainerStateTerminated{
								ExitCode: 127,
								Signal:   32,
							},
						},
					},
				}},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "Init:Signal:32",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case4 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Status: v1.PodStatus{
				InitContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Terminated: &v1.ContainerStateTerminated{
								Reason:   "OOMKill",
								ExitCode: 127,
							},
						},
					},
				}},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "Init:OOMKill",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case5 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Status: v1.PodStatus{
				InitContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Terminated: &v1.ContainerStateTerminated{
								Reason:   "OOMKill",
								ExitCode: 127,
							},
						},
					},
				}},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "Init:OOMKill",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case6 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Status: v1.PodStatus{
				InitContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{
								Reason: "ContainerCreating",
							},
						},
					},
				}},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "Init:ContainerCreating",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case7 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Spec: v1.PodSpec{
				InitContainers: []v1.Container{
					{Name: "test"},
				}},
			Status: v1.PodStatus{
				InitContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{},
						},
					},
				}},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "Init:0/1",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case8 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{
								Reason: "ContainerCreating",
							},
						},
					},
				},
			},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "ContainerCreating",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case9 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Terminated: &v1.ContainerStateTerminated{
								Reason: "OOMKilled",
							},
						},
					},
				},
			},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "OOMKilled",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case10 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Terminated: &v1.ContainerStateTerminated{
								Signal: 2,
							},
						},
					},
				},
			},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "Signal:2",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case11 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Terminated: &v1.ContainerStateTerminated{
								ExitCode: 127,
							},
						},
					},
				},
			},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "ExitCode:127",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case12 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Spec: v1.PodSpec{
				Containers: []v1.Container{{
					Name: "nginx",
				}},
			},
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Running: &v1.ContainerStateRunning{
								StartedAt: metav1.Now(),
							},
						},
						Ready: true,
					},
				},
				Phase: "Running",
			},
		},
		res: map[string]interface{}{
			"Ready":    "1/1",
			"Status":   "Running",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case13 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Spec: v1.PodSpec{
				Containers: []v1.Container{{
					Name: "nginx",
				}},
			},
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Running: &v1.ContainerStateRunning{
								StartedAt: metav1.Now(),
							},
						},
						Ready: true,
					},
				},
				Phase: "Completed",
				Conditions: []v1.PodCondition{
					{
						Type:   v1.PodReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		},
		res: map[string]interface{}{
			"Ready":    "1/1",
			"Status":   "Running",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case14 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			Spec: v1.PodSpec{
				Containers: []v1.Container{{
					Name: "nginx",
				}},
			},
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Running: &v1.ContainerStateRunning{
								StartedAt: metav1.Now(),
							},
						},
						Ready: true,
					},
				},
				Phase: "Completed",
			},
		},
		res: map[string]interface{}{
			"Ready":    "1/1",
			"Status":   "NotReady",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	case15 := testCase{
		pod: v1.Pod{TypeMeta: typeMeta,
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
		},
		res: map[string]interface{}{
			"Ready":    "0/0",
			"Status":   "Terminating",
			"Restarts": 0,
			"Age":      "<unknown>",
		},
	}

	testCases := map[string]testCase{
		"pod1":  case1,
		"pod2":  case2,
		"pod3":  case3,
		"pod4":  case4,
		"pod5":  case5,
		"pod6":  case6,
		"pod7":  case7,
		"pod8":  case8,
		"pod9":  case9,
		"pod10": case10,
		"pod11": case11,
		"pod12": case12,
		"pod13": case13,
		"pod14": case14,
		"pod15": case15,
	}

	for _, t2 := range testCases {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(t2.pod.DeepCopy())
		assert.NoError(t, err)
		res, err := additionalInfo(unstructured.Unstructured{Object: u})
		assert.NoError(t, err)
		assert.Equal(t, t2.res, res)
	}
}

func TestDeploymentAdditionalInfo(t *testing.T) {
	typeMeta := metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}

	type testCase struct {
		Deployment v12.Deployment
		res        map[string]interface{}
	}

	case1 := testCase{
		Deployment: v12.Deployment{
			TypeMeta: typeMeta,
			Spec:     v12.DeploymentSpec{Replicas: pointer.Int32(1)},
			Status: v12.DeploymentStatus{
				ReadyReplicas:     1,
				UpdatedReplicas:   1,
				AvailableReplicas: 1,
			},
		},
		res: map[string]interface{}{
			"Ready":     "1/1",
			"Update":    int32(1),
			"Available": int32(1),
			"Age":       "<unknown>",
		},
	}

	testCases := map[string]testCase{
		"deployment1": case1,
	}

	for _, t2 := range testCases {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(t2.Deployment.DeepCopy())
		assert.NoError(t, err)
		res, err := additionalInfo(unstructured.Unstructured{Object: u})
		assert.NoError(t, err)
		assert.Equal(t, t2.res, res)
	}
}

func TestStatefulSetAdditionalInfo(t *testing.T) {
	typeMeta := metav1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSet"}

	type testCase struct {
		StatefulSet v12.StatefulSet
		res         map[string]interface{}
	}

	case1 := testCase{
		StatefulSet: v12.StatefulSet{
			TypeMeta: typeMeta,
			Spec:     v12.StatefulSetSpec{Replicas: pointer.Int32(1)},
			Status: v12.StatefulSetStatus{
				ReadyReplicas: 1,
			},
		},
		res: map[string]interface{}{
			"Ready": "1/1",
			"Age":   "<unknown>",
		},
	}

	testCases := map[string]testCase{
		"statefulSet1": case1,
	}

	for _, t2 := range testCases {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(t2.StatefulSet.DeepCopy())
		assert.NoError(t, err)
		res, err := additionalInfo(unstructured.Unstructured{Object: u})
		assert.NoError(t, err)
		assert.Equal(t, t2.res, res)
	}
}

func TestSvcAdditionalInfo(t *testing.T) {
	typeMeta := metav1.TypeMeta{APIVersion: "v1", Kind: "Service"}

	type testCase struct {
		svc v1.Service
		res map[string]interface{}
	}

	case1 := testCase{
		svc: v1.Service{TypeMeta: typeMeta, Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
		},
			Status: v1.ServiceStatus{LoadBalancer: v1.LoadBalancerStatus{Ingress: []v1.LoadBalancerIngress{{IP: "145.2.2.1"}}}}},
		res: map[string]interface{}{
			"EIP": "145.2.2.1",
		},
	}

	case2 := testCase{
		svc: v1.Service{TypeMeta: typeMeta, Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
		},
			Status: v1.ServiceStatus{}},
		res: map[string]interface{}{
			"EIP": "pending",
		},
	}

	testCases := map[string]testCase{
		"svc1": case1,
		"svc2": case2,
	}

	for _, t2 := range testCases {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(t2.svc.DeepCopy())
		assert.NoError(t, err)
		res, err := additionalInfo(unstructured.Unstructured{Object: u})
		assert.NoError(t, err)
		assert.Equal(t, t2.res, res)
	}
}

var _ = Describe("unit-test to e2e test", func() {
	deploy1 := v12.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deploy1",
			Namespace: "test-namespace",
		},
		Spec: v12.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "deploy1",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "deploy1",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "nginx",
							Name:  "nginx",
						},
					},
				},
			},
		},
	}

	deploy2 := v12.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deploy2",
			Namespace: "test-namespace",
		},
		Spec: v12.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "deploy2",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "deploy2",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "nginx",
							Name:  "nginx",
						},
					},
				},
			},
		},
	}

	rs1 := v12.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "ReplicaSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs1",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "deploy1",
			},
		},
		Spec: v12.ReplicaSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "deploy1",
					"rs":  "rs1",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "deploy1",
						"rs":  "rs1",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "nginx",
							Name:  "nginx",
						},
					},
				},
			},
		},
	}

	rs2 := v12.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "ReplicaSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs2",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "deploy1",
			},
		},
		Spec: v12.ReplicaSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "deploy1",
					"rs":  "rs2",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "deploy1",
						"rs":  "rs2",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "nginx",
							Name:  "nginx",
						},
					},
				},
			},
		},
	}

	rs3 := v12.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "ReplicaSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs3",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "deploy2",
			},
		},
		Spec: v12.ReplicaSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "deploy2",
					"rs":  "rs3",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "deploy2",
						"rs":  "rs3",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "nginx",
							Name:  "nginx",
						},
					},
				},
			},
		},
	}

	pod1 := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "deploy1",
				"rs":  "rs1",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Image: "nginx",
					Name:  "nginx",
				},
			},
		},
	}

	pod2 := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod2",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "deploy1",
				"rs":  "rs2",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Image: "nginx",
					Name:  "nginx",
				},
			},
		},
	}

	pod3 := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod3",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "deploy2",
				"rs":  "rs3",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Image: "nginx",
					Name:  "nginx",
				},
			},
		},
	}

	rs4 := v12.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "ReplicaSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs4",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "deploy3",
			},
		},
		Spec: v12.ReplicaSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "deploy3",
					"rs":  "rs4",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "deploy3",
						"rs":  "rs4",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "nginx",
							Name:  "nginx",
						},
					},
				},
			},
		},
	}

	pod4 := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod4",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "deploy3",
				"rs":  "rs4",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Image: "nginx",
					Name:  "nginx",
				},
			},
		},
	}
	pod5 := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod5",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"job-name": "job2",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Image: "nginx",
					Name:  "nginx",
				},
			},
		},
	}

	job1 := batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job1",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "cronJob1",
			},
		},
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					RestartPolicy: "OnFailure",
					Containers: []v1.Container{
						{
							Image: "nginx",
							Name:  "nginx",
						},
					},
				},
			},
		},
	}

	manualSelector := true
	job2 := batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job2",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"job-name": "job2",
			},
		},
		Spec: batchv1.JobSpec{
			ManualSelector: &manualSelector,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"job-name": "job2",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"job-name": "job2",
					},
				},
				Spec: v1.PodSpec{
					RestartPolicy: "OnFailure",
					Containers: []v1.Container{
						{
							Image: "nginx",
							Name:  "nginx",
						},
					},
				},
			},
		},
	}

	cronJob1 := batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "CronJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cronjob1",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "cronJob1",
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "* * * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "cronJob1",
					},
				},
				Spec: batchv1.JobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							RestartPolicy: "OnFailure",
							Containers: []v1.Container{
								{
									Image: "nginx",
									Name:  "nginx",
								},
							},
						},
					},
				},
			},
		},
	}

	var objectList []client.Object
	objectList = append(objectList, &deploy1, &deploy1, &rs1, &rs2, &rs3, &rs4, &pod1, &pod2, &pod3, &rs4, &pod4, &pod5, &job1, &job2, &cronJob1)
	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, deploy1.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, deploy2.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, rs1.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, rs2.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, rs3.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, pod1.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, pod2.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, pod3.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		cRs4 := rs4.DeepCopy()
		Expect(k8sClient.Create(ctx, cRs4)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		cPod4 := pod4.DeepCopy()
		cPod4.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       cRs4.Name,
				UID:        cRs4.UID,
			},
		})
		Expect(k8sClient.Create(ctx, cPod4)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		cCronJob1 := cronJob1.DeepCopy()
		Expect(k8sClient.Create(ctx, cCronJob1)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		cJob1 := job1.DeepCopy()
		Expect(k8sClient.Create(ctx, cJob1)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		cJob2 := job2.DeepCopy()
		Expect(k8sClient.Create(ctx, cJob2)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		cPod5 := pod5.DeepCopy()
		Expect(k8sClient.Create(ctx, cPod5)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		for _, object := range objectList {
			Expect(k8sClient.Delete(ctx, object))
		}
	})

	It("test fetchObjectWithResourceTreeNode func", func() {
		rtn := types.ResourceTreeNode{
			Name:       "deploy1",
			Namespace:  "test-namespace",
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		}
		u, err := fetchObjectWithResourceTreeNode(ctx, "", k8sClient, rtn)
		Expect(err).Should(BeNil())
		Expect(u).ShouldNot(BeNil())
		Expect(u.GetName()).Should(BeEquivalentTo("deploy1"))
	})

	It("test list item by rule", func() {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(deploy1.DeepCopy())
		Expect(err).Should(BeNil())
		items, err := listItemByRule(ctx, k8sClient, ResourceType{APIVersion: "apps/v1", Kind: "ReplicaSet"}, unstructured.Unstructured{Object: u},
			defaultWorkloadLabelListOption, nil, true)
		Expect(err).Should(BeNil())
		Expect(len(items)).Should(BeEquivalentTo(2))

		u2, err := runtime.DefaultUnstructuredConverter.ToUnstructured(deploy2.DeepCopy())
		Expect(err).Should(BeNil())
		items2, err := listItemByRule(ctx, k8sClient, ResourceType{APIVersion: "apps/v1", Kind: "ReplicaSet"}, unstructured.Unstructured{Object: u2},
			nil, defaultWorkloadLabelListOption, true)
		Expect(len(items2)).Should(BeEquivalentTo(1))

		// test use ownerReference UId to filter
		u3 := unstructured.Unstructured{}
		u3.SetNamespace(rs4.Namespace)
		u3.SetName(rs4.Name)
		u3.SetAPIVersion("apps/v1")
		u3.SetKind("ReplicaSet")
		Expect(k8sClient.Get(ctx, types2.NamespacedName{Namespace: u3.GetNamespace(), Name: u3.GetName()}, &u3))
		Expect(err).Should(BeNil())
		items3, err := listItemByRule(ctx, k8sClient, ResourceType{APIVersion: "v1", Kind: "Pod"}, u3,
			nil, nil, true)
		Expect(err).Should(BeNil())
		Expect(len(items3)).Should(BeEquivalentTo(1))

		u4 := unstructured.Unstructured{}
		u4.SetNamespace(cronJob1.Namespace)
		u4.SetName(cronJob1.Name)
		u4.SetAPIVersion("batch/v1")
		u4.SetKind("CronJob")
		Expect(k8sClient.Get(ctx, types2.NamespacedName{Namespace: u4.GetNamespace(), Name: u4.GetName()}, &u4))
		Expect(err).Should(BeNil())
		item4, err := listItemByRule(ctx, k8sClient, ResourceType{APIVersion: "batch/v1", Kind: "Job"}, u4,
			cronJobLabelListOption, nil, true)
		Expect(err).Should(BeNil())
		Expect(len(item4)).Should(BeEquivalentTo(1))
	})

	It("iterate resource", func() {
		tn, err := iterateListSubResources(ctx, "", k8sClient, types.ResourceTreeNode{
			Cluster:    "",
			Namespace:  "test-namespace",
			Name:       "deploy1",
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		}, 1, func(node types.ResourceTreeNode) bool {
			return true
		})
		Expect(err).Should(BeNil())
		Expect(len(tn)).Should(BeEquivalentTo(2))
		Expect(len(tn[0].LeafNodes)).Should(BeEquivalentTo(1))
		Expect(len(tn[1].LeafNodes)).Should(BeEquivalentTo(1))

		tn, err = iterateListSubResources(ctx, "", k8sClient, types.ResourceTreeNode{
			Cluster:    "",
			Namespace:  "test-namespace",
			Name:       "cronjob1",
			APIVersion: "batch/v1",
			Kind:       "CronJob",
		}, 1, func(node types.ResourceTreeNode) bool {
			return true
		})
		Expect(err).Should(BeNil())
		Expect(len(tn)).Should(BeEquivalentTo(1))

		tn, err = iterateListSubResources(ctx, "", k8sClient, types.ResourceTreeNode{
			Cluster:    "",
			Namespace:  "test-namespace",
			Name:       "job2",
			APIVersion: "batch/v1",
			Kind:       "Job",
		}, 1, func(node types.ResourceTreeNode) bool {
			return true
		})
		Expect(err).Should(BeNil())
		Expect(len(tn)).Should(BeEquivalentTo(1))
	})

	It("test provider handler func", func() {
		app := v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app",
				Namespace: "test-namespace",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{},
			},
		}

		rt := v1beta1.ResourceTracker{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-test-v1-namespace",
				Labels: map[string]string{
					oam.LabelAppName:      "app",
					oam.LabelAppNamespace: "test-namespace",
				},
				Annotations: map[string]string{
					oam.AnnotationPublishVersion: "v1",
				},
			},
			Spec: v1beta1.ResourceTrackerSpec{
				Type: v1beta1.ResourceTrackerTypeVersioned,
				ManagedResources: []v1beta1.ManagedResource{
					{
						ClusterObjectReference: common.ClusterObjectReference{
							Cluster: "",
							ObjectReference: v1.ObjectReference{
								APIVersion: "apps/v1",
								Kind:       "Deployment",
								Namespace:  "test-namespace",
								Name:       "deploy1",
							},
						},
						OAMObjectReference: common.OAMObjectReference{
							Component: "deploy1",
						},
					},
					{
						ClusterObjectReference: common.ClusterObjectReference{
							Cluster: "",
							ObjectReference: v1.ObjectReference{
								APIVersion: "apps/v1",
								Kind:       "Deployment",
								Namespace:  "test-namespace",
								Name:       "deploy2",
							},
						},
						OAMObjectReference: common.OAMObjectReference{
							Component: "deploy2",
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &rt)).Should(BeNil())

		prd := provider{cli: k8sClient}
		opt := `app: {
				name: "app"
				namespace: "test-namespace"
				withTree: true
			}`
		v, err := value.NewValue(opt, nil, "")
		Expect(err).Should(BeNil())
		logCtx := monitorContext.NewTraceContext(ctx, "")
		Expect(prd.ListAppliedResources(logCtx, nil, v, nil)).Should(BeNil())
		type Res struct {
			List []types.AppliedResource `json:"list"`
		}
		var res Res
		err = v.UnmarshalTo(&res)
		Expect(err).Should(BeNil())
		Expect(len(res.List)).Should(Equal(2))
	})

	It("Test not exist api don't break whole process", func() {
		notExistRuleStr := `
- parentResourceType:
    group: apps
    kind: Deployment
  childrenResourceType:
    - apiVersion: v2
      kind: Pod
`
		notExistParentResourceStr := `
- parentResourceType:
    group: badgroup
    kind: Deployment
  childrenResourceType:
    - apiVersion: v2
      kind: Pod
`
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		badRuleConfigMap := v1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Namespace: types3.DefaultKubeVelaNS, Name: "bad-rule", Labels: map[string]string{oam.LabelResourceRules: "true"}},
			Data:       map[string]string{relationshipKey: notExistRuleStr},
		}
		Expect(k8sClient.Create(ctx, &badRuleConfigMap)).Should(BeNil())

		// clear after test
		objectList = append(objectList, &badRuleConfigMap)

		notExistParentConfigMap := v1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Namespace: types3.DefaultKubeVelaNS, Name: "not-exist-parent", Labels: map[string]string{oam.LabelResourceRules: "true"}},
			Data:       map[string]string{relationshipKey: notExistParentResourceStr},
		}
		Expect(k8sClient.Create(ctx, &notExistParentConfigMap)).Should(BeNil())

		// clear after test
		objectList = append(objectList, &badRuleConfigMap)

		prd := provider{cli: k8sClient}
		opt := `app: {
				name: "app"
				namespace: "test-namespace"
				withTree: true
			}`
		v, err := value.NewValue(opt, nil, "")

		Expect(err).Should(BeNil())
		logCtx := monitorContext.NewTraceContext(ctx, "")
		Expect(prd.ListAppliedResources(logCtx, nil, v, nil)).Should(BeNil())
		type Res struct {
			List []types.AppliedResource `json:"list"`
		}
		var res Res
		err = v.UnmarshalTo(&res)
		Expect(err).Should(BeNil())
		Expect(len(res.List)).Should(Equal(2))
	})
})

var _ = Describe("test merge globalRules", func() {
	cloneSetStr := `
- parentResourceType:
    group: apps.kruise.io
    kind: CloneSet
  childrenResourceType:
    - apiVersion: v1
      kind: Pod
      defaultLabelSelector: true
    - apiVersion: apps/v1
      kind: ControllerRevision
`
	clickhouseJsonStr := `
[
  {
    "parentResourceType": {
      "group": "clickhouse.altinity.com",
      "kind": "ClickHouseInstallation"
    },
    "childrenResourceType": [
      {
        "apiVersion": "apps/v1",
        "kind": "StatefulSet"
      },
      {
        "apiVersion": "v1",
        "kind": "Service"
      }
    ]
  }
]
`
	daemonSetStr := `
- parentResourceType:
    group: apps
    kind: DaemonSet
  childrenResourceType:
    - apiVersion: apps/v1
      kind: ControllerRevision
`
	stsStr := `
- parentResourceType:
    group: apps
    kind: StatefulSet
  childrenResourceType:
    - apiVersion: v1
      kind: Pod
    - apiVersion: apps/v1
      kind: ControllerRevision
`
	missConfigedStr := `
- parentResourceType:
    group: apps
    kind: StatefulSet
childrenResourceType:
    - apiVersion: v1
      kind: Pod
    - apiVersion: apps/v1
      kind: ControllerRevision
`

	It("test merge rules", func() {
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		cloneSetConfigMap := v1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Namespace: types3.DefaultKubeVelaNS, Name: "cloneset", Labels: map[string]string{
				oam.LabelResourceRules:      "true",
				oam.LabelResourceRuleFormat: oam.ResourceTopologyFormatYAML,
			}},
			Data: map[string]string{relationshipKey: cloneSetStr},
		}
		Expect(k8sClient.Create(ctx, &cloneSetConfigMap)).Should(BeNil())

		daemonSetConfigMap := v1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Namespace: types3.DefaultKubeVelaNS, Name: "daemonset", Labels: map[string]string{
				oam.LabelResourceRules:      "true",
				oam.LabelResourceRuleFormat: oam.ResourceTopologyFormatYAML,
			}},
			Data: map[string]string{relationshipKey: daemonSetStr},
		}
		Expect(k8sClient.Create(ctx, &daemonSetConfigMap)).Should(BeNil())

		stsConfigMap := v1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Namespace: types3.DefaultKubeVelaNS, Name: "sts", Labels: map[string]string{
				oam.LabelResourceRules:      "true",
				oam.LabelResourceRuleFormat: oam.ResourceTopologyFormatYAML,
			}},
			Data: map[string]string{relationshipKey: stsStr},
		}
		Expect(k8sClient.Create(ctx, &stsConfigMap)).Should(BeNil())

		missConfigedCm := v1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Namespace: types3.DefaultKubeVelaNS, Name: "miss-configed", Labels: map[string]string{oam.LabelResourceRules: "true"}},
			Data:       map[string]string{relationshipKey: missConfigedStr},
		}
		Expect(k8sClient.Create(ctx, &missConfigedCm)).Should(BeNil())

		clickhouseJsonCm := v1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Namespace: types3.DefaultKubeVelaNS, Name: "clickhouse", Labels: map[string]string{
				oam.LabelResourceRules:      "true",
				oam.LabelResourceRuleFormat: oam.ResourceTopologyFormatJSON,
			}},
			Data: map[string]string{relationshipKey: clickhouseJsonStr},
		}
		Expect(k8sClient.Create(ctx, &clickhouseJsonCm)).Should(BeNil())

		Expect(mergeCustomRules(ctx, k8sClient)).Should(BeNil())
		childrenResources, ok := globalRule.GetRule(GroupResourceType{Group: "apps.kruise.io", Kind: "CloneSet"})
		Expect(ok).Should(BeTrue())
		Expect(childrenResources.DefaultGenListOptionFunc).Should(BeNil())
		Expect(len(*childrenResources.SubResources)).Should(BeEquivalentTo(2))

		crPod := childrenResources.SubResources.Get(ResourceType{APIVersion: "v1", Kind: "Pod"})
		Expect(crPod).ShouldNot(BeNil())
		Expect(crPod.listOptions).ShouldNot(BeNil())

		dsChildrenResources, ok := globalRule.GetRule(GroupResourceType{Group: "apps", Kind: "DaemonSet"})
		Expect(ok).Should(BeTrue())
		Expect(dsChildrenResources.DefaultGenListOptionFunc).Should(BeNil())
		Expect(len(*dsChildrenResources.SubResources)).Should(BeEquivalentTo(2))

		// with the error version
		crPod2 := dsChildrenResources.SubResources.Get(ResourceType{APIVersion: "v1", Kind: "ControllerRevision"})
		Expect(crPod2).Should(BeNil())

		crPod3 := dsChildrenResources.SubResources.Get(ResourceType{APIVersion: "v1", Kind: "Pod"})
		Expect(crPod3).ShouldNot(BeNil())
		Expect(crPod3.listOptions).ShouldNot(BeNil())

		cr := dsChildrenResources.SubResources.Get(ResourceType{APIVersion: "apps/v1", Kind: "ControllerRevision"})
		Expect(cr).ShouldNot(BeNil())
		Expect(cr.listOptions).Should(BeNil())

		stsChildrenResources, ok := globalRule.GetRule(GroupResourceType{Group: "apps", Kind: "StatefulSet"})
		Expect(ok).Should(BeTrue())
		Expect(stsChildrenResources.DefaultGenListOptionFunc).Should(BeNil())
		Expect(len(*stsChildrenResources.SubResources)).Should(BeEquivalentTo(2))
		revisionCR := stsChildrenResources.SubResources.Get(ResourceType{APIVersion: "apps/v1", Kind: "ControllerRevision"})
		Expect(revisionCR).ShouldNot(BeNil())
		Expect(revisionCR.listOptions).Should(BeNil())

		chChildrenResources, ok := globalRule.GetRule(GroupResourceType{Group: "clickhouse.altinity.com", Kind: "ClickHouseInstallation"})
		Expect(ok).Should(BeTrue())
		Expect(chChildrenResources.DefaultGenListOptionFunc).Should(BeNil())
		Expect(len(*chChildrenResources.SubResources)).Should(BeEquivalentTo(2))

		chSts := chChildrenResources.SubResources.Get(ResourceType{APIVersion: "apps/v1", Kind: "StatefulSet"})
		Expect(chSts).ShouldNot(BeNil())
		Expect(chSts.listOptions).Should(BeNil())

		chSvc := chChildrenResources.SubResources.Get(ResourceType{APIVersion: "v1", Kind: "Service"})
		Expect(chSvc).ShouldNot(BeNil())
		Expect(chSvc.listOptions).Should(BeNil())

		// clear data
		Expect(k8sClient.Delete(context.TODO(), &missConfigedCm)).Should(BeNil())
		Expect(k8sClient.Delete(context.TODO(), &stsConfigMap)).Should(BeNil())
		Expect(k8sClient.Delete(context.TODO(), &daemonSetConfigMap)).Should(BeNil())
		Expect(k8sClient.Delete(context.TODO(), &cloneSetConfigMap)).Should(BeNil())
	})
})
