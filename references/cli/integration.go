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
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/integration"
	pkgUtils "github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/docgen"
)

// IntegrationCommandGroup commands for the integration
func IntegrationCommandGroup(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "integration",
		Short: i18n.T("Manage the integration secret."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
	}
	cmd.AddCommand(NewTemplateCommandGroup(f, streams))
	cmd.AddCommand(NewDistributionCommandGroup(f, streams))
	cmd.AddCommand(NewListIntegrationCommand(f, streams))
	cmd.AddCommand(NewApplyIntegrationCommand(f, streams))
	cmd.AddCommand(NewDeleteIntegrationCommand(f, streams))
	return cmd
}

// NewTemplateCommandGroup commands for the template of the integration
func NewTemplateCommandGroup(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "template",
		Aliases: []string{"t"},
		Short:   i18n.T("Manage the template of integration."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
	}
	cmd.AddCommand(NewTemplateApplyCommand(f, streams))
	cmd.AddCommand(NewTemplateListCommand(f, streams))
	cmd.AddCommand(NewTemplateDeleteCommand(f, streams))
	return cmd
}

// NewDistributionCommandGroup commands for the distribution of the integration
func NewDistributionCommandGroup(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "distribution",
		Aliases: []string{"d"},
		Short:   i18n.T("Manage the distribution of integration."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
	}
	cmd.AddCommand(NewDistributionApplyCommand(f, streams))
	cmd.AddCommand(NewDistributionListCommand(f, streams))
	cmd.AddCommand(NewDistributionDeleteCommand(f, streams))
	return cmd
}

// NewDistributionApplyCommand command for creating and updating the distribution
func NewDistributionApplyCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options DistributionApplyCommandOptions

	applyDistributionExample := templates.Examples(i18n.T(`
		# distribute the integration(test-registry) from the vela-system namespace to the default namespace in the local cluster.
		vela integration d apply --name=test -i=test-registry -t default

		# distribute the integration(test-registry) from the vela-system namespace to the other clusters.
		vela integration d apply --name=test -i=test-registry -t cluster1/default -t cluster2/default
		`))

	cmd := &cobra.Command{
		Use:     "apply",
		Short:   i18n.T("Apply a distribution."),
		Example: applyDistributionExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {

			inf := integration.NewIntegrationFactory(f.Client())
			ads := &integration.ApplyDistributionSpec{
				Targets:      []*integration.ClusterTarget{},
				Integrations: []*integration.NamespacedName{},
			}
			for _, t := range options.Targets {
				ti := strings.Split(t, "/")
				if len(ti) == 2 {
					ads.Targets = append(ads.Targets, &integration.ClusterTarget{
						ClusterName: ti[0],
						Namespace:   ti[1],
					})
				} else {
					ads.Targets = append(ads.Targets, &integration.ClusterTarget{
						ClusterName: types.ClusterLocalName,
						Namespace:   ti[0],
					})
				}
			}
			for _, t := range options.Integrations {
				ti := strings.Split(t, "/")
				if len(ti) == 2 {
					ads.Integrations = append(ads.Integrations, &integration.NamespacedName{
						Namespace: ti[0],
						Name:      ti[1],
					})
				} else {
					ads.Integrations = append(ads.Integrations, &integration.NamespacedName{
						Namespace: types.DefaultKubeVelaNS,
						Name:      ti[0],
					})
				}
			}
			if err := inf.ApplyDistribution(context.Background(), options.Namespace, options.Name, ads); err != nil {
				return err
			}
			streams.Infof("the distribution %s applied successfully\n", options.Name)
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&options.Integrations, "integration", "i", []string{}, "specify the integrations that want to distribute,the format is: <namespace>/<name>")
	cmd.Flags().StringArrayVarP(&options.Targets, "target", "t", []string{}, "specify the targets that want to distribute,the format is: <clusterName>/<namespace>")
	cmd.Flags().StringVarP(&options.Name, "name", "", "", "specify the name of the distribution")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the distribution")
	return cmd
}

// NewDistributionListCommand command for listing the distributions
func NewDistributionListCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options TemplateListCommandOptions
	cmd := &cobra.Command{
		Use:     "list",
		Short:   i18n.T("List the distributions."),
		Example: "vela integration distribution list [-A]",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			inf := integration.NewIntegrationFactory(f.Client())
			if options.AllNamespace {
				options.Namespace = ""
			}
			distributions, err := inf.ListDistributions(context.Background(), options.Namespace)
			if err != nil {
				return err
			}
			table := newUITable()
			header := []interface{}{"NAME", "INTEGRATIONS", "TARGETS", "STATUS", "CREATED-TIME"}
			if options.AllNamespace {
				header = append([]interface{}{"NAMESPACE"}, header...)
			}
			table.AddRow(header...)
			for _, t := range distributions {
				integrationShow := ""
				for _, integration := range t.Integrations {
					integrationShow += fmt.Sprintf("%s/%s,", integration.Namespace, integration.Name)
				}
				targetShow := ""
				for _, t := range t.Targets {
					targetShow += fmt.Sprintf("%s/%s,", t.ClusterName, t.Namespace)
				}
				status := t.Status.Phase
				if status == common.ApplicationRunning {
					status = "Completed"
				}
				row := []interface{}{
					t.Name,
					strings.TrimSuffix(integrationShow, ","),
					strings.TrimSuffix(targetShow, ","),
					status,
					t.CreatedTime,
				}
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

// NewDistributionDeleteCommand command for deleting the distribution
func NewDistributionDeleteCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options TemplateDeleteCommandOptions
	cmd := &cobra.Command{
		Use:     "delete",
		Short:   i18n.T("Delete a distribution."),
		Example: fmt.Sprintf("vela integration distribution delete <name> [-n %s]", types.DefaultKubeVelaNS),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("please must provides the distribution name")
			}
			options.Name = args[0]
			inf := integration.NewIntegrationFactory(f.Client())
			if err := inf.DeleteDistribution(context.Background(), options.Namespace, options.Name); err != nil {
				return err
			}
			streams.Infof("the distribution %s deleted successfully\n", options.Name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the template")
	return cmd
}

// TemplateApplyCommandOptions the options of the command that apply the integration template.
type TemplateApplyCommandOptions struct {
	File      string
	Namespace string
	Name      string
}

// TemplateDeleteCommandOptions the options of the command that delete the integration template.
type TemplateDeleteCommandOptions struct {
	Namespace string
	Name      string
}

// TemplateListCommandOptions the options of the command that list the integration templates.
type TemplateListCommandOptions struct {
	Namespace    string
	AllNamespace bool
}

// NewTemplateApplyCommand command for creating and updating the integration template
func NewTemplateApplyCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options TemplateApplyCommandOptions
	cmd := &cobra.Command{
		Use:   "apply",
		Short: i18n.T("Apply a integration template."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := pkgUtils.ReadRemoteOrLocalPath(options.File, false)
			if err != nil {
				return err
			}
			inf := integration.NewIntegrationFactory(f.Client())
			template, err := inf.ParseTemplate(options.Name, body)
			if err != nil {
				return err
			}
			if err := inf.ApplyTemplate(context.Background(), options.Namespace, template); err != nil {
				return err
			}
			streams.Infof("the integration template %s applied successfully\n", template.Name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.File, "file", "f", "", "specify the template file name")
	cmd.Flags().StringVarP(&options.Name, "name", "", "", "specify the integration template name")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the template")
	return cmd
}

// NewTemplateListCommand command for listing the integration templates
func NewTemplateListCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options TemplateListCommandOptions
	cmd := &cobra.Command{
		Use:     "list",
		Short:   i18n.T("List the integration templates."),
		Example: "vela integration template list [-A]",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			inf := integration.NewIntegrationFactory(f.Client())
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

// NewTemplateDeleteCommand command for deleting the integration template
func NewTemplateDeleteCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options TemplateDeleteCommandOptions
	cmd := &cobra.Command{
		Use:     "delete",
		Short:   i18n.T("Delete a integration template."),
		Example: fmt.Sprintf("vela integration template delete <name> [-n %s]", types.DefaultKubeVelaNS),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("please must provides the template name")
			}
			options.Name = args[0]
			inf := integration.NewIntegrationFactory(f.Client())
			if err := inf.DeleteTemplate(context.Background(), options.Namespace, options.Name); err != nil {
				return err
			}
			streams.Infof("the integration template %s deleted successfully\n", options.Name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the template")
	return cmd
}

// DistributionApplyCommandOptions the options of the command that apply the distribution.
type DistributionApplyCommandOptions struct {
	Targets      []string
	Integrations []string
	Name         string
	Namespace    string
}

// IntegrationApplyCommandOptions the options of the command that apply the integration.
type IntegrationApplyCommandOptions struct {
	Template       string
	Namespace      string
	Name           string
	File           string
	Properties     map[string]interface{}
	ShowProperties bool
	DryRun         bool
}

// Validate validate the options
func (i IntegrationApplyCommandOptions) Validate() error {
	if i.Name == "" && !i.ShowProperties {
		return fmt.Errorf("the integration name must be specified")
	}
	return nil
}

func (i *IntegrationApplyCommandOptions) parseProperties(args []string) error {
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

// IntegrationListCommandOptions the options of the command that list integrations.
type IntegrationListCommandOptions struct {
	Namespace    string
	Template     string
	AllNamespace bool
}

// NewListIntegrationCommand command for listing the integration secrets
func NewListIntegrationCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options IntegrationListCommandOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: i18n.T("List the integrations."),
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
			inf := integration.NewIntegrationFactory(f.Client())
			integrations, err := inf.ListIntegrations(context.Background(), options.Namespace, name, "")
			if err != nil {
				return err
			}
			table := newUITable()
			header := []interface{}{"NAME", "ALIAS", "SECRET", "TEMPLATE", "CREATED-TIME", "DESCRIPTION"}
			if options.AllNamespace {
				header = append([]interface{}{"NAMESPACE"}, header...)
			}
			table.AddRow(header...)
			for _, t := range integrations {
				row := []interface{}{t.Name, t.Alias, t.Secret.Name, fmt.Sprintf("%s/%s", t.Template.Namespace, t.Template.Name), t.CreateTime, t.Description}
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
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the integration")
	cmd.Flags().StringVarP(&options.Template, "template", "t", "", "specify the template of the integration")
	cmd.Flags().BoolVarP(&options.AllNamespace, "all-namespaces", "A", false, "If true, check the specified action in all namespaces.")
	return cmd
}

// NewApplyIntegrationCommand command for creating or patching the integration secret
func NewApplyIntegrationCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options IntegrationApplyCommandOptions
	applyIntegrationExample := templates.Examples(i18n.T(`
		# Generate a integration secret with the args
		vela integration apply --template=image-registry --name test-registry registry=index.docker.io auth.username=test auth.password=test
		
		# View the integration property options

		vela integration apply --template=image-registry --show-properties
		
		# Generate a integration secret with the file
		vela integration apply --template=image-registry --name test-vela -f config.yaml

		# Generate a integration secret without the template
		vela integration apply --name test-vela -f config.yaml
		
		`))

	cmd := &cobra.Command{
		Use:     "apply",
		Short:   i18n.T("Create or patch a integration secret."),
		Example: applyIntegrationExample,

		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			inf := integration.NewIntegrationFactory(f.Client())
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
			if options.ShowProperties {
				if name == "" {
					return fmt.Errorf("can not show the properties without the template")
				}
				template, err := inf.LoadTemplate(context.Background(), name, namespace)
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
			}
			if err := options.parseProperties(args); err != nil {
				return err
			}
			integration, err := inf.ParseIntegration(context.Background(), integration.NamespacedName{
				Name:      name,
				Namespace: namespace,
			}, integration.Metadata{
				NamespacedName: integration.NamespacedName{
					Name:      options.Name,
					Namespace: options.Namespace,
				},
				Properties: options.Properties,
			})
			if err != nil {
				return err
			}
			if options.DryRun {
				var outBuilder = bytes.NewBuffer(nil)
				out, err := yaml.Marshal(integration.Secret)
				if err != nil {
					return err
				}
				_, err = outBuilder.Write(out)
				if err != nil {
					return err
				}
				if integration.OutputObjects != nil {
					for k, object := range integration.OutputObjects {
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
			if err := inf.ApplyIntegration(context.Background(), integration, options.Namespace); err != nil {
				return err
			}
			streams.Infof("the integration %s applied successfully\n", options.Name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.Template, "template", "t", "", "specify the integration template name and namespace")
	cmd.Flags().StringVarP(&options.Name, "name", "", "", "specify the integration name")
	cmd.Flags().StringVarP(&options.File, "file", "f", "", "specify the integration properties file name")
	cmd.Flags().BoolVarP(&options.ShowProperties, "show-properties", "", false, "show the properties documents")
	cmd.Flags().BoolVarP(&options.DryRun, "dry-run", "", false, "Dry run to apply the integration")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the integration")
	return cmd
}

// IntegrationDeleteCommandOptions the options of the command that delete the integration.
type IntegrationDeleteCommandOptions struct {
	Namespace string
	Name      string
}

// NewDeleteIntegrationCommand command for deleting the integration secret
func NewDeleteIntegrationCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	var options IntegrationDeleteCommandOptions
	cmd := &cobra.Command{
		Use:   "delete",
		Short: i18n.T("Delete a integration secret."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("please must provides the integration name")
			}
			options.Name = args[0]
			inf := integration.NewIntegrationFactory(f.Client())
			if err := inf.DeleteIntegration(context.Background(), options.Namespace, options.Name); err != nil {
				return err
			}
			streams.Infof("the integration %s deleted successfully\n", options.Name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", types.DefaultKubeVelaNS, "specify the namespace of the integration")
	return cmd
}
