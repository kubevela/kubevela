package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloud-native-application/rudrx/api/types"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/ghodss/yaml"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	addonCenterConfigFile = ".rudr/addon_config"
	defaultAddonCenter    = "local"
)

//Used to store addon center config in file
type AddonCenterConfig struct {
	Name    string `json:"name"`
	IsLocal bool   `json:"isLocal"`
}
type PluginFile struct {
	Name string `json:"name"`
	Url  string `json:"download_url"`
	Sha  string `json:"sha"`
}

type Plugin struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Definition string `json:"definition"`
	Status     string `json:"status"`
	ApplesTo   string `json:"applies_to"`
}

func NewAddonConfigCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "addon:config",
		Short:   "Set the addon center, default is local (built-in ones)",
		Long:    "Set the addon center, default is local (built-in ones)",
		Example: `rudr addon:config <REPOSITORY>`,
		Run: func(cmd *cobra.Command, args []string) {
			argsLength := len(args)
			switch {
			case argsLength == 0:
				ioStreams.Errorf("Please set addon center, `local` or an URL.")
			case argsLength == 1:
				addonCenter := args[0]
				config := AddonCenterConfig{
					Name:    addonCenter,
					IsLocal: addonCenter == defaultAddonCenter,
				}
				var data []byte
				var err error
				var homeDir string
				if data, err = json.Marshal(config); err != nil {
					ioStreams.Errorf(fmt.Sprintf("Failed to configure Addon center: %s", addonCenter))
				}
				if homeDir, err = os.UserHomeDir(); err != nil {
					ioStreams.Errorf(fmt.Sprintf("Failed to configure Addon center: %s", addonCenter))
				}
				if err = ioutil.WriteFile(filepath.Join(homeDir, addonCenterConfigFile), data, 0644); err != nil {
					ioStreams.Errorf(fmt.Sprintf("Failed to configure Addon center: %s", addonCenter))
				}
				ioStreams.Info(fmt.Sprintf("Successfully configured Addon center: %s", addonCenter))
			case argsLength > 1:
				ioStreams.Errorf("Unnecessary arguments are specified, please try again")
			}
		},
	}
	return cmd
}

func NewAddonListCommand(c client.Client, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "addon:ls",
		Short:   "List addons",
		Long:    "List addons of workloads and traits",
		Example: `rudr addon:ls`,
		Run: func(cmd *cobra.Command, args []string) {
			env, err := GetEnv()
			if err != nil {
				ioStreams.Errorf("Failed to get Env information:%s", err)
				os.Exit(1)
			}
			err = retrievePlugins(ctx, c, ioStreams, env.Namespace)
			if err != nil {
				ioStreams.Errorf("Failed to list Addons:%s", err)
				os.Exit(1)
			}
		},
	}
	return cmd
}

func retrievePlugins(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, namespace string) error {
	var pluginList []Plugin
	var config AddonCenterConfig
	var data []byte
	var err error
	var homeDir string
	if homeDir, err = os.UserHomeDir(); err != nil {
		ioStreams.Errorf("Failed to retrieve addon center configuration, please run `rudr addon:config` first")
	}
	if data, err = ioutil.ReadFile(filepath.Join(homeDir, addonCenterConfigFile)); err != nil {
		ioStreams.Errorf("Failed to retrieve addon center configuration, please run `rudr addon:config` first")
	}
	if err := json.Unmarshal(data, &config); err != nil {
		ioStreams.Errorf("Failed to retrieve addon center configuration, please run `rudr addon:config` first")
	}

	if config.IsLocal {
		//TODO(zzxwill) merge `rudr traits` and `rudr workloads`
		return nil
	} else {
		resp, err := http.Get(config.Name)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		result, _ := ioutil.ReadAll(resp.Body)
		var manifests []PluginFile
		var traitManifestPrefix, workloadManifestPrefix = "TraitDefinition", "WorkloadDefinition"
		err = json.Unmarshal(result, &manifests)
		if err != nil {
			return err
		}
		var manifestResp *http.Response
		for _, d := range manifests {
			var template types.Template
			var workloadDefinition corev1alpha2.WorkloadDefinition
			var traitDefinition corev1alpha2.TraitDefinition
			if manifestResp, err = http.Get(d.Url); err != nil {
				return err
			}
			defer manifestResp.Body.Close()

			if strings.Contains(strings.ToLower(d.Name), strings.ToLower(workloadManifestPrefix)) {
				result, err := ioutil.ReadAll(manifestResp.Body)
				if err != nil {
					return err
				}
				err = yaml.Unmarshal(result, &workloadDefinition)
				if err != nil {
					return err
				}
				var definitionName string
				if workloadDefinition.Spec.Extension != nil {
					template, err = types.ConvertTemplateJson2Object(workloadDefinition.Spec.Extension)
					if err != nil {
						return err
					}
					definitionName = template.Name
				} else {
					definitionName = workloadDefinition.Name
				}

				if err != nil {
					return err
				}
				//Check whether the definition is applied
				var status = "uninstalled"
				if _, err = cmdutil.GetWorkloadDefinitionByName(ctx, c, namespace, workloadDefinition.Name); err == nil {
					status = "installed"
				}
				pluginList = append(pluginList, Plugin{
					Name:       definitionName,
					Type:       "workload",
					Definition: workloadDefinition.Spec.Reference.Name,
					Status:     status,
					ApplesTo:   "-",
				})
			} else if strings.Contains(strings.ToLower(d.Name), strings.ToLower(traitManifestPrefix)) {
				result, err := ioutil.ReadAll(manifestResp.Body)
				if err != nil {
					return err
				}
				err = yaml.Unmarshal(result, &traitDefinition)
				if err != nil {
					return err
				}
				var definitionName string
				if traitDefinition.Spec.Extension != nil {
					template, err = types.ConvertTemplateJson2Object(traitDefinition.Spec.Extension)
					if err != nil {
						return err
					}
					definitionName = template.Name
				} else {
					definitionName = traitDefinition.Name
				}

				//Check whether the definition is applied
				var status = "uninstalled"
				if _, err = cmdutil.GetTraitDefinitionByName(ctx, c, namespace, traitDefinition.Name); err == nil {
					status = "installed"
				}
				pluginList = append(pluginList, Plugin{
					Name:       definitionName,
					Type:       "trait",
					Definition: traitDefinition.Spec.Reference.Name,
					Status:     status,
					ApplesTo:   strings.Join(traitDefinition.Spec.AppliesToWorkloads, ","),
				})
			} else {
				ioStreams.Errorf(fmt.Sprintf("Those manifests in addon repository should start with %s or %s",
					workloadManifestPrefix, traitManifestPrefix))
				os.Exit(1)
			}
		}

		table := uitable.New()
		table.MaxColWidth = 60
		table.AddRow("NAME", "TYPE", "DEFINITION", "STATUS", "APPLIES-TO")
		for _, p := range pluginList {
			table.AddRow(p.Name, p.Type, p.Definition, p.Status, p.ApplesTo)
		}
		ioStreams.Info(table.String())
	}
	return nil
}
