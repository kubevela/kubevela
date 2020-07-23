package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloud-native-application/rudrx/api/v1alpha2"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TemplateLabel is the Annotation refer to template
const TemplateLabel = "rudrx.oam.dev/template"

type commandOptions struct {
	Namespace string
	Template  v1alpha2.Template
	Component corev1alpha2.Component
	AppConfig corev1alpha2.ApplicationConfiguration
	Client    client.Client
	cmdutil.IOStreams
}

// NewCommandOptions bind command options
func NewCommandOptions(ioStreams cmdutil.IOStreams) *commandOptions {
	return &commandOptions{IOStreams: ioStreams}
}

// NewBindCommand return bind command
func NewBindCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) *cobra.Command {
	cmd := newBindCommand()
	cmd.SetArgs(args)
	cmd.SetOut(ioStreams.Out)
	cmd.DisableFlagParsing = true

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runSubBindCommand(cmd, f, c, ioStreams, args)
	}
	return cmd
}

func runSubBindCommand(parentCmd *cobra.Command, f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) error {
	ctx := context.Background()
	o := NewCommandOptions(ioStreams)
	o.Client = c

	// init fake command and pass args to fake command
	// flags and subcommand append to fake comand and parent command
	// run fake command only, show tips in parent command only
	fakeCommand := newBindCommand()
	fakeCommand.SilenceUsage = true
	fakeCommand.SilenceErrors = true
	fakeCommand.DisableAutoGenTag = true
	fakeCommand.DisableFlagsInUseLine = true
	fakeCommand.DisableSuggestions = true
	fakeCommand.SetOut(o.Out)
	if len(args) > 0 {
		fakeCommand.SetArgs(args)
	} else {
		fakeCommand.SetArgs([]string{})
	}

	var traitDefinitions corev1alpha2.TraitDefinitionList
	err := c.List(ctx, &traitDefinitions)
	if err != nil {
		return fmt.Errorf("Listing trait definitions hit an issue: %v", err)
	}

	for _, template := range traitDefinitions.Items {
		templateName := template.Annotations[TemplateLabel]
		// skip tarit that without template
		if templateName == "" {
			continue
		}

		var traitTemplate v1alpha2.Template
		err := c.Get(ctx, client.ObjectKey{Namespace: "default", Name: templateName}, &traitTemplate)
		if err != nil {
			return fmt.Errorf("Listing trait template hit an issue: %v", err)
		}

		for _, p := range traitTemplate.Spec.Parameters {
			if p.Type == "int" {
				v, err := strconv.Atoi(p.Default)
				if err != nil {
					return fmt.Errorf("Parameters type is wrong: %v .Please report this to OAM maintainer, thanks.", err)
				}
				fakeCommand.PersistentFlags().Int(p.Name, v, p.Usage)
				parentCmd.PersistentFlags().Int(p.Name, v, p.Usage)
			} else {
				fakeCommand.PersistentFlags().String(p.Name, p.Default, p.Usage)
				parentCmd.PersistentFlags().String(p.Name, p.Default, p.Usage)
			}
		}
	}

	fakeCommand.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Complete(fakeCommand, f, args, ctx); err != nil {
			return err
		}
		return o.Run(f, fakeCommand, ctx)
	}
	return fakeCommand.Execute()
}

func (o *commandOptions) Complete(cmd *cobra.Command, f cmdutil.Factory, args []string, ctx context.Context) error {
	argsLength := len(args)
	var componentName string
	c := o.Client

	if argsLength == 0 {
		return errors.New("Please append the name of an application. Use `rudr bind -h` for more " +
			"detailed information.")
	} else if argsLength <= 2 {
		componentName = args[0]
		err := c.Get(ctx, client.ObjectKey{Namespace: "default", Name: componentName}, &o.AppConfig)
		if err != nil {
			return err
		}
		ns := o.AppConfig.Namespace

		var component corev1alpha2.Component
		err = c.Get(ctx, client.ObjectKey{Namespace: ns, Name: componentName}, &component)
		if err != nil {
			return fmt.Errorf("%s. Please choose an existed component name", err)
		}

		// Retrieve all traits which can be used for the following 1) help and 2) validating
		traitList, err := RetrieveTraitsByWorkload(ctx, o.Client, "")
		if err != nil {
			return fmt.Errorf("List available traits hit an issue: %s", err)
		}

		switch argsLength {
		case 1:
			// Validate component and suggest trait
			errTip := "Error: No trait specified.\nPlease choose a trait: "
			for _, trait := range traitList {
				n := trait.Short
				if n == "" {
					n = trait.Name
				}
				errTip += n + " "
			}
			return errors.New(errTip)
		case 2:
			// validate trait
			traitName := args[1]
			var traitLongName string

			validTrait := false
			for _, trait := range traitList {
				// Support trait name or trait short name case-sensitively
				if strings.EqualFold(trait.Name, traitName) || strings.EqualFold(trait.Short, traitName) {
					validTrait = true
					traitLongName = trait.Name
					break
				}
			}

			if !validTrait {
				return fmt.Errorf("The trait `%s` is NOT valid, please try a valid one.", traitName)
			}

			var traitDefinition corev1alpha2.TraitDefinition
			c.Get(ctx, client.ObjectKey{Namespace: ns, Name: traitLongName}, &traitDefinition)

			var traitTemplate v1alpha2.Template
			c.Get(ctx, client.ObjectKey{Namespace: "default", Name: traitDefinition.ObjectMeta.Annotations[TemplateLabel]}, &traitTemplate)

			pvd := fieldpath.Pave(traitTemplate.Spec.Object.Object)
			for _, v := range traitTemplate.Spec.Parameters {
				flagSet := cmd.Flag(v.Name)
				for _, path := range v.FieldPaths {
					fValue := flagSet.Value.String()
					if v.Type == "int" {
						portValue, _ := strconv.ParseFloat(fValue, 64)
						pvd.SetNumber(path, portValue)
						continue
					}
					pvd.SetString(path, fValue)
				}
			}

			// metadata.name needs to be in lower case.
			pvd.SetString("metadata.name", strings.ToLower(traitName))

			var t corev1alpha2.ComponentTrait
			t.Trait.Object = &unstructured.Unstructured{Object: pvd.UnstructuredContent()}
			o.Component.Name = componentName
			o.AppConfig.Spec.Components = []corev1alpha2.ApplicationConfigurationComponent{{
				ComponentName: componentName,
				Traits:        []corev1alpha2.ComponentTrait{t},
			},
			}
		}
	} else {
		return errors.New("Unknown command is specified, please check and try again.")
	}
	return nil
}

// Run command
func (o *commandOptions) Run(f cmdutil.Factory, cmd *cobra.Command, ctx context.Context) error {
	o.Infof("Applying trait for component %s\n", o.Component.Name)
	c := o.Client
	err := c.Update(ctx, &o.AppConfig)
	if err != nil {
		return fmt.Errorf("Applying trait hit an issue: %s", err)
	}

	o.Info("Succeeded!")
	return nil
}

func newBindCommand() *cobra.Command {
	return &cobra.Command{
		Use:                   "bind APPLICATION-NAME TRAIT-NAME [FLAG]",
		DisableFlagsInUseLine: true,
		Short:                 "Attach a trait to a component",
		Long:                  "Attach a trait to a component.",
		Example:               `rudr bind frontend scaler --max=5`,
	}
}
