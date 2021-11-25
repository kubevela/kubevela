/*
 Copyright 2021. The KubeVela Authors.

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

package dispatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestIsTrackedResources(t *testing.T) {
	testcases := []struct {
		oldRT  *v1beta1.ResourceTracker
		newRT  *v1beta1.ResourceTracker
		expect bool
	}{{
		oldRT: &v1beta1.ResourceTracker{
			Status: v1beta1.ResourceTrackerStatus{
				TrackedResources: []common.ClusterObjectReference{{
					ObjectReference: corev1.ObjectReference{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
						Name:       "test",
						Namespace:  "default",
					},
				}, {
					ObjectReference: corev1.ObjectReference{
						Kind:       "Pod",
						APIVersion: "v1",
						Name:       "test",
						Namespace:  "default",
					},
				}},
			},
		},
		newRT: &v1beta1.ResourceTracker{
			Status: v1beta1.ResourceTrackerStatus{
				TrackedResources: []common.ClusterObjectReference{{
					ObjectReference: corev1.ObjectReference{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
						Name:       "test",
						Namespace:  "default",
					},
				}, {
					ObjectReference: corev1.ObjectReference{
						Kind:       "Pod",
						APIVersion: "v1",
						Name:       "test",
						Namespace:  "default",
					},
				}},
			},
		},
		expect: false,
	}, {
		oldRT: &v1beta1.ResourceTracker{
			Status: v1beta1.ResourceTrackerStatus{
				TrackedResources: []common.ClusterObjectReference{{
					ObjectReference: corev1.ObjectReference{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
						Name:       "test",
						Namespace:  "default",
					},
				}, {
					ObjectReference: corev1.ObjectReference{
						Kind:       "Pod",
						APIVersion: "v1",
						Name:       "hello",
						Namespace:  "default",
					},
				}},
			},
		},
		newRT: &v1beta1.ResourceTracker{
			Status: v1beta1.ResourceTrackerStatus{
				TrackedResources: []common.ClusterObjectReference{{
					ObjectReference: corev1.ObjectReference{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
						Name:       "test",
						Namespace:  "default",
					},
				}, {
					ObjectReference: corev1.ObjectReference{
						Kind:       "Pod",
						APIVersion: "v1",
						Name:       "test",
						Namespace:  "default",
					},
				}},
			},
		},
		expect: true,
	}, {
		oldRT: &v1beta1.ResourceTracker{},
		newRT: &v1beta1.ResourceTracker{
			Status: v1beta1.ResourceTrackerStatus{
				TrackedResources: []common.ClusterObjectReference{{
					ObjectReference: corev1.ObjectReference{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
						Name:       "test",
						Namespace:  "default",
					},
				}, {
					ObjectReference: corev1.ObjectReference{
						Kind:       "Pod",
						APIVersion: "v1",
						Name:       "test",
						Namespace:  "default",
					},
				}},
			},
		},
		expect: false,
	}, {
		oldRT: &v1beta1.ResourceTracker{
			Status: v1beta1.ResourceTrackerStatus{
				TrackedResources: []common.ClusterObjectReference{{
					ObjectReference: corev1.ObjectReference{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
						Name:       "test",
						Namespace:  "default",
					},
				}, {
					ObjectReference: corev1.ObjectReference{
						Kind:       "Pod",
						APIVersion: "v1",
						Name:       "test",
						Namespace:  "default",
					},
				}},
			},
		},
		newRT:  &v1beta1.ResourceTracker{},
		expect: true,
	}}

	for _, testcase := range testcases {
		assert.Equal(t, testcase.expect, isTrackedResources(testcase.oldRT, testcase.newRT))
	}
}
