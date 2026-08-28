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
	"io"
	"net/url"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	addonRegistryType  = "type"
	addonEndpoint      = "endpoint"
	addonOssBucket     = "bucket"
	addonPath          = "path"
	addonGitToken      = "gitToken"
	addonOssType       = "OSS"
	addonGitType       = "git"
	addonGiteeType     = "gitee"
	addonGitlabType    = "gitlab"
	addonHelmType      = "helm"
	addonOCIType       = "oci"
	addonUsername      = "username"
	addonPassword      = "password"
	addonPasswordStdin = "password-stdin"
	// only gitlab registry need set this flag
	addonRepoName            = "gitlabRepoName"
	addonHelmInsecureSkipTLS = "insecureSkipTLS"
)

// NewAddonRegistryCommand return an addon registry command
func NewAddonRegistryCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage addon registry.",
		Long:  "Manage addon registry.",
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
func NewAddAddonRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an addon registry.",
		Long:  "Add an addon registry.",
		Example: `add a helm repo registry: vela addon registry add --type=helm my-repo --endpoint=<URL>
add a github registry: vela addon registry add my-repo --type git --endpoint=<URL> --path=<path> --gitToken=<git token>
add a specified github registry: vela addon registry add my-repo --type git --endpoint=https://github.com/kubevela/catalog --path=addons --gitToken=<git token>
add a gitlab registry: vela addon registry add my-repo --type gitlab --endpoint=<URL> --gitlabRepoName=<repoName> --path=<path> --gitToken=<git token>
add a specified gitlab registry: vela addon registry add my-repo --type gitlab --endpoint=http://gitlab.xxx.com/xxx/catalog --path=addons --gitlabRepoName=catalog --gitToken=<git token>
add an OCI registry: printf '%s' "$REGISTRY_PASSWORD" | vela addon registry add my-repo --type oci --endpoint=oci://<registry>/<repo> --username=<user> --password-stdin
add a private ECR registry: aws ecr get-login-password --region <region> | vela addon registry add ecr --type oci --endpoint=oci://<acct>.dkr.ecr.<region>.amazonaws.com/addon --username=AWS --password-stdin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setRegistryPasswordFromStdin(cmd); err != nil {
				return err
			}
			registry, err := getRegistryFromArgs(cmd, args)
			if err != nil {
				return err
			}
			if registry.Helm != nil {
				versionedRegistry := pkgaddon.BuildVersionedRegistry(registry.Name, registry.Helm.URL, &common.HTTPOption{
					Username:        registry.Helm.Username,
					Password:        registry.Helm.Password,
					InsecureSkipTLS: registry.Helm.InsecureSkipTLS,
				})
				_, err = versionedRegistry.ListAddon()
				if err != nil {
					return fmt.Errorf("fail to add registry %s: %w", registry.Name, err)
				}
			}
			return addAddonRegistry(context.Background(), c, *registry)
		},
	}
	parseArgsFromFlag(cmd)
	return cmd
}

// NewGetAddonRegistryCommand return an addon registry get command
func NewGetAddonRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "get",
		Short:   "Get an addon registry.",
		Long:    "Get an addon registry.",
		Example: "vela addon registry get <registry name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("must specify the registry name")
			}
			name := args[0]
			err := getAddonRegistry(context.Background(), c, name)
			if err != nil {
				return err
			}
			return nil
		},
	}
}

// NewListAddonRegistryCommand return an addon registry list command
func NewListAddonRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List addon registries.",
		Long:    "List addon registries.",
		Example: "vela addon registry list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listAddonRegistry(context.Background(), c)
		},
	}
}

// NewUpdateAddonRegistryCommand return an addon registry update command
func NewUpdateAddonRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Update an addon registry.",
		Long:    "Update an addon registry.",
		Example: "vela addon registry update <registry-name> --type OSS --endpoint=<URL> --bucket=<bucket name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setRegistryPasswordFromStdin(cmd); err != nil {
				return err
			}
			registry, err := getRegistryFromArgs(cmd, args)
			if err != nil {
				return err
			}
			return updateAddonRegistry(context.Background(), c, *registry)
		},
	}
	parseArgsFromFlag(cmd)
	return cmd
}

// NewDeleteAddonRegistryCommand return an addon registry delete command
func NewDeleteAddonRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "delete",
		Short:   "Delete an addon registry",
		Long:    "Delete an addon registry",
		Example: "vela addon registry delete <registry-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("must specify the registry name")
			}
			name := args[0]
			err := deleteAddonRegistry(context.Background(), c, name)
			if err != nil {
				return err
			}
			return nil
		},
	}
}

func listAddonRegistry(ctx context.Context, c common.Args) error {
	client, err := c.GetClient()
	if err != nil {
		return err
	}
	ds := pkgaddon.NewRegistryDataStore(client)
	registries, err := ds.ListRegistries(ctx)
	if err != nil {
		return err
	}
	table := uitable.New()
	table.AddRow("Name", "Type", "URL")
	for _, registry := range registries {
		var repoType, repoURL string
		switch {
		case registry.OSS != nil:
			repoType = "OSS"
			u, err := url.Parse(registry.OSS.Endpoint)
			if err != nil {
				continue
			}
			if registry.OSS.Bucket == "" {
				repoURL = u.String()
			} else {
				if u.Scheme == "" {
					u.Scheme = "https"
				}
				repoURL = fmt.Sprintf("%s://%s.%s", u.Scheme, registry.OSS.Bucket, u.Host)
			}

		case registry.Git != nil:
			repoType = "git"
			repoURL = fmt.Sprintf("%s/tree/master/%s", registry.Git.URL, registry.Git.Path)
		case registry.Gitee != nil:
			repoType = "gitee"
			repoURL = fmt.Sprintf("%s/tree/master/%s", registry.Gitee.URL, registry.Gitee.Path)
		case registry.Helm != nil:
			repoType = "helm"
			repoURL = registry.Helm.URL
		case registry.Gitlab != nil:
			repoType = "gitlab"
			repoURL = registry.Gitlab.URL
		case registry.OCI != nil:
			repoType = "oci"
			repoURL = registry.OCI.URL
		}

		table.AddRow(registry.Name, repoType, repoURL)
	}
	fmt.Println(table.String())
	return nil
}

func getAddonRegistry(ctx context.Context, c common.Args, name string) error {
	client, err := c.GetClient()
	if err != nil {
		return err
	}
	ds := pkgaddon.NewRegistryDataStore(client)
	registry, err := ds.GetRegistry(ctx, name)
	if err != nil {
		return err
	}
	table := uitable.New()
	switch {
	case registry.OSS != nil:
		table.AddRow("NAME", "Type", "ENDPOINT", "BUCKET", "PATH")
		table.AddRow(registry.Name, "OSS", registry.OSS.Endpoint, registry.OSS.Bucket, registry.OSS.Path)
	case registry.Helm != nil:
		table.AddRow("NAME", "Type", "ENDPOINT")
		table.AddRow(registry.Name, "Helm", registry.Helm.URL)
	case registry.Gitee != nil:
		table.AddRow("NAME", "Type", "ENDPOINT", "PATH")
		table.AddRow(registry.Name, "Gitee", registry.Gitee.URL, registry.Gitee.Path)
	case registry.Gitlab != nil:
		table.AddRow("NAME", "Type", "ENDPOINT", "REPOSITORY", "PATH")
		table.AddRow(registry.Name, "Gitlab", registry.Gitlab.URL, registry.Gitlab.Repo, registry.Gitlab.Path)
	case registry.Git != nil:
		table.AddRow("NAME", "Type", "ENDPOINT", "PATH")
		table.AddRow(registry.Name, "Git", registry.Git.URL, registry.Git.Path)
	case registry.OCI != nil:
		table.AddRow("NAME", "Type", "ENDPOINT")
		table.AddRow(registry.Name, "OCI", registry.OCI.URL)
	default:
		table.AddRow("Name")
		table.AddRow(registry.Name)
	}
	fmt.Println(table.String())
	return nil
}

func deleteAddonRegistry(ctx context.Context, c common.Args, name string) error {
	client, err := c.GetClient()
	if err != nil {
		return err
	}
	ds := pkgaddon.NewRegistryDataStore(client)
	if err := ds.DeleteRegistry(ctx, name); err != nil {
		return err
	}
	fmt.Printf("Successfully delete an addon registry %s \n", name)
	return nil
}

func addAddonRegistry(ctx context.Context, c common.Args, registry pkgaddon.Registry) error {
	client, err := c.GetClient()
	if err != nil {
		return err
	}
	ds := pkgaddon.NewRegistryDataStore(client)
	if err := ds.AddRegistry(ctx, registry); err != nil {
		return err
	}
	fmt.Printf("Successfully add an addon registry %s \n", registry.Name)
	return nil
}

func updateAddonRegistry(ctx context.Context, c common.Args, registry pkgaddon.Registry) error {
	client, err := c.GetClient()
	if err != nil {
		return err
	}
	ds := pkgaddon.NewRegistryDataStore(client)
	if err := ds.UpdateRegistry(ctx, registry); err != nil {
		return err
	}
	fmt.Printf("Successfully update an addon registry %s \n", registry.Name)
	return nil
}

func parseArgsFromFlag(cmd *cobra.Command) {
	cmd.Flags().StringP(addonRegistryType, "", "", "specify the addon registry type")
	cmd.Flags().StringP(addonEndpoint, "", "", "specify the addon registry endpoint")
	cmd.Flags().StringP(addonOssBucket, "", "", "specify the OSS bucket name")
	cmd.Flags().StringP(addonPath, "", "", "specify the addon registry path, must be set when addons are not in root of registry")
	cmd.Flags().StringP(addonGitToken, "", "", "specify the github repo token")
	cmd.Flags().StringP(addonUsername, "", "", "specify the Helm addon registry username")
	cmd.Flags().StringP(addonPassword, "", "", "specify the Helm or OCI addon registry password")
	cmd.Flags().Bool(addonPasswordStdin, false, "read the Helm or OCI addon registry password from stdin")
	cmd.Flags().StringP(addonRepoName, "", "", "specify the gitlab addon registry repoName, must be set when registry is gitlab")
	cmd.Flags().BoolP(addonHelmInsecureSkipTLS, "", false,
		"specify the Helm addon registry skip tls verify")
}

func setRegistryPasswordFromStdin(cmd *cobra.Command) error {
	passwordStdin, err := cmd.Flags().GetBool(addonPasswordStdin)
	if err != nil {
		return err
	}
	if !passwordStdin {
		return nil
	}
	if cmd.Flags().Changed(addonPassword) {
		return errors.New("--password and --password-stdin cannot be used together")
	}
	password, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return errors.Wrap(err, "failed to read addon registry password from stdin")
	}
	value := strings.TrimRight(string(password), "\r\n")
	if value == "" {
		return errors.New("addon registry password read from stdin is empty")
	}
	return cmd.Flags().Set(addonPassword, value)
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

	endpoint, err := cmd.Flags().GetString(addonEndpoint)
	if err != nil {
		return nil, err
	}
	if endpoint == "" {
		return nil, errors.New("addon registry must set --endpoint flag")
	}

	switch registryType {
	case addonOssType:
		r.OSS = &pkgaddon.OSSAddonSource{}

		r.OSS.Endpoint = endpoint
		bucket, err := cmd.Flags().GetString(addonOssBucket)
		if err != nil {
			return nil, err
		}
		r.OSS.Bucket = bucket
		path, err := cmd.Flags().GetString(addonPath)
		if err != nil {
			return nil, err
		}
		r.OSS.Path = path
	case addonGitType:
		r.Git = &pkgaddon.GitAddonSource{}
		r.Git.URL = endpoint
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
	case addonGiteeType:
		r.Gitee = &pkgaddon.GiteeAddonSource{}
		r.Gitee.URL = endpoint
		path, err := cmd.Flags().GetString(addonPath)
		if err != nil {
			return nil, err
		}
		r.Gitee.Path = path
		token, err := cmd.Flags().GetString(addonGitToken)
		if err != nil {
			return nil, err
		}
		r.Gitee.Token = token
	case addonGitlabType:
		r.Gitlab = &pkgaddon.GitlabAddonSource{}
		r.Gitlab.URL = endpoint
		path, err := cmd.Flags().GetString(addonPath)
		if err != nil {
			return nil, err
		}
		r.Gitlab.Path = path
		token, err := cmd.Flags().GetString(addonGitToken)
		if err != nil {
			return nil, err
		}
		r.Gitlab.Token = token
		gitLabRepoName, err := cmd.Flags().GetString(addonRepoName)
		if err != nil {
			return nil, err
		}
		r.Gitlab.Repo = gitLabRepoName
	case addonHelmType:
		r.Helm = &pkgaddon.HelmSource{}
		r.Helm.URL = endpoint
		r.Helm.Username, err = cmd.Flags().GetString(addonUsername)
		if err != nil {
			return nil, err
		}
		r.Helm.Password, err = cmd.Flags().GetString(addonPassword)
		if err != nil {
			return nil, err
		}
		r.Helm.InsecureSkipTLS, err = cmd.Flags().GetBool(addonHelmInsecureSkipTLS)
		if err != nil {
			return nil, err
		}
	case addonOCIType:
		r.OCI = &pkgaddon.OCIAddonSource{}
		r.OCI.URL = endpoint
		r.OCI.Username, err = cmd.Flags().GetString(addonUsername)
		if err != nil {
			return nil, err
		}
		r.OCI.Token, err = cmd.Flags().GetString(addonPassword)
		if err != nil {
			return nil, err
		}
		if (r.OCI.Username == "") != (r.OCI.Token == "") {
			return nil, errors.New("OCI registry username and password must be supplied together; omit both for anonymous access")
		}

	default:
		return nil, errors.New("not support addon registry type")
	}
	return r, nil
}
