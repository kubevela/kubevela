package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/cloud-native-application/rudrx/pkg/plugins"

	"github.com/cloud-native-application/rudrx/api/types"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type commandOptions struct {
	Env        *EnvMeta
	Template   types.Template
	Component  corev1alpha2.Component
	AppConfig  corev1alpha2.ApplicationConfiguration
	Client     client.Client
	TraitAlias string
	Detach     bool
	cmdutil.IOStreams
}

func NewCommandOptions(ioStreams cmdutil.IOStreams) *commandOptions {
	return &commandOptions{IOStreams: ioStreams}
}

func AddTraitPlugins(parentCmd *cobra.Command, c client.Client, ioStreams cmdutil.IOStreams) error {
	templates, err := plugins.GetTraitsFromCluster(context.TODO(), types.DefaultOAMNS, c)
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, tmp := range templates {
		var name = tmp.Alias
		o := NewCommandOptions(ioStreams)
		o.Client = c
		o.Env, _ = GetEnv()
		pluginCmd := &cobra.Command{
			Use:                   name + " <appname> [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Attach " + name + " trait to an app",
			Long:                  "Attach " + name + " trait to an app",
			Example:               `rudr scale frontend --max=5`,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := o.Complete(cmd, args, ctx); err != nil {
					return err
				}
				return o.Run(cmd, ctx)
			},
		}
		pluginCmd.SetOut(o.Out)
		for _, v := range tmp.Parameters {
			pluginCmd.Flags().StringP(v.Name, v.Short, v.Default, v.Usage)
			if v.Required {
				pluginCmd.MarkFlagRequired(v.Name)
			}
		}

		o.Template = tmp
		parentCmd.AddCommand(pluginCmd)
	}
	return nil
}

func (o *commandOptions) Complete(cmd *cobra.Command, args []string, ctx context.Context) error {
	argsLength := len(args)
	var appName string

	c := o.Client

	namespace := o.Env.Namespace

	if argsLength < 1 {
		return errors.New("please specify the name of the app")
	}

	// Get AppConfig
	// TODO(wonderflow): appName is Component Name here, check if it's has appset with a different name
	appName = args[0]
	if err := c.Get(ctx, client.ObjectKey{Namespace: o.Env.Namespace, Name: appName}, &o.AppConfig); err != nil {
		return err
	}

	// Get component
	var component corev1alpha2.Component
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appName}, &component); err != nil {
		return err
	}

	pvd := fieldpath.Pave(o.Template.Object)
	for _, v := range o.Template.Parameters {
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
	pvd.SetString("metadata.name", strings.ToLower(fmt.Sprintf("%s-%s-trait", appName, o.Template.Alias)))
	curObj := &unstructured.Unstructured{Object: pvd.UnstructuredContent()}
	var updated bool
	for ic, c := range o.AppConfig.Spec.Components {
		if c.ComponentName != appName {
			continue
		}
		for it, t := range c.Traits {
			g, v, k := GetGVKFromRawExtension(t.Trait)

			// TODO(wonderflow): we should get GVK from Definition instead of assuming template object contains
			gvk := curObj.GroupVersionKind()
			if gvk.Group == g && gvk.Version == v && gvk.Kind == k {
				updated = true
				c.Traits[it] = corev1alpha2.ComponentTrait{Trait: runtime.RawExtension{Object: curObj}}
				break
			}
		}
		if !updated {
			c.Traits = append(c.Traits, corev1alpha2.ComponentTrait{Trait: runtime.RawExtension{Object: curObj}})
		}
		o.AppConfig.Spec.Components[ic] = c
		break
	}
	return nil
}

func DetachTraitPlugins(parentCmd *cobra.Command, c client.Client, ioStreams cmdutil.IOStreams) error {
	templates, err := plugins.GetTraitsFromCluster(context.TODO(), types.DefaultOAMNS, c)
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, tmp := range templates {
		var name = tmp.Alias
		o := NewCommandOptions(ioStreams)
		o.Client = c
		o.Env, _ = GetEnv()
		pluginCmd := &cobra.Command{
			Use:                   name + ":detach <appname>",
			DisableFlagsInUseLine: true,
			Short:                 "Detach " + name + " trait from an app",
			Long:                  "Detach " + name + " trait from an app",
			Example:               `rudr scale:detach frontend`,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := o.DetachTrait(cmd, args, ctx); err != nil {
					return err
				}
				return o.Run(cmd, ctx)
			},
		}
		pluginCmd.SetOut(o.Out)
		o.TraitAlias = name
		o.Detach = true
		parentCmd.AddCommand(pluginCmd)
	}
	return nil
}

func (o *commandOptions) DetachTrait(cmd *cobra.Command, args []string, ctx context.Context) error {
	argsLength := len(args)
	if argsLength < 1 {
		return errors.New("please specify the name of the app")
	}
	c := o.Client
	namespace := o.Env.Namespace

	var appName = args[0]
	if err := c.Get(ctx, client.ObjectKey{Namespace: o.Env.Namespace, Name: appName}, &o.AppConfig); err != nil {
		return err
	}

	_, _, tKind := cmdutil.GetTraitNameAliasKind(ctx, c, namespace, o.TraitAlias)
	var traitDefinition corev1alpha2.TraitDefinition

	for i, com := range o.AppConfig.Spec.Components {
		traits := com.Traits
		if com.ComponentName == appName {
			for j := 0; j < len(traits); j++ {
				err := json.Unmarshal(traits[j].Trait.Raw, &traitDefinition)
				if err != nil {
					return err
				}
				if strings.EqualFold(traitDefinition.Kind, tKind) {
					traits = append(traits[:j], traits[j+1:]...)
					j--
				}
			}
		}
		o.AppConfig.Spec.Components[i].Traits = traits
	}

	return nil
}

func (o *commandOptions) Run(cmd *cobra.Command, ctx context.Context) error {
	if o.Detach {
		o.Info("Detaching trait from app", o.Component.Name)
	} else {
		o.Info("Applying trait for app", o.Component.Name)
	}
	c := o.Client
	err := c.Update(ctx, &o.AppConfig)
	if err != nil {
		return err
	}
	o.Info("Succeeded!")
	return nil
}

func GetGVKFromRawExtension(extension runtime.RawExtension) (string, string, string) {
	if extension.Object != nil {
		gvk := extension.Object.GetObjectKind().GroupVersionKind()
		return gvk.Group, gvk.Version, gvk.Kind
	}
	var data map[string]interface{}
	// leverage Admission Controller to do the check
	_ = json.Unmarshal(extension.Raw, &data)
	obj := unstructured.Unstructured{Object: data}
	gvk := obj.GroupVersionKind()
	return gvk.Group, gvk.Version, gvk.Kind
}
