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

package cli

import (
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var (
	packageListExample = templates.Examples(i18n.T(`
		# List all packages
		vela package list`))

	packageShowExample = templates.Examples(i18n.T(`
		# Show details of a package
		vela package show my-package`))
)

// NewPackageCommand creates the 'vela package' command
func NewPackageCommand(f velacmd.Factory, order string, ioStreams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package",
		Short: i18n.T("Manage packages."),
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeAuxiliary,
			types.TagCommandOrder: order,
		},
	}
	cmd.AddCommand(NewPackageListCommand(f, ioStreams))
	cmd.AddCommand(NewPackageShowCommand(f, ioStreams))
	return cmd
}

// NewPackageListCommand creates the 'vela package list' command
func NewPackageListCommand(f velacmd.Factory, ioStreams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   i18n.T("List packages."),
		Example: packageListExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeAuxiliary,
		},
		Run: func(cmd *cobra.Command, args []string) {
			objs := &unstructured.UnstructuredList{}
			objs.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "cue.oam.dev",
				Version: "v1alpha1",
				Kind:    "PackageList",
			})
			cmdutil.CheckErr(f.Client().List(cmd.Context(), objs))

			table := newUITable()
			table.AddRow("NAME", "PATH", "DESCRIPTION")
			for _, pkg := range objs.Items {
				name := pkg.GetName()
				path, _, _ := unstructured.NestedString(pkg.Object, "spec", "path")
				description, _, _ := unstructured.NestedString(pkg.Object, "spec", "description")
				table.AddRow(name, path, description)
			}
			ioStreams.Info(table.String())
		},
	}
	return cmd
}

// NewPackageShowCommand creates the 'vela package show <name>' command
func NewPackageShowCommand(f velacmd.Factory, ioStreams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show <name>",
		Short:   i18n.T("Show details of a package."),
		Example: packageShowExample,
		Args:    cobra.ExactArgs(1),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeAuxiliary,
		},
		Run: func(cmd *cobra.Command, args []string) {
			pkg := &unstructured.Unstructured{}
			pkg.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "cue.oam.dev",
				Version: "v1alpha1",
				Kind:    "Package",
			})
			cmdutil.CheckErr(f.Client().Get(cmd.Context(), k8stypes.NamespacedName{Name: args[0]}, pkg))
			name := pkg.GetName()

			description, _, _ := unstructured.NestedString(pkg.Object, "spec", "description")
			path, _, _ := unstructured.NestedString(pkg.Object, "spec", "path")
			proto, _, _ := unstructured.NestedString(pkg.Object, "spec", "provider", "protocol")
			endpoint, _, _ := unstructured.NestedString(pkg.Object, "spec", "provider", "endpoint")
			pkgTemplates, _, _ := unstructured.NestedStringMap(pkg.Object, "spec", "templates")

			ioStreams.Infof("Name:         %s\n", name)
			ioStreams.Infof("Scope:        Cluster\n")
			ioStreams.Infof("Description:  %s\n", description)
			ioStreams.Infof("\n")
			ioStreams.Infof("# Usage:\n")
			ioStreams.Infof("# Add this import to your CUE file to use this package:\n")
			ioStreams.Infof("import \"%s\"\n", name)
			ioStreams.Infof("\n")
			ioStreams.Infof("# Source Information:\n")
			ioStreams.Infof("Protocol:     %s\n", proto)
			ioStreams.Infof("Endpoint:     %s\n", endpoint)
			ioStreams.Infof("Path:         %s\n", path)
			if len(pkgTemplates) > 0 {
				ioStreams.Infof("\n")
				ioStreams.Infof("# Exported Templates: (%d total)\n", len(pkgTemplates))
				for k := range pkgTemplates {
					ioStreams.Infof("  - %s\n", k)
				}
			}
		},
	}
	return cmd
}
