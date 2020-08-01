package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloud-native-application/rudrx/api/types"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type detachCommandOptions struct {
	Env       *EnvMeta
	Template  types.Template
	Component corev1alpha2.Component
	AppConfig corev1alpha2.ApplicationConfiguration
	Client    client.Client
	cmdutil.IOStreams
}

func NewDetachCommandOptions(ioStreams cmdutil.IOStreams) *detachCommandOptions {
	return &detachCommandOptions{IOStreams: ioStreams}
}

func NewDetachCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	o := NewDetachCommandOptions(ioStreams)
	o.Client = c
	var err error
	o.Env, err = GetEnv()
	if err != nil {
		fmt.Printf("Listing trait definitions hit an issue: %v\n", err)
		os.Exit(1)
	}

	cmd := &cobra.Command{
		Use:     "detach APPLICATION-NAME TRAIT-NAME",
		Short:   "detach the trait from the application",
		Long:    "detach the trait from the application",
		Example: `rudr detach frontend ManualScaler`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd, args, ctx))
			cmdutil.CheckErr(o.Apply(f, cmd, ctx))
		},
	}

	var traitDefinitions corev1alpha2.TraitDefinitionList
	if err = c.List(ctx, &traitDefinitions); err != nil {
		fmt.Println("Listing trait definitions hit an issue:", err)
		os.Exit(1)
	}
	return cmd
}

func (o *detachCommandOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string, ctx context.Context) error {
	namespace := o.Env.Namespace
	argsLength := len(args)
	var applicationName string

	c := o.Client

	if argsLength == 0 {
		cmdutil.PrintErrorMessage("please append an application name", 1)
	} else if argsLength <= 2 {
		applicationName = args[0]
		// Check the validity of the specified application name
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: applicationName}, &o.AppConfig); err != nil {
			fmt.Print("Hint: please choose an existed application.")
			return err
		}

		traitNames := cmdutil.GetTraitNamesByApplicationConfiguration(o.AppConfig)
		var traitAlias []string
		for _, n := range traitNames {
			aliasName := cmdutil.GetTraitAliasByName(ctx, c, namespace, n)
			if aliasName == "" {
				aliasName = n
			}
			traitAlias = append(traitAlias, aliasName)
		}

		switch argsLength {
		case 1:
			// suggest available traits alias names from the application
			fmt.Print("Error: no trait specified!")
			if len(traitNames) != 0 {
				fmt.Printf(" Please choose the trait you would like to deatch: %s", strings.Join(traitAlias, ","))
			}
		case 2:
			// validate trait
			traitName := args[1]

			tName, tAlias, tKind := cmdutil.GetTraitNameAliasKind(ctx, c, namespace, traitName)
			if tName == "" && tAlias == "" && tKind == "" {
				errMsg := fmt.Sprintf("Error: trait name `%s` is NOT valid, please try again.", traitName)
				cmdutil.PrintErrorMessage(errMsg, 1)
			}

			traits := o.AppConfig.Spec.Components[0].Traits
			traitDefinitionList := cmdutil.ListTraitDefinitionsByApplicationConfiguration(o.AppConfig)
			for i := 0; i < len(o.AppConfig.Spec.Components[0].Traits); i++ {
				if strings.EqualFold(traitDefinitionList[i].Kind, tKind) {
					o.AppConfig.Spec.Components[0].Traits = append(traits[:i], traits[i+1:]...)
					i--
				}
			}
		}
	} else {
		cmdutil.PrintErrorMessage("Unknown command is specified, please check and try again.", 1)
	}
	return nil
}

func (o *detachCommandOptions) Apply(f cmdutil.Factory, cmd *cobra.Command, ctx context.Context) error {
	fmt.Println("Detaching trait for component", o.Component.Name)
	c := o.Client
	err := c.Update(ctx, &o.AppConfig)
	if err != nil {
		msg := fmt.Sprintf("Detaching the trait hit an issue: %s", err)
		cmdutil.PrintErrorMessage(msg, 1)
	}
	fmt.Println("Succeeded!")
	return nil
}
