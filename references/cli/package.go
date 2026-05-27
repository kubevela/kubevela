package cli

import (
	"context" // Provides context for kubernetes API calls

	"github.com/pkg/errors" // Wraps errors with more context for better debugging messages
	"github.com/spf13/cobra" // CLI framework used to define commands like vela package list
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured" // // Allows working with kubernetes resources without go structs
	"k8s.io/apimachinery/pkg/runtime/schema" // Defines GroupVersionKind to identify kubernetes resource types

	"github.com/oam-dev/kubevela/apis/types" // Contains CLI metadata constants
	common2 "github.com/oam-dev/kubevela/pkg/utils/common" // Provides shared CLI utilities
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util" // provides IOStreams for consistent CLI input and output handling
)

// Makes the 'package' command and attaches its subcommands
func NewPackageCommand(c common2.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "package",
		Aliases: []string{"packages"},
		Short:   "List installed packages.",
		Long:    "List Package objects installed in the cluster.",
		Example: `vela package list`,
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeExtension,
			types.TagCommandOrder: order,
		},
	}

	cmd.SetOut(ioStreams.Out)
	cmd.AddCommand(
		NewPackageListCommand(c, ioStreams),
	)
	return cmd
}

// Makes the 'list' subcommand
func NewPackageListCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List packages.",
		Long:  "List Package objects installed in the cluster.",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return PrintInstalledPackages(c, ioStreams)
		},
	}
	return cmd
}

// Connects to the cluster, lists Package CRDs, and prints them in a table
func PrintInstalledPackages(c common2.Args, io cmdutil.IOStreams) error {
	clt, err := c.GetClient()
	if err != nil {
		return err
	}

	var list unstructured.UnstructuredList
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cue.oam.dev",
		Version: "v1alpha1",
		Kind:    "PackageList",
	})

	err = clt.List(context.Background(), &list)
	if err != nil {
		return errors.Wrap(err, "get package list error")
	}

	table := newUITable()
	table.AddRow("NAME", "NAMESPACE")

	for _, pkg := range list.Items {
		table.AddRow(pkg.GetName(), pkg.GetNamespace())
	}

	io.Info(table.String())
	return nil
}