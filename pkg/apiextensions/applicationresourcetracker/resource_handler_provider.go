/*
Copyright 2022 The KubeVela Authors.

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

package applicationresourcetracker

import (
	"context"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	apirest "k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/rest"
	builderrest "sigs.k8s.io/apiserver-runtime/pkg/builder/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/apiextensions.core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// NewResourceHandlerProvider resource handler provider for handling ApplicationResourceTracker requests
func NewResourceHandlerProvider(cfg *rest.Config) builderrest.ResourceHandlerProvider {
	return func(s *runtime.Scheme, g generic.RESTOptionsGetter) (apirest.Storage, error) {
		cli, err := client.New(cfg, client.Options{Scheme: common.Scheme})
		if err != nil {
			return nil, err
		}
		return &storage{
			cli:            cli,
			TableConvertor: apirest.NewDefaultTableConvertor(v1alpha1.ApplicationResourceTrackerGroupResource),
		}, nil
	}
}

type storage struct {
	cli client.Client
	apirest.TableConvertor
}

// New returns an empty object that can be used with Create and Update after request data has been put into it.
func (s *storage) New() runtime.Object {
	return new(v1alpha1.ApplicationResourceTracker)
}

// NamespaceScoped returns true if the storage is namespaced
func (s *storage) NamespaceScoped() bool {
	return true
}

// ShortNames delivers a list of short names for a resource.
func (s *storage) ShortNames() []string {
	return []string{"apprt"}
}

// Get finds a resource in the storage by name and returns it.
func (s *storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	rt := v1beta1.ResourceTracker{}
	if err := s.cli.Get(ctx, types.NamespacedName{Name: name + "-" + request.NamespaceValue(ctx)}, &rt); err != nil {
		return nil, err
	}
	appRt := ConvertRT2AppRT(rt)
	return &appRt, nil
}

// NewList returns an empty object that can be used with the List call.
func (s *storage) NewList() runtime.Object {
	return &v1alpha1.ApplicationResourceTrackerList{}
}

// List selects resources in the storage which match to the selector. 'options' can be nil.
func (s *storage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	rts := v1beta1.ResourceTrackerList{}
	sel := matchingLabelsSelector{namespace: request.NamespaceValue(ctx)}
	if options != nil {
		sel.selector = options.LabelSelector
	}
	if err := s.cli.List(ctx, &rts, sel); err != nil {
		return nil, err
	}
	appRts := v1alpha1.ApplicationResourceTrackerList{}
	for _, rt := range rts.Items {
		appRts.Items = append(appRts.Items, ConvertRT2AppRT(rt))
	}
	return &appRts, nil
}

type matchingLabelsSelector struct {
	selector  labels.Selector
	namespace string
}

// ApplyToList applies this configuration to the given list options.
func (m matchingLabelsSelector) ApplyToList(opts *client.ListOptions) {
	opts.LabelSelector = m.selector
	if opts.LabelSelector == nil {
		opts.LabelSelector = labels.NewSelector()
	}
	if m.namespace != "" {
		sel := labels.SelectorFromValidatedSet(map[string]string{oam.LabelAppNamespace: m.namespace})
		r, _ := sel.Requirements()
		opts.LabelSelector = opts.LabelSelector.Add(r...)
	}
}
