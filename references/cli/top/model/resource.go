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

package model

import (
	"bytes"
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/printers"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
)

// ResourceList an abstract kinds of resource list which can convert it to data of view in the form of table
type ResourceList interface {
	// Header generate header of table in resource view
	Header() []string
	// Body generate body of table in resource view
	Body() [][]string
}

// Resource info used by GVR
type Resource struct {
	Kind      string
	Name      string
	Namespace string
	Cluster   string
}

// GVR is Group, Version, Resource
type GVR struct {
	GV string
	R  Resource
}

func collectResource(ctx context.Context, c client.Client, opt query.Option) ([]unstructured.Unstructured, error) {
	app := new(v1beta1.Application)
	appKey := client.ObjectKey{Name: opt.Name, Namespace: opt.Namespace}
	if err := c.Get(context.Background(), appKey, app); err != nil {
		return nil, err
	}
	collector := query.NewAppCollector(c, opt)
	appResList, err := collector.ListApplicationResources(context.Background(), app)
	if err != nil {
		return nil, err
	}
	var resources = make([]unstructured.Unstructured, 0)
	for _, res := range appResList {
		if res.ResourceTree != nil {
			resources = append(resources, sonLeafResource(*res, res.ResourceTree, opt.Filter.Kind, opt.Filter.APIVersion)...)
		}
		if (opt.Filter.Kind == "" && opt.Filter.APIVersion == "") || (res.Kind == opt.Filter.Kind && res.APIVersion == opt.Filter.APIVersion) {
			var object unstructured.Unstructured
			object.SetAPIVersion(opt.Filter.APIVersion)
			object.SetKind(opt.Filter.Kind)
			if err := c.Get(ctx, apimachinerytypes.NamespacedName{Namespace: res.Namespace, Name: res.Name}, &object); err == nil {
				resources = append(resources, object)
			}
		}
	}
	return resources, nil
}

func sonLeafResource(res querytypes.AppliedResource, node *querytypes.ResourceTreeNode, kind string, apiVersion string) []unstructured.Unstructured {
	objects := make([]unstructured.Unstructured, 0)
	if node.LeafNodes != nil {
		for i := 0; i < len(node.LeafNodes); i++ {
			objects = append(objects, sonLeafResource(res, node.LeafNodes[i], kind, apiVersion)...)
		}
	}
	if (kind == "" && apiVersion == "") || (node.Kind == kind && node.APIVersion == apiVersion) {
		objects = append(objects, node.Object)
	}
	return objects
}

// GetResourceObject get the resource object refer to the GVR data
func GetResourceObject(c client.Client, gvr *GVR) (runtime.Object, error) {
	obj := new(unstructured.Unstructured)
	obj.SetAPIVersion(gvr.GV)
	obj.SetKind(gvr.R.Kind)
	key := client.ObjectKey{
		Name:      gvr.R.Name,
		Namespace: gvr.R.Namespace,
	}
	ctx := multicluster.ContextWithClusterName(context.Background(), gvr.R.Cluster)
	err := c.Get(ctx, key, obj)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// ToYaml load the yaml text of object
func ToYaml(o runtime.Object) (string, error) {
	var (
		buff bytes.Buffer
		p    printers.YAMLPrinter
	)
	err := p.PrintObj(o, &buff)
	if err != nil {
		return "", err
	}
	return buff.String(), nil
}
