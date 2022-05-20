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
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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
func (opt *KubeApplyOptions) Complete(f velacmd.Factory, cmd *cobra.Command) error {
	opt.namespace = velacmd.GetNamespace(f, cmd)
	opt.clusters = velacmd.GetClusters(cmd)

	var paths []string
	for _, file := range opt.files {
		path := strings.TrimSpace(file)
		if !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		data, err := utils.LoadDataFromPath(cmd.Context(), path, utils.IsJSONOrYAMLFile)
		if err != nil {
			return err
		}
		opt.filesData = append(opt.filesData, data...)
	}
	return nil
}

// Validate .
func (opt *KubeApplyOptions) Validate() error {
	if len(opt.files) == 0 {
		return fmt.Errorf("at least one file should be specified with the --file flag")
	}
	if len(opt.filesData) == 0 {
		return fmt.Errorf("not file found")
	}
	for _, fileData := range opt.filesData {
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
	}
	return nil
}

// Run .
func (opt *KubeApplyOptions) Run(f velacmd.Factory, cmd *cobra.Command) error {
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
		ctx := multicluster.ContextWithClusterName(cmd.Context(), cluster)
		for _, obj := range opt.objects {
			copiedObj := &unstructured.Unstructured{}
			bs, err := obj.MarshalJSON()
			if err != nil {
				return err
			}
			if err = copiedObj.UnmarshalJSON(bs); err != nil {
				return err
			}
			res, err := controllerutil.CreateOrPatch(ctx, f.Client(), copiedObj, nil)
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
		apply. If -n/--namespace is used, the original object namespace will be overrode.

		You can use -f/--file to specify the object file to apply. Multiple file inputs are allowed.
		Directory input and web url input is supported as well.`))

	kubeApplyExample = templates.Examples(i18n.T(`
		# Apply single object file in managed cluster
		vela kube apply -f my.yaml --cluster cluster-1
		
		# Apply multiple object files in multiple managed clusters
		vela kube apply -f my-1.yaml -f my-2.yaml --cluster cluster-1 --cluster cluster-2

		# Apply object file with web url in control plane
		vela kube apply -f https://raw.githubusercontent.com/kubevela/kubevela/master/docs/examples/app-with-probe/app-with-probe.yaml
		
		# Apply object files in directory to specified namespace in managed clusters 
		vela kube apply -f ./resources -n demo --cluster cluster-1 --cluster cluster-2`))
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
		Args: cobra.ExactValidArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(f, cmd))
		},
	}
	cmd.Flags().StringSliceVarP(&o.files, "file", "f", o.files, "Files that include native Kubernetes objects to apply.")
	cmd.Flags().BoolVarP(&o.dryRun, "dryrun", "", o.dryRun, "Setting this flag will not apply resources in clusters. It will print out the resource to be applied.")
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
