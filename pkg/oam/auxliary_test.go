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

package oam

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOAMAuxiliary(t *testing.T) {
	t.Run("ClusterLabel", func(t *testing.T) {
		r := require.New(t)
		deploy := &appsv1.Deployment{}
		clusterName1 := "cluster-1"
		clusterName2 := "cluster-2"

		r.Equal("", GetCluster(deploy), "GetCluster should return empty string for new object")

		SetClusterIfEmpty(deploy, clusterName1)
		r.Equal(clusterName1, GetCluster(deploy), "SetClusterIfEmpty should set label on empty object")

		SetClusterIfEmpty(deploy, clusterName2)
		r.Equal(clusterName1, GetCluster(deploy), "SetClusterIfEmpty should not overwrite existing label")

		SetCluster(deploy, clusterName2)
		r.Equal(clusterName2, GetCluster(deploy), "SetCluster should overwrite existing label")
	})

	t.Run("VersionAnnotations", func(t *testing.T) {
		r := require.New(t)
		deploy := &appsv1.Deployment{}
		publishVersion := "v1.0.0"
		deployVersion := "app-v1"

		r.Equal("", GetPublishVersion(deploy), "GetPublishVersion should return empty string for new object")
		SetPublishVersion(deploy, publishVersion)
		r.Equal(publishVersion, GetPublishVersion(deploy))

		r.Equal("", GetDeployVersion(deploy), "GetDeployVersion should return empty string for new object")
		annotations := deploy.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[AnnotationDeployVersion] = deployVersion
		deploy.SetAnnotations(annotations)
		r.Equal(deployVersion, GetDeployVersion(deploy))
	})

	t.Run("LastAppliedTimeAnnotation", func(t *testing.T) {
		r := require.New(t)
		fixedTime := time.Now().Truncate(time.Second)
		creationTime := fixedTime.Add(-time.Hour)

		testCases := []struct {
			name         string
			annotations  map[string]string
			expectedTime time.Time
		}{
			{
				name:         "no annotation",
				annotations:  nil,
				expectedTime: creationTime,
			},
			{
				name:         "valid annotation",
				annotations:  map[string]string{AnnotationLastAppliedTime: fixedTime.Format(time.RFC3339)},
				expectedTime: fixedTime,
			},
			{
				name:         "invalid annotation",
				annotations:  map[string]string{AnnotationLastAppliedTime: "invalid-time"},
				expectedTime: creationTime,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				deploy := &appsv1.Deployment{}
				deploy.SetCreationTimestamp(metav1.NewTime(creationTime))
				deploy.SetAnnotations(tc.annotations)
				r.Equal(tc.expectedTime, GetLastAppliedTime(deploy))
			})
		}
	})

	t.Run("ControllerRequirementAnnotation", func(t *testing.T) {
		r := require.New(t)
		deploy := &appsv1.Deployment{}
		requirement := "controller-x"

		r.Equal("", GetControllerRequirement(deploy), "GetControllerRequirement should be empty for new object")

		SetControllerRequirement(deploy, requirement)
		r.Equal(requirement, GetControllerRequirement(deploy))
		r.Contains(deploy.GetAnnotations(), AnnotationControllerRequirement)

		SetControllerRequirement(deploy, "")
		r.Equal("", GetControllerRequirement(deploy), "GetControllerRequirement should be empty after setting to empty string")
		r.NotContains(deploy.GetAnnotations(), AnnotationControllerRequirement, "Annotation should be removed when set to empty")
	})
}
