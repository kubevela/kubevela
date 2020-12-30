package commands

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	k8sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/storage/driver"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
	"github.com/oam-dev/kubevela/pkg/application"
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
	appFile     *appfile.AppFile
	application *v1alpha2.Application
	scopes      []oam.Object
}

func (o *AppfileOptions) export(filePath string, quiet bool) (*buildResult, []byte, error) {
	var app *appfile.AppFile
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
		app, err = appfile.LoadFromFile(filePath)
	} else {
		app, err = appfile.Load()
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

	appHandler := driver.NewApplication(app, tm)
	appHandler, err = appHandler.InitTasks(o.IO)
	if err != nil {
		return nil, nil, err
	}

	retApplication, scopes, err := appHandler.Object(o.Env.Namespace)
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
	return o.apply(result.application, result.scopes)
}

func (o *AppfileOptions) saveToAppDir(f *appfile.AppFile) error {
	app := &driver.Application{AppFile: f}
	return application.Save(app, o.Env.Name)
}

func (o *AppfileOptions) apply(app *v1alpha2.Application, scopes []oam.Object) error {
	if err := application.Run(context.TODO(), o.Kubecli, nil, nil, scopes, app); err != nil {
		return err
	}

	o.IO.Info(o.Info(app))
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
