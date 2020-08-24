package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/cloud-native-application/rudrx/api/types"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAppShowCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "show <APPLICATION-NAME>",
		Short:   "get detail spec of your app",
		Long:    "get detail spec of your app, including its workload and trait",
		Example: `vela app show <APPLICATION-NAME>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength == 0 {
				ioStreams.Errorf("Hint: please specify the application name")
				os.Exit(1)
			}
			appName := args[0]
			env, err := GetEnv(cmd)
			if err != nil {
				ioStreams.Errorf("Error: failed to get Env: %s", err)
				return err
			}

			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}

			return showApplication(ctx, newClient, cmd, env, appName)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

type Unkown struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec              interface{} `json:"spec"`
	Status            interface{} `json:"status"`
}

func showApplication(ctx context.Context, c client.Client, cmd *cobra.Command, env *types.EnvMeta, appName string) error {
	var (
		application corev1alpha2.ApplicationConfiguration
	)
	namespace := env.Namespace

	if err := c.Get(ctx, client.ObjectKey{Name: appName, Namespace: namespace}, &application); err != nil {
		return fmt.Errorf("Fetch application with Err: %s", err)
	}

	if len(application.Spec.Components) == 0 {
		cmd.Println("About:")
		cmd.Printf("  Appset: %s", appName)
		cmd.Printf("  ENV: %s", namespace)
		return nil
	}

	// current only support one component
	componentName := application.Spec.Components[0].ComponentName
	component, err := cmdutil.GetComponent(ctx, c, componentName, namespace)
	if err != nil {
		return fmt.Errorf("Fetch component %s with Err: %s", componentName, err)
	}
	if component.Labels == nil {
		return fmt.Errorf("Can't get workloadDef, please check component %s label \"%s\" is correct.",
			componentName, types.ComponentWorkloadDefLabel)
	}

	traitDefinitions := cmdutil.ListTraitDefinitionsByApplicationConfiguration(application)
	componentOut, _ := yaml.JSONToYAML(component.Spec.Workload.Raw)

	cmd.Println("About:")
	cmd.Printf("  Appset: %s\n", appName)
	cmd.Printf("  ENV: %s\n", namespace)
	cmd.Printf("%s", string(componentOut))

	if len(traitDefinitions) != 0 {
		cmd.Println("Traits:")

		traitOut, _ := yaml.Marshal(application.Spec.Components[0].Traits)
		cmd.Println(string(traitOut))
	}

	return nil
}
