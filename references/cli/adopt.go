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
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/storage"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	adoptTypeHelm     = "helm"
	adoptModeReadOnly = v1alpha1.ReadOnlyPolicyType
	adoptModeTakeOver = v1alpha1.TakeOverPolicyType
	helmDriverEnvKey  = "HELM_DRIVER"
	defaultHelmDriver = "secret"
	adoptCUETempVal   = "adopt"
	adoptCUETempFunc  = "#Adopt"
)

//go:embed adopt.cue
var defaultAdoptTemplate string

var (
	adoptTypes = []string{adoptTypeHelm}
	adoptModes = []string{adoptModeReadOnly, adoptModeTakeOver}
)

// AdoptOptions options for vela adopt command
type AdoptOptions struct {
	Type         string `json:"type"`
	Mode         string `json:"mode"`
	AppName      string `json:"appName"`
	AppNamespace string `json:"appNamespace"`

	HelmReleaseName      string
	HelmReleaseNamespace string
	HelmDriver           string
	HelmStore            *storage.Storage
	HelmRelease          *release.Release
	HelmReleases         []*release.Release

	Apply   bool
	Recycle bool

	AdoptTemplateFile     string
	AdoptTemplate         string
	AdoptTemplateCUEValue cue.Value

	Resources []*unstructured.Unstructured `json:"resources"`

	util.IOStreams
}

// Complete autofill fields in opts
func (opt *AdoptOptions) Complete(f velacmd.Factory, cmd *cobra.Command, args []string) error {
	opt.AppNamespace = velacmd.GetNamespace(f, cmd)
	switch opt.Type {
	case adoptTypeHelm:
		if len(args) > 0 {
			opt.HelmReleaseName = args[0]
		}
		if len(opt.HelmDriver) == 0 {
			opt.HelmDriver = os.Getenv(helmDriverEnvKey)
		}
		if len(opt.HelmDriver) == 0 {
			opt.HelmDriver = defaultHelmDriver
		}
		if opt.AppName == "" {
			opt.AppName = opt.HelmReleaseName
		}
		opt.HelmReleaseNamespace = opt.AppNamespace
	default:
	}
	if opt.AdoptTemplateFile != "" {
		bs, err := os.ReadFile(opt.AdoptTemplateFile)
		if err != nil {
			return fmt.Errorf("failed to load file %s", opt.AdoptTemplateFile)
		}
		opt.AdoptTemplate = string(bs)
	} else {
		opt.AdoptTemplate = defaultAdoptTemplate
	}
	opt.AdoptTemplateCUEValue = cuecontext.New().CompileString(fmt.Sprintf("%s\n\n%s: %s", opt.AdoptTemplate, adoptCUETempVal, adoptCUETempFunc))
	return nil
}

// Validate if opts is valid
func (opt *AdoptOptions) Validate() error {
	switch opt.Type {
	case adoptTypeHelm:
		if len(opt.HelmReleaseName) == 0 {
			return fmt.Errorf("helm release name must not be empty")
		}
	default:
		return fmt.Errorf("invalid adopt type: %s, available types: [%s]", opt.Type, strings.Join(adoptTypes, ", "))
	}
	if slices.Index(adoptModes, opt.Mode) < 0 {
		return fmt.Errorf("invalid adopt mode: %s, available modes: [%s]", opt.Mode, strings.Join(adoptModes, ", "))
	}
	if opt.Recycle && !opt.Apply {
		return fmt.Errorf("old data can only be recycled when the adoption application is applied")
	}
	return nil
}

func (opt *AdoptOptions) loadHelm(f velacmd.Factory) error {
	actionConfig := new(action.Configuration)
	err := actionConfig.Init(
		util.NewRestConfigGetterByConfig(f.Config(), opt.HelmReleaseNamespace),
		opt.HelmReleaseNamespace,
		opt.HelmDriver,
		klog.Infof)
	if err != nil {
		return err
	}
	opt.HelmStore = actionConfig.Releases
	releases, err := opt.HelmStore.History(opt.HelmReleaseName)
	if err != nil {
		return fmt.Errorf("helm release %s/%s not loaded: %w", opt.HelmReleaseNamespace, opt.HelmReleaseName, err)
	}
	if len(releases) == 0 {
		return fmt.Errorf("helm release %s/%s not found", opt.HelmReleaseNamespace, opt.HelmReleaseName)
	}
	releaseutil.SortByRevision(releases)
	opt.HelmRelease = releases[len(releases)-1]
	opt.HelmReleases = releases
	manifests := releaseutil.SplitManifests(opt.HelmRelease.Manifest)
	var objs []*unstructured.Unstructured
	for _, val := range manifests {
		obj := &unstructured.Unstructured{}
		if err = yaml.Unmarshal([]byte(val), obj); err != nil {
			klog.Warningf("unable to decode object %s: %s", val, err)
			continue
		}
		objs = append(objs, obj)
	}
	opt.Resources = objs
	return nil
}

func (opt *AdoptOptions) render() (*v1beta1.Application, error) {
	app := &v1beta1.Application{}
	val := opt.AdoptTemplateCUEValue.FillPath(cue.ParsePath(adoptCUETempVal+".$args"), opt)
	bs, err := val.LookupPath(cue.ParsePath(adoptCUETempVal + ".$returns")).MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to parse adoption template: %w", err)
	}
	if err = json.Unmarshal(bs, app); err != nil {
		return nil, fmt.Errorf("failed to parse template $returns into application: %w", err)
	}
	return app, nil
}

// Run collect resources, assemble into application and print/apply
func (opt *AdoptOptions) Run(f velacmd.Factory, cmd *cobra.Command) error {
	switch opt.Type {
	case adoptTypeHelm:
		if err := opt.loadHelm(f); err != nil {
			return fmt.Errorf("failed to load resources from helm release %s/%s", opt.HelmReleaseNamespace, opt.HelmReleaseName)
		}
	default:
	}
	app, err := opt.render()
	if err != nil {
		return fmt.Errorf("failed to make adoption application for resources: %w", err)
	}
	if opt.Apply {
		if err = apply.NewAPIApplicator(f.Client()).Apply(cmd.Context(), app); err != nil {
			return fmt.Errorf("failed to apply application %s/%s: %w", app.Namespace, app.Name, err)
		}
		_, _ = fmt.Fprintf(opt.Out, "resources adopted in app %s/%s\n", app.Namespace, app.Name)
	} else {
		var bs []byte
		if bs, err = yaml.Marshal(app); err != nil {
			return fmt.Errorf("failed to encode application into YAML format: %w", err)
		}
		_, _ = opt.Out.Write(bs)
	}
	if opt.Recycle && opt.Apply {
		spinner := newTrackingSpinner("")
		spinner.Writer = opt.Out
		spinner.Start()
		err = wait.PollImmediate(time.Second, time.Minute, func() (done bool, err error) {
			_app := &v1beta1.Application{}
			if err = f.Client().Get(cmd.Context(), client.ObjectKeyFromObject(app), _app); err != nil {
				return false, err
			}
			spinner.UpdateCharSet([]string{fmt.Sprintf("waiting application %s/%s to be running, current status: %s", app.Namespace, app.Name, _app.Status.Phase)})
			return _app.Status.Phase == common.ApplicationRunning, nil
		})
		spinner.Stop()
		if err != nil {
			return fmt.Errorf("failed to wait application %s/%s to be running: %w", app.Namespace, app.Name, err)
		}
		switch opt.Type {
		case adoptTypeHelm:
			for _, r := range opt.HelmReleases {
				if _, err = opt.HelmStore.Delete(r.Name, r.Version); err != nil {
					return fmt.Errorf("failed to clean up helm release: %w", err)
				}
			}
			_, _ = fmt.Fprintf(opt.Out, "successfully clean up old helm release\n")
		default:
		}
	}
	return nil
}

var (
	adoptLong = templates.LongDesc(i18n.T(`
		Adopt resources into applications

		Adopt resources into a KubeVela application. This command is useful when you already
		have resources applied in your Kubernetes cluster with other tools, such as Helm.
		This command will automatically find out the resources to be adopted and assemble
		them into a new application.

		There are two modes for the adoption by far, read-only mode (by default) and 
		take-over mode.
		1. In read-only mode, adopted resources will not be touched. You can leverage vela 
		tools (like Vela CLI or VelaUX) to observe those resources and attach traits to add 
		new capabilities. The adopted resources will not be recycled or updated. This mode
		is recommended if you still want to keep using other tools to manage resources updates
		or deletion, like Helm.
		2. In take-over mode, adopted resources can be modified. You can use traits or
		directly modify the component to make edits to those resources. This mode can be 
		helpful if you want to migrate existing resources into KubeVela system and let KubeVela
		to handle the life-cycle of target resources.
	`))
	adoptExample = templates.Examples(i18n.T(`
		# Adopt resources in a deployed helm chart
		vela adopt my-chart -n my-namespace --type helm
		
		# Adopt resources in a deployed helm chart with take-over mode
		vela adopt my-chart --type helm --mode take-over

		# Adopt resources in a deployed helm chart in an application and apply it into cluster
		vela adopt my-chart --type helm --apply

		# Adopt resources in a deployed helm chart in an application, apply it into cluster, and recycle the old helm release after the adoption application successfully runs
		vela adopt my-chart --type helm --apply --recycle
	`))
)

// NewAdoptCommand command for adopt resources into KubeVela Application
func NewAdoptCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	o := &AdoptOptions{
		Type:      adoptTypeHelm,
		Mode:      adoptModeReadOnly,
		IOStreams: streams,
	}
	cmd := &cobra.Command{
		Use:     "adopt",
		Short:   i18n.T("Adopt resources into new applications"),
		Long:    adoptLong,
		Example: adoptExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(f, cmd))
		},
	}
	cmd.Flags().StringVarP(&o.Type, "type", "t", o.Type, fmt.Sprintf("The type of adoption. Available values: [%s]", strings.Join(adoptTypes, ", ")))
	cmd.Flags().StringVarP(&o.Mode, "mode", "m", o.Mode, fmt.Sprintf("The mode of adoption. Available values: [%s]", strings.Join(adoptModes, ", ")))
	cmd.Flags().StringVarP(&o.AppName, "app-name", "", o.AppName, "The name of application for adoption. If empty for helm type adoption, it will inherit the helm chart's name.")
	cmd.Flags().StringVarP(&o.AdoptTemplateFile, "adopt-template", "", o.AdoptTemplate, "The CUE template for adoption. If not provided, the default template will be used when --auto is switched on.")
	cmd.Flags().StringVarP(&o.HelmDriver, "driver", "d", o.HelmDriver, "The storage backend of helm adoption. Only take effect when --type=helm.")
	cmd.Flags().BoolVarP(&o.Apply, "apply", "", o.Apply, "If true, the application for adoption will be applied. Otherwise, it will only be printed.")
	cmd.Flags().BoolVarP(&o.Recycle, "recycle", "", o.Recycle, "If true, when the adoption application is successfully applied, the old storage (like Helm secret) will be recycled.")
	return velacmd.NewCommandBuilder(f, cmd).
		WithNamespaceFlag().
		WithResponsiveWriter().
		Build()
}
