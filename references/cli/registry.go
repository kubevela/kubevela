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
	"fmt"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/plugins"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// NewRegistryCommand Manage Capability Center
func NewRegistryCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry <command>",
		Short: "Manage Registry",
		Long:  "Manage Registry with config, remove, list",
	}
	cmd.AddCommand(
		NewRegistryConfigCommand(ioStream),
		NewRegistryListCommand(ioStream),
		NewRegistryRemoveCommand(ioStream),
	)
	return cmd
}

// NewRegistryListCommand List all registry
func NewRegistryListCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Short:   "List all registry",
		Long:    "List all configured registry",
		Example: `vela registry ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listCapRegistrys(ioStreams)
		},
	}
	return cmd
}

// NewRegistryConfigCommand Configure (add if not exist) a registry, default is local (built-in capabilities)
func NewRegistryConfigCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config <registryName> <centerURL>",
		Short:   "Configure (add if not exist) a registry, default is local (built-in capabilities)",
		Long:    "Configure (add if not exist) a registry, default is local (built-in capabilities)",
		Example: `vela registry config my-registry https://github.com/oam-dev/catalog/tree/master/registry`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength < 2 {
				return errors.New("please set registry with <centerName> and <centerURL>")
			}
			capName := args[0]
			capURL := args[1]
			token := cmd.Flag("token").Value.String()
			if err := addRegistry(capName, capURL, token); err != nil {
				return err
			}
			ioStreams.Infof("Successfully configured registry %s\n", capName)
			return nil
		},
	}
	cmd.PersistentFlags().StringP("token", "t", "", "Github Repo token")
	return cmd
}

// NewRegistryRemoveCommand Remove specified registry
func NewRegistryRemoveCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"rm"},
		Use:     "remove <centerName>",
		Short:   "Remove specified registry",
		Long:    "Remove specified registry",
		Example: "vela registry remove mycenter",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("you must specify <name> for capability center you want to remove")
			}
			centerName := args[0]
			msg, err := removeRegistry(centerName)
			if err == nil {
				ioStreams.Info(msg)
			}
			return err
		},
	}
	return cmd
}

func listCapRegistrys(ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	table.MaxColWidth = 80
	table.AddRow("NAME", "URL")

	registrys, err := plugins.ListRegistryConfig()
	if err != nil {
		return errors.Wrap(err, "list registry error")
	}
	for _, c := range registrys {
		tokenShow := ""
		if len(c.Token) > 0 {
			tokenShow = "***"
		}
		table.AddRow(c.Name, c.URL, tokenShow)
	}
	ioStreams.Info(table.String())
	return nil
}

// addRegistry will add a registry
func addRegistry(regName, regURL, regToken string) error {
	regConfig := plugins.RegistryConfig{
		Name: regName, URL: regURL, Token: regToken,
	}
	repos, err := plugins.ListRegistryConfig()
	if err != nil {
		return err
	}
	var updated bool
	for idx, r := range repos {
		if r.Name == regConfig.Name {
			repos[idx] = regConfig
			updated = true
			break
		}
	}
	if !updated {
		repos = append(repos, regConfig)
	}
	if err = plugins.StoreRepos(repos); err != nil {
		return err
	}
	return nil
}

// removeRegistry will remove a registry from local
func removeRegistry(regName string) (string, error) {
	var message string
	var err error

	regConfigs, err := plugins.ListRegistryConfig()
	if err != nil {
		return message, err
	}
	found := false
	for idx, r := range regConfigs {
		if r.Name == regName {
			regConfigs = append(regConfigs[:idx], regConfigs[idx+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Sprintf("registry %s not found", regName), nil
	}
	if err = plugins.StoreRepos(regConfigs); err != nil {
		return message, err
	}
	message = fmt.Sprintf("%s registry center removed successfully", regName)
	return message, err
}
