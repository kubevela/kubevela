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
	"context"
	"fmt"
	"net/url"

	"github.com/gosuri/uitable"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	addonRegistryType = "type"
	addonOssEndpoint  = "ossEndpoint"
	addonOssBucket    = "ossBucket"
	addonGitURL       = "gitUrl"
	addonPath         = "path"
	addonGitToken     = "gitToken"
	addonOssType      = "oss"
	addonGitType      = "git"
)

// NewAddonRegistryCommand return an addon registry command
func NewAddonRegistryCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage addon registry in KubeVela",
		Long:  "Manage addon registry in KubeVela",
	}
	cmd.AddCommand(
		NewAddAddonRegistryCommand(c, ioStreams),
		NewListAddonRegistryCommand(c, ioStreams),
		NewUpdateAddonRegistryCommand(c, ioStreams),
		NewDeleteAddonRegistryCommand(c, ioStreams),
		NewGetAddonRegistryCommand(c, ioStreams),
	)
	return cmd
}

// NewAddAddonRegistryCommand return an addon registry create command
func NewAddAddonRegistryCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add an addon registry in KubeVela",
		Long:    "Add an addon registry in KubeVela",
		Example: "vela addon registry add my-repo --type oss --ossEndpoint=xxxxx --ossBucket=xxxx",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := getRegistryFromArgs(cmd, args)
			if err != nil {
				return err
			}
			if err := addAddonRegistry(context.Background(), *registry); err != nil {
				return err
			}
			return nil
		},
	}
	parseArgsFromFlag(cmd)
	return cmd
}

// NewGetAddonRegistryCommand return an addon registry get command
func NewGetAddonRegistryCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "get",
		Short:   "Get an addon registry in KubeVela",
		Long:    "Get an addon registry in KubeVela",
		Example: "vela addon registry get my-repo ",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("must specify the registry name")
			}
			name := args[0]
			err := getAddonRegistry(context.Background(), name)
			if err != nil {
				return err
			}
			return nil
		},
	}
}

// NewListAddonRegistryCommand return an addon registry list command
func NewListAddonRegistryCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List addon registries in KubeVela",
		Long:    "List addon registries in KubeVela",
		Example: "vela addon registry list",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := listAddonRegistry(context.Background()); err != nil {
				return err
			}
			return nil
		},
	}
}

// NewUpdateAddonRegistryCommand return an addon registry update command
func NewUpdateAddonRegistryCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Update an addon registry in KubeVela",
		Long:    "Update an addon registry in KubeVela",
		Example: "vela addon registry update my-repo --type oss --ossEndpoint=xxxxx --ossBucket=xxxx",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := getRegistryFromArgs(cmd, args)
			if err != nil {
				return err
			}
			if err := updateAddonRegistry(context.Background(), *registry); err != nil {
				return err
			}
			return nil
		},
	}
	parseArgsFromFlag(cmd)
	return cmd
}

// NewDeleteAddonRegistryCommand return an addon registry delete command
func NewDeleteAddonRegistryCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "delete",
		Short:   "Delete an addon registry in KubeVela",
		Long:    "Delete an addon registry in KubeVela",
		Example: "vela addon registry delete my-repo ",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("must specify the registry name")
			}
			name := args[0]
			err := deleteAddonRegistry(context.Background(), name)
			if err != nil {
				return err
			}
			return nil
		},
	}
}

func listAddonRegistry(ctx context.Context) error {
	ds := pkgaddon.NewRegistryDataStore(clt)
	registries, err := ds.ListRegistries(ctx)
	if err != nil {
		return err
	}
	table := uitable.New()
	table.AddRow("Name", "Type", "URL")
	for _, registry := range registries {
		var repoType, repoURL string
		if registry.Oss != nil {
			repoType = "Oss"
			u, err := url.Parse(registry.Oss.Endpoint)
			if err != nil {
				continue
			}
			if registry.Oss.Bucket == "" {
				repoURL = u.String()
			} else {
				if u.Scheme == "" {
					u.Scheme = "https"
				}
				repoURL = fmt.Sprintf("%s://%s.%s", u.Scheme, registry.Oss.Bucket, u.Host)
			}
		} else {
			repoType = "Git"
			repoURL = fmt.Sprintf("%s/tree/master/%s", registry.Git.URL, registry.Git.Path)
		}
		table.AddRow(registry.Name, repoType, repoURL)
	}
	fmt.Println(table.String())
	return nil
}

func getAddonRegistry(ctx context.Context, name string) error {
	ds := pkgaddon.NewRegistryDataStore(clt)
	registry, err := ds.GetRegistry(ctx, name)
	if err != nil {
		return err
	}
	table := uitable.New()
	if registry.Oss != nil {
		table.AddRow("NAME", "ENDPOINT", "BUCKET")
		table.AddRow(registry.Name, registry.Oss.Endpoint, registry.Oss.Bucket)
	} else {
		table.AddRow("NAME", "URL", "PATH")
		table.AddRow(registry.Name, registry.Git.URL, registry.Git.Path)
	}
	fmt.Println(table.String())
	return nil
}

func deleteAddonRegistry(ctx context.Context, name string) error {
	ds := pkgaddon.NewRegistryDataStore(clt)
	if err := ds.DeleteRegistry(ctx, name); err != nil {
		return err
	}
	fmt.Printf("Successfully delete an addon registry %s \n", name)
	return nil
}

func addAddonRegistry(ctx context.Context, registry pkgaddon.Registry) error {
	ds := pkgaddon.NewRegistryDataStore(clt)
	if err := ds.AddRegistry(ctx, registry); err != nil {
		return err
	}
	fmt.Printf("Successfully add an addon registry %s \n", registry.Name)
	return nil
}

func updateAddonRegistry(ctx context.Context, registry pkgaddon.Registry) error {
	ds := pkgaddon.NewRegistryDataStore(clt)
	if err := ds.UpdateRegistry(ctx, registry); err != nil {
		return err
	}
	fmt.Printf("Successfully update an addon registry %s \n", registry.Name)
	return nil
}

func parseArgsFromFlag(cmd *cobra.Command) {
	cmd.Flags().StringP(addonRegistryType, "", "", "specify the addon registry type")
	cmd.Flags().StringP(addonOssEndpoint, "", "", "specify the oss endpoint")
	cmd.Flags().StringP(addonOssBucket, "", "", "specify the oss bucket")
	cmd.Flags().StringP(addonGitURL, "", "", "specify the git repo url")
	cmd.Flags().StringP(addonPath, "", "", "specify the repo path")
	cmd.Flags().StringP(addonGitToken, "", "", "specify the github repo token")
}

func getRegistryFromArgs(cmd *cobra.Command, args []string) (*pkgaddon.Registry, error) {
	r := &pkgaddon.Registry{}
	if len(args) != 1 {
		return nil, errors.New("must specify the registry name")
	}
	r.Name = args[0]

	registryType, err := cmd.Flags().GetString(addonRegistryType)
	if err != nil {
		return nil, err
	}

	switch registryType {
	case addonOssType:
		r.Oss = &pkgaddon.OSSAddonSource{}
		endpoint, err := cmd.Flags().GetString(addonOssEndpoint)
		if err != nil {
			return nil, err
		}
		if endpoint == "" {
			return nil, errors.New("oss type registry must set --ossEndpoint")
		}
		r.Oss.Endpoint = endpoint
		bucket, err := cmd.Flags().GetString(addonOssBucket)
		if err != nil {
			return nil, err
		}
		r.Oss.Bucket = bucket
	case addonGitType:
		r.Git = &pkgaddon.GitAddonSource{}
		gitURL, err := cmd.Flags().GetString(addonGitURL)
		if err != nil {
			return nil, err
		}
		if gitURL == "" {
			return nil, errors.New("oss type registry must set --gitUrl")
		}
		r.Git.URL = gitURL
		path, err := cmd.Flags().GetString(addonPath)
		if err != nil {
			return nil, err
		}
		r.Git.Path = path
		token, err := cmd.Flags().GetString(addonGitToken)
		if err != nil {
			return nil, err
		}
		r.Git.Token = token
	default:
		return nil, errors.New("not support addon registry type")
	}
	return r, nil
}
