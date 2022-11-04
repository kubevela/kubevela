/*
Copyright 2022 The KubeVela Authors.

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
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/yaml"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/config"
	pkgUtils "github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/docgen"
)

// ConfigCommandGroup commands for the config
func ConfigCommandGroup(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: i18n.T("Manage the configs."),
		Long:  i18n.T("Manage the configs, such as the terraform provider, image registry, helm repository, etc."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
	}
	cmd.AddCommand(NewListConfigCommand(f, streams))
	cmd.AddCommand(NewCreateConfigCommand(f, streams))
	cmd.AddCommand(NewDistributeConfigCommand(f, streams))
	cmd.AddCommand(NewDeleteConfigCommand(f, streams))
	return cmd
}

// TemplateCommandGroup commands for the template of the config
func TemplateCommandGroup(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config-template",
		Aliases: []string{"ct"},
		Short:   i18n.T("Manage the template of config."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeExtension,
		},
	}
	cmd.AddCommand(NewTemplateApplyCommand(f, streams))
	cmd.AddCommand(NewTemplateListCommand(f, streams))
	cmd.AddCommand(NewTemplateDeleteCommand(f, streams))
	cmd.AddCommand(NewTemplateShowCommand(f, streams))
	return cmd
}

// TemplateApplyCommandOptions the options of the command that apply the config template.
type TemplateApplyCommandOptions struct {
	File      string
	Namespace string
	Name      string
}

// TemplateCommandOptions the options of the command that delete or show the config template.
type TemplateCommandOptions struct {
	Namespace string
	Name      string
}

// TemplateListCommandOptions the options of the command that list the config templates.
type TemplateListCommandOptions struct {
	Namespace    string
	AllNamespace bool
}

// NewTemplateApplyCommand command for creating and updating the config template
func NewTemplateApplyCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options TemplateApplyCommandOptions
	cmd := &cobra.Command{
		Use:   "apply",
		Short: i18n.T("Apply a config template."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeExtension,
		},
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := pkgUtils.ReadRemoteOrLocalPath(options.File, false)
			if err != nil {
				return err
			}
			inf := config.NewConfigFactory(f.Client())
			template, err := inf.ParseTemplate(options.Name, body)
			if err != nil {
				return err
			}
			if err := inf.CreateOrUpdateConfigTemplate(context.Background(), options.Namespace, template); err != nil {
				return err
			}
			streams.Infof("the config template %s applied successfully\n", template.Name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.File, "file", "f", "", "specify the template file name")
	cmd.Flags().StringVarP(&options.Name, "name", "", "", "specify the config template name")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the template")
	return cmd
}

// NewTemplateListCommand command for listing the config templates
func NewTemplateListCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options TemplateListCommandOptions
	cmd := &cobra.Command{
		Use:     "list",
		Short:   i18n.T("List the config templates."),
		Example: "vela config template list [-A]",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeExtension,
		},
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			inf := config.NewConfigFactory(f.Client())
			if options.AllNamespace {
				options.Namespace = ""
			}
			templates, err := inf.ListTemplates(context.Background(), options.Namespace, "")
			if err != nil {
				return err
			}
			table := newUITable()
			header := []interface{}{"NAME", "ALIAS", "SCOPE", "SENSITIVE", "CREATED-TIME"}
			if options.AllNamespace {
				header = append([]interface{}{"NAMESPACE"}, header...)
			}
			table.AddRow(header...)
			for _, t := range templates {
				row := []interface{}{t.Name, t.Alias, t.Scope, t.Sensitive, t.CreateTime}
				if options.AllNamespace {
					row = append([]interface{}{t.Namespace}, row...)
				}
				table.AddRow(row...)
			}
			if _, err := streams.Out.Write(table.Bytes()); err != nil {
				return err
			}
			if _, err := streams.Out.Write([]byte("\n")); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the template")
	cmd.Flags().BoolVarP(&options.AllNamespace, "all-namespaces", "A", false, "If true, check the specified action in all namespaces.")
	return cmd
}

// NewTemplateShowCommand command for show the properties document
func NewTemplateShowCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options TemplateCommandOptions
	cmd := &cobra.Command{
		Use:     "show",
		Short:   i18n.T("Show the documents of the template properties"),
		Example: "vela config-template show <name> [-n]",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeExtension,
		},
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Name = args[0]
			if options.Name == "" {
				return fmt.Errorf("can not show the properties without the template")
			}
			inf := config.NewConfigFactory(f.Client())
			template, err := inf.LoadTemplate(context.Background(), options.Name, options.Namespace)
			if err != nil {
				return err
			}
			doc, err := docgen.GenerateConsoleDocument(template.Schema.Title, template.Schema)
			if err != nil {
				return err
			}
			if _, err := streams.Out.Write([]byte(doc)); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the template")
	return cmd
}

// NewTemplateDeleteCommand command for deleting the config template
func NewTemplateDeleteCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options TemplateCommandOptions
	cmd := &cobra.Command{
		Use:     "delete",
		Short:   i18n.T("Delete a config template."),
		Example: fmt.Sprintf("vela config template delete <name> [-n %s]", types.DefaultKubeVelaNS),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("please must provides the template name")
			}
			options.Name = args[0]
			userInput := &UserInput{
				Writer: streams.Out,
				Reader: bufio.NewReader(streams.In),
			}
			if !assumeYes {
				userConfirmation := userInput.AskBool("Do you want to delete this template", &UserInputOptions{assumeYes})
				if !userConfirmation {
					return fmt.Errorf("stopping deleting")
				}
			}
			inf := config.NewConfigFactory(f.Client())
			if err := inf.DeleteTemplate(context.Background(), options.Namespace, options.Name); err != nil {
				return err
			}
			streams.Infof("the config template %s deleted successfully\n", options.Name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the template")
	return cmd
}

// DistributeConfigCommandOptions the options of the command that distribute the config.
type DistributeConfigCommandOptions struct {
	Targets   []string
	Config    string
	Namespace string
	Recalled  bool
}

// CreateConfigCommandOptions the options of the command that create the config.
type CreateConfigCommandOptions struct {
	Template    string
	Namespace   string
	Name        string
	File        string
	Properties  map[string]interface{}
	DryRun      bool
	Targets     []string
	Description string
	Alias       string
}

// Validate validate the options
func (i CreateConfigCommandOptions) Validate() error {
	if i.Name == "" {
		return fmt.Errorf("the config name must be specified")
	}
	if len(i.Targets) > 0 && i.DryRun {
		return fmt.Errorf("can not set the distribution in dry-run mode")
	}
	return nil
}

func (i *CreateConfigCommandOptions) parseProperties(args []string) error {
	if i.File != "" {
		body, err := pkgUtils.ReadRemoteOrLocalPath(i.File, false)
		if err != nil {
			return err
		}
		var properties = map[string]interface{}{}
		if err := yaml.Unmarshal(body, &properties); err != nil {
			return err
		}
		i.Properties = properties
		return nil
	}
	res := map[string]interface{}{}
	for _, arg := range args {
		if err := strvals.ParseInto(arg, res); err != nil {
			return err
		}
	}
	i.Properties = res
	return nil
}

// ConfigListCommandOptions the options of the command that list configs.
type ConfigListCommandOptions struct {
	Namespace    string
	Template     string
	AllNamespace bool
}

// NewListConfigCommand command for listing the config secrets
func NewListConfigCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options ConfigListCommandOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: i18n.T("List the configs."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := options.Template
			if strings.Contains(options.Template, "/") {
				namespacedName := strings.SplitN(options.Template, "/", 2)
				name = namespacedName[1]
			}
			if options.AllNamespace {
				options.Namespace = ""
			}
			inf := config.NewConfigFactory(f.Client())
			configs, err := inf.ListConfigs(context.Background(), options.Namespace, name, "", true)
			if err != nil {
				return err
			}
			table := newUITable()
			header := []interface{}{"NAME", "ALIAS", "DISTRIBUTION", "TEMPLATE", "CREATED-TIME", "DESCRIPTION"}
			if options.AllNamespace {
				header = append([]interface{}{"NAMESPACE"}, header...)
			}
			table.AddRow(header...)
			for _, t := range configs {
				var targetShow = ""
				for _, target := range t.Targets {
					if targetShow != "" {
						targetShow += " "
					}
					switch target.Status {
					case string(workflowv1alpha1.WorkflowStepPhaseSucceeded):
						targetShow += green.Sprintf("%s/%s", target.ClusterName, target.Namespace)
					case string(workflowv1alpha1.WorkflowStepPhaseFailed):
						targetShow += red.Sprintf("%s/%s", target.ClusterName, target.Namespace)
					default:
						targetShow += yellow.Sprintf("%s/%s", target.ClusterName, target.Namespace)
					}
				}
				row := []interface{}{t.Name, t.Alias, targetShow, fmt.Sprintf("%s/%s", t.Template.Namespace, t.Template.Name), t.CreateTime, t.Description}
				if options.AllNamespace {
					row = append([]interface{}{t.Namespace}, row...)
				}
				table.AddRow(row...)
			}
			if _, err := streams.Out.Write(table.Bytes()); err != nil {
				return err
			}
			if _, err := streams.Out.Write([]byte("\n")); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the config")
	cmd.Flags().StringVarP(&options.Template, "template", "t", "", "specify the template of the config")
	cmd.Flags().BoolVarP(&options.AllNamespace, "all-namespaces", "A", false, "If true, check the specified action in all namespaces.")
	return cmd
}

// NewCreateConfigCommand command for creating the config
func NewCreateConfigCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options CreateConfigCommandOptions
	createConfigExample := templates.Examples(i18n.T(`
		# Generate a config with the args
		vela config create test-registry --template=image-registry registry=index.docker.io auth.username=test auth.password=test
		
		# Generate a config with the file
		vela config create test-config --template=image-registry  -f config.yaml

		# Generate a config without the template
		vela config create test-vela -f config.yaml
		`))

	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"c"},
		Short:   i18n.T("Create a config."),
		Example: createConfigExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			inf := config.NewConfigFactory(f.Client())
			options.Name = args[0]
			if err := options.Validate(); err != nil {
				return err
			}
			name := options.Template
			namespace := types.DefaultKubeVelaNS
			if strings.Contains(options.Template, "/") {
				namespacedName := strings.SplitN(options.Template, "/", 2)
				namespace = namespacedName[0]
				name = namespacedName[1]
			}
			if err := options.parseProperties(args[1:]); err != nil {
				return err
			}
			configItem, err := inf.ParseConfig(context.Background(), config.NamespacedName{
				Name:      name,
				Namespace: namespace,
			}, config.Metadata{
				NamespacedName: config.NamespacedName{
					Name:      options.Name,
					Namespace: options.Namespace,
				},
				Properties:  options.Properties,
				Alias:       options.Alias,
				Description: options.Description,
			})
			if err != nil {
				return err
			}
			if options.DryRun {
				var outBuilder = bytes.NewBuffer(nil)
				out, err := yaml.Marshal(configItem.Secret)
				if err != nil {
					return err
				}
				_, err = outBuilder.Write(out)
				if err != nil {
					return err
				}
				if configItem.OutputObjects != nil {
					for k, object := range configItem.OutputObjects {
						_, err = outBuilder.WriteString("# Object: \n ---" + k)
						if err != nil {
							return err
						}
						out, err := yaml.Marshal(object)
						if err != nil {
							return err
						}
						if _, err := outBuilder.Write(out); err != nil {
							return err
						}
					}
				}
				_, err = streams.Out.Write(outBuilder.Bytes())
				return err
			}
			if err := inf.CreateOrUpdateConfig(context.Background(), configItem, options.Namespace); err != nil {
				return err
			}
			if len(options.Targets) > 0 {
				ads := &config.CreateDistributionSpec{
					Targets: []*config.ClusterTarget{},
					Configs: []*config.NamespacedName{
						&configItem.NamespacedName,
					},
				}
				for _, t := range options.Targets {
					ti := strings.Split(t, "/")
					if len(ti) == 2 {
						ads.Targets = append(ads.Targets, &config.ClusterTarget{
							ClusterName: ti[0],
							Namespace:   ti[1],
						})
					} else {
						ads.Targets = append(ads.Targets, &config.ClusterTarget{
							ClusterName: types.ClusterLocalName,
							Namespace:   ti[0],
						})
					}
				}
				name := config.DefaultDistributionName(options.Name)
				if err := inf.CreateOrUpdateDistribution(context.Background(), options.Namespace, name, ads); err != nil {
					return err
				}
			}
			streams.Infof("the config %s applied successfully\n", options.Name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.Template, "template", "t", "", "specify the config template name and namespace")
	cmd.Flags().StringVarP(&options.File, "file", "f", "", "specify the file name of the config properties")
	cmd.Flags().StringArrayVarP(&options.Targets, "target", "", []string{}, "this config will be distributed if this flag is set")
	cmd.Flags().BoolVarP(&options.DryRun, "dry-run", "", false, "Dry run to apply the config")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the config")
	cmd.Flags().StringVarP(&options.Description, "description", "", "", "specify the description of the config")
	cmd.Flags().StringVarP(&options.Alias, "alias", "", "", "specify the alias of the config")
	return cmd
}

// NewDistributeConfigCommand command for distributing the config
func NewDistributeConfigCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options DistributeConfigCommandOptions

	distributionExample := templates.Examples(i18n.T(`
		# distribute the config(test-registry) from the vela-system namespace to the default namespace in the local cluster.
		vela config d test-registry -t default

		# distribute the config(test-registry) from the vela-system namespace to the other clusters.
		vela config d test-registry -t cluster1/default -t cluster2/default

		# recall the config
		vela config d test-registry --recall
		`))

	cmd := &cobra.Command{
		Use:     "distribute",
		Aliases: []string{"d"},
		Short:   i18n.T("Distribute a config"),
		Example: distributionExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inf := config.NewConfigFactory(f.Client())
			options.Config = args[0]
			name := config.DefaultDistributionName(options.Config)
			if options.Recalled {
				userInput := &UserInput{
					Writer: streams.Out,
					Reader: bufio.NewReader(streams.In),
				}
				if !assumeYes {
					userConfirmation := userInput.AskBool("Do you want to recall this config", &UserInputOptions{assumeYes})
					if !userConfirmation {
						return fmt.Errorf("recalling stopped")
					}
				}
				if err := inf.DeleteDistribution(context.Background(), options.Namespace, name); err != nil {
					return err
				}
				streams.Infof("the distribution %s deleted successfully\n", name)
				return nil
			}

			ads := &config.CreateDistributionSpec{
				Targets: []*config.ClusterTarget{},
				Configs: []*config.NamespacedName{
					{
						Name:      options.Config,
						Namespace: options.Namespace,
					},
				},
			}
			for _, t := range options.Targets {
				ti := strings.Split(t, "/")
				if len(ti) == 2 {
					ads.Targets = append(ads.Targets, &config.ClusterTarget{
						ClusterName: ti[0],
						Namespace:   ti[1],
					})
				} else {
					ads.Targets = append(ads.Targets, &config.ClusterTarget{
						ClusterName: types.ClusterLocalName,
						Namespace:   ti[0],
					})
				}
			}
			if err := inf.CreateOrUpdateDistribution(context.Background(), options.Namespace, name, ads); err != nil {
				return err
			}
			streams.Infof("the distribution %s applied successfully\n", name)
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&options.Targets, "target", "t", []string{}, "specify the targets that want to distribute,the format is: <clusterName>/<namespace>")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the distribution")
	cmd.Flags().BoolVarP(&options.Recalled, "recall", "r", false, "this field means recalling the configs from all targets.")
	return cmd
}

// ConfigDeleteCommandOptions the options of the command that delete the config.
type ConfigDeleteCommandOptions struct {
	Namespace string
	Name      string
	NotRecall bool
}

// NewDeleteConfigCommand command for deleting the config secret
func NewDeleteConfigCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options ConfigDeleteCommandOptions
	cmd := &cobra.Command{
		Use:   "delete",
		Short: i18n.T("Delete a config."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Name = args[0]
			inf := config.NewConfigFactory(f.Client())
			userInput := &UserInput{
				Writer: streams.Out,
				Reader: bufio.NewReader(streams.In),
			}
			if !assumeYes {
				userConfirmation := userInput.AskBool("Do you want to delete this config", &UserInputOptions{assumeYes})
				if !userConfirmation {
					return fmt.Errorf("deleting stopped")
				}
			}

			if !options.NotRecall {
				if err := inf.DeleteDistribution(context.Background(), options.Namespace, config.DefaultDistributionName(options.Name)); err != nil && !errors.Is(err, config.ErrNotFoundDistribution) {
					return err
				}
			}

			if err := inf.DeleteConfig(context.Background(), options.Namespace, options.Name); err != nil {
				return err
			}

			streams.Infof("the config %s deleted successfully\n", options.Name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the config")
	cmd.Flags().BoolVarP(&options.NotRecall, "not-recall", "", false, "means only deleting the config from the local and do not recall from targets.")
	return cmd
}
