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

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	velaclient "github.com/kubevela/pkg/controller/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	utilscommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	// RefObjectsAvailableScopeGlobal ref-objects component can refer to arbitrary objects in any cluster
	RefObjectsAvailableScopeGlobal = "global"
	// RefObjectsAvailableScopeCluster ref-objects component can only refer to objects inside the hub cluster
	RefObjectsAvailableScopeCluster = "cluster"
	// RefObjectsAvailableScopeNamespace ref-objects component can only refer to objects inside the application namespace
	RefObjectsAvailableScopeNamespace = "namespace"
)

// RefObjectsAvailableScope indicates the available scope for objects to refer
var RefObjectsAvailableScope = RefObjectsAvailableScopeGlobal

// GetLabelSelectorFromRefObjectSelector extract labelSelector from `labelSelector` first. If empty, extract from `selector`
func GetLabelSelectorFromRefObjectSelector(selector v1alpha1.ObjectReferrer) map[string]string {
	if selector.LabelSelector != nil {
		return selector.LabelSelector
	}
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.DeprecatedObjectLabelSelector) {
		return selector.DeprecatedLabelSelector
	}
	return nil
}

// GetGroupVersionKindFromRefObjectSelector extract GroupVersionKind by Resource if provided, otherwise, extract from APIVersion and Kind directly
func GetGroupVersionKindFromRefObjectSelector(mapper meta.RESTMapper, selector v1alpha1.ObjectReferrer) (schema.GroupVersionKind, error) {
	if selector.Resource != "" {
		gvks, err := mapper.KindsFor(schema.GroupVersionResource{Group: selector.Group, Resource: selector.Resource})
		if err != nil {
			return schema.GroupVersionKind{}, err
		}
		if len(gvks) == 0 {
			return schema.GroupVersionKind{}, errors.Errorf("no kind found for resource %s", selector.Resource)
		}
		return gvks[0], nil
	}
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.LegacyObjectTypeIdentifier) {
		if selector.APIVersion != "" && selector.Kind != "" {
			gv, err := schema.ParseGroupVersion(selector.APIVersion)
			if err != nil {
				return schema.GroupVersionKind{}, errors.Wrapf(err, "invalid APIVersion")
			}
			return gv.WithKind(selector.Kind), nil
		}
		return schema.GroupVersionKind{}, errors.Errorf("neither resource or apiVersion/kind is set for referring objects")
	}
	return schema.GroupVersionKind{}, errors.Errorf("resource is not set and legacy object type identifier is disabled for referring objects")
}

// ValidateRefObjectSelector validate if exclusive fields are set for the selector
func ValidateRefObjectSelector(selector v1alpha1.ObjectReferrer) error {
	labelSelector := GetLabelSelectorFromRefObjectSelector(selector)
	if labelSelector != nil && selector.Name != "" {
		return errors.Errorf("invalid object selector for ref-objects, name and labelSelector cannot be both set")
	}
	return nil
}

// ClearRefObjectForDispatch reset the objects for dispatch
func ClearRefObjectForDispatch(un *unstructured.Unstructured) {
	un.SetResourceVersion("")
	un.SetGeneration(0)
	un.SetOwnerReferences(nil)
	un.SetDeletionTimestamp(nil)
	un.SetManagedFields(nil)
	un.SetUID("")
	unstructured.RemoveNestedField(un.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(un.Object, "status")
	// TODO(somefive): make the following logic more generalizable
	if un.GetKind() == "Service" && un.GetAPIVersion() == "v1" {
		if clusterIP, exist, _ := unstructured.NestedString(un.Object, "spec", "clusterIP"); exist && clusterIP != corev1.ClusterIPNone {
			unstructured.RemoveNestedField(un.Object, "spec", "clusterIP")
			unstructured.RemoveNestedField(un.Object, "spec", "clusterIPs")
		}
	}
}

// SelectRefObjectsForDispatch select objects by selector from kubernetes
func SelectRefObjectsForDispatch(ctx context.Context, cli client.Client, appNs string, compName string, selector v1alpha1.ObjectReferrer) (objs []*unstructured.Unstructured, err error) {
	if err = ValidateRefObjectSelector(selector); err != nil {
		return nil, err
	}
	labelSelector := GetLabelSelectorFromRefObjectSelector(selector)
	ns := appNs
	if selector.Namespace != "" {
		if RefObjectsAvailableScope == RefObjectsAvailableScopeNamespace {
			return nil, errors.Errorf("cannot refer to objects outside the application's namespace")
		}
		ns = selector.Namespace
	}
	if selector.Cluster != "" && selector.Cluster != multicluster.ClusterLocalName {
		if RefObjectsAvailableScope != RefObjectsAvailableScopeGlobal {
			return nil, errors.Errorf("cannot refer to objects outside control plane")
		}
		ctx = multicluster.ContextWithClusterName(ctx, selector.Cluster)
	}
	gvk, err := GetGroupVersionKindFromRefObjectSelector(cli.RESTMapper(), selector)
	if err != nil {
		return nil, err
	}
	isNamespaced, err := IsGroupVersionKindNamespaceScoped(cli.RESTMapper(), gvk)
	if err != nil {
		return nil, err
	}
	if selector.Name == "" && labelSelector != nil {
		uns := &unstructured.UnstructuredList{}
		uns.SetGroupVersionKind(gvk)
		opts := []client.ListOption{client.MatchingLabels(labelSelector)}
		if isNamespaced {
			opts = append(opts, client.InNamespace(ns))
		}
		if err = cli.List(ctx, uns, opts...); err != nil {
			return nil, errors.Wrapf(err, "failed to load ref object %s with selector", gvk.Kind)
		}
		for _, _un := range uns.Items {
			objs = append(objs, _un.DeepCopy())
		}
	} else {
		un := &unstructured.Unstructured{}
		un.SetGroupVersionKind(gvk)
		un.SetName(selector.Name)
		if isNamespaced {
			un.SetNamespace(ns)
		}
		if selector.Name == "" {
			un.SetName(compName)
		}
		if err := cli.Get(ctx, client.ObjectKeyFromObject(un), un); err != nil {
			return nil, errors.Wrapf(err, "failed to load ref object %s %s/%s", un.GetKind(), un.GetNamespace(), un.GetName())
		}
		objs = append(objs, un)
	}
	for _, obj := range objs {
		ClearRefObjectForDispatch(obj)
	}
	return objs, nil
}

// ReferredObjectsDelegatingClient delegate client get/list function by retrieving ref-objects from existing objects
func ReferredObjectsDelegatingClient(cli client.Client, objs []*unstructured.Unstructured) client.Client {
	objs = utilscommon.FilterObjectsByCondition(objs, func(obj *unstructured.Unstructured) bool {
		return obj.GetAnnotations() == nil || obj.GetAnnotations()[oam.AnnotationResourceURL] == ""
	})
	return velaclient.DelegatingHandlerClient{
		Client: cli,
		Getter: func(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			un, ok := obj.(*unstructured.Unstructured)
			if !ok {
				return errors.Errorf("ReferredObjectsDelegatingClient does not support non-unstructured type")
			}
			gvk := un.GroupVersionKind()
			for _, _un := range objs {
				if gvk == _un.GroupVersionKind() && key == client.ObjectKeyFromObject(_un) {
					_un.DeepCopyInto(un)
					return nil
				}
			}
			return apierrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, un.GetName())
		},
		Lister: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			uns, ok := list.(*unstructured.UnstructuredList)
			if !ok {
				return errors.Errorf("ReferredObjectsDelegatingClient does not support non-unstructured type")
			}
			gvk := uns.GroupVersionKind()
			gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")
			listOpts := &client.ListOptions{}
			for _, opt := range opts {
				opt.ApplyToList(listOpts)
			}
			for _, _un := range objs {
				if gvk != _un.GroupVersionKind() {
					continue
				}
				if listOpts.Namespace != "" && listOpts.Namespace != _un.GetNamespace() {
					continue
				}
				if listOpts.LabelSelector != nil && !listOpts.LabelSelector.Matches(labels.Set(_un.GetLabels())) {
					continue
				}
				uns.Items = append(uns.Items, *_un)
			}
			return nil
		},
	}
}

// AppendUnstructuredObjects add new objects into object list if not exists
func AppendUnstructuredObjects(objs []*unstructured.Unstructured, newObjs ...*unstructured.Unstructured) []*unstructured.Unstructured {
	for _, newObj := range newObjs {
		idx := -1
		for i, oldObj := range objs {
			if oldObj.GroupVersionKind() == newObj.GroupVersionKind() && client.ObjectKeyFromObject(oldObj) == client.ObjectKeyFromObject(newObj) {
				idx = i
				break
			}
		}
		if idx >= 0 {
			objs[idx] = newObj
		} else {
			objs = append(objs, newObj)
		}
	}
	return objs
}

// ConvertUnstructuredsToReferredObjects convert unstructured objects into ReferredObjects
func ConvertUnstructuredsToReferredObjects(uns []*unstructured.Unstructured) (refObjs []common.ReferredObject, err error) {
	for _, obj := range uns {
		bs, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		refObjs = append(refObjs, common.ReferredObject{RawExtension: runtime.RawExtension{Raw: bs}})
	}
	return refObjs, nil
}

// IsGroupVersionKindNamespaceScoped check if the target GroupVersionKind is namespace scoped resource
func IsGroupVersionKindNamespaceScoped(mapper meta.RESTMapper, gvk schema.GroupVersionKind) (bool, error) {
	mappings, err := mapper.RESTMappings(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, err
	}
	if len(mappings) == 0 {
		return false, fmt.Errorf("unable to fund the mappings for gvk %s", gvk)
	}
	return mappings[0].Scope.Name() == meta.RESTScopeNameNamespace, nil
}
