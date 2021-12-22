package utils

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam/util"
	velaerr "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// CreateOrUpdateNamespace will create a namespace if not exist, it will also update a namespace if exists
// It will report an error if the labels conflict while it will override the annotations
func CreateOrUpdateNamespace(ctx context.Context, kubeClient client.Client, name string, labels, annotations map[string]string) error {
	var namespace corev1.Namespace
	err := kubeClient.Get(ctx, k8stypes.NamespacedName{Name: name}, &namespace)
	if apierrors.IsNotFound(err) {
		return kubeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
			},
			Spec: corev1.NamespaceSpec{},
		})
	}
	if err != nil {
		return err
	}
	if namespace.Labels == nil {
		namespace.Labels = map[string]string{}
	}
	// check and fill the labels
	for k, v := range labels {
		ev, ok := namespace.Labels[k]
		if ok && ev != v {
			return fmt.Errorf("%s for namespace %s, key: %s, conflicts value: %s <-> %s", velaerr.LabelConflict, name, k, ev, v)
		}
		namespace.Labels[k] = ev
	}
	util.AddAnnotations(&namespace, annotations)
	if err = kubeClient.Update(ctx, &namespace); err != nil {
		return err
	}
	return nil
}
