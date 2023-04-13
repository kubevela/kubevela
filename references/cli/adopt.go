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
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/storage"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/resourcetopology"
	velaslices "github.com/kubevela/pkg/util/slices"

	"github.com/kubevela/pkg/multicluster"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/env"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	adoptTypeNative     = "native"
	adoptTypeHelm       = "helm"
	adoptModeReadOnly   = v1alpha1.ReadOnlyPolicyType
	adoptModeTakeOver   = v1alpha1.TakeOverPolicyType
	helmDriverEnvKey    = "HELM_DRIVER"
	defaultHelmDriver   = "secret"
	adoptCUETempVal     = "adopt"
	adoptCUETempFunc    = "#Adopt"
	defaultLocalCluster = "local"
)

//go:embed adopt-templates/default.cue
var defaultAdoptTemplate string

//go:embed resource-topology/builtin-rule.cue
var defaultResourceTopologyRule string

var (
	adoptTypes = []string{adoptTypeNative, adoptTypeHelm}
	adoptModes = []string{adoptModeReadOnly, adoptModeTakeOver}
)

type resourceRef struct {
	schema.GroupVersionKind
	apitypes.NamespacedName
	Cluster string
	Arg     string
}

// AdoptOptions options for vela adopt command
type AdoptOptions struct {
	Type         string `json:"type"`
	Mode         string `json:"mode"`
	AppName      string `json:"appName"`
	AppNamespace string `json:"appNamespace"`

	HelmReleaseName      string
	HelmReleaseNamespace string
	HelmDriver           string
	HelmConfig           *action.Configuration
	HelmStore            *storage.Storage
	HelmRelease          *release.Release
	HelmReleaseRevisions []*release.Release

	NativeResourceRefs []*resourceRef

	Apply   bool
	Recycle bool
	Yes     bool
	All     bool

	AdoptTemplateFile     string
	AdoptTemplate         string
	AdoptTemplateCUEValue cue.Value

	ResourceTopologyRuleFile string
	ResourceTopologyRule     string
	AllGVKs                  []schema.GroupVersionKind

	Resources []*unstructured.Unstructured `json:"resources"`

	util.IOStreams
}

func (opt *AdoptOptions) parseResourceGVK(f velacmd.Factory, arg string) (schema.GroupVersionKind, error) {
	_, gr := schema.ParseResourceArg(arg)
	gvks, err := f.Client().RESTMapper().KindsFor(gr.WithVersion(""))
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("failed to find types for resource %s: %w", arg, err)
	}
	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no schema found for resource %s: %w", arg, err)
	}
	return gvks[0], nil
}

func (opt *AdoptOptions) parseResourceRef(f velacmd.Factory, cmd *cobra.Command, arg string) (*resourceRef, error) {
	parts := strings.Split(arg, "/")
	gvk, err := opt.parseResourceGVK(f, parts[0])
	if err != nil {
		return nil, err
	}
	mappings, err := f.Client().RESTMapper().RESTMappings(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to find mappings for resource %s: %w", arg, err)
	}
	if len(mappings) == 0 {
		return nil, fmt.Errorf("no mappings found for resource %s: %w", arg, err)
	}
	mapping := mappings[0]
	or := &resourceRef{GroupVersionKind: gvk, Cluster: defaultLocalCluster, Arg: arg}
	switch len(parts) {
	case 2:
		or.Name = parts[1]
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			or.Namespace = velacmd.GetNamespace(f, cmd)
			if or.Namespace == "" {
				or.Namespace = env.DefaultEnvNamespace
			}
		}
	case 3:
		or.Namespace = parts[1]
		or.Name = parts[2]
	case 4:
		or.Cluster = parts[1]
		or.Namespace = parts[2]
		or.Name = parts[3]
	default:
		return nil, fmt.Errorf("resource should be like <type>/<name> or <type>/<namespace>/<name> or <type>/<cluster>/<namespace>/<name>")
	}
	return or, nil
}

// Init .
func (opt *AdoptOptions) Init(f velacmd.Factory, cmd *cobra.Command, args []string) (err error) {
	if opt.All {
		if len(args) > 0 {
			for _, arg := range args {
				gvk, err := opt.parseResourceGVK(f, arg)
				if err != nil {
					return err
				}
				opt.AllGVKs = append(opt.AllGVKs, gvk)
				apiVersion, kind := gvk.ToAPIVersionAndKind()
				_, _ = fmt.Fprintf(opt.Out, "Adopt all %s/%s resources\n", apiVersion, kind)
			}
		}
		if len(opt.AllGVKs) == 0 {
			opt.AllGVKs = []schema.GroupVersionKind{
				appsv1.SchemeGroupVersion.WithKind("Deployment"),
				appsv1.SchemeGroupVersion.WithKind("StatefulSet"),
				appsv1.SchemeGroupVersion.WithKind("DaemonSet"),
			}
			_, _ = opt.Out.Write([]byte("No arguments specified, adopt all Deployment/StatefulSet/DaemonSet resources by default\n"))
		}
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
	if opt.ResourceTopologyRuleFile != "" {
		bs, err := os.ReadFile(opt.ResourceTopologyRuleFile)
		if err != nil {
			return fmt.Errorf("failed to load file %s", opt.ResourceTopologyRuleFile)
		}
		opt.ResourceTopologyRule = string(bs)
	} else {
		opt.ResourceTopologyRule = defaultResourceTopologyRule
	}
	opt.AppNamespace = velacmd.GetNamespace(f, cmd)
	opt.AdoptTemplateCUEValue, err = cuex.CompileString(cmd.Context(), fmt.Sprintf("%s\n\n%s: %s", opt.AdoptTemplate, adoptCUETempVal, adoptCUETempFunc))
	if err != nil {
		return fmt.Errorf("failed to compile template: %w", err)
	}
	switch opt.Type {
	case adoptTypeNative:
		if opt.Recycle {
			return fmt.Errorf("native resource adoption does not support --recycle flag")
		}
	case adoptTypeHelm:
		if len(opt.HelmDriver) == 0 {
			opt.HelmDriver = os.Getenv(helmDriverEnvKey)
		}
		if len(opt.HelmDriver) == 0 {
			opt.HelmDriver = defaultHelmDriver
		}
		actionConfig := new(action.Configuration)
		opt.HelmReleaseNamespace = opt.AppNamespace
		if err := actionConfig.Init(
			util.NewRestConfigGetterByConfig(f.Config(), opt.HelmReleaseNamespace),
			opt.HelmReleaseNamespace,
			opt.HelmDriver,
			klog.Infof); err != nil {
			return err
		}
		opt.HelmConfig = actionConfig
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

// MultipleRun .
func (opt *AdoptOptions) MultipleRun(f velacmd.Factory, cmd *cobra.Command) error {
	resources := make([][]*unstructured.Unstructured, 0)
	releases := make([]*release.Release, 0)
	var err error
	ctx := context.Background()

	matchLabels := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      oam.LabelAppName,
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(&matchLabels)
	if err != nil {
		return err
	}

	switch opt.Type {
	case adoptTypeNative:
		for _, gvk := range opt.AllGVKs {
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(gvk)
			if err := f.Client().List(ctx, list, &client.ListOptions{Namespace: opt.AppNamespace, LabelSelector: selector}); err != nil {
				apiVersion, kind := gvk.ToAPIVersionAndKind()
				_, _ = fmt.Fprintf(opt.Out, "Warning: failed to list resources from %s/%s: %s\n", apiVersion, kind, err.Error())
				continue
			}
			dedup := make([]k8s.ResourceIdentifier, 0)
			for _, item := range list.Items {
				engine := resourcetopology.New(opt.ResourceTopologyRule)
				itemIdentifier := k8s.ResourceIdentifier{
					Name:       item.GetName(),
					Namespace:  item.GetNamespace(),
					Kind:       item.GetKind(),
					APIVersion: item.GetAPIVersion(),
				}
				if velaslices.Contains(dedup, itemIdentifier) {
					continue
				}
				firstElement := item
				r := []*unstructured.Unstructured{&firstElement}
				peers, err := engine.GetPeerResources(ctx, itemIdentifier)
				if err != nil {
					_, _ = fmt.Fprintf(opt.Out, "Warning: failed to get peer resources for %s/%s: %s\n", itemIdentifier.APIVersion, itemIdentifier.Kind, err.Error())
					resources = append(resources, r)
					continue
				}
				dedup = append(dedup, peers...)
				for _, peer := range peers {
					gvk, err := k8s.GetGVKFromResource(peer)
					if err != nil {
						_, _ = fmt.Fprintf(opt.Out, "Warning: failed to get gvk from resource %s/%s: %s\n", peer.APIVersion, peer.Kind, err.Error())
						continue
					}
					peerResource := &unstructured.Unstructured{}
					peerResource.SetGroupVersionKind(gvk)
					if err := f.Client().Get(ctx, apitypes.NamespacedName{Namespace: peer.Namespace, Name: peer.Name}, peerResource); err != nil {
						_, _ = fmt.Fprintf(opt.Out, "Warning: failed to get resource %s/%s: %s\n", peer.Namespace, peer.Name, err.Error())
						continue
					}
					r = append(r, peerResource)
				}
				resources = append(resources, r)
			}
		}
	case adoptTypeHelm:
		releases, err = opt.HelmConfig.Releases.List(func(release *release.Release) bool {
			return true
		})
		if err != nil {
			return err
		}
	}
	for _, r := range resources {
		opt.Resources = r
		opt.AppName = r[0].GetName()
		opt.AppNamespace = r[0].GetNamespace()
		if err := opt.Run(f, cmd); err != nil {
			_, _ = fmt.Fprintf(opt.Out, "Error: failed to adopt %s/%s: %s", opt.AppNamespace, opt.AppName, err.Error())
			continue
		}
	}
	for _, r := range releases {
		opt.AppName = r.Name
		opt.AppNamespace = r.Namespace
		opt.HelmReleaseName = r.Name
		opt.HelmReleaseNamespace = r.Namespace
		// TODO(fog): filter the helm that already adopted by vela
		if err := opt.loadHelm(); err != nil {
			_, _ = fmt.Fprintf(opt.Out, "Error: failed to load helm for %s/%s: %s", opt.AppNamespace, opt.AppName, err.Error())
			continue
		}
		if err := opt.Run(f, cmd); err != nil {
			_, _ = fmt.Fprintf(opt.Out, "Error: failed to adopt %s/%s: %s", opt.AppNamespace, opt.AppName, err.Error())
			continue
		}
	}
	return nil
}

// Complete autofill fields in opts
func (opt *AdoptOptions) Complete(f velacmd.Factory, cmd *cobra.Command, args []string) (err error) {
	opt.AppNamespace = velacmd.GetNamespace(f, cmd)
	switch opt.Type {
	case adoptTypeNative:
		for _, arg := range args {
			or, err := opt.parseResourceRef(f, cmd, arg)
			if err != nil {
				return err
			}
			opt.NativeResourceRefs = append(opt.NativeResourceRefs, or)
		}
		if opt.AppName == "" && velaslices.All(opt.NativeResourceRefs, func(ref *resourceRef) bool {
			return ref.Name == opt.NativeResourceRefs[0].Name
		}) {
			opt.AppName = opt.NativeResourceRefs[0].Name
		}
		if opt.AppNamespace == "" {
			opt.AppNamespace = opt.NativeResourceRefs[0].Namespace
		}
		if err := opt.loadNative(f, cmd); err != nil {
			return err
		}
	case adoptTypeHelm:
		if len(args) > 0 {
			opt.HelmReleaseName = args[0]
		}
		if len(args) > 1 {
			return fmt.Errorf("helm type adoption only support one helm release by far")
		}
		if opt.AppName == "" {
			opt.AppName = opt.HelmReleaseName
		}
		if err := opt.loadHelm(); err != nil {
			return err
		}
	default:
	}
	if opt.AppName != "" {
		app := &v1beta1.Application{}
		err := f.Client().Get(cmd.Context(), apitypes.NamespacedName{Namespace: opt.AppNamespace, Name: opt.AppName}, app)
		if err == nil && app != nil {
			if !opt.Yes && opt.Apply {
				userInput := NewUserInput()
				confirm := userInput.AskBool(
					fmt.Sprintf("Application '%s' already exists, apply will override the existing app with the adopted one, please confirm [Y/n]: ", opt.AppName),
					&UserInputOptions{AssumeYes: false})
				if !confirm {
					return nil
				}
			}
		}
	}
	opt.AdoptTemplateCUEValue, err = cuex.CompileString(cmd.Context(), fmt.Sprintf("%s\n\n%s: %s", opt.AdoptTemplate, adoptCUETempVal, adoptCUETempFunc))
	if err != nil {
		return fmt.Errorf("failed to compile cue template: %w", err)
	}
	return err
}

// Validate if opts is valid
func (opt *AdoptOptions) Validate() error {
	switch opt.Type {
	case adoptTypeNative:
		if len(opt.NativeResourceRefs) == 0 {
			return fmt.Errorf("at least one resource should be specified")
		}
		if opt.AppName == "" {
			return fmt.Errorf("app-name flag must be set for native resource adoption when multiple resources have different names")
		}
	case adoptTypeHelm:
		if len(opt.HelmReleaseName) == 0 {
			return fmt.Errorf("helm release name must not be empty")
		}
	}
	return nil
}

func (opt *AdoptOptions) loadNative(f velacmd.Factory, cmd *cobra.Command) error {
	for _, ref := range opt.NativeResourceRefs {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(ref.GroupVersionKind)
		if err := f.Client().Get(multicluster.WithCluster(cmd.Context(), ref.Cluster), apitypes.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, obj); err != nil {
			return fmt.Errorf("fail to get resource for %s: %w", ref.Arg, err)
		}
		annos := map[string]string{
			oam.LabelAppCluster: ref.Cluster,
		}
		obj.SetAnnotations(annos)
		opt.Resources = append(opt.Resources, obj)
	}
	return nil
}

func (opt *AdoptOptions) loadHelm() error {
	opt.HelmStore = opt.HelmConfig.Releases
	revisions, err := opt.HelmStore.History(opt.HelmReleaseName)
	if err != nil {
		return fmt.Errorf("helm release %s/%s not loaded: %w", opt.HelmReleaseNamespace, opt.HelmReleaseName, err)
	}
	if len(revisions) == 0 {
		return fmt.Errorf("helm release %s/%s not found", opt.HelmReleaseNamespace, opt.HelmReleaseName)
	}
	releaseutil.SortByRevision(revisions)
	opt.HelmRelease = revisions[len(revisions)-1]
	opt.HelmReleaseRevisions = revisions
	manifests := releaseutil.SplitManifests(opt.HelmRelease.Manifest)
	var objs []*unstructured.Unstructured
	for _, val := range manifests {
		obj := &unstructured.Unstructured{}
		if err = yaml.Unmarshal([]byte(val), obj); err != nil {
			klog.Warningf("unable to decode object %s: %s", val, err)
			continue
		}
		annos := map[string]string{
			oam.LabelAppCluster: defaultLocalCluster,
		}
		obj.SetAnnotations(annos)
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
	if app.Name == "" {
		app.Name = opt.AppName
	}
	if app.Namespace == "" {
		app.Namespace = opt.AppNamespace
	}
	return app, nil
}

// Run collect resources, assemble into application and print/apply
func (opt *AdoptOptions) Run(f velacmd.Factory, cmd *cobra.Command) error {
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
		if opt.All {
			_, _ = opt.Out.Write([]byte("\n---\n"))
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
			for _, r := range opt.HelmReleaseRevisions {
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
		have resources applied in your Kubernetes cluster. These resources could be applied
		natively or with other tools, such as Helm. This command will automatically find out
		the resources to be adopted and assemble them into a new application which won't 
		trigger any damage such as restart on the adoption.

		There are two types of adoption supported by far, 'native' Kubernetes resources (by
		default) and 'helm' releases.
		1. For 'native' type, you can specify a list of resources you want to adopt in the
		application. Only resources in local cluster are supported for now.
		2. For 'helm' type, you can specify a helm release name. This helm release should
		be already published in the local cluster. The command will find the resources
		managed by the helm release and convert them into an adoption application.

		There are two working mechanism (called 'modes' here) for the adoption by far, 
		'read-only' mode (by default) and 'take-over' mode.
		1. In 'read-only' mode, adopted resources will not be touched. You can leverage vela 
		tools (like Vela CLI or VelaUX) to observe those resources and attach traits to add 
		new capabilities. The adopted resources will not be recycled or updated. This mode
		is recommended if you still want to keep using other tools to manage resources updates
		or deletion, like Helm.
		2. In 'take-over' mode, adopted resources are completely managed by application which 
		means they can be modified. You can use traits or directly modify the component to make
		edits to those resources. This mode can be helpful if you want to migrate existing 
		resources into KubeVela system and let KubeVela to handle the life-cycle of target
		resources.

		The adopted application can be customized. You can provide a CUE template file to
		the command and make your own assemble rules for the adoption application. You can
		refer to https://github.com/kubevela/kubevela/blob/master/references/cli/adopt-templates/default.cue
		to see the default implementation of adoption rules.

		If you want to adopt all resources with resource topology rule to Applications,
		you can use: 'vela adopt --all'. The resource topology rule can be customized by
		'--resource-topology-rule' flag.
	`))
	adoptExample = templates.Examples(i18n.T(`
		# Native Resources Adoption

		## Adopt resources into new application

		## Adopt all resources to Applications with resource topology rule
		## Use: vela adopt <resources-type> --all
		vela adopt --all
		vela adopt deployment --all --resource-topology-rule myrule.cue

		## Use: vela adopt <resources-type>[/<resource-namespace>]/<resource-name> <resources-type>[/<resource-namespace>]/<resource-name> ...
		vela adopt deployment/my-app configmap/my-app

		## Adopt resources into new application with specified app name
		vela adopt deployment/my-deploy configmap/my-config --app-name my-app

		## Adopt resources into new application in specified namespace
		vela adopt deployment/my-app configmap/my-app -n demo

		## Adopt resources into new application across multiple namespace
		vela adopt deployment/ns-1/my-app configmap/ns-2/my-app

		## Adopt resources into new application with take-over mode
		vela adopt deployment/my-app configmap/my-app --mode take-over

		## Adopt resources into new application and apply it into cluster
		vela adopt deployment/my-app configmap/my-app --apply

		-----------------------------------------------------------

		# Helm Chart Adoption

		## Adopt all helm releases to Applications with resource topology rule
		## Use: vela adopt <resources-type> --all
		vela adopt --all --type helm
		vela adopt my-chart --all --resource-topology-rule myrule.cue --type helm

		## Adopt resources in a deployed helm chart
		vela adopt my-chart -n my-namespace --type helm
		
		## Adopt resources in a deployed helm chart with take-over mode
		vela adopt my-chart --type helm --mode take-over

		## Adopt resources in a deployed helm chart in an application and apply it into cluster
		vela adopt my-chart --type helm --apply

		## Adopt resources in a deployed helm chart in an application, apply it into cluster, and recycle the old helm release after the adoption application successfully runs
		vela adopt my-chart --type helm --apply --recycle

		-----------------------------------------------------------

		## Customize your adoption rules
		vela adopt my-chart -n my-namespace --type helm --adopt-template my-rules.cue
	`))
)

// NewAdoptCommand command for adopt resources into KubeVela Application
func NewAdoptCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	o := &AdoptOptions{
		Type:      adoptTypeNative,
		Mode:      adoptModeReadOnly,
		IOStreams: streams,
	}
	cmd := &cobra.Command{
		Use:     "adopt",
		Short:   i18n.T("Adopt resources into new application"),
		Long:    adoptLong,
		Example: adoptExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Init(f, cmd, args))
			if o.All {
				cmdutil.CheckErr(o.MultipleRun(f, cmd))
				return
			}
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(f, cmd))
		},
	}
	cmd.Flags().StringVarP(&o.Type, "type", "t", o.Type, fmt.Sprintf("The type of adoption. Available values: [%s]", strings.Join(adoptTypes, ", ")))
	cmd.Flags().StringVarP(&o.Mode, "mode", "m", o.Mode, fmt.Sprintf("The mode of adoption. Available values: [%s]", strings.Join(adoptModes, ", ")))
	cmd.Flags().StringVarP(&o.AppName, "app-name", "", o.AppName, "The name of application for adoption. If empty for helm type adoption, it will inherit the helm chart's name.")
	cmd.Flags().StringVarP(&o.AdoptTemplateFile, "adopt-template", "", o.AdoptTemplate, "The CUE template for adoption. If not provided, the default template will be used when --auto is switched on.")
	cmd.Flags().StringVarP(&o.ResourceTopologyRuleFile, "resource-topology-rule", "", o.ResourceTopologyRule, "The CUE template for specify the rule of the resource topology. If not provided, the default rule will be used.")
	cmd.Flags().StringVarP(&o.HelmDriver, "driver", "d", o.HelmDriver, "The storage backend of helm adoption. Only take effect when --type=helm.")
	cmd.Flags().BoolVarP(&o.Apply, "apply", "", o.Apply, "If true, the application for adoption will be applied. Otherwise, it will only be printed.")
	cmd.Flags().BoolVarP(&o.Recycle, "recycle", "", o.Recycle, "If true, when the adoption application is successfully applied, the old storage (like Helm secret) will be recycled.")
	cmd.Flags().BoolVarP(&o.Yes, "yes", "y", o.Yes, "Skip confirmation prompt")
	cmd.Flags().BoolVarP(&o.All, "all", "", o.All, "Adopt all resources in the namespace")
	return velacmd.NewCommandBuilder(f, cmd).
		WithNamespaceFlag().
		WithResponsiveWriter().
		Build()
}
