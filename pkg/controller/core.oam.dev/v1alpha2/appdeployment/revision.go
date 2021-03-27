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

package appdeployment

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

type revisionsDiff struct {
	Del []*revision
	Mod []*revision
	Add []*revision

	Unchanged []*revision
}

type revision struct {
	RevisionName string

	// Empty string indicates self cluster
	ClusterName string

	Replicas int
}

func newRevision(rev, cluster string, replica int) *revision {
	return &revision{
		RevisionName: rev,
		ClusterName:  cluster,
		Replicas:     replica,
	}
}

func (rd *revisionsDiff) Empty() bool {
	return len(rd.Del) == 0 && len(rd.Mod) == 0 && len(rd.Add) == 0
}

type applyOverlayFunc func(u *unstructured.Unstructured) error

func overlayReplica(replicas int) applyOverlayFunc {
	return func(u *unstructured.Unstructured) error {
		return unstructured.SetNestedField(u.Object, float64(replicas), "spec", "replicas")

	}
}

func overlayLabels(appd, revName string) applyOverlayFunc {
	return func(u *unstructured.Unstructured) error {
		util.AddLabels(u, map[string]string{
			oam.LabelAppDeployment: appd,
			oam.LabelAppRevision:   revName,
		})
		return nil
	}
}

func applyOverlayToWorkload(workloads []*workload, overlayFuncs ...applyOverlayFunc) error {

	iterateThruFns := func(u *unstructured.Unstructured) error {
		for _, fn := range overlayFuncs {
			if err := fn(u); err != nil {
				return err
			}
		}
		return nil
	}

	for _, wl := range workloads {
		if err := iterateThruFns(wl.Object); err != nil {
			return err
		}
		for _, tr := range wl.traits {
			if err := iterateThruFns(tr.Object); err != nil {
				return err
			}
		}
	}
	return nil
}

func makePlacement(revisions []*revision) []oamcore.PlacementStatus {
	r := make([]oamcore.PlacementStatus, 0)
	m := make(map[string][]oamcore.ClusterPlacementStatus)
	for _, rev := range revisions {
		s := oamcore.ClusterPlacementStatus{
			ClusterName: rev.ClusterName,
			Replicas:    rev.Replicas,
		}
		m[rev.RevisionName] = append(m[rev.RevisionName], s)
	}
	for name, clusters := range m {
		p := oamcore.PlacementStatus{
			RevisionName: name,
			Clusters:     clusters,
		}
		r = append(r, p)
	}
	return r
}

func makeService(compName, ns, revName string, port int) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%d", revName, compName, port),
			Namespace: ns,
			Labels: map[string]string{
				oam.LabelAppRevision:  revName,
				oam.LabelAppComponent: compName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				oam.LabelAppComponent: compName,
			},
			Ports: []corev1.ServicePort{{Protocol: "TCP", Port: int32(port)}},
		},
	}
}

func makeRevisionName(name, revision string) string {
	splits := strings.Split(revision, "-")
	return fmt.Sprintf("%s-%s", name, splits[len(splits)-1])
}
