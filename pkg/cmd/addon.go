package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/gosuri/uitable"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	"github.com/cloud-native-application/rudrx/pkg/plugins"

	"github.com/cloud-native-application/rudrx/api/types"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
)

func AddonCommandGroup(parentCmd *cobra.Command, ioStream cmdutil.IOStreams) {
	parentCmd.AddCommand(
		NewAddonConfigCommand(ioStream),
		NewAddonListCommand(ioStream),
		NewAddonUpdateCommand(ioStream),
	)
}

func NewAddonConfigCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "addon:config <reponame> <url>",
		Short:   "Set the addon center, default is local (built-in ones)",
		Long:    "Set the addon center, default is local (built-in ones)",
		Example: `vela addon:config myhub https://github.com/oam-dev/catalog/repository`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength < 2 {
				return errors.New("please set addon repo with <RepoName> and <URL>")
			}
			repos, err := plugins.LoadRepos()
			if err != nil {
				return err
			}
			config := &plugins.RepoConfig{
				Name:    args[0],
				Address: args[1],
				Token:   cmd.Flag("token").Value.String(),
			}
			var updated bool
			for idx, r := range repos {
				if r.Name == config.Name {
					repos[idx] = *config
					updated = true
					break
				}
			}
			if !updated {
				repos = append(repos, *config)
			}
			if err = plugins.StoreRepos(repos); err != nil {
				return err
			}
			ioStreams.Info(fmt.Sprintf("Successfully configured Addon repo: %s, please use 'vela addon:update %s' to sync addons", args[0], args[0]))
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	cmd.PersistentFlags().StringP("token", "t", "", "Github Repo token")
	return cmd
}

func NewAddonUpdateCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "addon:update <repoName>",
		Short:   "Update addon repositories, default for all repo",
		Long:    "Update addon repositories, default for all repo",
		Example: `vela addon:update myrepo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repos, err := plugins.LoadRepos()
			if err != nil {
				return err
			}
			var specified string
			if len(args) > 0 {
				specified = args[0]
			}
			find := false
			if specified != "" {
				for idx, r := range repos {
					if r.Name == specified {
						repos = []plugins.RepoConfig{repos[idx]}
						find = true
						break
					}
				}
				if !find {
					return fmt.Errorf("%s repo not exist", specified)
				}
			}
			ctx := context.Background()
			for _, d := range repos {
				client, err := plugins.NewAddClient(ctx, d.Name, d.Address, d.Token)
				err = client.SyncRemoteAddons()
				if err != nil {
					return err
				}
			}
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	return cmd
}

func NewAddonListCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "addon:ls <repoName>",
		Short:   "List addons",
		Long:    "List addons of workloads and traits",
		Example: `vela addon:ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var repoName string
			if len(args) > 0 {
				repoName = args[0]
			}
			dir, err := system.GetRepoDir()
			if err != nil {
				return err
			}
			table := uitable.New()
			table.AddRow("NAME", "TYPE", "DEFINITION", "STATUS", "APPLIES-TO")
			if repoName != "" {
				return ListRepoAddons(table, filepath.Join(dir, repoName), ioStreams)
			}
			dirs, err := ioutil.ReadDir(dir)
			if err != nil {
				return err
			}
			for _, dd := range dirs {
				if !dd.IsDir() {
					continue
				}
				if err = ListRepoAddons(table, filepath.Join(dir, dd.Name()), ioStreams); err != nil {
					return err
				}
			}
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	return cmd
}

func ListRepoAddons(table *uitable.Table, repoDir string, ioStreams cmdutil.IOStreams) error {
	templates, err := plugins.LoadTempFromLocal(repoDir)
	if err != nil {
		return err
	}
	if len(templates) < 1 {
		return nil
	}
	baseDir := filepath.Base(repoDir)
	var status string
	//TODO(wonderflow): check status whether install or not
	status = "uninstalled"
	for _, p := range templates {
		table.AddRow(baseDir+"/"+p.Name, p.Type, p.Type, status, p.AppliesTo)
	}
	ioStreams.Info(table.String())
	return nil
}
