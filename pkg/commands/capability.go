package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func CapabilityCommandGroup(c types.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cap",
		Short: "Capability Management",
		Long:  "Capability Management with config, list, add, remove capabilities",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	cmd.AddCommand(
		NewCenterCommand(c, ioStream),
		NewCapListCommand(ioStream),
		NewCapAddCommand(c, ioStream),
		NewCapRemoveCommand(c, ioStream),
	)
	return cmd
}

func NewCenterCommand(c types.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "center <command>",
		Short: "Manage Capability Center",
		Long:  "Manage Capability Center with config, sync, list",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	cmd.AddCommand(
		NewCapCenterConfigCommand(ioStream),
		NewCapCenterSyncCommand(ioStream),
		NewCapCenterListCommand(ioStream),
		NewCapCenterRemoveCommand(ioStream),
	)
	return cmd
}

func NewCapCenterConfigCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config <centerName> <centerURL>",
		Short:   "Configure or add the capability center, default is local (built-in capabilities)",
		Long:    "Configure or add the capability center, default is local (built-in capabilities)",
		Example: `vela cap center config mycenter https://github.com/oam-dev/catalog/cap-center`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength < 2 {
				return errors.New("please set capability center with <centerName> and <centerURL>")
			}
			capName := args[0]
			capURL := args[1]
			token := cmd.Flag("token").Value.String()
			if err := oam.AddCapabilityCenter(capName, capURL, token); err != nil {
				return err
			}
			ioStreams.Infof("Successfully configured capability center: %s, start to sync from remote", capName)
			if err := oam.SyncCapabilityFromCenter(capName, capURL, token); err != nil {
				return err
			}
			ioStreams.Info("sync finished")
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	cmd.PersistentFlags().StringP("token", "t", "", "Github Repo token")
	return cmd
}

func NewCapAddCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add <center>/<name>",
		Short:   "Add capability into cluster",
		Long:    "Add capability into cluster",
		Example: `vela cap add mycenter/route`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			argsLength := len(args)
			if argsLength < 1 {
				return errors.New("you must specify <center>/<name> for capability you want to add")
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			if _, err = oam.AddCapabilityIntoCluster(newClient, args[0]); err != nil {
				return err
			}
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	cmd.PersistentFlags().StringP("token", "t", "", "Github Repo token")
	return cmd
}

func NewCapRemoveCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <name>",
		Short:   "Remove capability from cluster",
		Long:    "Remove capability from cluster",
		Example: `vela cap remove route`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("you must specify <name> for capability you want to remove")
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			name := args[0]
			if strings.Contains(name, "/") {
				l := strings.Split(name, "/")
				if len(l) > 2 {
					return fmt.Errorf("invalid format '%s', you can't contain more than one / in name", name)
				}
				name = l[1]
			}
			return oam.RemoveCapability(newClient, name, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	cmd.PersistentFlags().StringP("token", "t", "", "Github Repo token")
	return cmd
}

func NewCapCenterSyncCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sync [centerName]",
		Short:   "Sync capabilities from remote center, default to sync all centers",
		Long:    "Sync capabilities from remote center, default to sync all centers",
		Example: `vela cap center sync mycenter`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var specified string
			if len(args) > 0 {
				specified = args[0]
			}
			if err := oam.SyncCapabilityCenter(specified); err != nil {
				return err
			}
			ioStreams.Info("sync finished")
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	return cmd
}

func NewCapListCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls [centerName]",
		Short:   "List all capabilities in center",
		Long:    "List all capabilities in center",
		Example: `vela cap ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var repoName string
			if len(args) > 0 {
				repoName = args[0]
			}
			capabilityList, err := oam.ListCapabilities(repoName)
			if err != nil {
				return err
			}
			table := uitable.New()
			table.AddRow("NAME", "CENTER", "TYPE", "DEFINITION", "STATUS", "APPLIES-TO")

			for _, c := range capabilityList {
				table.AddRow(c.Name, c.Center, c.Type, c.CrdName, c.Status, c.AppliesTo)
			}
			ioStreams.Info(table.String())
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	return cmd
}

func NewCapCenterListCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Short:   "List all capability centers",
		Long:    "List all configured capability centers",
		Example: `vela cap center ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ListCapCenters(args, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	return cmd
}

func NewCapCenterRemoveCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <centerName>",
		Short:   "Remove specified capability center",
		Long:    "Remove specified capability center",
		Example: "vela cap center remove mycenter",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RemoveCapCenter(args, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	return cmd
}

func ListCapCenters(args []string, ioStreams cmdutil.IOStreams) error {
	table := uitable.New()
	table.AddRow("NAME", "ADDRESS")
	capabilityCenterList, err := oam.ListCapabilityCenters()
	if err != nil {
		return err
	}
	for _, c := range capabilityCenterList {
		table.AddRow(c.Name, c.URL)
	}
	ioStreams.Info(table.String())
	return nil
}

func RemoveCapCenter(args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return errors.New("you must specify <name> for capability center you want to remove")
	}
	centerName := args[0]
	msg, err := oam.RemoveCapabilityCenter(centerName)
	if err == nil {
		ioStreams.Info(msg)
	}
	return err
}
