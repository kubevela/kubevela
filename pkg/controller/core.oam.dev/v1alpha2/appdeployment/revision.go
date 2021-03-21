package appdeployment

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam"
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

func (rd *revisionsDiff) Empty() bool {
	return len(rd.Del) == 0 && len(rd.Mod) == 0 && len(rd.Add) == 0
}

type applyOverlayFunc func(u *unstructured.Unstructured) error

func overlayReplicaFn(replicas int) applyOverlayFunc {
	return func(u *unstructured.Unstructured) error {
		return unstructured.SetNestedField(u.Object, float64(replicas), "spec", "replicas")

	}
}

func overlayComponentLabelFn(compName string) applyOverlayFunc {
	return func(u *unstructured.Unstructured) error {
		labelmap := map[string]string{
			oam.LabelAppComponent: compName,
		}
		if err := unstructured.SetNestedStringMap(u.Object, labelmap, "spec", "selector", "matchLabels"); err != nil {
			return err
		}
		if err := unstructured.SetNestedStringMap(u.Object, labelmap, "spec", "template", "metadata", "labels"); err != nil {
			return err
		}
		return nil
	}
}

func ApplyOverlayToWorkload(comps []*oamcore.Component, overlayFuncs ...applyOverlayFunc) error {
	for _, comp := range comps {
		u := &unstructured.Unstructured{}
		if err := json.Unmarshal(comp.Spec.Workload.Raw, u); err != nil {
			return nil
		}

		overlayFuncs = append(overlayFuncs, overlayComponentLabelFn(comp.Name))
		for _, fn := range overlayFuncs {
			if err := fn(u); err != nil {
				return err
			}
		}

		raw, err := json.Marshal(u)
		if err != nil {
			return err
		}
		comp.Spec.Workload = runtime.RawExtension{Raw: raw}
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

func makeService(rawCompName, ns, revName string, port int) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%d", revName, rawCompName, port),
			Namespace: ns,
			Labels: map[string]string{
				oam.LabelAppRevision:  revName,
				oam.LabelAppComponent: makeRevisionName(rawCompName, revName),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				oam.LabelAppComponent: makeRevisionName(rawCompName, revName),
			},
			Ports: []corev1.ServicePort{{Protocol: "TCP", Port: int32(port)}},
		},
	}
}
