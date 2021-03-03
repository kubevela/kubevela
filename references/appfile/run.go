package appfile

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Run will deploy OAM objects and other assistant K8s Objects including ConfigMap, OAM Scope Custom Resource.
func Run(ctx context.Context, client client.Client, app *v1alpha2.Application, assistantObjects []oam.Object) error {
	if app != nil {
		assistantObjects = append(assistantObjects, app)
	}
	return CreateOrUpdateObjects(ctx, client, assistantObjects)
}

// CreateOrUpdateObjects will create or update all scopes
func CreateOrUpdateObjects(ctx context.Context, client client.Client, objects []oam.Object) error {
	for _, obj := range objects {
		key := ctypes.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
		err := client.Get(ctx, key, u)
		if err == nil {
			if obj.GetResourceVersion() != u.GetResourceVersion() {
				obj.SetResourceVersion(u.GetResourceVersion())
				return client.Update(ctx, obj)
			}
		}
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err = client.Create(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}
