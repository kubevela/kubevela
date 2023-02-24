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

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// KubeCommandGroup command group for native resource management
func KubeCommandGroup(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kube",
		Short: i18n.T("Managing native Kubernetes resources across clusters."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Run: func(cmd *cobra.Command, args []string) {

		},
	}
	cmd.AddCommand(NewKubeApplyCommand(f, streams))
	cmd.AddCommand(NewKubeDeleteCommand(f, streams))
	return cmd
}

// KubeApplyOptions options for kube apply
type KubeApplyOptions struct {
	files     []string
	clusters  []string
	namespace string
	dryRun    bool

	filesData []utils.FileData
	objects   []*unstructured.Unstructured

	util.IOStreams
}

// Complete .
func (opt *KubeApplyOptions) Complete(ctx context.Context) error {
	var paths []string
	for _, file := range opt.files {
		path := strings.TrimSpace(file)
		if !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		data, err := utils.LoadDataFromPath(ctx, path, utils.IsJSONYAMLorCUEFile)
		if err != nil {
			return err
		}
		opt.filesData = append(opt.filesData, data...)
	}
	return nil
}

// Validate will not only validate the args but also read from files and generate the objects
func (opt *KubeApplyOptions) Validate() error {
	if len(opt.files) == 0 {
		return fmt.Errorf("at least one file should be specified with the --file flag")
	}
	if len(opt.filesData) == 0 {
		return fmt.Errorf("not file found")
	}
	if len(opt.clusters) == 0 {
		opt.clusters = []string{"local"}
	}
	jsonObj := func(data []byte, path string) (*unstructured.Unstructured, error) {
		obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
		err := json.Unmarshal(data, &obj.Object)
		if err != nil {
			return nil, fmt.Errorf("failed to decode object in %s: %w", path, err)
		}
		if opt.namespace != "" {
			obj.SetNamespace(opt.namespace)
		} else if obj.GetNamespace() == "" {
			obj.SetNamespace(metav1.NamespaceDefault)
		}
		return obj, nil
	}

	for _, fileData := range opt.filesData {
		switch {
		case strings.HasSuffix(fileData.Path, ".yaml"), strings.HasSuffix(fileData.Path, ".yml"):
			decoder := yaml.NewDecoder(bytes.NewReader(fileData.Data))
			for {
				obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
				err := decoder.Decode(obj.Object)
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					return fmt.Errorf("failed to decode object in %s: %w", fileData.Path, err)
				}
				if opt.namespace != "" {
					obj.SetNamespace(opt.namespace)
				} else if obj.GetNamespace() == "" {
					obj.SetNamespace(metav1.NamespaceDefault)
				}
				opt.objects = append(opt.objects, obj)
			}
		case strings.HasSuffix(fileData.Path, ".json"):
			obj, err := jsonObj(fileData.Data, fileData.Path)
			if err != nil {
				return err
			}
			opt.objects = append(opt.objects, obj)
		case strings.HasSuffix(fileData.Path, ".cue"):
			val, err := value.NewValue(string(fileData.Data), nil, "")
			if err != nil {
				return fmt.Errorf("failed to decode object in %s: %w", fileData.Path, err)
			}
			data, err := val.CueValue().MarshalJSON()
			if err != nil {
				return fmt.Errorf("failed to marhsal to json for CUE object in %s: %w", fileData.Path, err)
			}
			obj, err := jsonObj(data, fileData.Path)
			if err != nil {
				return err
			}
			opt.objects = append(opt.objects, obj)
		}
	}
	return nil
}

// Run will apply objects to clusters
func (opt *KubeApplyOptions) Run(ctx context.Context, cli client.Client) error {
	if opt.dryRun {
		for i, obj := range opt.objects {
			if i > 0 {
				_, _ = fmt.Fprintf(opt.Out, "---\n")
			}
			bs, err := yaml.Marshal(obj.Object)
			if err != nil {
				return err
			}
			_, _ = opt.Out.Write(bs)
		}
		return nil
	}
	for i, cluster := range opt.clusters {
		if i > 0 {
			_, _ = fmt.Fprintf(opt.Out, "\n")
		}
		_, _ = fmt.Fprintf(opt.Out, "Apply objects in cluster %s.\n", cluster)
		ctx := multicluster.ContextWithClusterName(ctx, cluster)
		for _, obj := range opt.objects {
			copiedObj := &unstructured.Unstructured{}
			bs, err := obj.MarshalJSON()
			if err != nil {
				return err
			}
			if err = copiedObj.UnmarshalJSON(bs); err != nil {
				return err
			}
			res, err := utils.CreateOrUpdate(ctx, cli, copiedObj)
			if err != nil {
				return err
			}
			key := strings.TrimPrefix(obj.GetNamespace()+"/"+obj.GetName(), "/")
			_, _ = fmt.Fprintf(opt.Out, "  %s %s %s.\n", obj.GetKind(), key, res)
		}
	}
	return nil
}

var (
	kubeApplyLong = templates.LongDesc(i18n.T(`
		Apply Kubernetes objects in clusters

		Apply Kubernetes objects in multiple clusters. Use --clusters to specify which clusters to
		apply. If -n/--namespace is used, the original object namespace will be overridden.

		You can use -f/--file to specify the object file/folder to apply. Multiple file inputs are allowed.
		Directory input and web url input is supported as well.
		File format can be in YAML, JSON or CUE.
`))

	kubeApplyExample = templates.Examples(i18n.T(`
		# Apply single object file in managed cluster
		vela kube apply -f my.yaml --cluster cluster-1

		# Apply object in CUE, the whole CUE file MUST follow the kubernetes API and contain only one object.
		vela kube apply -f my.cue --cluster cluster-1

		# Apply object in JSON, the whole JSON file MUST follow the kubernetes API and contain only one object.
		vela kube apply -f my.json --cluster cluster-1

		# Apply multiple object files in multiple managed clusters
		vela kube apply -f my-1.yaml -f my-2.cue --cluster cluster-1 --cluster cluster-2

		# Apply object file with web url in control plane
		vela kube apply -f https://raw.githubusercontent.com/kubevela/kubevela/master/docs/examples/app-with-probe/app-with-probe.yaml
		
		# Apply object files in directory to specified namespace in managed clusters 
		vela kube apply -f ./resources -n demo --cluster cluster-1 --cluster cluster-2

		# Use dry-run to see what will be rendered out in YAML
		vela kube apply -f my.cue --cluster cluster-1 --dry-run
`))
)

// NewKubeApplyCommand kube apply command
func NewKubeApplyCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	o := &KubeApplyOptions{IOStreams: streams}
	cmd := &cobra.Command{
		Use:     "apply",
		Short:   i18n.T("Apply resources in Kubernetes YAML file to clusters."),
		Long:    kubeApplyLong,
		Example: kubeApplyExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			o.namespace = velacmd.GetNamespace(f, cmd)
			o.clusters = velacmd.GetClusters(cmd)

			cmdutil.CheckErr(o.Complete(cmd.Context()))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(cmd.Context(), f.Client()))
		},
	}
	cmd.Flags().StringSliceVarP(&o.files, "file", "f", o.files, "Files that include native Kubernetes objects to apply.")
	cmd.Flags().BoolVarP(&o.dryRun, FlagDryRun, "", o.dryRun, "Setting this flag will not apply resources in clusters. It will print out the resource to be applied.")
	return velacmd.NewCommandBuilder(f, cmd).
		WithNamespaceFlag(
			velacmd.NamespaceFlagDisableEnvOption{},
			velacmd.UsageOption("The namespace to apply objects. If empty, the namespace declared in the YAML will be used."),
		).
		WithClusterFlag(velacmd.UsageOption("The cluster to apply objects. Setting multiple clusters will apply objects in order.")).
		WithStreams(streams).
		WithResponsiveWriter().
		Build()
}

// KubeDeleteOptions options for kube delete
type KubeDeleteOptions struct {
	clusters     []string
	namespace    string
	deleteAll    bool
	resource     string
	resourceName string

	util.IOStreams
}

var (
	kubeDeleteLong = templates.LongDesc(i18n.T(`
		Delete Kubernetes objects in clusters

		Delete Kubernetes objects in multiple clusters. Use --clusters to specify which clusters to
		delete. Use -n/--namespace flags to specify which cluster the target resource locates.

		Use --all flag to delete all this kind of objects in the target namespace and clusters.`))

	kubeDeleteExample = templates.Examples(i18n.T(`
		# Delete the deployment nginx in default namespace in cluster-1
		vela kube delete deployment nginx --cluster cluster-1

		# Delete the deployment nginx in demo namespace in cluster-1 and cluster-2
		vela kube delete deployment nginx -n demo --cluster cluster-1 --cluster cluster-2

		# Delete all deployments in demo namespace in cluster-1
		vela kube delete deployment --all -n demo --cluster cluster-1`))
)

// Complete .
func (opt *KubeDeleteOptions) Complete(f velacmd.Factory, cmd *cobra.Command, args []string) {
	opt.namespace = velacmd.GetNamespace(f, cmd)
	if opt.namespace == "" {
		opt.namespace = metav1.NamespaceDefault
	}
	opt.clusters = velacmd.GetClusters(cmd)
	opt.resource = args[0]
	if len(args) == 2 {
		opt.resourceName = args[1]
	}
}

// Validate .
func (opt *KubeDeleteOptions) Validate() error {
	if opt.resourceName == "" && !opt.deleteAll {
		return fmt.Errorf("either resource name or flag --all should be set")
	}
	if opt.resourceName != "" && opt.deleteAll {
		return fmt.Errorf("cannot set resource name and flag --all at the same time")
	}
	return nil
}

// Run .
func (opt *KubeDeleteOptions) Run(f velacmd.Factory, cmd *cobra.Command) error {
	gvks, err := f.Client().RESTMapper().KindsFor(schema.GroupVersionResource{Resource: opt.resource})
	if err != nil {
		return fmt.Errorf("failed to find kinds for resource %s: %w", opt.resource, err)
	}
	if len(gvks) == 0 {
		return fmt.Errorf("no kinds found for resource %s", opt.resource)
	}
	gvk := gvks[0]
	mappings, err := f.Client().RESTMapper().RESTMappings(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("failed to get mappings for resource %s: %w", opt.resource, err)
	}
	if len(mappings) == 0 {
		return fmt.Errorf("no mappings found for resource %s", opt.resource)
	}
	mapping := mappings[0]
	namespaced := mapping.Scope.Name() == meta.RESTScopeNameNamespace
	for _, cluster := range opt.clusters {
		ctx := multicluster.ContextWithClusterName(cmd.Context(), cluster)
		objs, obj := &unstructured.UnstructuredList{}, &unstructured.Unstructured{}
		objs.SetGroupVersionKind(gvk)
		obj.SetGroupVersionKind(gvk)
		switch {
		case opt.deleteAll && namespaced:
			err = f.Client().List(ctx, objs, client.InNamespace(opt.namespace))
		case opt.deleteAll && !namespaced:
			err = f.Client().List(ctx, objs)
		case !opt.deleteAll && namespaced:
			err = f.Client().Get(ctx, apitypes.NamespacedName{Namespace: opt.namespace, Name: opt.resourceName}, obj)
		case !opt.deleteAll && !namespaced:
			err = f.Client().Get(ctx, apitypes.NamespacedName{Name: opt.resourceName}, obj)
		}
		if err != nil && !apierrors.IsNotFound(err) && !runtime.IsNotRegisteredError(err) && !meta.IsNoMatchError(err) {
			return fmt.Errorf("failed to retrieve %s in cluster %s: %w", opt.resource, cluster, err)
		}
		for _, toDel := range append(objs.Items, *obj) {
			key := toDel.GetName()
			if key == "" {
				continue
			}
			if namespaced {
				key = toDel.GetNamespace() + "/" + key
			}
			if err = f.Client().Delete(ctx, toDel.DeepCopy()); err != nil {
				return fmt.Errorf("failed to delete %s %s in cluster %s: %w", opt.resource, key, cluster, err)
			}
			_, _ = fmt.Fprintf(opt.IOStreams.Out, "%s %s in cluster %s deleted.\n", opt.resource, key, cluster)
		}
	}
	return nil
}

// NewKubeDeleteCommand kube delete command
func NewKubeDeleteCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	o := &KubeDeleteOptions{IOStreams: streams}
	cmd := &cobra.Command{
		Use:     "delete",
		Short:   i18n.T("Delete resources in clusters."),
		Long:    kubeDeleteLong,
		Example: kubeDeleteExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			o.Complete(f, cmd, args)
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(f, cmd))
		},
	}
	cmd.Flags().BoolVarP(&o.deleteAll, "all", "", o.deleteAll, "Setting this flag will delete all this kind of resources.")
	return velacmd.NewCommandBuilder(f, cmd).
		WithNamespaceFlag(
			velacmd.NamespaceFlagDisableEnvOption{},
			velacmd.UsageOption("The namespace to delete objects. If empty, the default namespace will be used."),
		).
		WithClusterFlag(velacmd.UsageOption("The cluster to delete objects. Setting multiple clusters will delete objects in order.")).
		WithStreams(streams).
		WithResponsiveWriter().
		Build()
}
