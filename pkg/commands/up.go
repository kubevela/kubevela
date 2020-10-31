package commands

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

var (
	appFilePath string
)

func NewUpCommand(c types.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "up",
		DisableFlagsInUseLine: true,
		Short:                 "Apply an appfile",
		Long:                  "Apply an appfile, by default vela.yaml",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			velaEnv, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			kubecli, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}

			o := &appfileOptions{
				Kubecli: kubecli,
				IO:      ioStream,
				Env:     velaEnv,
			}
			filePath, err := cmd.Flags().GetString(appFilePath)
			if err != nil {
				return err
			}
			return o.Run(filePath)
		},
	}
	cmd.SetOut(ioStream.Out)

	cmd.Flags().StringP(appFilePath, "f", "", "specify file path for appfile")
	return cmd
}

type appfileOptions struct {
	Kubecli client.Client
	IO      cmdutil.IOStreams
	Env     *types.EnvMeta
}

func (o *appfileOptions) Run(filePath string) error {
	var app *appfile.AppFile
	var err error

	o.IO.Info("Parsing vela.yaml ...")
	if filePath != "" {
		app, err = appfile.LoadFromFile(filePath)
	} else {
		app, err = appfile.Load()
	}
	if err != nil {
		return err
	}

	o.IO.Info("Loading templates ...")
	tm, err := template.Load()
	if err != nil {
		return err
	}

	comps, appConfig, scopes, err := app.BuildOAM(o.Env.Namespace, o.IO, tm, false)
	if err != nil {
		return err
	}

	var w bytes.Buffer

	enc := k8sjson.NewYAMLSerializer(k8sjson.DefaultMetaFactory, nil, nil)
	appConfig.TypeMeta = metav1.TypeMeta{
		APIVersion: v1alpha2.ApplicationConfigurationGroupVersionKind.GroupVersion().String(),
		Kind:       v1alpha2.ApplicationConfigurationKind,
	}
	err = enc.Encode(appConfig, &w)
	if err != nil {
		return fmt.Errorf("yaml encode AppConfig failed: %w", err)
	}
	w.WriteByte('\n')

	for _, comp := range comps {
		w.WriteString("---\n")
		comp.TypeMeta = metav1.TypeMeta{
			APIVersion: v1alpha2.ComponentGroupVersionKind.GroupVersion().String(),
			Kind:       v1alpha2.ComponentKind,
		}
		err = enc.Encode(comp, &w)
		if err != nil {
			return fmt.Errorf("yaml encode service (%s) failed: %w", comp.Name, err)
		}
		w.WriteByte('\n')
	}
	for _, scope := range scopes {
		w.WriteString("---\n")
		err = enc.Encode(scope, &w)
		if err != nil {
			return fmt.Errorf("yaml encode scope (%s) failed: %w", scope.GetName(), err)
		}
		w.WriteByte('\n')
	}

	deployFilePath := ".vela/deploy.yaml"
	o.IO.Infof("writing deploy config to (%s)\n", deployFilePath)
	if err := os.MkdirAll(filepath.Dir(deployFilePath), 0700); err != nil {
		return err
	}
	if err := ioutil.WriteFile(deployFilePath, w.Bytes(), 0600); err != nil {
		return errors.Wrap(err, "write deploy config manifests failed")
	}

	if err := o.saveToAppDir(app); err != nil {
		return errors.Wrap(err, "save to app dir failed")
	}

	o.IO.Infof("\nApplying deploy configs ...\n")
	return o.ApplyAppConfig(appConfig, comps, scopes)
}

func (o *appfileOptions) saveToAppDir(f *appfile.AppFile) error {
	app := &application.Application{AppFile: f}
	return app.Save(o.Env.Name)
}

// Apply deploy config resources for the app.
// It differs by create and update:
// - for create, it displays app status along with information of url, metrics, ssh, logging.
// - for update, it rolls out a canary deployment and prints its information. User can verify the canary deployment.
//   This will wait for user approval. If approved, it continues upgrading the whole; otherwise, it would rollback.
func (o *appfileOptions) ApplyAppConfig(ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component, scopes []oam.Object) error {
	key := apitypes.NamespacedName{
		Namespace: ac.Namespace,
		Name:      ac.Name,
	}
	o.IO.Infof("Checking if app has been deployed...\n")
	var tmpAC v1alpha2.ApplicationConfiguration
	err := o.Kubecli.Get(context.TODO(), key, &tmpAC)
	switch {
	case apierrors.IsNotFound(err):
		o.IO.Infof("app has not been deployed, creating a new deployment...\n")
	case err == nil:
		o.IO.Infof("app existed, updating existing deployment...\n")
	default:
		return err
	}
	if err := o.apply(ac, comps, scopes); err != nil {
		return err
	}
	o.info(ac.Name, comps)
	return nil
}

func (o *appfileOptions) apply(ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component, scopes []oam.Object) error {
	for _, comp := range comps {
		if err := application.CreateOrUpdateComponent(context.TODO(), o.Kubecli, comp); err != nil {
			return err
		}
	}

	if err := application.CreateScopes(context.TODO(), o.Kubecli, scopes); err != nil {
		return err
	}
	return application.CreateOrUpdateAppConfig(context.TODO(), o.Kubecli, ac)
}

func (o *appfileOptions) info(appName string, comps []*v1alpha2.Component) {
	o.IO.Infof("✅ app has been deployed 🚀🚀🚀\n")
	o.IO.Infof("    Port forward: vela port-forward %s\n", appName)
	o.IO.Infof("             SSH: vela exec %s\n", appName)
	o.IO.Infof("         Logging: vela logs %s\n", appName)
	o.IO.Infof("      App status: vela app status %s\n", appName)
	for _, comp := range comps {
		o.IO.Infof("  Service status: vela svc status %s\n", comp.Name)
	}
}
