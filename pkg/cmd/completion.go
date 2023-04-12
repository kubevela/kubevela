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

package cmd

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func listObjectNamesForCompletion(ctx context.Context, f Factory, gvk schema.GroupVersionKind, listOptions []client.ListOption, toComplete string) ([]string, cobra.ShellCompDirective) {
	uns := &unstructured.UnstructuredList{}
	uns.SetGroupVersionKind(gvk)
	if err := f.Client().List(ctx, uns, listOptions...); err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var candidates []string
	for _, obj := range uns.Items {
		if name := obj.GetName(); strings.HasPrefix(name, toComplete) {
			candidates = append(candidates, name)
		}
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

// GetNamespacesForCompletion auto-complete the namespace
func GetNamespacesForCompletion(ctx context.Context, f Factory, toComplete string) ([]string, cobra.ShellCompDirective) {
	return listObjectNamesForCompletion(ctx, f, corev1.SchemeGroupVersion.WithKind("Namespace"), nil, toComplete)
}

// GetServiceAccountForCompletion auto-complete serviceaccount
func GetServiceAccountForCompletion(ctx context.Context, f Factory, namespace string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var options []client.ListOption
	if namespace != "" {
		options = append(options, client.InNamespace(namespace))
	}
	return listObjectNamesForCompletion(ctx, f, corev1.SchemeGroupVersion.WithKind("ServiceAccount"), options, toComplete)
}

// GetRevisionForCompletion auto-complete the revision according to the application
func GetRevisionForCompletion(ctx context.Context, f Factory, appName string, namespace string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var options []client.ListOption
	if namespace != "" {
		options = append(options, client.InNamespace(namespace))
	}
	if appName != "" {
		options = append(options, client.MatchingLabels{oam.LabelAppName: appName})
	}
	return listObjectNamesForCompletion(ctx, f, v1beta1.SchemeGroupVersion.WithKind(v1beta1.ApplicationRevisionKind), options, toComplete)
}

// GetApplicationsForCompletion auto-complete application
func GetApplicationsForCompletion(ctx context.Context, f Factory, namespace string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var options []client.ListOption
	if namespace != "" {
		options = append(options, client.InNamespace(namespace))
	}
	return listObjectNamesForCompletion(ctx, f, v1beta1.SchemeGroupVersion.WithKind(v1beta1.ApplicationKind), options, toComplete)
}

// GetClustersForCompletion auto-complete the cluster
func GetClustersForCompletion(ctx context.Context, f Factory, toComplete string) ([]string, cobra.ShellCompDirective) {
	clusters, err := multicluster.NewClusterClient(f.Client()).List(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var candidates []string
	for _, obj := range clusters.Items {
		if name := obj.GetName(); strings.HasPrefix(name, toComplete) {
			candidates = append(candidates, name)
		}
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}
