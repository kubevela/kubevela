package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	mycue "github.com/cloud-native-application/rudrx/pkg/cue"

	"cuelang.org/go/cue"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"

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
	Template   types.Template
	Component  corev1alpha2.Component
	AppConfig  corev1alpha2.ApplicationConfiguration
	Client     client.Client
	TraitAlias string
	Detach     bool
	Env        *types.EnvMeta
	cmdutil.IOStreams
}

func NewCommandOptions(ioStreams cmdutil.IOStreams) *commandOptions {
	return &commandOptions{IOStreams: ioStreams}
}

func AddTraitPlugins(parentCmd *cobra.Command, c client.Client, ioStreams cmdutil.IOStreams) error {
	dir, _ := system.GetDefinitionDir()
	templates, err := plugins.GetTraitsFromCluster(context.TODO(), types.DefaultOAMNS, c, dir, nil)
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, tmp := range templates {
		var name = tmp.Name
		o := NewCommandOptions(ioStreams)
		o.Client = c
		o.Env, _ = GetEnv()
		pluginCmd := &cobra.Command{
			Use:                   name + " <appname> [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Attach " + name + " trait to an app",
			Long:                  "Attach " + name + " trait to an app",
			Example:               `vela scale frontend --max=5`,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := o.Complete(cmd, args, ctx); err != nil {
					return err
				}
				return o.Run(cmd, ctx)
			},
		}
		pluginCmd.SetOut(o.Out)
		for _, v := range tmp.Parameters {
			types.SetFlagBy(pluginCmd, v)
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

	var traitData = make(map[string]interface{})

	var tp = o.Template.Name

	for _, v := range o.Template.Parameters {
		flagSet := cmd.Flag(v.Name)
		switch v.Type {
		case cue.IntKind:
			d, _ := strconv.ParseInt(flagSet.Value.String(), 10, 64)
			traitData[v.Name] = d
		case cue.StringKind:
			traitData[v.Name] = flagSet.Value.String()
		case cue.BoolKind:
			d, _ := strconv.ParseBool(flagSet.Value.String())
			traitData[v.Name] = d
		case cue.NumberKind, cue.FloatKind:
			d, _ := strconv.ParseFloat(flagSet.Value.String(), 64)
			traitData[v.Name] = d
		}
	}

	jsondata, err := mycue.Eval(o.Template.DefinitionPath, tp, traitData)
	if err != nil {
		return err
	}
	var obj = make(map[string]interface{})
	if err = json.Unmarshal([]byte(jsondata), &obj); err != nil {
		return err
	}

	pvd := fieldpath.Pave(obj)
	// metadata.name needs to be in lower case.
	pvd.SetString("metadata.name", strings.ToLower(fmt.Sprintf("%s-%s-trait", appName, o.Template.Name)))
	curObj := &unstructured.Unstructured{Object: pvd.UnstructuredContent()}
	var updated bool
	for ic, c := range o.AppConfig.Spec.Components {
		if c.ComponentName != appName {
			continue
		}
		for it, t := range c.Traits {
			g, v, k := GetGVKFromRawExtension(t.Trait)

			// TODO(wonderflow): we should get GVK from DefinitionPath instead of assuming template object contains
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
	dir, _ := system.GetDefinitionDir()
	templates, err := plugins.GetTraitsFromCluster(context.TODO(), types.DefaultOAMNS, c, dir, nil)
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, tmp := range templates {
		var name = tmp.Name
		o := NewCommandOptions(ioStreams)
		o.Client = c
		o.Env, _ = GetEnv()
		pluginCmd := &cobra.Command{
			Use:                   name + ":detach <appname>",
			DisableFlagsInUseLine: true,
			Short:                 "Detach " + name + " trait from an app",
			Long:                  "Detach " + name + " trait from an app",
			Example:               `vela scale:detach frontend`,
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

	apiVersion, kind, err := cmdutil.GetTraitApiVersionKind(ctx, c, namespace, o.TraitAlias)
	if err != nil {
		return err
	}
	for i, com := range o.AppConfig.Spec.Components {
		if com.ComponentName != appName {
			continue
		}
		var traits []corev1alpha2.ComponentTrait
		for _, tr := range com.Traits {
			a, k := tr.Trait.Object.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
			if a == apiVersion && k == kind {
				continue
			}
			traits = append(traits, tr)
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
