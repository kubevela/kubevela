package commands

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/oam-dev/kubevela/pkg/appfile/api"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var (
	appFilePath string
)

// NewUpCommand will create command for applying an AppFile
func NewUpCommand(c types.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "up",
		DisableFlagsInUseLine: true,
		Short:                 "Apply an appfile",
		Long:                  "Apply an appfile",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
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

			o := &AppfileOptions{
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

// AppfileOptions is some configuration that modify options for an Appfile
type AppfileOptions struct {
	Kubecli client.Client
	IO      cmdutil.IOStreams
	Env     *types.EnvMeta
}

func saveRemoteAppfile(url string) (string, error) {
	body, err := common.HTTPGet(context.Background(), url)
	if err != nil {
		return "", err
	}
	ext := filepath.Ext(url)
	dest := "Appfile"
	if ext == ".json" {
		dest = "vela.json"
	} else if ext == ".yaml" || ext == ".yml" {
		dest = "vela.yaml"
	}
	//nolint:gosec
	return dest, ioutil.WriteFile(dest, body, 0644)
}

type buildResult struct {
	appFile     *api.AppFile
	application *v1alpha2.Application
	scopes      []oam.Object
}

func (o *AppfileOptions) export(filePath string, quiet bool) (*buildResult, []byte, error) {
	var app *api.AppFile
	var err error
	if !quiet {
		o.IO.Info("Parsing vela appfile ...")
	}
	if filePath != "" {
		if strings.HasPrefix(filePath, "https://") || strings.HasPrefix(filePath, "http://") {
			filePath, err = saveRemoteAppfile(filePath)
			if err != nil {
				return nil, nil, err
			}
		}
		app, err = api.LoadFromFile(filePath)
	} else {
		app, err = api.Load()
	}
	if err != nil {
		return nil, nil, err
	}

	if !quiet {
		o.IO.Info("Load Template ...")
	}

	tm, err := template.Load()
	if err != nil {
		return nil, nil, err
	}

	appHandler := appfile.NewApplication(app, tm)

	// new
	retApplication, scopes, err := appHandler.BuildOAMApplication(o.Env, o.IO, appHandler.Tm, quiet)
	if err != nil {
		return nil, nil, err
	}

	var w bytes.Buffer

	enc := k8sjson.NewYAMLSerializer(k8sjson.DefaultMetaFactory, nil, nil)
	err = enc.Encode(retApplication, &w)
	if err != nil {
		return nil, nil, fmt.Errorf("yaml encode application failed: %w", err)
	}
	w.WriteByte('\n')

	for _, scope := range scopes {
		w.WriteString("---\n")
		err = enc.Encode(scope, &w)
		if err != nil {
			return nil, nil, fmt.Errorf("yaml encode scope (%s) failed: %w", scope.GetName(), err)
		}
		w.WriteByte('\n')
	}

	result := &buildResult{
		appFile:     app,
		application: retApplication,
		scopes:      scopes,
	}
	return result, w.Bytes(), nil
}

// Run starts an application according to Appfile
func (o *AppfileOptions) Run(filePath string) error {
	result, data, err := o.export(filePath, false)
	if err != nil {
		return err
	}
	deployFilePath := ".vela/deploy.yaml"
	o.IO.Infof("Writing deploy config to (%s)\n", deployFilePath)
	if err := os.MkdirAll(filepath.Dir(deployFilePath), 0700); err != nil {
		return err
	}

	if err := ioutil.WriteFile(deployFilePath, data, 0600); err != nil {
		return errors.Wrap(err, "write deploy config manifests failed")
	}

	if err := o.saveToAppDir(result.appFile); err != nil {
		return errors.Wrap(err, "save to app dir failed")
	}

	o.IO.Infof("\nApplying application ...\n")
	return o.ApplyApp(result.application, result.scopes)
}

func (o *AppfileOptions) saveToAppDir(f *api.AppFile) error {
	app := &api.Application{AppFile: f}
	return appfile.Save(app, o.Env.Name)
}

// ApplyApp applys config resources for the app.
// It differs by create and update:
// - for create, it displays app status along with information of url, metrics, ssh, logging.
// - for update, it rolls out a canary deployment and prints its information. User can verify the canary deployment.
//   This will wait for user approval. If approved, it continues upgrading the whole; otherwise, it would rollback.
func (o *AppfileOptions) ApplyApp(app *v1alpha2.Application, scopes []oam.Object) error {
	key := apitypes.NamespacedName{
		Namespace: app.Namespace,
		Name:      app.Name,
	}
	o.IO.Infof("Checking if app has been deployed...\n")
	var tmpApp v1alpha2.Application
	err := o.Kubecli.Get(context.TODO(), key, &tmpApp)
	switch {
	case apierrors.IsNotFound(err):
		o.IO.Infof("App has not been deployed, creating a new deployment...\n")
	case err == nil:
		o.IO.Infof("App exists, updating existing deployment...\n")
	default:
		return err
	}
	if err := o.apply(app, scopes); err != nil {
		return err
	}
	o.IO.Infof(o.Info(app))
	return nil
}

func (o *AppfileOptions) apply(app *v1alpha2.Application, scopes []oam.Object) error {
	if err := appfile.Run(context.TODO(), o.Kubecli, app, scopes); err != nil {
		return err
	}
	return nil
}

// Info shows the status of each service in the Appfile
func (o *AppfileOptions) Info(app *v1alpha2.Application) string {
	appName := app.Name
	var appUpMessage = "✅ App has been deployed 🚀🚀🚀\n" +
		fmt.Sprintf("    Port forward: vela port-forward %s\n", appName) +
		fmt.Sprintf("             SSH: vela exec %s\n", appName) +
		fmt.Sprintf("         Logging: vela logs %s\n", appName) +
		fmt.Sprintf("      App status: vela status %s\n", appName)
	for _, comp := range app.Spec.Components {
		appUpMessage += fmt.Sprintf("  Service status: vela status %s --svc %s\n", appName, comp.Name)
	}
	return appUpMessage
}
