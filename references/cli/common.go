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

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// constants used in `svc` command
const (
	App       = "app"
	Namespace = "namespace"

	// FlagDescription command flag to specify the description of the definition
	FlagDescription = "desc"
	// FlagAlias command flag to specify the alias of the definition
	FlagAlias = "alias"
	// FlagDryRun command flag to disable actual changes and only display intend changes
	FlagDryRun = "dry-run"
	// FlagTemplateYAML command flag to specify which existing template YAML file to use
	FlagTemplateYAML = "template-yaml"
	// FlagOutput command flag to specify which file to save
	FlagOutput = "output"
	// FlagMessage command flag to specify which file to save
	FlagMessage = "message"
	// FlagType command flag to specify which definition type to use
	FlagType = "type"
	// FlagProvider command flag to specify which provider the cloud resource definition belongs to. Only `alibaba`, `aws`, `azure` are supported.
	FlagProvider = "provider"
	// FlagGit command flag to specify which git repository the configuration(HCL) is stored in
	FlagGit = "git"
	// FlagLocal command flag to specify the local path of Terraform module or resource HCL file
	FlagLocal = "local"
	// FlagPath command flag to specify which path the configuration(HCL) is stored in the Git repository
	FlagPath = "path"
	// FlagNamespace command flag to specify which namespace to use
	FlagNamespace = "namespace"
	// FlagInteractive command flag to specify the use of interactive process
	FlagInteractive = "interactive"
)

func addNamespaceAndEnvArg(cmd *cobra.Command) {
	cmd.Flags().StringP(Namespace, "n", "", "specify the Kubernetes namespace to use")

	cmd.PersistentFlags().StringP("env", "e", "", "specify environment name for application")
}

// GetFlagNamespaceOrEnv will get env and namespace flag, namespace flag takes the priority
func GetFlagNamespaceOrEnv(cmd *cobra.Command, args common.Args) (string, error) {
	namespace, err := cmd.Flags().GetString(Namespace)
	if err != nil {
		return "", err
	}
	if namespace != "" {
		return namespace, nil
	}
	velaEnv, err := GetFlagEnvOrCurrent(cmd, args)
	if err != nil {
		return "", err
	}
	return velaEnv.Namespace, nil

}
