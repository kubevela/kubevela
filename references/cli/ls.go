package cli

import (
	"context"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// NewListCommand creates `ls` command and its nested children command
func NewListCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "ls",
		Aliases:               []string{"list"},
		DisableFlagsInUseLine: true,
		Short:                 "List applications",
		Long:                  "List all applications in cluster",
		Example:               `vela ls`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			namespace, err := cmd.Flags().GetString(Namespace)
			if err != nil {
				return err
			}
			if namespace == "" {
				namespace = env.Namespace
			}
			return printApplicationList(ctx, newClient, namespace, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.PersistentFlags().StringP(Namespace, "n", "", "specify the namespace the application want to list, default is the current env namespace")
	return cmd
}

func printApplicationList(ctx context.Context, c client.Reader, namespace string, ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	table.AddRow("APP", "COMPONENT", "TYPE", "TRAITS", "PHASE", "HEALTHY", "STATUS", "CREATED-TIME")
	applist := v1alpha2.ApplicationList{}
	if err := c.List(ctx, &applist, client.InNamespace(namespace)); err != nil {
		if apierrors.IsNotFound(err) {
			ioStreams.Info(table.String())
			return nil
		}
		return err
	}

	for _, a := range applist.Items {
		for idx, cmp := range a.Spec.Components {
			var appName = a.Name
			if idx > 0 {
				appName = "├─"
				if idx == len(a.Spec.Components)-1 {
					appName = "└─"
				}
			}
			var healthy, status string
			if len(a.Status.Services) > idx {
				if a.Status.Services[idx].Healthy {
					healthy = "healthy"
				} else {
					healthy = "unhealthy"
				}
				status = a.Status.Services[idx].Message
			}
			var traits []string
			for _, tr := range cmp.Traits {
				traits = append(traits, tr.Name)
			}
			table.AddRow(appName, cmp.Name, cmp.WorkloadType, strings.Join(traits, ","), a.Status.Phase, healthy, status, a.CreationTimestamp)
		}
	}
	ioStreams.Info(table.String())
	return nil
}
