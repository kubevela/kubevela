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
	"fmt"
	"testing"

	types3 "github.com/oam-dev/kubevela/apis/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/oam"

	types2 "k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/pkg/oam/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/stretchr/testify/assert"

	v12 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"

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
	for d, s := range testCases {
		fmt.Println(d)
		rs := s.input.DeepCopy()
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&rs)
		assert.NoError(t, err)
		res, err := checkReplicaSetStatus(unstructured.Unstructured{Object: u})
		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, s.res, *res)
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
	dls, err := deploy2RsLabelListOption(unstructured.Unstructured{Object: du})
	assert.NoError(t, err)
	assert.Equal(t, listOption, dls)

	rs := v12.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}, Spec: v12.ReplicaSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"testKey": "testVal"}}}}
	rsu, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&rs)
	assert.NoError(t, err)
	assert.NotNil(t, du)
	rsls, err := rs2PodLabelListOption(unstructured.Unstructured{Object: rsu})
	assert.NoError(t, err)
	assert.Equal(t, listOption, rsls)

	sts := v12.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}, Spec: v12.StatefulSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"testKey": "testVal"}}}}
	stsu, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&sts)
	assert.NoError(t, err)
	assert.NotNil(t, stsu)
	stsls, err := statefulSet2PodListOption(unstructured.Unstructured{Object: stsu})
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

	var objectList []client.Object
	objectList = append(objectList, &deploy1, &deploy1, &rs1, &rs2, &rs3, &rs4, &pod1, &pod2, &pod3, &rs4, &pod4)
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
			deploy2RsLabelListOption, nil)
		Expect(err).Should(BeNil())
		Expect(len(items)).Should(BeEquivalentTo(2))

		u2, err := runtime.DefaultUnstructuredConverter.ToUnstructured(deploy2.DeepCopy())
		Expect(err).Should(BeNil())
		items2, err := listItemByRule(ctx, k8sClient, ResourceType{APIVersion: "apps/v1", Kind: "ReplicaSet"}, unstructured.Unstructured{Object: u2},
			nil, deploy2RsLabelListOption)
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
			nil, nil)
		Expect(err).Should(BeNil())
		Expect(len(items3)).Should(BeEquivalentTo(1))
	})

	It("iterate resource", func() {
		tn, err := iteratorChildResources(ctx, "", k8sClient, types.ResourceTreeNode{
			Cluster:    "",
			Namespace:  "test-namespace",
			Name:       "deploy1",
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		}, 1)
		Expect(err).Should(BeNil())
		Expect(len(tn)).Should(BeEquivalentTo(2))
		Expect(len(tn[0].LeafNodes)).Should(BeEquivalentTo(1))
		Expect(len(tn[1].LeafNodes)).Should(BeEquivalentTo(1))
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
			}`
		v, err := value.NewValue(opt, nil, "")
		Expect(err).Should(BeNil())
		Expect(prd.GetApplicationResourceTree(nil, v, nil)).Should(BeNil())
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
    - apiVersion: apps/v1
      kind: ControllerRevision
`
	daemonSetStr := `
- parentResourceType:
    group: apps
    kind: DaemonSet
  childrenResourceType:
    - apiVersion: v1
      kind: Pod
    - apiVersion: apps/v1
      kind: ControllerRevision
`

	It("test merge rules", func() {
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		cloneSetConfigMap := v1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Namespace: types3.DefaultKubeVelaNS, Name: "cloneset", Labels: map[string]string{oam.LabelResourceRules: "true"}},
			Data:       map[string]string{relationshipKey: cloneSetStr},
		}
		Expect(k8sClient.Create(ctx, &cloneSetConfigMap)).Should(BeNil())

		daemonSetConfigMap := v1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Namespace: types3.DefaultKubeVelaNS, Name: "daemonset", Labels: map[string]string{oam.LabelResourceRules: "true"}},
			Data:       map[string]string{relationshipKey: daemonSetStr},
		}
		Expect(k8sClient.Create(ctx, &daemonSetConfigMap)).Should(BeNil())

		Expect(mergeCustomRules(ctx, k8sClient)).Should(BeNil())
		childrenResources, ok := globalRule[GroupResourceType{Group: "apps.kruise.io", Kind: "CloneSet"}]
		Expect(ok).Should(BeTrue())
		Expect(childrenResources.DefaultGenListOptionFunc).Should(BeNil())
		Expect(len(childrenResources.CareResource)).Should(BeEquivalentTo(2))
		specifyFunc, ok := childrenResources.CareResource[ResourceType{APIVersion: "v1", Kind: "Pod"}]
		Expect(ok).Should(BeTrue())
		Expect(specifyFunc).Should(BeNil())

		dsChildrenResources, ok := globalRule[GroupResourceType{Group: "apps", Kind: "DaemonSet"}]
		//rule := globalRule
		//fmt.Println(rule)
		Expect(ok).Should(BeTrue())
		Expect(childrenResources.DefaultGenListOptionFunc).Should(BeNil())
		Expect(len(dsChildrenResources.CareResource)).Should(BeEquivalentTo(2))
		dsSpecifyFunc, ok := childrenResources.CareResource[ResourceType{APIVersion: "v1", Kind: "Pod"}]
		Expect(ok).Should(BeTrue())
		Expect(dsSpecifyFunc).Should(BeNil())
		crSpecifyFunc, ok := childrenResources.CareResource[ResourceType{APIVersion: "apps/v1", Kind: "ControllerRevision"}]
		Expect(ok).Should(BeTrue())
		Expect(crSpecifyFunc).Should(BeNil())

		Expect(k8sClient.Delete(ctx, &cloneSetConfigMap)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, &daemonSetConfigMap)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}})).Should(BeNil())
	})
})
