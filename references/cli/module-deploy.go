/*
Copyright 2026 The KubeVela Authors.

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
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	modulesvc "github.com/oam-dev/kubevela/pkg/module/service"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// moduleComponentType is the ComponentDefinition the deploy command builds a
// component of. Its template calls the module render service, which fetches the
// module and renders the owned Application holding the install tiers.
const moduleComponentType = "module"

// moduleComponentProperties are the parameters of the type: module component.
// It is a struct rather than a map so the rendered manifest has a stable field
// order.
type moduleComponentProperties struct {
	Module    string `json:"module"`
	Registry  string `json:"registry"`
	Namespace string `json:"namespace"`
	Version   string `json:"version"`
}

// moduleDeployAppName is the name of the Application the deploy command
// creates. The "-deploy" suffix keeps it distinct from ownedModuleAppName: the
// render service names the owned Application "module-<name>", and the two
// collide when both live in the same namespace.
func moduleDeployAppName(moduleName string) string {
	return "module-" + moduleName + "-deploy"
}

// ownedModuleAppName is the name the render service gives the Application it
// renders for a module, mirroring RenderApplication in
// pkg/module/service/render.go.
func ownedModuleAppName(moduleName string) string {
	return "module-" + moduleName
}

// buildModuleApplication builds the one-component Application that installs a
// module. The registry name is the resolved one, not the raw flag, so the
// applied manifest records which registry was chosen. version selects the
// module package version; empty installs latest.
func buildModuleApplication(moduleName, registryName, namespace, version string) (*v1beta1.Application, error) {
	props, err := json.Marshal(moduleComponentProperties{
		Module:    moduleName,
		Registry:  registryName,
		Namespace: namespace,
		Version:   version,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode the module component properties: %w", err)
	}
	return &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1beta1.SchemeGroupVersion.String(),
			Kind:       v1beta1.ApplicationKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      moduleDeployAppName(moduleName),
			Namespace: namespace,
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []oamcommon.ApplicationComponent{{
				Name:       moduleName,
				Type:       moduleComponentType,
				Properties: &runtime.RawExtension{Raw: props},
			}},
		},
	}, nil
}

// moduleTierNames returns the install tier names the render service gave the
// owned Application's components, in spec order. Reading them from the
// Application itself.
func moduleTierNames(app *v1beta1.Application) []string {
	names := make([]string, 0, len(app.Spec.Components))
	for _, c := range app.Spec.Components {
		names = append(names, c.Name)
	}
	return names
}

const (
	moduleDeployRegistryFlag = "registry"
	moduleDeployDryRunFlag   = "dry-run"
	moduleDeployTimeoutFlag  = "timeout"
	moduleDeployVersionFlag  = "version"

	// defaultModuleDeployTimeout is how long deploy waits for every tier to
	// become healthy before giving up.
	defaultModuleDeployTimeout = 5 * time.Minute
	// defaultModuleDeployPollInterval is how often deploy re-reads the two
	// Applications while waiting.
	defaultModuleDeployPollInterval = 2 * time.Second
)

// moduleDeployOptions holds one run of the deploy command. fetch and
// pollInterval are seams: production wires the registry-backed fetch and the
// default interval, tests inject a stub and a millisecond interval.
type moduleDeployOptions struct {
	module    string
	registry  string
	namespace string
	dryRun    bool
	// version selects the module package version to install; empty installs
	// the latest published version.
	version      string
	timeout      time.Duration
	pollInterval time.Duration
	fetch        func(ctx context.Context, registry, moduleName, version string) (*pkgmodule.Module, error)
}

// NewModuleDeployCommand returns the vela module deploy command. It builds and
// applies an Application with a single type: module component, then reports the
// install tiers as they become healthy.
func NewModuleDeployCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	o := &moduleDeployOptions{}
	cmd := &cobra.Command{
		Use:   "deploy <module>",
		Short: "Deploy a module.",
		Long:  "Build and apply an Application that installs a module's enabled API lines, then report the install tiers as they become healthy.",
		Example: `  Deploy a module from the default registry:
	vela module deploy s3

  Deploy from a named registry, into a namespace:
	vela module deploy s3 --registry catalog -n platform

  Print the Application without applying it, capturing it for GitOps:
	vela module deploy s3 --dry-run > s3-module.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := c.GetClient()
			if err != nil {
				return err
			}
			o.module = args[0]
			o.namespace, err = cmd.Flags().GetString("namespace")
			if err != nil {
				return err
			}
			if o.namespace == "" {
				o.namespace = velatypes.DefaultKubeVelaNS
			}
			return o.run(cmd.Context(), cli, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&o.registry, moduleDeployRegistryFlag, "", "The module registry to deploy from. Empty means the configured default.")
	cmd.Flags().StringVar(&o.version, moduleDeployVersionFlag, "", "The module package version (OCI/ECR tag) to install. Empty installs the latest published version. Ignored for a git registry, which always installs from the default branch.")
	cmd.Flags().BoolVar(&o.dryRun, moduleDeployDryRunFlag, false, "Print the Application without applying it.")
	cmd.Flags().DurationVar(&o.timeout, moduleDeployTimeoutFlag, defaultModuleDeployTimeout, "How long to wait for the module to become healthy.")
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// run validates the registry and the module, builds the Application, and either
// prints it or applies it and waits.
func (o *moduleDeployOptions) run(ctx context.Context, cli client.Client, out io.Writer) error {
	if errs := validation.IsDNS1123Label(o.module); len(errs) > 0 {
		return fmt.Errorf("invalid module name %q: %s", o.module, errs[0])
	}
	if o.namespace == "" {
		o.namespace = velatypes.DefaultKubeVelaNS
	}
	if o.pollInterval <= 0 {
		o.pollInterval = defaultModuleDeployPollInterval
	}
	if o.timeout <= 0 {
		o.timeout = defaultModuleDeployTimeout
	}

	store := pkgmodule.NewStore(cli)
	reg, err := pkgmodule.ResolveRegistry(ctx, store, o.registry)
	if err != nil {
		return err
	}

	fetch := o.fetch
	if fetch == nil {
		fetch = modulesvc.NewService(store).FetchModule
	}
	if _, err := fetch(ctx, reg.Name, o.module, o.version); err != nil {
		return err
	}

	app, err := buildModuleApplication(o.module, reg.Name, o.namespace, o.version)
	if err != nil {
		return err
	}
	if o.dryRun {
		manifest, err := yaml.Marshal(app)
		if err != nil {
			return fmt.Errorf("failed to encode the Application manifest: %w", err)
		}
		_, err = out.Write(manifest)
		return err
	}

	if err := apply.NewAPIApplicator(cli).Apply(ctx, app); err != nil {
		return fmt.Errorf("failed to apply Application %s/%s: %w", app.Namespace, app.Name, err)
	}
	fmt.Fprintf(out, "Applied Application %s/%s\n", app.Namespace, app.Name)

	return o.waitForModule(ctx, cli, out)
}

// waitForModule polls the deploy Application and the owned module Application
// until every tier is healthy, printing the tier table whenever it changes.
//
// It reads both Applications because they carry different halves of the answer:
// the deploy Application's phase is where a fetch or render failure surfaces,
// while the tier shape and per-tier health live only on the owned Application
// the render service creates.
func (o *moduleDeployOptions) waitForModule(ctx context.Context, cli client.Client, out io.Writer) error {
	deadline := time.Now().Add(o.timeout)
	lastTable := ""

	for {
		var deployApp v1beta1.Application
		if err := cli.Get(ctx, types.NamespacedName{Name: moduleDeployAppName(o.module), Namespace: o.namespace}, &deployApp); err != nil {
			return fmt.Errorf("failed to read Application %s/%s: %w", o.namespace, moduleDeployAppName(o.module), err)
		}
		switch deployApp.Status.Phase {
		case oamcommon.ApplicationWorkflowFailed, oamcommon.ApplicationWorkflowTerminated, oamcommon.ApplicationDeleting:
			return fmt.Errorf("Application %s/%s is in phase %s: %s",
				o.namespace, deployApp.Name, deployApp.Status.Phase, moduleComponentMessage(&deployApp))
		}

		var tiers []string
		var services []oamcommon.ApplicationComponentStatus
		var ownedApp v1beta1.Application
		err := cli.Get(ctx, types.NamespacedName{Name: ownedModuleAppName(o.module), Namespace: o.namespace}, &ownedApp)
		switch {
		case apierrors.IsNotFound(err):
			// The render service has not created the owned Application yet, so
			// the tier shape is not known.
		case err != nil:
			return fmt.Errorf("failed to read Application %s/%s: %w", o.namespace, ownedModuleAppName(o.module), err)
		default:
			tiers = moduleTierNames(&ownedApp)
			services = ownedApp.Status.Services
		}

		table := "Rendering module (install tiers are not known yet)..."
		if len(tiers) > 0 {
			table = renderModuleTierTable(tiers, services)
		}
		if table != lastTable {
			fmt.Fprintln(out, table)
			lastTable = table
		}

		tier, message := firstUnhealthyTier(tiers, services)
		if tier == "" && len(tiers) > 0 && deployApp.Status.Phase == oamcommon.ApplicationRunning {
			if resolved := ownedApp.Annotations[velatypes.AnnoDefinitionModuleVersion]; resolved != "" {
				fmt.Fprintf(out, "Module %q (version %s) is installed in namespace %q\n", o.module, resolved, o.namespace)
			} else {
				fmt.Fprintf(out, "Module %q is installed in namespace %q\n", o.module, o.namespace)
			}
			return nil
		}

		if time.Now().After(deadline) {
			if len(tiers) == 0 {
				return fmt.Errorf("timed out after %s waiting for module %q: the owned Application %s/%s was not created; deploy Application is in phase %s: %s",
					o.timeout, o.module, o.namespace, ownedModuleAppName(o.module), deployApp.Status.Phase, moduleComponentMessage(&deployApp))
			}
			if tier == "" {
				return fmt.Errorf("timed out after %s waiting for module %q: every tier is healthy but Application %s/%s is in phase %s",
					o.timeout, o.module, o.namespace, deployApp.Name, deployApp.Status.Phase)
			}
			return fmt.Errorf("timed out after %s waiting for module %q: tier %q is not ready: %s",
				o.timeout, o.module, tier, message)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(o.pollInterval):
		}
	}
}

// moduleComponentMessage returns the message of the deploy Application's module
// component, which is where a server-side fetch or render error surfaces.
func moduleComponentMessage(app *v1beta1.Application) string {
	for _, svc := range app.Status.Services {
		if svc.Message != "" {
			return svc.Message
		}
	}
	return "no component message reported"
}

// renderModuleTierTable renders every expected tier with its reported health. A
// tier the owned Application has not reported yet is Pending, so the operator
// sees the whole install shape from the first poll.
func renderModuleTierTable(tiers []string, services []oamcommon.ApplicationComponentStatus) string {
	byName := make(map[string]oamcommon.ApplicationComponentStatus, len(services))
	for _, svc := range services {
		byName[svc.Name] = svc
	}
	table := uitable.New()
	table.AddRow("TIER", "STATUS", "MESSAGE")
	for _, tier := range tiers {
		svc, reported := byName[tier]
		switch {
		case !reported:
			table.AddRow(tier, "Pending", "")
		case svc.Healthy:
			table.AddRow(tier, "Healthy", svc.Message)
		default:
			table.AddRow(tier, "Unhealthy", svc.Message)
		}
	}
	return table.String()
}

// firstUnhealthyTier returns the first tier that is not healthy and why, or an
// empty tier name when every tier is healthy. Tiers are checked in install
// order, so the tier named is the one the install is actually stuck on.
func firstUnhealthyTier(tiers []string, services []oamcommon.ApplicationComponentStatus) (string, string) {
	byName := make(map[string]oamcommon.ApplicationComponentStatus, len(services))
	for _, svc := range services {
		byName[svc.Name] = svc
	}
	for _, tier := range tiers {
		svc, reported := byName[tier]
		switch {
		case !reported:
			return tier, "not reported yet"
		case !svc.Healthy:
			message := svc.Message
			if message == "" {
				message = "not healthy"
			}
			return tier, message
		}
	}
	return "", ""
}
