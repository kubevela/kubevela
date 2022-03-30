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

package resourcetracker

import (
	"bytes"
	"context"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/kubectl/pkg/cmd/get"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/errors"
)

type ResourceTreeNode struct {
	v1beta1.ManagedResource
	Message string
}

//func (node *ResourceTreeNode) addResource(mr v1beta1.ManagedResource) *ResourceTreeNode {
//	child := &ResourceTreeNode{
//		Mana: mr,
//		Parent: node,
//		Children: []*ResourceTreeNode{},
//	}
//	node.Children = append(node.Children, child)
//	return child
//}

//func BuildResourceTreeFromResourceTracker(app *v1beta1.Application, rt *v1beta1.ResourceTracker) *ResourceTreeNode {
//	root := &ResourceTreeNode{
//		ManagedResource: v1beta1.ManagedResource{
//			ClusterObjectReference: common.ClusterObjectReference{
//				ObjectReference: ,
//			},
//		},
//	}
//}

type ResourcePrinter func(v1beta1.ManagedResource) (string, error)

type ResourceTreePrintOptions struct {
	PrintComponent bool
	PrintTrait bool
	MessageRetriever ResourcePrinter
}

func (options ResourceTreePrintOptions) PrintResourceTree(writer io.Writer, rt *v1beta1.ResourceTracker) (err error) {
	var resources []ResourceTreeNode
	for _, rsc := range rt.Spec.ManagedResources {
		if !rsc.Deleted {
			node := ResourceTreeNode{ManagedResource: rsc}
			if options.MessageRetriever != nil {
				if node.Message, err = options.MessageRetriever(rsc); err != nil {
					return err
				}
			}
			resources = append(resources, node)
		}
	}
	table := options.addResourcesToTable(resources, nil)
	header := []string{"CLUSTER", "NAMESPACE", "COMPONENT", "TRAIT", "RESOURCE", "MESSAGE"}
	table = append([][]string{header}, table...)
	colSpans := make([]int, len(header))
	msgRowIdx := len(header)-1
	for i := range colSpans {
		colSpans[i] = len(header[i]) + 1
		for y := 1; y < len(table); y++ {
			if i == msgRowIdx {
				for _, msg := range strings.Split(table[y][i], "\n") {
					if len(msg) + 1 > colSpans[i] {
						colSpans[i] = len(msg) + 1
					}
				}
			} else if len(table[y][i]) + 1 > colSpans[i] {
				colSpans[i] = len(table[y][i]) + 1
			}
		}
	}
	for y := range table {
		messageLines := strings.Split(table[y][msgRowIdx], "\n")
		startPrint := false
		for subRowIdx, msg := range messageLines {
			for i := range colSpans {
				if !options.PrintComponent && header[i] == "COMPONENT" {
					continue
				}
				if !options.PrintTrait && header[i] == "TRAIT" {
					continue
				}
				if y < 2 || table[y][i] != table[y-1][i] || i > 3 {
					startPrint = true
				}
				var sb strings.Builder
				if i == msgRowIdx {
					sb.WriteString(msg)
				} else if startPrint && subRowIdx == 0 {
					sb.WriteString(table[y][i])
				}
				for j := sb.Len(); j < colSpans[i]; j++ {
					sb.WriteByte(' ')
				}
				if _, err = writer.Write([]byte(sb.String())); err != nil {
					return err
				}
			}
			if _, err = writer.Write([]byte{'\n'}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (options ResourceTreePrintOptions) addResourcesToTable(nodes []ResourceTreeNode, args []string) [][]string {
	depth := len(args)
	keyFunc := func(node ResourceTreeNode) string {
		switch depth {
		case 0:
			if node.Cluster == "" {
				return multicluster.ClusterLocalName
			}
			return node.Cluster
		case 1:
			if node.Namespace == "" {
				return types.DefaultAppNamespace
			}
			return node.Namespace
		case 2:
			return node.Component
		case 3:
			return node.Trait
		case 4:
			return node.Kind + "/" + node.Name
		default:
			return node.Message
		}
	}
	keyOrders, groupedResources := groupManagedResources(nodes, keyFunc)
	var table [][]string
	for _, key := range keyOrders {
		if depth < 5 {
			table = append(table, options.addResourcesToTable(groupedResources[key], append(args, key))...)
		} else {
			table = append(table, append(args, key))
		}
	}
	return table
}

func groupManagedResources(nodes []ResourceTreeNode, keyFunc func(ResourceTreeNode) string) (keyOrders []string, groupedResources map[string][]ResourceTreeNode) {
	groupedResources = make(map[string][]ResourceTreeNode)
	for _, node := range nodes {
		key := keyFunc(node)
		if _, found := groupedResources[key]; found {
			groupedResources[key] = append(groupedResources[key], node)
		} else {
			keyOrders = append(keyOrders, key)
			groupedResources[key] = []ResourceTreeNode{node}
		}
	}
	return keyOrders, groupedResources
}

func RetrieveKubeCtlGetMessageGenerator(cli client.Client) ResourcePrinter {
	return func(mr v1beta1.ManagedResource) (string, error) {
		var buf bytes.Buffer
		opt := get.NewGetOptions("", genericclioptions.IOStreams{In: &buf, Out: &buf, ErrOut: &buf})
		opt.PrintFlags.OutputFormat = pointer.String("wide")
		printer, err := opt.PrintFlags.ToPrinter()
		if err != nil {
			return "", err
		}
		printer, err = printers.NewTypeSetter(common.Scheme).WrapToPrinter(printer, nil)
		if err != nil {
			return "", err
		}
		printer = &get.TablePrinter{Delegate: printer}
		un := &unstructured.Unstructured{}
		un.SetAPIVersion(mr.APIVersion)
		un.SetKind(mr.Kind)
		if err = cli.Get(multicluster.ContextWithClusterName(context.Background(), mr.Cluster), mr.NamespacedName(), un); err != nil {
			if multicluster.IsNotFoundOrClusterNotExists(err) || errors.IsCRDNotExists(err) {
				return "Error: " + err.Error(), nil
			}
			return "", err
		}
		if err = printer.PrintObj(un, &buf); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
}