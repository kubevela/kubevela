package cmd

import (
	"context"
	"fmt"
	"os"
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

type commandOptions struct {
	Namespace string
	Template  v1alpha2.Template
	Component corev1alpha2.Component
	AppConfig corev1alpha2.ApplicationConfiguration
	Client    client.Client
	cmdutil.IOStreams
}

func NewCommandOptions(ioStreams cmdutil.IOStreams) *commandOptions {
	return &commandOptions{IOStreams: ioStreams}
}

func NewCmdBind(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()

	o := NewCommandOptions(ioStreams)
	cmd := &cobra.Command{
		Use:                   "bind APPLICATION-NAME TRAIT-NAME [FLAG]",
		DisableFlagsInUseLine: true,
		Short:                 "Attach a trait to a component",
		Long:                  "Attach a trait to a component.",
		Example:               `rudr bind frontend scaler --max=5`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd, args, ctx))
			cmdutil.CheckErr(o.Run(f, cmd, ctx))
		},
	}
	//var traitList []Trait

	var traitDefinitions corev1alpha2.TraitDefinitionList
	err := c.List(ctx, &traitDefinitions)
	if err != nil {
		fmt.Println("Listing trait definitions hit an issue:", err)
		os.Exit(1)
	}

	for _, t := range traitDefinitions.Items {
		template := t.ObjectMeta.Annotations["defatultTemplateRef"]

		//traitList = append(traitList, Trait{
		//	Name: t.Name,
		//	Short: t.ObjectMeta.Annotations["short"],
		//})

		var traitTemplate v1alpha2.Template
		err := c.Get(ctx, client.ObjectKey{Namespace: "default", Name: template}, &traitTemplate)
		if err != nil {
			fmt.Println("Listing trait template hit an issue:", err)
			os.Exit(1)
		}

		o.Client = c

		for _, p := range traitTemplate.Spec.Parameters {
			if p.Type == "int" {
				v, err := strconv.Atoi(p.Default)
				if err != nil {
					fmt.Println("Parameters type is wrong: ", err, ".Please report this to OAM maintainer, thanks.")
				}
				cmd.PersistentFlags().Int(p.Name, v, p.Usage)
			} else {
				cmd.PersistentFlags().String(p.Name, p.Default, p.Usage)
			}
		}

		// traitTemplate.DeepCopyInto(&o.Template)
	}

	return cmd
}

func (o *commandOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string, ctx context.Context) error {
	argsLength := len(args)
	var componentName string

	c := o.Client

	if argsLength == 0 {
		cmdutil.PrintErrorMessage("Please append the name of an application. Use `rudr bind -h` for more "+
			"detailed information.", 1)
	} else if argsLength <= 2 {
		componentName = args[0]
		err := c.Get(ctx, client.ObjectKey{Namespace: "default", Name: componentName}, &o.AppConfig)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		ns := o.AppConfig.Namespace

		var component corev1alpha2.Component
		err = c.Get(ctx, client.ObjectKey{Namespace: ns, Name: componentName}, &component)
		if err != nil {
			errMsg := fmt.Sprintf("%s. Please choose an existed component name.", err)
			cmdutil.PrintErrorMessage(errMsg, 1)
		}

		// Retrieve all traits which can be used for the following 1) help and 2) validating
		traitList, err := RetrieveTraitsByWorkload(ctx, o.Client, "")
		if err != nil {
			errMsg := fmt.Sprintf("List available traits hit an issue: %s", err)
			cmdutil.PrintErrorMessage(errMsg, 1)
		}

		switch argsLength {
		case 1:
			// Validate component and suggest trait
			fmt.Print("Error: No trait specified.\nPlease choose a trait: ")
			for _, t := range traitList {
				n := t.Short
				if n == "" {
					n = t.Name
				}
				fmt.Print(n, " ")
			}
			os.Exit(1)

		case 2:
			// validate trait
			traitName := args[1]
			var traitLongName string

			validTrait := false
			for _, t := range traitList {
				// Support trait name or trait short name case-sensitively
				if strings.EqualFold(t.Name, traitName) || strings.EqualFold(t.Short, traitName) {
					validTrait = true
					traitLongName = t.Name
					break
				}
			}

			if !validTrait {
				msg := fmt.Sprintf("The trait `%s` is NOT valid, please try a valid one.", traitName)
				cmdutil.PrintErrorMessage(msg, 1)
			}

			var traitDefinition corev1alpha2.TraitDefinition
			c.Get(ctx, client.ObjectKey{Namespace: ns, Name: traitLongName}, &traitDefinition)

			var traitTemplate v1alpha2.Template
			c.Get(ctx, client.ObjectKey{Namespace: "default", Name: traitDefinition.ObjectMeta.Annotations["defatultTemplateRef"]}, &traitTemplate)

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
		cmdutil.PrintErrorMessage("Unknown command is specified, please check and try again.", 1)
	}
	return nil
}

func (o *commandOptions) Run(f cmdutil.Factory, cmd *cobra.Command, ctx context.Context) error {
	fmt.Println("Applying trait for component", o.Component.Name)
	c := o.Client
	err := c.Update(ctx, &o.AppConfig)
	if err != nil {
		// msg := fmt.Sprintf("Applying trait %s to component %s failed: %s", traitName, componentName, err)
		msg := fmt.Sprintf("Applying trait hit an issue: %s", err)
		cmdutil.PrintErrorMessage(msg, 1)
	}

	msg := fmt.Sprintf("Succeeded!")
	fmt.Println(msg)
	return nil
}
