/*
Copyright 2021 The KubeVela Authors.

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
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"reflect"

	"github.com/ghodss/yaml"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

var (
	clonesetGroupVersion = kruise.GroupVersion.String()
	cloensetKind         = reflect.TypeOf(kruise.CloneSet{}).Name()
	deployGroupVersion   = appsv1.SchemeGroupVersion.String()
	deploymentKind       = reflect.TypeOf(appsv1.Deployment{}).Name()
)

// AdoptCmdOptions contains options to execute 'adopt' cmd that will adopt a
// living workload into a new application.
type AdoptCmdOptions struct {
	// AppName is the name of newly created application
	AppName string
	// Namespace is the namespace where the workload lives
	Namespace string
	// DeploymentName is name of a apps/v1 Deployment, if specified then no need
	// to specify GroupVersion and Kind
	DeploymentName string
	// ClonesetName is name of a apps.kruise.io/v1alpha1 ClonetSet, if specified
	// then no need to specify GroupVersion and Kind
	ClonesetName string
	// Name of target workload, it's required if DeploymentName and ClonesetName
	// both are unspecified
	Name string
	// GroupVersion of target workload, it's required if DeploymentName and ClonesetName
	// both are unspecified
	GroupVersion string
	// Kind of target workload, it's required if DeploymentName and ClonesetName
	// both are unspecified
	Kind string
}

// NewAdoptCommand creates `adopt` command
func NewAdoptCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	adoptOpt := &AdoptCmdOptions{}
	cmd := &cobra.Command{
		Use:                   "adopt",
		DisableFlagsInUseLine: true,
		Short:                 "Adopt a living workload in the cluster and convert it into a component embedded in an application",
		Long: "Adopt a living workload in the cluster, which is not created by KubeVela originally, " +
			"and convert it into a component embedded in an application. Then it can use Trait system seamlessly.",
		Example: `vela adopt`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}
			ns, err := cmd.Flags().GetString(Namespace)
			if err != nil {
				return err
			}
			if ns == "" {
				ns = env.Namespace
			}
			adoptOpt.Namespace = ns
			output, err := adoptOpt.Exec(ctx, k8sClient)
			if err != nil {
				return err
			}
			ioStreams.Info(output.String())
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.Flags().StringVarP(&adoptOpt.DeploymentName, "deployment", "d", "", "name of a Deployment(apps/v1), GVK can be omitted")
	cmd.Flags().StringVarP(&adoptOpt.ClonesetName, "cloneset", "c", "", "name of a CloneSet(apps.kruise.io/v1alpha1), GVK can be omitempty")

	cmd.Flags().StringVarP(&adoptOpt.Name, "name", "", "", "name of a workload, and GVK must be specified")
	cmd.Flags().StringVarP(&adoptOpt.GroupVersion, "groupversion", "", "apps/v1", "group version of the workload")
	cmd.Flags().StringVarP(&adoptOpt.Kind, "kind", "k", "Deployment", "kind of the workload")

	cmd.Flags().StringVarP(&adoptOpt.AppName, "app", "", "", "name of the application being created to adopt the workload, "+
		"if unspecified, it will generate a name automatically")

	cmd.PersistentFlags().StringP(Namespace, "n", "", "specify the namespace the target workload belongs to, default is the current env namespace")

	return cmd
}

// Exec fulfill the main function of `adopt` cmd
func (o *AdoptCmdOptions) Exec(ctx context.Context, k8sClient client.Client) (*bytes.Buffer, error) {
	wlName, wlGVK, err := o.getWorkloadRef()
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("workload reference is invalid: "+
			"apiVersion: %q, kind: %q, name: %q",
			o.GroupVersion, o.Kind, o.Name))
	}

	wl := &unstructured.Unstructured{}
	wl.SetGroupVersionKind(wlGVK)
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: wlName, Namespace: o.Namespace}, wl); err != nil {
		return nil, errors.Wrap(err, "cannot get the living workload from cluster")
	}

	// verify the workload is not controlled by an Application (AppContext)
	if ctrlOwner := metav1.GetControllerOf(wl); ctrlOwner != nil {
		if ctrlOwner.APIVersion == v1alpha2.SchemeGroupVersion.String() &&
			ctrlOwner.Kind == v1alpha2.ApplicationContextKind &&
			ctrlOwner.Controller != nil && *ctrlOwner.Controller {
			return nil, errors.Errorf("the workload is already controlled by an application %q", ctrlOwner.Name)
		}
	}

	// prune workload fields setted by api-server
	// only preserve name, namespace and spec
	prunedWL, err := pruneWorkloadMetadataAndStatus(wl)
	if err != nil {
		return nil, errors.WithMessage(err, "cannot get prune the living workload")
	}
	wlRaw, err := prunedWL.MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal the workload to JSON")
	}

	// constuct and create new application
	appName := fmt.Sprintf("%s-adopted", wlName)
	if o.AppName != "" {
		appName = o.AppName
	}
	app := &v1beta1.Application{}
	app.SetGroupVersionKind(v1beta1.ApplicationKindVersionKind)
	app.SetName(appName)
	app.SetNamespace(o.Namespace)
	app.Spec.Components = []v1beta1.ApplicationComponent{
		{
			// must use workload name as component name
			Name:       wlName,
			Type:       types.NomadComponentDefinition,
			Properties: runtime.RawExtension{Raw: wlRaw},
		},
	}
	if err := k8sClient.Create(ctx, app.DeepCopy()); err != nil {
		return nil, errors.Wrap(err, "cannot creat the application")
	}

	// write application YAML into a local file
	b, err := beautifyAppYAML(app)
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal app into YAML")
	}
	appYAMLFile := appName + ".yaml"
	if err := ioutil.WriteFile(appYAMLFile, b, 0600); err != nil {
		return nil, errors.Wrapf(err, "cannot write the config of new application into file %q", appYAMLFile)
	}

	report := &bytes.Buffer{}
	if _, err := report.WriteString(fmt.Sprintf(
		"Successfully adopt a workload into a new application %q\n"+
			"---\n%s\n---\n"+
			"The application has been wrote into %q\n",
		appName, string(b), appYAMLFile)); err != nil {
		return nil, errors.Wrap(err, "cannot output execution result report")
	}
	return report, nil
}

func (o *AdoptCmdOptions) getWorkloadRef() (string, schema.GroupVersionKind, error) {
	if o.Name == "" && o.DeploymentName == "" && o.ClonesetName == "" {
		return "", schema.GroupVersionKind{}, errors.New("workload name is required")
	}

	if o.DeploymentName != "" {
		return o.DeploymentName, schema.FromAPIVersionAndKind(deployGroupVersion, deploymentKind), nil
	}
	if o.ClonesetName != "" {
		return o.ClonesetName, schema.FromAPIVersionAndKind(clonesetGroupVersion, cloensetKind), nil
	}
	if o.GroupVersion == "" || o.Kind == "" {
		return "", schema.GroupVersionKind{}, errors.New("group version and kind are required")
	}
	return o.Name, schema.FromAPIVersionAndKind(o.GroupVersion, o.Kind), nil
}

func pruneWorkloadMetadataAndStatus(wl *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	prunedWL := &unstructured.Unstructured{}
	prunedWL.SetGroupVersionKind(wl.GroupVersionKind())

	prunedWL.SetName(wl.GetName())
	prunedWL.SetNamespace(wl.GetNamespace())

	spec, _, err := unstructured.NestedMap(wl.Object, "spec")
	if err != nil {
		return nil, errors.Wrap(err, "cannot get the spec of living workload")
	}
	if err := unstructured.SetNestedMap(prunedWL.Object, spec, "spec"); err != nil {
		return nil, err
	}
	return prunedWL, nil
}

// beautifyAppYAML remove 'null' fields cased by empty structs
func beautifyAppYAML(app *v1beta1.Application) ([]byte, error) {
	unstructApp := &unstructured.Unstructured{}
	unstructApp.SetGroupVersionKind(v1beta1.ApplicationKindVersionKind)
	unstructApp.SetName(app.GetName())
	unstructApp.SetNamespace(app.GetNamespace())
	if len(app.Spec.Components) < 1 {
		// this is almost impossible
		return nil, errors.New("at least one component is required")
	}
	comp, err := util.Object2Unstructured(app.Spec.Components[0])
	if err != nil {
		return nil, err
	}
	if err := unstructured.SetNestedSlice(unstructApp.Object,
		[]interface{}{comp.Object}, "spec", "components"); err != nil {
		return nil, err
	}
	b, err := yaml.Marshal(unstructApp.Object)
	if err != nil {
		return nil, err
	}
	return b, nil
}
