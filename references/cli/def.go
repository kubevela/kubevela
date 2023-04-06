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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/encoding/gocode/gocodec"
	"github.com/kubevela/workflow/pkg/cue/model/sets"
	crossplane "github.com/oam-dev/terraform-controller/api/types/crossplane-runtime"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	types2 "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontype "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/definition/gen_sdk"
	"github.com/oam-dev/kubevela/pkg/utils"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/filters"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	// HelmChartNamespacePlaceholder is used as a placeholder for rendering definitions into helm chart format
	HelmChartNamespacePlaceholder = "###HELM_NAMESPACE###"
	// HelmChartFormatEnvName is the name of the environment variable to enable render helm chart format YAML
	HelmChartFormatEnvName = "AS_HELM_CHART"
)

// DefinitionCommandGroup create the command group for `vela def` command to manage definitions
func DefinitionCommandGroup(c common.Args, order string, ioStreams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "def",
		Short: "Manage Definitions",
		Long:  "Manage X-Definitions for extension.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeExtension,
		},
	}
	cmd.AddCommand(
		NewDefinitionGetCommand(c),
		NewDefinitionListCommand(c),
		NewDefinitionEditCommand(c),
		NewDefinitionRenderCommand(c),
		NewDefinitionApplyCommand(c, ioStreams),
		NewDefinitionDelCommand(c),
		NewDefinitionInitCommand(c),
		NewDefinitionValidateCommand(c),
		NewDefinitionGenDocCommand(c, ioStreams),
		NewCapabilityShowCommand(c, ioStreams),
		NewDefinitionGenAPICommand(c),
	)
	return cmd
}

func getPrompt(cmd *cobra.Command, reader *bufio.Reader, description string, prompt string, validate func(string) error) (string, error) {
	cmd.Printf(description)
	for {
		cmd.Printf(prompt)
		resp, err := reader.ReadString('\n')
		resp = strings.TrimSpace(resp)
		if err != nil {
			return "", errors.Wrapf(err, "failed to read user response")
		}
		if validate == nil {
			return resp, nil
		}
		err = validate(resp)
		if err != nil {
			cmd.Println(err)
		} else {
			return resp, nil
		}
	}
}

// nolint:staticcheck
func buildTemplateFromYAML(templateYAML string, def *pkgdef.Definition) error {
	templateYAMLBytes, err := utils.ReadRemoteOrLocalPath(templateYAML, false)
	if err != nil {
		return errors.Wrapf(err, "failed to get template YAML file %s", templateYAML)
	}
	yamlStrings := regexp.MustCompile(`\n---[^\n]*\n`).Split(string(templateYAMLBytes), -1)
	templateObject := map[string]interface{}{
		process.OutputFieldName:    map[string]interface{}{},
		process.OutputsFieldName:   map[string]interface{}{},
		process.ParameterFieldName: map[string]interface{}{},
	}
	for index, yamlString := range yamlStrings {
		var yamlObject map[string]interface{}
		if err = yaml.Unmarshal([]byte(yamlString), &yamlObject); err != nil {
			return errors.Wrapf(err, "failed to unmarshal template yaml file")
		}
		if index == 0 {
			templateObject[process.OutputFieldName] = yamlObject
		} else {
			name, _, _ := unstructured.NestedString(yamlObject, "metadata", "name")
			if name == "" {
				name = fmt.Sprintf("output-%d", index)
			}
			templateObject[process.OutputsFieldName].(map[string]interface{})[name] = yamlObject
		}
	}
	codec := gocodec.New((*cue.Runtime)(cuecontext.New()), &gocodec.Config{})
	val, err := codec.Decode(templateObject)
	if err != nil {
		return errors.Wrapf(err, "failed to decode template into cue")
	}
	templateString, err := sets.ToString(val)
	if err != nil {
		return errors.Wrapf(err, "failed to encode template cue string")
	}
	err = unstructured.SetNestedField(def.Object, templateString, pkgdef.DefinitionTemplateKeys...)
	if err != nil {
		return errors.Wrapf(err, "failed to merge template cue string")
	}
	return nil
}

// NewDefinitionInitCommand create the `vela def init` command to help user initialize a definition locally
func NewDefinitionInitCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init DEF_NAME",
		Short: "Init a new definition",
		Long: "Init a new definition with given arguments or interactively\n* We support parsing a single YAML file (like kubernetes objects) into the cue-style template. \n" +
			"However, we do not support variables in YAML file currently, which prevents users from directly feeding files like helm chart directly. \n" +
			"We may introduce such features in the future.",
		Example: "# Command below initiate an empty TraitDefinition named my-ingress\n" +
			"> vela def init my-ingress -t trait --desc \"My ingress trait definition.\" > ./my-ingress.cue\n" +
			"# Command below initiate a definition named my-def interactively and save it to ./my-def.cue\n" +
			"> vela def init my-def -i --output ./my-def.cue\n" +
			"# Command below initiate a ComponentDefinition named my-webservice with the template parsed from ./template.yaml.\n" +
			"> vela def init my-webservice -i --template-yaml ./template.yaml\n" +
			"# Initiate a Terraform ComponentDefinition named vswitch from Github for Alibaba Cloud.\n" +
			"> vela def init vswitch --type component --provider alibaba --desc xxx --git https://github.com/kubevela-contrib/terraform-modules.git --path alibaba/vswitch\n" +
			"# Initiate a Terraform ComponentDefinition named redis from local file for AWS.\n" +
			"> vela def init redis --type component --provider aws --desc \"Terraform configuration for AWS Redis\" --local redis.tf",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var defStr string
			definitionType, err := cmd.Flags().GetString(FlagType)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagType)
			}
			alias, err := cmd.Flags().GetString(FlagAlias)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagAlias)
			}
			desc, err := cmd.Flags().GetString(FlagDescription)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagDescription)
			}
			templateYAML, err := cmd.Flags().GetString(FlagTemplateYAML)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagTemplateYAML)
			}
			output, err := cmd.Flags().GetString(FlagOutput)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagOutput)
			}
			interactive, err := cmd.Flags().GetBool(FlagInteractive)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagInteractive)
			}

			if interactive {
				reader := bufio.NewReader(cmd.InOrStdin())
				if definitionType == "" {
					if definitionType, err = getPrompt(cmd, reader, "Please choose one definition type from the following values: "+strings.Join(pkgdef.ValidDefinitionTypes(), ", ")+"\n", "> Definition type: ", func(resp string) error {
						if _, ok := pkgdef.DefinitionTypeToKind[resp]; !ok {
							return errors.New("invalid definition type")
						}
						return nil
					}); err != nil {
						return err
					}
				}
				if desc == "" {
					if desc, err = getPrompt(cmd, reader, "", "> Definition description: ", nil); err != nil {
						return err
					}
				}
				if templateYAML == "" {
					if templateYAML, err = getPrompt(cmd, reader, "Please enter the location the template YAML file to build definition. Leave it empty to generate default template.\n", "> Definition template filename: ", func(resp string) error {
						if resp == "" {
							return nil
						}
						_, err = os.Stat(resp)
						return err
					}); err != nil {
						return err
					}
				}
				if output == "" {
					if output, err = getPrompt(cmd, reader, "Please enter the output location of the generated definition. Leave it empty to print definition to stdout.\n", "> Definition output filename: ", nil); err != nil {
						return err
					}
				}
			}

			kind, ok := pkgdef.DefinitionTypeToKind[definitionType]
			if !ok {
				return errors.New("invalid definition type")
			}

			name := args[0]
			provider, err := cmd.Flags().GetString(FlagProvider)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagProvider)
			}
			if provider != "" {
				defStr, err = generateTerraformTypedComponentDefinition(cmd, name, kind, provider, desc)
				if err != nil {
					return errors.Wrapf(err, "failed to generate Terraform typed component definition")
				}
			} else {
				def := pkgdef.Definition{Unstructured: unstructured.Unstructured{}}
				def.SetGVK(kind)
				def.SetName(name)
				def.SetAnnotations(map[string]string{
					pkgdef.DescriptionKey: desc,
					pkgdef.AliasKey:       alias,
				})
				def.SetLabels(map[string]string{})
				def.Object["spec"] = pkgdef.GetDefinitionDefaultSpec(def.GetKind())
				if templateYAML != "" {
					if err = buildTemplateFromYAML(templateYAML, &def); err != nil {
						return err
					}
				}
				defStr, err = def.ToCUEString()
				if err != nil {
					return errors.Wrapf(err, "failed to generate cue string")
				}
			}
			if output != "" {
				if err = os.WriteFile(path.Clean(output), []byte(defStr), 0600); err != nil {
					return errors.Wrapf(err, "failed to write definition into %s", output)
				}
				cmd.Printf("Definition written to %s\n", output)
			} else if _, err = cmd.OutOrStdout().Write([]byte(defStr + "\n")); err != nil {
				return errors.Wrapf(err, "failed to write out cue string")
			}
			return nil
		},
	}
	cmd.Flags().StringP(FlagType, "t", "", "Specify the type of the new definition. Valid types: "+strings.Join(pkgdef.ValidDefinitionTypes(), ", "))
	cmd.Flags().StringP(FlagDescription, "d", "", "Specify the description of the new definition.")
	cmd.Flags().StringP(FlagAlias, "a", "", "Specify the alias of the new definition.")
	cmd.Flags().StringP(FlagTemplateYAML, "f", "", "Specify the template yaml file that definition will use to build the schema. If empty, a default template for the given definition type will be used.")
	cmd.Flags().StringP(FlagOutput, "o", "", "Specify the output path of the generated definition. If empty, the definition will be printed in the console.")
	cmd.Flags().BoolP(FlagInteractive, "i", false, "Specify whether use interactive process to help generate definitions.")
	cmd.Flags().StringP(FlagProvider, "p", "", "Specify which provider the cloud resource definition belongs to. Only `alibaba`, `aws`, `azure`, `gcp`, `baidu`, `tencent`, `elastic`, `ucloud`, `vsphere` are supported.")
	cmd.Flags().StringP(FlagGit, "", "", "Specify which git repository the configuration(HCL) is stored in. Valid when --provider/-p is set.")
	cmd.Flags().StringP(FlagLocal, "", "", "Specify the local path of the configuration(HCL) file. Valid when --provider/-p is set.")
	cmd.Flags().StringP(FlagPath, "", "", "Specify which path the configuration(HCL) is stored in the Git repository. Valid when --git is set.")
	return cmd
}

func generateTerraformTypedComponentDefinition(cmd *cobra.Command, name, kind, provider, desc string) (string, error) {
	if kind != v1beta1.ComponentDefinitionKind {
		return "", errors.New("provider is only valid when the type of the definition is component")
	}

	switch provider {
	case "aws", "azure", "alibaba", "tencent", "gcp", "baidu", "elastic", "ucloud", "vsphere", "huawei":
		var terraform *commontype.Terraform

		git, err := cmd.Flags().GetString(FlagGit)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get `%s`", FlagGit)
		}
		local, err := cmd.Flags().GetString(FlagLocal)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get `%s`", FlagLocal)
		}
		if git != "" && local != "" {
			return "", errors.New("only one of --git and --local can be set")
		}
		gitPath, err := cmd.Flags().GetString(FlagPath)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get `%s`", FlagPath)
		}
		if git != "" {
			if !strings.HasPrefix(git, "https://") || !strings.HasSuffix(git, ".git") {
				return "", errors.Errorf("invalid git url: %s", git)
			}
			terraform = &commontype.Terraform{
				Configuration: git,
				Type:          "remote",
				Path:          gitPath,
			}
		} else if local != "" {
			hcl, err := os.ReadFile(filepath.Clean(local))
			if err != nil {
				return "", errors.Wrapf(err, "failed to read Terraform configuration from file %s", local)
			}
			terraform = &commontype.Terraform{
				Configuration: string(hcl),
			}
		}
		def := v1beta1.ComponentDefinition{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "core.oam.dev/v1beta1",
				Kind:       "ComponentDefinition",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", provider, name),
				Namespace: types.DefaultKubeVelaNS,
				Annotations: map[string]string{
					"definition.oam.dev/description": desc,
				},
				Labels: map[string]string{
					"type": "terraform",
				},
			},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload: commontype.WorkloadTypeDescriptor{
					Definition: commontype.WorkloadGVK{
						APIVersion: "terraform.core.oam.dev/v1beta2",
						Kind:       "Configuration",
					},
				},
				Schematic: &commontype.Schematic{
					Terraform: terraform,
				},
			},
		}
		if provider != "alibaba" {
			def.Spec.Schematic.Terraform.ProviderReference = &crossplane.Reference{
				Name:      provider,
				Namespace: "default",
			}
		}
		var out bytes.Buffer
		err = json.NewSerializerWithOptions(json.DefaultMetaFactory, nil, nil, json.SerializerOptions{Yaml: true}).Encode(&def, &out)
		if err != nil {
			return "", errors.Wrapf(err, "failed to marshal component definition")
		}
		return out.String(), nil
	default:
		return "", errors.Errorf("Provider `%s` is not supported. Only `alibaba`, `aws`, `azure`, `gcp`, `baidu`, `tencent`, `elastic`, `ucloud`, `vsphere` are supported.", provider)
	}
}

func getSingleDefinition(cmd *cobra.Command, definitionName string, client client.Client, definitionType string, namespace string) (*pkgdef.Definition, error) {
	definitions, err := pkgdef.SearchDefinition(client, definitionType, namespace, filters.ByName(definitionName))
	if err != nil {
		return nil, err
	}
	if len(definitions) == 0 {
		return nil, fmt.Errorf("definition not found")
	}
	if len(definitions) > 1 {
		table := newUITable()
		table.AddRow("NAME", "TYPE", "NAMESPACE", "DESCRIPTION")
		for _, definition := range definitions {
			desc := ""
			if annotations := definition.GetAnnotations(); annotations != nil {
				desc = annotations[pkgdef.DescriptionKey]
			}
			table.AddRow(definition.GetName(), definition.GetKind(), definition.GetNamespace(), desc)
		}
		cmd.Println(table)
		return nil, fmt.Errorf("found %d definitions, please specify which one to select with more arguments", len(definitions))
	}
	return &pkgdef.Definition{Unstructured: definitions[0]}, nil
}

// getDefRevs will search for DefinitionRevisions with specified conditions.
// Check SearchDefinitionRevisions for details.
func getDefRevs(ctx context.Context, client client.Client, ns, defTypeStr, defName string, rev int64) ([]v1beta1.DefinitionRevision, error) {
	defType, ok := pkgdef.StringToDefinitionType[defTypeStr]
	// Empty definition type is intentionally allowed, to allow the user to match all definition types
	if defTypeStr != "" && !ok {
		return nil, fmt.Errorf("%s is not a valid type. Valid types are %v", defTypeStr, reflect.ValueOf(pkgdef.StringToDefinitionType).MapKeys())
	}

	return pkgdef.SearchDefinitionRevisions(ctx, client, ns, defName, defType, rev)
}

// printDefRevs will print DefinitionRevisions
func printDefRevs(ctx context.Context, cmd *cobra.Command, client client.Client, ns, defTypeStr, defName string) error {
	revs, err := getDefRevs(ctx, client, ns, defTypeStr, defName, 0)
	if err != nil {
		return err
	}

	table := newUITable()
	table.AddRow("NAME", "REVISION", "TYPE", "HASH")
	for _, rev := range revs {
		table.AddRow(defName, rev.Spec.Revision, rev.Spec.DefinitionType, rev.Spec.RevisionHash)
	}
	cmd.Println(table)

	return nil
}

// NewDefinitionGetCommand create the `vela def get` command to get definition from k8s
func NewDefinitionGetCommand(c common.Args) *cobra.Command {
	var listRevisions bool
	var targetRevision string
	cmd := &cobra.Command{
		Use:   "get NAME",
		Short: "Get definition",
		Long:  "Get definition from kubernetes cluster",
		Example: "# Command below will get the ComponentDefinition(or other definitions if exists) of webservice in all namespaces\n" +
			"> vela def get webservice\n" +
			"# Command below will get the TraitDefinition of annotations in namespace vela-system\n" +
			"> vela def get annotations --type trait --namespace vela-system",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			definitionType, err := cmd.Flags().GetString(FlagType)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagType)
			}
			namespace, err := cmd.Flags().GetString(FlagNamespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", Namespace)
			}
			k8sClient, err := c.GetClient()
			if err != nil {
				return errors.Wrapf(err, "failed to get k8s client")
			}

			if listRevisions {
				return printDefRevs(context.Background(), cmd, k8sClient, namespace, definitionType, args[0])
			}

			var def *pkgdef.Definition

			// Get history Definition from DefinitionRevisions
			if targetRevision != "" {
				// "v1", "1", both need to work
				targetRevision = strings.TrimPrefix(targetRevision, "v")
				ver, err := strconv.Atoi(targetRevision)
				if err != nil {
					return fmt.Errorf("invalid version: %w", err)
				}

				// Get the user-specified revision.
				revs, err := getDefRevs(context.Background(), k8sClient, namespace, definitionType, args[0], int64(ver))
				if err != nil {
					return err
				}
				if len(revs) == 0 {
					return fmt.Errorf("no %s with revision %s found in namespace %s", args[0], targetRevision, namespace)
				}

				// Now we have at least one DefinitionRevision (typically it will only be one).
				// They all fit user's conditions. We will use the first one.
				// Extract Definition from DefinitionRevision that we just got.
				def, err = pkgdef.GetDefinitionFromDefinitionRevision(&revs[0])
				if err != nil {
					return err
				}
			} else {
				def, err = getSingleDefinition(cmd, args[0], k8sClient, definitionType, namespace)
				if err != nil {
					return err
				}
			}

			cueString, err := def.ToCUEString()
			if err != nil {
				return errors.Wrapf(err, "failed to get cue format definition")
			}
			if _, err = cmd.OutOrStdout().Write([]byte(cueString + "\n")); err != nil {
				return errors.Wrapf(err, "failed to write out cue string")
			}
			return nil
		},
	}
	cmd.Flags().StringP(FlagType, "t", "", "Specify which definition type to get. If empty, all types will be searched. Valid types: "+strings.Join(pkgdef.ValidDefinitionTypes(), ", "))
	cmd.Flags().BoolVarP(&listRevisions, "revisions", "", false, "List revisions of the specified definition.")
	cmd.Flags().StringVarP(&targetRevision, "revision", "r", "", "Get the specified version of a definition.")
	cmd.Flags().StringP(Namespace, "n", types.DefaultKubeVelaNS, "Specify which namespace the definition locates.")
	return cmd
}

// NewDefinitionGenDocCommand create the `vela def doc-gen` command to generate documentation of definitions
func NewDefinitionGenDocCommand(c common.Args, ioStreams util.IOStreams) *cobra.Command {
	var docPath, location, i18nPath string
	cmd := &cobra.Command{
		Use:   "doc-gen NAME",
		Short: "Generate documentation for definitions",
		Long:  "Generate documentation for definitions",
		Example: "1. Generate documentation for ComponentDefinition webservice:\n" +
			"> vela def doc-gen webservice -n vela-system\n" +
			"2. Generate documentation for local CUE Definition file webservice.cue:\n" +
			"> vela def doc-gen webservice.cue\n" +
			"3. Generate documentation for local Cloud Resource Definition YAML alibaba-vpc.yaml:\n" +
			"> vela def doc-gen alibaba-vpc.yaml\n",
		Deprecated: "This command has been replaced by 'vela show' or 'vela def show'.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("please specify definition name, cue file or a cloud resource definition yaml")
			}
			namespace, err := cmd.Flags().GetString(FlagNamespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", Namespace)
			}
			return ShowReferenceMarkdown(context.Background(), c, ioStreams, args[0], docPath, location, i18nPath, namespace, 0)

		},
	}
	cmd.Flags().StringVarP(&docPath, "path", "p", "", "Specify the path for of the doc generated from definition.")
	cmd.Flags().StringVarP(&location, "location", "l", "", "specify the location for of the doc generated from definition, now supported options 'zh', 'en'. ")
	cmd.Flags().StringP(Namespace, "n", types.DefaultKubeVelaNS, "Specify which namespace the definition locates.")
	cmd.Flags().StringVarP(&i18nPath, "i18n", "", "https://kubevela.io/reference-i18n.json", "specify the location for of the doc generated from definition, now supported options 'zh', 'en'. ")
	return cmd
}

// NewDefinitionListCommand create the `vela def list` command to list definition from k8s
func NewDefinitionListCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List definitions.",
		Long:  "List definitions in kubernetes cluster.",
		Example: "# Command below will list all definitions in all namespaces\n" +
			"> vela def list\n" +
			"# Command below will list all definitions in the vela-system namespace\n" +
			"> vela def get annotations --type trait --namespace vela-system",
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			definitionType, err := cmd.Flags().GetString(FlagType)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagType)
			}
			namespace, err := cmd.Flags().GetString(FlagNamespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", Namespace)
			}
			addonName, err := cmd.Flags().GetString("from")
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", "from")
			}
			k8sClient, err := c.GetClient()
			if err != nil {
				return errors.Wrapf(err, "failed to get k8s client")
			}
			definitions, err := pkgdef.SearchDefinition(k8sClient,
				definitionType,
				namespace,
				filters.ByOwnerAddon(addonName))
			if err != nil {
				return err
			}
			if len(definitions) == 0 {
				cmd.Println("No definition found.")
				return nil
			}
			// Determine if there is a definition in the list from some addons
			// This is used to tell if we want the SOURCE-ADDON column
			showSourceAddon := false
			for _, def := range definitions {
				ownerRef := def.GetOwnerReferences()
				if len(ownerRef) > 0 && strings.HasPrefix(ownerRef[0].Name, addonutil.AddonAppPrefix) {
					showSourceAddon = true
					break
				}
			}
			table := newUITable()

			// We only include SOURCE-ADDON if there is at least one definition from an addon
			if showSourceAddon {
				table.AddRow("NAME", "TYPE", "NAMESPACE", "SOURCE-ADDON", "DESCRIPTION")
			} else {
				table.AddRow("NAME", "TYPE", "NAMESPACE", "DESCRIPTION")
			}

			for _, definition := range definitions {
				desc := ""
				if annotations := definition.GetAnnotations(); annotations != nil {
					desc = annotations[pkgdef.DescriptionKey]
				}

				// Do not show SOURCE-ADDON column
				if !showSourceAddon {
					table.AddRow(definition.GetName(), definition.GetKind(), definition.GetNamespace(), desc)
					continue
				}

				sourceAddon := ""
				if len(definition.GetOwnerReferences()) > 0 {
					sourceAddon = strings.TrimPrefix(definition.GetOwnerReferences()[0].Name, "addon-")
				}
				table.AddRow(definition.GetName(), definition.GetKind(), definition.GetNamespace(), sourceAddon, desc)
			}
			cmd.Println(table)
			return nil
		},
	}
	cmd.Flags().StringP(FlagType, "t", "", "Specify which definition type to list. If empty, all types will be searched. Valid types: "+strings.Join(pkgdef.ValidDefinitionTypes(), ", "))
	cmd.Flags().String("from", "", "Filter definitions by which addon installed them.")
	cmd.Flags().StringP(Namespace, "n", types.DefaultKubeVelaNS, "Specify which namespace the definition locates.")
	return cmd
}

// NewDefinitionEditCommand create the `vela def edit` command to help user edit remote definitions
func NewDefinitionEditCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit NAME",
		Short: "Edit X-Definition.",
		Long: "Edit X-Definition in kubernetes. If type and namespace are not specified, the command will automatically search all possible results.\n" +
			"By default, this command will use the vi editor and can be altered by setting EDITOR environment variable.",
		Example: "# Command below will edit the ComponentDefinition (and other definitions if exists) of webservice in kubernetes\n" +
			"> vela def edit webservice\n" +
			"# Command below will edit the TraitDefinition of ingress in vela-system namespace\n" +
			"> vela def edit ingress --type trait --namespace vela-system",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			definitionType, err := cmd.Flags().GetString(FlagType)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagType)
			}
			namespace, err := cmd.Flags().GetString(FlagNamespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", Namespace)
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			k8sClient, err := c.GetClient()
			if err != nil {
				return errors.Wrapf(err, "failed to get k8s client")
			}
			def, err := getSingleDefinition(cmd, args[0], k8sClient, definitionType, namespace)
			if err != nil {
				return err
			}
			cueString, err := def.ToCUEString()
			if err != nil {
				return errors.Wrapf(err, "failed to get cue format definition")
			}
			cleanup := func(filePath string) {
				if err := os.Remove(filePath); err != nil {
					cmd.PrintErrf("failed to remove file %s: %v", filePath, err)
				}
			}
			filename := fmt.Sprintf("vela-def-%d", time.Now().UnixNano())
			tempFilePath := filepath.Join(os.TempDir(), filename+".cue")
			if err := os.WriteFile(tempFilePath, []byte(cueString), 0600); err != nil {
				return errors.Wrapf(err, "failed to write temporary file")
			}
			defer cleanup(tempFilePath)
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			scriptFilePath := filepath.Join(os.TempDir(), filename+".sh")
			if err := os.WriteFile(scriptFilePath, []byte(editor+" "+tempFilePath), 0600); err != nil {
				return errors.Wrapf(err, "failed to write temporary script file")
			}
			defer cleanup(scriptFilePath)

			editCmd := exec.Command("sh", path.Clean(scriptFilePath)) //nolint:gosec
			editCmd.Stdin = os.Stdin
			editCmd.Stdout = os.Stdout
			editCmd.Stderr = os.Stderr
			if err = editCmd.Run(); err != nil {
				return errors.Wrapf(err, "failed to run editor %s at path %s", editor, scriptFilePath)
			}
			newBuf, err := os.ReadFile(path.Clean(tempFilePath))
			if err != nil {
				return errors.Wrapf(err, "failed to read temporary file %s", tempFilePath)
			}
			if cueString == string(newBuf) {
				cmd.Printf("definition unchanged\n")
				return nil
			}
			if err := def.FromCUEString(string(newBuf), config); err != nil {
				return errors.Wrapf(err, "failed to load edited cue string")
			}
			if err := k8sClient.Update(context.Background(), def); err != nil {
				return errors.Wrapf(err, "failed to apply changes to kubernetes")
			}
			cmd.Printf("Definition edited successfully.\n")
			return nil
		},
	}
	cmd.Flags().StringP(FlagType, "t", "", "Specify which definition type to get. If empty, all types will be searched. Valid types: "+strings.Join(pkgdef.ValidDefinitionTypes(), ", "))
	cmd.Flags().StringP(Namespace, "n", types.DefaultKubeVelaNS, "Specify which namespace the definition locates.")
	return cmd
}

func prettyYAMLMarshal(obj map[string]interface{}) (string, error) {
	var b bytes.Buffer
	encoder := yaml.NewEncoder(&b)
	encoder.SetIndent(2)
	err := encoder.Encode(&obj)
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

// NewDefinitionRenderCommand create the `vela def render` command to help user render definition cue file into k8s YAML file, if used without kubernetes environment, set IGNORE_KUBE_CONFIG=true
func NewDefinitionRenderCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "render DEFINITION.cue",
		Short: "Render X-Definition.",
		Long:  "Render X-Definition with cue format into kubernetes YAML format. Could be used to check whether the cue format definition is working as expected. If a directory is used as input, all cue definitions in the directory will be rendered.",
		Example: "# Command below will render my-webservice.cue into YAML format and print it out.\n" +
			"> vela def render my-webservice.cue\n" +
			"# Command below will render my-webservice.cue and save it in my-webservice.yaml.\n" +
			"> vela def render my-webservice.cue -o my-webservice.yaml" +
			"# Command below will render all cue format definitions in the ./defs/cue/ directory and save the YAML objects in ./defs/yaml/.\n" +
			"> vela def render ./defs/cue/ -o ./defs/yaml/",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			output, err := cmd.Flags().GetString(FlagOutput)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagOutput)
			}
			message, err := cmd.Flags().GetString(FlagMessage)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagMessage)
			}

			render := func(inputFilename, outputFilename string) error {
				cueBytes, err := utils.ReadRemoteOrLocalPath(inputFilename, false)
				if err != nil {
					return errors.Wrapf(err, "failed to get %s", args[0])
				}
				config, err := c.GetConfig()
				if err != nil {
					return err
				}
				def := pkgdef.Definition{Unstructured: unstructured.Unstructured{}}
				if err := def.FromCUEString(string(cueBytes), config); err != nil {
					return errors.Wrapf(err, "failed to parse CUE")
				}

				helmChartFormatEnv := strings.ToLower(os.Getenv(HelmChartFormatEnvName))
				if helmChartFormatEnv == "true" {
					def.SetNamespace(HelmChartNamespacePlaceholder)
				} else if helmChartFormatEnv == "system" {
					def.SetNamespace(types.DefaultKubeVelaNS)
				}
				if len(def.GetLabels()) == 0 {
					def.SetLabels(nil)
				}
				s, err := prettyYAMLMarshal(def.Object)
				if err != nil {
					return errors.Wrapf(err, "failed to marshal CRD into YAML")
				}
				s = strings.ReplaceAll(s, "'"+HelmChartNamespacePlaceholder+"'", "{{ include \"systemDefinitionNamespace\" . }}") + "\n"
				if outputFilename == "" {
					s = fmt.Sprintf("--- %s ---\n%s", filepath.Base(inputFilename), s)
					cmd.Print(s)
				} else {
					if message != "" {
						s = "# " + strings.ReplaceAll(message, "{{INPUT_FILENAME}}", filepath.Base(inputFilename)) + "\n" + s
					}
					s = "# Code generated by KubeVela templates. DO NOT EDIT. Please edit the original cue file.\n" + s
					if err := os.WriteFile(outputFilename, []byte(s), 0600); err != nil {
						return errors.Wrapf(err, "failed to write YAML format definition to file %s", outputFilename)
					}
				}
				return nil
			}
			inputFilenames := []string{args[0]}
			outputFilenames := []string{output}
			fi, err := os.Stat(args[0])
			if err != nil {
				return errors.Wrapf(err, "failed to get input %s", args[0])
			}
			if fi.IsDir() {
				inputFilenames = []string{}
				outputFilenames = []string{}
				err := filepath.Walk(args[0], func(path string, info os.FileInfo, err error) error {
					filename := filepath.Base(path)
					fileSuffix := filepath.Ext(path)
					if fileSuffix != ".cue" {
						return nil
					}
					inputFilenames = append(inputFilenames, path)
					if output != "" {
						outputFilenames = append(outputFilenames, filepath.Join(output, strings.ReplaceAll(filename, ".cue", ".yaml")))
					} else {
						outputFilenames = append(outputFilenames, "")
					}
					return nil
				})
				if err != nil {
					return errors.Wrapf(err, "failed to read directory %s", args[0])
				}
			}
			for i, inputFilename := range inputFilenames {
				if err = render(inputFilename, outputFilenames[i]); err != nil {
					if _, err = fmt.Fprintf(cmd.ErrOrStderr(), "failed to render %s, reason: %v", inputFilename, err); err != nil {
						return errors.Wrapf(err, "failed to write err")
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringP(FlagOutput, "o", "", "Specify the output path of the rendered definition YAML. If empty, the definition will be printed in the console. If input is a directory, the output path is expected to be a directory as well.")
	cmd.Flags().StringP(FlagMessage, "", "", "Specify the header message of the generated YAML file. For example, declaring author information.")
	return cmd
}

// NewDefinitionApplyCommand create the `vela def apply` command to help user apply local definitions to k8s
func NewDefinitionApplyCommand(c common.Args, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply DEFINITION.cue",
		Short: "Apply X-Definition.",
		Long:  "Apply X-Definition from local storage to kubernetes cluster. It will apply file to vela-system namespace by default.",
		Example: "# Command below will apply the local my-webservice.cue file to kubernetes vela-system namespace\n" +
			"> vela def apply my-webservice.cue\n" +
			"# Apply the local directory including all files(YAML and CUE definition) to kubernetes vela-system namespace\n" +
			"> vela def apply def/\n" +
			"# Command below will apply the ./defs/my-trait.cue file to kubernetes default namespace\n" +
			"> vela def apply ./defs/my-trait.cue --namespace default" +
			"# Command below will convert the ./defs/my-trait.cue file to kubernetes CRD object and print it without applying it to kubernetes\n" +
			"> vela def apply ./defs/my-trait.cue --dry-run" +
			"# Apply a CUE from URL \n" +
			"> vela def apply https://my-host-to-def/my-trait.cue --dry-run" +
			"# Apply a CUE from stdin \n" +
			"> vela def apply -",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			dryRun, err := cmd.Flags().GetBool(FlagDryRun)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagDryRun)
			}
			namespace, err := cmd.Flags().GetString(FlagNamespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", Namespace)
			}
			if len(args) < 1 {
				return errors.New("you must specify the definition path, directory or URL")
			}
			return defApplyAll(ctx, c, streams, namespace, args[0], dryRun)
		},
	}

	cmd.Flags().BoolP(FlagDryRun, "", false, "only build definition from CUE into CRB object without applying it to kubernetes clusters")
	cmd.Flags().StringP(Namespace, "n", types.DefaultKubeVelaNS, "Specify which namespace the definition locates.")
	return cmd
}

func defApplyAll(ctx context.Context, c common.Args, io util.IOStreams, namespace, path string, dryRun bool) error {
	files, err := utils.LoadDataFromPath(ctx, path, utils.IsJSONYAMLorCUEFile)
	if err != nil {
		return errors.Wrapf(err, "failed to get from %s", path)
	}
	for _, f := range files {
		result, err := defApplyOne(ctx, c, namespace, f.Path, f.Data, dryRun)
		if err != nil {
			return err
		}
		io.Infonln(result)
	}
	return nil
}

func defApplyOne(ctx context.Context, c common.Args, namespace, defpath string, defBytes []byte, dryRun bool) (string, error) {
	config, err := c.GetConfig()
	if err != nil {
		return "", err
	}
	k8sClient, err := c.GetClient()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get k8s client")
	}

	def := pkgdef.Definition{Unstructured: unstructured.Unstructured{}}

	switch {
	case strings.HasSuffix(defpath, ".yaml") || strings.HasSuffix(defpath, ".yml"):
		// In this case, it's not in cue format, it's a yaml
		if err = def.FromYAML(defBytes); err != nil {
			return "", errors.Wrapf(err, "failed to parse YAML to definition")
		}
		if dryRun {
			return "", errors.New("dry-run will render CUE to YAML, while the input is already in yaml")
		}
		// YAML won't validate or format CUE schematic
		op, err := utils.CreateOrUpdate(ctx, k8sClient, &def)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s in namespace %s %s.\n", def.GetKind(), def.GetName(), def.GetNamespace(), op), nil
	default:
		if err := def.FromCUEString(string(defBytes), config); err != nil {
			return "", errors.Wrapf(err, "failed to parse CUE for definition")
		}
		def.SetNamespace(namespace)
	}

	if dryRun {
		s, err := prettyYAMLMarshal(def.Object)
		if err != nil {
			return "", errors.Wrapf(err, "failed to marshal CRD into YAML")
		}
		return s, nil
	}

	oldDef := pkgdef.Definition{Unstructured: unstructured.Unstructured{}}
	oldDef.SetGroupVersionKind(def.GroupVersionKind())
	err = k8sClient.Get(ctx, types2.NamespacedName{
		Namespace: def.GetNamespace(),
		Name:      def.GetName(),
	}, &oldDef)
	if err != nil {
		if errors2.IsNotFound(err) {
			kind := def.GetKind()
			if err = k8sClient.Create(ctx, &def); err != nil {
				return "", errors.Wrapf(err, "failed to create new definition in kubernetes")
			}
			return fmt.Sprintf("%s %s created in namespace %s.\n", kind, def.GetName(), def.GetNamespace()), nil
		}
		return "", errors.Wrapf(err, "failed to check existence of target definition in kubernetes")
	}
	if err := oldDef.FromCUEString(string(defBytes), config); err != nil {
		return "", errors.Wrapf(err, "failed to merge with existing definition")
	}
	if err = k8sClient.Update(ctx, &oldDef); err != nil {
		return "", errors.Wrapf(err, "failed to update existing definition in kubernetes")
	}
	return fmt.Sprintf("%s %s in namespace %s updated.\n", oldDef.GetKind(), oldDef.GetName(), oldDef.GetNamespace()), nil
}

// NewDefinitionDelCommand create the `vela def del` command to help user delete existing definitions conveniently
func NewDefinitionDelCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "del DEFINITION_NAME",
		Short: "Delete X-Definition.",
		Long:  "Delete X-Definition in kubernetes cluster.",
		Example: "# Command below will delete TraitDefinition of annotations in default namespace\n" +
			"> vela def del annotations -t trait -n default",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			definitionType, err := cmd.Flags().GetString(FlagType)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", FlagType)
			}
			namespace, err := cmd.Flags().GetString(FlagNamespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get `%s`", Namespace)
			}
			k8sClient, err := c.GetClient()
			if err != nil {
				return errors.Wrapf(err, "failed to get k8s client")
			}
			def, err := getSingleDefinition(cmd, args[0], k8sClient, definitionType, namespace)
			if err != nil {
				return err
			}
			desc := def.GetAnnotations()[pkgdef.DescriptionKey]
			toDelete := false
			_, err = getPrompt(cmd, bufio.NewReader(cmd.InOrStdin()),
				fmt.Sprintf("Are you sure to delete the following definition in namespace %s?\n", def.GetNamespace())+
					fmt.Sprintf("%s %s: %s\n", def.GetKind(), def.GetName(), desc),
				"[yes|no] > ",
				func(resp string) error {
					switch strings.ToLower(resp) {
					case "yes":
						toDelete = true
					case "y":
						toDelete = true
					case "no":
						toDelete = false
					case "n":
						toDelete = false
					default:
						return errors.New("invalid input")
					}
					return nil
				})
			if err != nil {
				return err
			}
			if !toDelete {
				return nil
			}
			if err := k8sClient.Delete(context.Background(), def); err != nil {
				return errors.Wrapf(err, "failed to delete %s %s in namespace %s", def.GetKind(), def.GetName(), def.GetNamespace())
			}
			cmd.Printf("%s %s in namespace %s deleted.\n", def.GetKind(), def.GetName(), def.GetNamespace())
			return nil
		},
	}
	cmd.Flags().StringP(FlagType, "t", "", "Specify the definition type of target. Valid types: "+strings.Join(pkgdef.ValidDefinitionTypes(), ", "))
	cmd.Flags().StringP(Namespace, "n", types.DefaultKubeVelaNS, "Specify which namespace the definition locates.")
	return cmd
}

// NewDefinitionValidateCommand create the `vela def vet` command to help user validate the definition
func NewDefinitionValidateCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vet DEFINITION.cue",
		Short: "Validate X-Definition.",
		Long: "Validate definition file by checking whether it has the valid cue format with fields set correctly\n" +
			"* Currently, this command only checks the cue format. This function is still working in progress and we will support more functional validation mechanism in the future.",
		Example: "# Command below will validate the my-def.cue file.\n" +
			"> vela def vet my-def.cue\n" +
			"# Validate every cue file provided\n" +
			"> vela def vet my-def1.cue my-def2.cue my-def3.cue\n" +
			"# Validate every cue file in the specified directories" +
			"> vela def vet ./test1/ ./test2/",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				files, err := utils.LoadDataFromPath(cmd.Context(), arg, utils.IsCUEFile)
				if err != nil {
					return errors.Wrapf(err, "failed to get file from %s", arg)
				}
				for _, file := range files {
					validateRes, err := validateSingleCueFile(file.Path, file.Data, c)
					if err != nil {
						return err
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s", validateRes)
				}
			}
			return nil
		},
	}
	return cmd
}

func validateSingleCueFile(fileName string, fileData []byte, c common.Args) (string, error) {
	def := pkgdef.Definition{Unstructured: unstructured.Unstructured{}}
	config, err := c.GetConfig()
	if err != nil {
		return "", err
	}
	if err := def.FromCUEString(string(fileData), config); err != nil {
		return "", errors.Wrapf(err, "failed to parse CUE: %s", fileName)
	}
	return fmt.Sprintf("Validation %s succeed.\n", fileName), nil
}

// NewDefinitionGenAPICommand create the `vela def gen-api` command to help user generate Go code from the definition
func NewDefinitionGenAPICommand(c common.Args) *cobra.Command {
	meta := gen_sdk.GenMeta{}
	var languageArgs []string

	cmd := &cobra.Command{
		Use:   "gen-api DEFINITION.cue",
		Short: "Generate SDK from X-Definition.",
		Long: "Generate SDK from X-definition file.\n" +
			"* This command leverage openapi-generator project. Therefore demands \"docker\" exist in PATH\n" +
			"* Currently, this function is still working in progress and not all formats of parameter in X-definition are supported yet.",
		Example: "# Generate SDK for golang with scaffold initialized\n" +
			"> vela def gen-api --init --language go -f /path/to/def -o /path/to/sdk\n" +
			"# Generate incremental definition files to existing sdk directory\n" +
			"> vela def gen-api --language go -f /path/to/def -o /path/to/sdk\n" +
			"# Generate definitions to a sub-module\n" +
			"> vela def gen-api --language go -f /path/to/def -o /path/to/sdk --submodule --api-dir path/relative/to/output --language-args arg1=val1,arg2=val2\n",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := meta.Init(c, languageArgs)
			if err != nil {
				return err
			}
			err = meta.CreateScaffold()
			if err != nil {
				return err
			}
			err = meta.PrepareGeneratorAndTemplate()
			if err != nil {
				return err
			}
			err = meta.Run()
			if err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&meta.Output, "output", "o", "./apis", "Output directory path")
	cmd.Flags().StringVar(&meta.APIDirectory, "api-dir", "", "API directory path to put definition API files, relative to output directory. Default value: go: pkg/apis")
	cmd.Flags().BoolVar(&meta.IsSubModule, "submodule", false, "Whether the generated code is a submodule of the project. If set, the directory specified by `api-dir` will be treated as a submodule of the project")
	cmd.Flags().StringVarP(&meta.Package, "package", "p", gen_sdk.PackagePlaceHolder, "Package name of generated code")
	cmd.Flags().StringVarP(&meta.Lang, "language", "g", "go", "Language to generate code. Valid languages: go")
	cmd.Flags().StringVarP(&meta.Template, "template", "t", "", "Template file path, if not specified, the default template will be used")
	cmd.Flags().StringSliceVarP(&meta.File, "file", "f", nil, "File name of definitions, can be specified multiple times, or use comma to separate multiple files. If directory specified, all files found recursively in the directory will be used")
	cmd.Flags().BoolVar(&meta.InitSDK, "init", false, "Init the whole SDK project, if not set, only the API file will be generated")
	cmd.Flags().BoolVarP(&meta.Verbose, "verbose", "v", false, "Print verbose logs")
	var langArgsDescStr string
	for lang, args := range gen_sdk.LangArgsRegistry {
		langArgsDescStr += lang + ": \n"
		for key, arg := range args {
			langArgsDescStr += fmt.Sprintf("\t%s: %s(default: %s)\n", key, arg.Name, arg.Default)
		}
	}
	cmd.Flags().StringSliceVar(&languageArgs, "language-args", []string{},
		fmt.Sprintf("language-specific arguments to pass to the go generator, available options: \n"+langArgsDescStr),
	)

	return cmd
}
