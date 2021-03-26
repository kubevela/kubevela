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
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

// CapabilityCommandGroup commands for capability center
func CapabilityCommandGroup(c common2.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cap",
		Short: "Manage capability centers and installing/uninstalling capabilities",
		Long:  "Manage capability centers and installing/uninstalling capabilities",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCap,
		},
	}
	cmd.AddCommand(
		NewCenterCommand(ioStream),
		NewCapListCommand(c, ioStream),
		NewCapInstallCommand(c, ioStream),
		NewCapUninstallCommand(c, ioStream),
	)
	return cmd
}

// NewCenterCommand Manage Capability Center
func NewCenterCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "center <command>",
		Short: "Manage Capability Center",
		Long:  "Manage Capability Center with config, sync, list",
	}
	cmd.AddCommand(
		NewCapCenterConfigCommand(ioStream),
		NewCapCenterSyncCommand(ioStream),
		NewCapCenterListCommand(ioStream),
		NewCapCenterRemoveCommand(ioStream),
	)
	return cmd
}

// NewCapCenterConfigCommand Configure (add if not exist) a capability center, default is local (built-in capabilities)
func NewCapCenterConfigCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config <centerName> <centerURL>",
		Short:   "Configure (add if not exist) a capability center, default is local (built-in capabilities)",
		Long:    "Configure (add if not exist) a capability center, default is local (built-in capabilities)",
		Example: `vela cap center config mycenter https://github.com/oam-dev/catalog/cap-center`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength < 2 {
				return errors.New("please set capability center with <centerName> and <centerURL>")
			}
			capName := args[0]
			capURL := args[1]
			token := cmd.Flag("token").Value.String()
			if err := common.AddCapabilityCenter(capName, capURL, token); err != nil {
				return err
			}
			ioStreams.Infof("Successfully configured capability center %s and sync from remote\n", capName)
			return nil
		},
	}
	AddTokenVarFlags(cmd)
	return cmd
}

// NewCapInstallCommand Install capability into cluster
func NewCapInstallCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "install <center>/<name>",
		Short:   "Install capability into cluster",
		Long:    "Install capability into cluster",
		Example: `vela cap install mycenter/route`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			argsLength := len(args)
			if argsLength < 1 {
				return errors.New("you must specify <center>/<name> for capability you want to install")
			}
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			mapper, err := discoverymapper.New(c.Config)
			if err != nil {
				return err
			}
			if _, err = common.AddCapabilityIntoCluster(newClient, mapper, args[0]); err != nil {
				return err
			}
			return nil
		},
	}
	AddTokenVarFlags(cmd)
	return cmd
}

// NewCapUninstallCommand Uninstall capability from cluster
func NewCapUninstallCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "uninstall <name>",
		Short:   "Uninstall capability from cluster",
		Long:    "Uninstall capability from cluster",
		Example: `vela cap uninstall route`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("you must specify <name> for capability you want to uninstall")
			}
			newClient, err := c.GetClient()
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
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			return common.RemoveCapability(env.Namespace, c, newClient, name, ioStreams)
		},
	}
	AddTokenVarFlags(cmd)
	return cmd
}

// NewCapCenterSyncCommand Sync capabilities from remote center, default to sync all centers
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
			if err := common.SyncCapabilityCenter(specified); err != nil {
				return err
			}
			ioStreams.Info("sync finished")
			return nil
		},
	}
	return cmd
}

// NewCapListCommand List capabilities from cap-center
func NewCapListCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls [cap-center]",
		Short:   "List capabilities from cap-center",
		Long:    "List capabilities from cap-center",
		Example: `vela cap ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var repoName string
			if len(args) > 0 {
				repoName = args[0]
			}
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			capabilityList, err := common.ListCapabilities(env.Namespace, c, repoName)
			if err != nil {
				return err
			}
			table := newUITable()
			table.AddRow("NAME", "CENTER", "TYPE", "DEFINITION", "STATUS", "APPLIES-TO")

			for _, c := range capabilityList {
				table.AddRow(c.Name, c.Center, c.Type, c.CrdName, c.Status, c.AppliesTo)
			}
			ioStreams.Info(table.String())
			return nil
		},
	}
	return cmd
}

// NewCapCenterListCommand List all capability centers
func NewCapCenterListCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Short:   "List all capability centers",
		Long:    "List all configured capability centers",
		Example: `vela cap center ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listCapCenters(ioStreams)
		},
	}
	return cmd
}

// NewCapCenterRemoveCommand Remove specified capability center
func NewCapCenterRemoveCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <centerName>",
		Short:   "Remove specified capability center",
		Long:    "Remove specified capability center",
		Example: "vela cap center remove mycenter",
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeCapCenter(args, ioStreams)
		},
	}
	return cmd
}

func listCapCenters(ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	table.MaxColWidth = 80
	table.AddRow("NAME", "ADDRESS")
	capabilityCenterList, err := common.ListCapabilityCenters()
	if err != nil {
		return err
	}
	for _, c := range capabilityCenterList {
		table.AddRow(c.Name, c.URL)
	}
	ioStreams.Info(table.String())
	return nil
}

func removeCapCenter(args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return errors.New("you must specify <name> for capability center you want to remove")
	}
	centerName := args[0]
	msg, err := common.RemoveCapabilityCenter(centerName)
	if err == nil {
		ioStreams.Info(msg)
	}
	return err
}
