package runtime

import (
	"context"

	"github.com/pkg/errors"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
)

var (
	// Scheme defines the default KubeVela schema
	Scheme = k8sruntime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = crdv1.AddToScheme(Scheme)
	_ = core.AddToScheme(Scheme)
	// +kubebuilder:scaffold:scheme
}

// Get get method for kubernetes resource
func Get(c client.Client, resource k8sruntime.Object, result interface{}, name, namespace string) error {
	fun := func(gvk schema.GroupVersionKind, c client.Client) (map[string]interface{}, error) {
		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}
		obj, err := get(context.Background(), c, key, gvk)
		if err != nil {
			return nil, err
		}

		return obj.(*unstructured.Unstructured).UnstructuredContent(), nil
	}

	return getUnstructuredObj(c, resource, result, fun)
}

// List list method for kubernetes resource
func List(c client.Client, options client.ListOption, resource k8sruntime.Object, result interface{}) error {
	fun := func(gvk schema.GroupVersionKind, c client.Client) (map[string]interface{}, error) {
		var obj k8sruntime.Object
		obj, err := list(context.Background(), c, gvk, options)
		if err != nil {
			return nil, errors.Wrap(err, "fail to list resource")
		}

		return obj.(*unstructured.UnstructuredList).UnstructuredContent(), nil
	}

	return getUnstructuredObj(c, resource, result, fun)
}

func list(ctx context.Context, a client.Client, gvk schema.GroupVersionKind, listOptions client.ListOption) (k8sruntime.Object, error) {
	u := &unstructured.UnstructuredList{}
	u.SetGroupVersionKind(gvk)
	if err := a.List(ctx, u, listOptions); err != nil {
		return nil, errors.Wrap(err, "fail to list resource")
	}

	return u, nil
}

func get(ctx context.Context, c client.Client, namespaceName types.NamespacedName, groupVersion schema.GroupVersionKind) (k8sruntime.Object, error) {
	existing := &unstructured.Unstructured{}
	existing.GetObjectKind().SetGroupVersionKind(groupVersion)
	if err := c.Get(ctx, namespaceName, existing); err != nil {
		return nil, errors.Wrap(err, "fail to get resource")
	}

	return existing, nil
}

func getUnstructuredObj(c client.Client, resource k8sruntime.Object, result interface{},
	hook func(schema.GroupVersionKind, client.Client) (map[string]interface{}, error)) error {
	gvk, err := apiutil.GVKForObject(resource, Scheme)
	if err != nil {
		return errors.Wrap(err, "fail to get resource gvk")
	}

	obj, err := hook(gvk, c)
	if err != nil {
		return errors.Wrap(err, "fail to get resource object")
	}

	if err := k8sruntime.DefaultUnstructuredConverter.FromUnstructured(obj, result); err != nil {
		return errors.Wrap(err, "fail to convert unstructured object to result")
	}

	return nil
}
