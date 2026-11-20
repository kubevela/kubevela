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
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	v1alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	modulesvc "github.com/oam-dev/kubevela/pkg/module/service"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/policy/placement"
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
	Module   string `json:"module"`
	Registry string `json:"registry"`
	Version  string `json:"version"`
	// Placement is omitted when the installer passed no placement flags, so
	// the printed manifest stays what it was before the flags existed.
	Placement *v1alpha1.Placement `json:"placement,omitempty"`
}

// moduleDeployAppName is the name of the Application the deploy command
// creates. The "-deploy" suffix keeps it distinct from the owned Application:
// the two still collide when a module is deployed into vela-system, which is
// where every owned Application lives.
func moduleDeployAppName(moduleName string) string {
	return "module-" + moduleName + "-deploy"
}

// ownedModuleAppName is the name the render service gives the Application it
// renders for a module, mirroring RenderApplication in
// pkg/module/service/render.go. It carries no namespace because a module
// installs once cluster-wide.
func ownedModuleAppName(moduleName string) string {
	return "module-" + moduleName
}

// buildModuleApplication builds the one-component Application that installs a
// module. The registry name is the resolved one, not the raw flag, so the
// applied manifest records which registry was chosen. version selects the
// module package version; empty installs latest. place is the installer's
// cluster request; nil leaves the placement property off.
func buildModuleApplication(moduleName, registryName, namespace, version string, place *v1alpha1.Placement) (*v1beta1.Application, error) {
	props, err := json.Marshal(moduleComponentProperties{
		Module:    moduleName,
		Registry:  registryName,
		Version:   version,
		Placement: place,
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

	moduleDeployClustersFlag        = "clusters"
	moduleDeployClusterSelectorFlag = "cluster-selector"
	moduleDeployAllowEmptyFlag      = "allow-empty"

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
	version string
	// clusters, clusterSelector and allowEmpty become the placement property
	// on the module component. Runtime objects follow it; hub objects and
	// definitions stay on the local cluster.
	clusters        []string
	clusterSelector map[string]string
	allowEmpty      bool
	timeout         time.Duration
	pollInterval    time.Duration
	fetch           func(ctx context.Context, registry, moduleName, version string) (*pkgmodule.Module, error)
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
	vela module deploy s3 --dry-run > s3-module.yaml

  Put the module's runtime objects on two managed clusters:
	vela module deploy s3 --clusters spoke-a,spoke-b

  Put them on every cluster with a label, even when none match yet:
	vela module deploy s3 --cluster-selector tier=runtime --allow-empty`,
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
	cmd.Flags().StringVar(&o.version, moduleDeployVersionFlag, "", "The module package version (OCI/ECR tag) to install. Empty installs the latest published version.")
	cmd.Flags().BoolVar(&o.dryRun, moduleDeployDryRunFlag, false, "Print the Application without applying it.")
	cmd.Flags().StringSliceVar(&o.clusters, moduleDeployClustersFlag, nil, "The clusters to put the module's runtime objects on. Hub objects and definitions stay on the local cluster.")
	cmd.Flags().StringToStringVar(&o.clusterSelector, moduleDeployClusterSelectorFlag, nil, "Put the module's runtime objects on the clusters with these labels, as k=v[,k=v]. Cannot be used with --clusters.")
	cmd.Flags().BoolVar(&o.allowEmpty, moduleDeployAllowEmptyFlag, false, "Do not fail when --clusters or --cluster-selector selects no cluster.")
	cmd.Flags().DurationVar(&o.timeout, moduleDeployTimeoutFlag, defaultModuleDeployTimeout, "How long to wait for the module to become healthy.")
	addNamespaceAndEnvArg(cmd)
	cmd.SetOut(ioStreams.Out)
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
	place, err := o.placement()
	if err != nil {
		return err
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

	app, err := buildModuleApplication(o.module, reg.Name, o.namespace, o.version, place)
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

// placement builds the placement property from the flags. It returns nil when
// no placement flag is set. It reports flag misuse by flag name, then applies
// the same check the module provider applies to a component property.
func (o *moduleDeployOptions) placement() (*v1alpha1.Placement, error) {
	hasClusters, hasSelector := len(o.clusters) > 0, len(o.clusterSelector) > 0
	switch {
	case hasClusters && hasSelector:
		return nil, fmt.Errorf("--%s and --%s cannot be used together", moduleDeployClustersFlag, moduleDeployClusterSelectorFlag)
	case !hasClusters && !hasSelector:
		if o.allowEmpty {
			return nil, fmt.Errorf("--%s needs --%s or --%s", moduleDeployAllowEmptyFlag, moduleDeployClustersFlag, moduleDeployClusterSelectorFlag)
		}
		return nil, nil
	}
	place := &v1alpha1.Placement{AllowEmpty: o.allowEmpty}
	if hasClusters {
		place.Clusters = o.clusters
	} else {
		place.ClusterLabelSelector = o.clusterSelector
	}
	if err := placement.Validate(place); err != nil {
		return nil, err
	}
	return place, nil
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
			return fmt.Errorf("application %s/%s is in phase %s: %s",
				o.namespace, deployApp.Name, deployApp.Status.Phase, moduleComponentMessage(&deployApp))
		default:
			// Every remaining phase is a stage the Application is still moving
			// through (rendering, generating policies, running the workflow), so
			// the poll loop below keeps waiting rather than deciding here.
		}

		var tiers []string
		var services []oamcommon.ApplicationComponentStatus
		var ownedApp v1beta1.Application
		ownedSettled := false
		ownedCurrent := false
		// The owned Application lives in the system namespace whatever namespace
		// the definitions install into, so it is read from there, not o.namespace.
		ownedName := ownedModuleAppName(o.module)
		err := cli.Get(ctx, types.NamespacedName{Name: ownedName, Namespace: velatypes.DefaultKubeVelaNS}, &ownedApp)
		switch {
		case apierrors.IsNotFound(err):
			// The render service has not created the owned Application yet, so
			// the tier shape is not known.
		case err != nil:
			return fmt.Errorf("failed to read Application %s/%s: %w", velatypes.DefaultKubeVelaNS, ownedName, err)
		default:
			tiers = moduleTierNames(&ownedApp)
			services = ownedApp.Status.Services
			ownedCurrent = ownedAppCurrent(&deployApp, &ownedApp)
			ownedSettled = ownedCurrent && appSettled(&ownedApp)
			if ownedCurrent {
				if err := terminalOwnedPhase(&ownedApp); err != nil {
					return err
				}
			}
		}

		table := "Rendering module (install tiers are not known yet)..."
		if len(tiers) > 0 {
			table = renderModuleTierTable(tiers, services, ownedSettled)
			if step, msg := failedStep(&ownedApp); step != "" && ownedCurrent {
				table += fmt.Sprintf("\nStep %s failed: %s", step, msg)
			}
		}
		if table != lastTable {
			fmt.Fprintln(out, table)
			lastTable = table
		}

		tier, message := firstUnhealthyTier(tiers, services, ownedSettled)
		if tier == "" && len(tiers) > 0 && appSettled(&deployApp) && ownedSettled {
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
					o.timeout, o.module, velatypes.DefaultKubeVelaNS, ownedName, deployApp.Status.Phase, moduleComponentMessage(&deployApp))
			}
			if step, msg := failedStep(&ownedApp); step != "" && ownedCurrent {
				return fmt.Errorf("timed out after %s waiting for module %q: Application %s/%s step %s failed: %s",
					o.timeout, o.module, velatypes.DefaultKubeVelaNS, ownedName, step, msg)
			}
			if tier == "" {
				return fmt.Errorf("timed out after %s waiting for module %q: every tier is healthy but Application %s/%s is in phase %s and Application %s/%s is in phase %s",
					o.timeout, o.module, o.namespace, deployApp.Name, deployApp.Status.Phase,
					velatypes.DefaultKubeVelaNS, ownedName, ownedApp.Status.Phase)
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

// appSettled reports whether the controller has reconciled app's latest spec and
// finished its workflow. A status from before the latest change does not count,
// so a redeploy waits for the new spec instead of reading the old result.
func appSettled(app *v1beta1.Application) bool {
	return app.Status.Phase == oamcommon.ApplicationRunning && app.Status.ObservedGeneration == app.Generation
}

// ownedAppCurrent reports whether the owned Application was written by the
// deploy Application's latest spec. Until then it still carries the result of
// the previous request, so neither its tiers nor its failures describe this
// deploy. The deploy Application's own phase is not used: the module
// component's health is the owned Application's health, so the deploy
// Application never reaches running while the owned workflow is failing.
func ownedAppCurrent(deployApp, ownedApp *v1beta1.Application) bool {
	rev := deployApp.Status.LatestRevision
	if rev == nil || rev.Name == "" || deployApp.Status.ObservedGeneration != deployApp.Generation {
		return false
	}
	switch deployApp.Status.Phase {
	case oamcommon.ApplicationStarting, oamcommon.ApplicationRendering:
		// Only an error before the revision is finalized writes these phases,
		// so latestRevision may still name the previous request.
		return false
	default:
		return ownedApp.Labels[oam.LabelAppRevision] == rev.Name
	}
}

// failedStep returns the first failed workflow step on app and its message, or
// an empty name. A runtime step whose cluster selector matches nothing fails
// here while every hub tier is healthy. A failed step from before the latest
// change is stale, so nothing is returned until the controller has reconciled
// the current spec.
func failedStep(app *v1beta1.Application) (string, string) {
	if app.Status.Workflow == nil || app.Status.ObservedGeneration != app.Generation {
		return "", ""
	}
	for _, st := range app.Status.Workflow.Steps {
		if st.Phase == workflowv1alpha1.WorkflowStepPhaseFailed {
			return st.Name, st.Message
		}
	}
	return "", ""
}

// terminalOwnedPhase returns an error when the owned Application has stopped
// in a terminal phase for its current spec. A failed step that KubeVela still
// retries leaves the phase at runningWorkflow, so that case keeps waiting.
func terminalOwnedPhase(app *v1beta1.Application) error {
	if app.Status.ObservedGeneration != app.Generation {
		return nil
	}
	switch app.Status.Phase {
	case oamcommon.ApplicationWorkflowFailed, oamcommon.ApplicationWorkflowTerminated:
		reason := "no failed step reported"
		if step, msg := failedStep(app); step != "" {
			reason = fmt.Sprintf("step %s failed: %s", step, msg)
		}
		return fmt.Errorf("application %s/%s is in phase %s: %s", app.Namespace, app.Name, app.Status.Phase, reason)
	default:
		return nil
	}
}

// tierStatuses folds the owned Application's services into one status per
// tier. A tier placed on several clusters reports once per cluster, and it is
// healthy only when every cluster reports it healthy.
func tierStatuses(services []oamcommon.ApplicationComponentStatus) map[string]oamcommon.ApplicationComponentStatus {
	byName := make(map[string]oamcommon.ApplicationComponentStatus, len(services))
	for _, svc := range services {
		if prev, seen := byName[svc.Name]; seen && !prev.Healthy {
			continue
		}
		byName[svc.Name] = svc
	}
	return byName
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
// sees the whole install shape from the first poll. Once the owned Application
// has settled, an unreported tier went to no cluster and is Not placed.
func renderModuleTierTable(tiers []string, services []oamcommon.ApplicationComponentStatus, settled bool) string {
	byName := tierStatuses(services)
	table := uitable.New()
	table.AddRow("TIER", "STATUS", "MESSAGE")
	for _, tier := range tiers {
		svc, reported := byName[tier]
		switch {
		case !reported && settled:
			table.AddRow(tier, "Not placed", "no cluster selected")
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
// order, so the tier named is the one the install is actually stuck on. When
// settled is set, an unreported tier went to no cluster and is skipped.
func firstUnhealthyTier(tiers []string, services []oamcommon.ApplicationComponentStatus, settled bool) (string, string) {
	byName := tierStatuses(services)
	for _, tier := range tiers {
		svc, reported := byName[tier]
		switch {
		case !reported && settled:
			continue
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
