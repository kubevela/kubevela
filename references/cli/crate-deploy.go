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
	pkgcrate "github.com/oam-dev/kubevela/pkg/crate"
	cratesvc "github.com/oam-dev/kubevela/pkg/crate/service"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/policy/placement"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// crateComponentType is the ComponentDefinition the deploy command builds a
// component of. Its template calls the crate render service, which fetches the
// crate and renders the owned Application holding the install tiers.
const crateComponentType = "crate"

// crateComponentProperties are the parameters of the type: crate component.
// It is a struct rather than a map so the rendered manifest has a stable field
// order.
type crateComponentProperties struct {
	Crate    string `json:"crate"`
	Registry string `json:"registry"`
	Version  string `json:"version"`
	// Placement is omitted when the installer passed no placement flags, so
	// the printed manifest stays what it was before the flags existed.
	Placement *v1alpha1.Placement `json:"placement,omitempty"`
}

// crateDeployAppName is the name of the Application the deploy command
// creates. The "-deploy" suffix keeps it distinct from the owned Application:
// the two still collide when a crate is deployed into vela-system, which is
// where every owned Application lives.
func crateDeployAppName(crateName string) string {
	return "crate-" + crateName + "-deploy"
}

// ownedCrateAppName is the name the render service gives the Application it
// renders for a crate, mirroring RenderApplication in
// pkg/crate/service/render.go. It carries no namespace because a crate
// installs once cluster-wide.
func ownedCrateAppName(crateName string) string {
	return "crate-" + crateName
}

// buildCrateApplication builds the one-component Application that installs a
// crate. The registry name is the resolved one, not the raw flag, so the
// applied manifest records which registry was chosen. version selects the
// crate package version; empty installs latest. place is the installer's
// cluster request; nil leaves the placement property off.
func buildCrateApplication(crateName, registryName, namespace, version string, place *v1alpha1.Placement) (*v1beta1.Application, error) {
	props, err := json.Marshal(crateComponentProperties{
		Crate:     crateName,
		Registry:  registryName,
		Version:   version,
		Placement: place,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode the crate component properties: %w", err)
	}
	return &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1beta1.SchemeGroupVersion.String(),
			Kind:       v1beta1.ApplicationKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      crateDeployAppName(crateName),
			Namespace: namespace,
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []oamcommon.ApplicationComponent{{
				Name:       crateName,
				Type:       crateComponentType,
				Properties: &runtime.RawExtension{Raw: props},
			}},
		},
	}, nil
}

// crateTierNames returns the install tier names the render service gave the
// owned Application's components, in spec order. Reading them from the
// Application itself.
func crateTierNames(app *v1beta1.Application) []string {
	names := make([]string, 0, len(app.Spec.Components))
	for _, c := range app.Spec.Components {
		names = append(names, c.Name)
	}
	return names
}

const (
	crateDeployRegistryFlag = "registry"
	crateDeployDryRunFlag   = "dry-run"
	crateDeployTimeoutFlag  = "timeout"
	crateDeployVersionFlag  = "version"

	crateDeployClustersFlag        = "clusters"
	crateDeployClusterSelectorFlag = "cluster-selector"
	crateDeployAllowEmptyFlag      = "allow-empty"

	// defaultCrateDeployTimeout is how long deploy waits for every tier to
	// become healthy before giving up.
	defaultCrateDeployTimeout = 5 * time.Minute
	// defaultCrateDeployPollInterval is how often deploy re-reads the two
	// Applications while waiting.
	defaultCrateDeployPollInterval = 2 * time.Second
)

// crateDeployOptions holds one run of the deploy command. fetch and
// pollInterval are seams: production wires the registry-backed fetch and the
// default interval, tests inject a stub and a millisecond interval.
type crateDeployOptions struct {
	crate     string
	registry  string
	namespace string
	dryRun    bool
	// version selects the crate package version to install; empty installs
	// the latest published version.
	version string
	// clusters, clusterSelector and allowEmpty become the placement property
	// on the crate component. Runtime objects follow it; hub objects and
	// definitions stay on the local cluster.
	clusters        []string
	clusterSelector map[string]string
	allowEmpty      bool
	timeout         time.Duration
	pollInterval    time.Duration
	fetch           func(ctx context.Context, registry, crateName, version string) (*pkgcrate.Crate, error)
}

// NewCrateDeployCommand returns the vela crate deploy command. It builds and
// applies an Application with a single type: crate component, then reports the
// install tiers as they become healthy.
func NewCrateDeployCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	o := &crateDeployOptions{}
	cmd := &cobra.Command{
		Use:   "deploy <crate>",
		Short: "Deploy a crate.",
		Long:  "Build and apply an Application that installs a crate's enabled API lines, then report the install tiers as they become healthy.",
		Example: `  Deploy a crate from the default registry:
	vela crate deploy s3

  Deploy from a named registry, into a namespace:
	vela crate deploy s3 --registry catalog -n platform

  Print the Application without applying it, capturing it for GitOps:
	vela crate deploy s3 --dry-run > s3-crate.yaml

  Put the crate's runtime objects on two managed clusters:
	vela crate deploy s3 --clusters spoke-a,spoke-b

  Put them on every cluster with a label, even when none match yet:
	vela crate deploy s3 --cluster-selector tier=runtime --allow-empty`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := c.GetClient()
			if err != nil {
				return err
			}
			o.crate = args[0]
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
	cmd.Flags().StringVar(&o.registry, crateDeployRegistryFlag, "", "The crate registry to deploy from. Empty means the configured default.")
	cmd.Flags().StringVar(&o.version, crateDeployVersionFlag, "", "The crate package version (OCI/ECR tag) to install. Empty resolves to the highest published tag when the installer runs; a newer tag does not upgrade the install, so set it on each install and upgrade.")
	cmd.Flags().BoolVar(&o.dryRun, crateDeployDryRunFlag, false, "Print the Application without applying it.")
	cmd.Flags().StringSliceVar(&o.clusters, crateDeployClustersFlag, nil, "The clusters to put the crate's runtime objects on. Hub objects and definitions stay on the local cluster.")
	cmd.Flags().StringToStringVar(&o.clusterSelector, crateDeployClusterSelectorFlag, nil, "Put the crate's runtime objects on the clusters with these labels, as k=v[,k=v]. Cannot be used with --clusters.")
	cmd.Flags().BoolVar(&o.allowEmpty, crateDeployAllowEmptyFlag, false, "Do not fail when --clusters or --cluster-selector selects no cluster.")
	cmd.Flags().DurationVar(&o.timeout, crateDeployTimeoutFlag, defaultCrateDeployTimeout, "How long to wait for the crate to become healthy.")
	addNamespaceAndEnvArg(cmd)
	cmd.SetOut(ioStreams.Out)
	return cmd
}

// run validates the registry and the crate, builds the Application, and either
// prints it or applies it and waits.
func (o *crateDeployOptions) run(ctx context.Context, cli client.Client, out io.Writer) error {
	if errs := validation.IsDNS1123Label(o.crate); len(errs) > 0 {
		return fmt.Errorf("invalid crate name %q: %s", o.crate, errs[0])
	}
	if o.namespace == "" {
		o.namespace = velatypes.DefaultKubeVelaNS
	}
	if o.pollInterval <= 0 {
		o.pollInterval = defaultCrateDeployPollInterval
	}
	if o.timeout <= 0 {
		o.timeout = defaultCrateDeployTimeout
	}
	place, err := o.placement()
	if err != nil {
		return err
	}

	store := pkgcrate.NewStore(cli)
	reg, err := pkgcrate.ResolveRegistry(ctx, store, o.registry)
	if err != nil {
		return err
	}

	fetch := o.fetch
	if fetch == nil {
		fetch = cratesvc.NewService(store).FetchCrate
	}
	if _, err := fetch(ctx, reg.Name, o.crate, o.version); err != nil {
		return err
	}

	app, err := buildCrateApplication(o.crate, reg.Name, o.namespace, o.version, place)
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

	return o.waitForCrate(ctx, cli, out)
}

// placement builds the placement property from the flags. It returns nil when
// no placement flag is set. It reports flag misuse by flag name, then applies
// the same check the crate provider applies to a component property.
func (o *crateDeployOptions) placement() (*v1alpha1.Placement, error) {
	hasClusters, hasSelector := len(o.clusters) > 0, len(o.clusterSelector) > 0
	switch {
	case hasClusters && hasSelector:
		return nil, fmt.Errorf("--%s and --%s cannot be used together", crateDeployClustersFlag, crateDeployClusterSelectorFlag)
	case !hasClusters && !hasSelector:
		if o.allowEmpty {
			return nil, fmt.Errorf("--%s needs --%s or --%s", crateDeployAllowEmptyFlag, crateDeployClustersFlag, crateDeployClusterSelectorFlag)
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

// waitForCrate polls the deploy Application and the owned crate Application
// until every tier is healthy, printing the tier table whenever it changes.
//
// It reads both Applications because they carry different halves of the answer:
// the deploy Application's phase is where a fetch or render failure surfaces,
// while the tier shape and per-tier health live only on the owned Application
// the render service creates.
func (o *crateDeployOptions) waitForCrate(ctx context.Context, cli client.Client, out io.Writer) error {
	deadline := time.Now().Add(o.timeout)
	lastTable := ""

	for {
		var deployApp v1beta1.Application
		if err := cli.Get(ctx, types.NamespacedName{Name: crateDeployAppName(o.crate), Namespace: o.namespace}, &deployApp); err != nil {
			return fmt.Errorf("failed to read Application %s/%s: %w", o.namespace, crateDeployAppName(o.crate), err)
		}
		switch deployApp.Status.Phase {
		case oamcommon.ApplicationWorkflowFailed, oamcommon.ApplicationWorkflowTerminated, oamcommon.ApplicationDeleting:
			return fmt.Errorf("application %s/%s is in phase %s: %s",
				o.namespace, deployApp.Name, deployApp.Status.Phase, crateComponentMessage(&deployApp))
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
		ownedName := ownedCrateAppName(o.crate)
		err := cli.Get(ctx, types.NamespacedName{Name: ownedName, Namespace: velatypes.DefaultKubeVelaNS}, &ownedApp)
		switch {
		case apierrors.IsNotFound(err):
			// The render service has not created the owned Application yet, so
			// the tier shape is not known.
		case err != nil:
			return fmt.Errorf("failed to read Application %s/%s: %w", velatypes.DefaultKubeVelaNS, ownedName, err)
		default:
			tiers = crateTierNames(&ownedApp)
			services = ownedApp.Status.Services
			ownedCurrent = ownedAppCurrent(&deployApp, &ownedApp)
			ownedSettled = ownedCurrent && appSettled(&ownedApp)
			if ownedCurrent {
				if err := terminalOwnedPhase(&ownedApp); err != nil {
					return err
				}
			}
		}

		table := "Rendering crate (install tiers are not known yet)..."
		if len(tiers) > 0 {
			table = renderCrateTierTable(tiers, services, ownedSettled)
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
			if resolved := ownedApp.Annotations[velatypes.AnnoDefinitionCrateVersion]; resolved != "" {
				fmt.Fprintf(out, "Crate %q (version %s) is installed in namespace %q\n", o.crate, resolved, o.namespace)
			} else {
				fmt.Fprintf(out, "Crate %q is installed in namespace %q\n", o.crate, o.namespace)
			}
			return nil
		}

		if time.Now().After(deadline) {
			if len(tiers) == 0 {
				return fmt.Errorf("timed out after %s waiting for crate %q: the owned Application %s/%s was not created; deploy Application is in phase %s: %s",
					o.timeout, o.crate, velatypes.DefaultKubeVelaNS, ownedName, deployApp.Status.Phase, crateComponentMessage(&deployApp))
			}
			if step, msg := failedStep(&ownedApp); step != "" && ownedCurrent {
				return fmt.Errorf("timed out after %s waiting for crate %q: Application %s/%s step %s failed: %s",
					o.timeout, o.crate, velatypes.DefaultKubeVelaNS, ownedName, step, msg)
			}
			if tier == "" {
				return fmt.Errorf("timed out after %s waiting for crate %q: every tier is healthy but Application %s/%s is in phase %s and Application %s/%s is in phase %s",
					o.timeout, o.crate, o.namespace, deployApp.Name, deployApp.Status.Phase,
					velatypes.DefaultKubeVelaNS, ownedName, ownedApp.Status.Phase)
			}
			return fmt.Errorf("timed out after %s waiting for crate %q: tier %q is not ready: %s",
				o.timeout, o.crate, tier, message)
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
// deploy. The deploy Application's own phase is not used: the crate
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

// crateComponentMessage returns the message of the deploy Application's crate
// component, which is where a server-side fetch or render error surfaces.
func crateComponentMessage(app *v1beta1.Application) string {
	for _, svc := range app.Status.Services {
		if svc.Message != "" {
			return svc.Message
		}
	}
	return "no component message reported"
}

// renderCrateTierTable renders every expected tier with its reported health. A
// tier the owned Application has not reported yet is Pending, so the operator
// sees the whole install shape from the first poll. Once the owned Application
// has settled, an unreported tier went to no cluster and is Not placed.
func renderCrateTierTable(tiers []string, services []oamcommon.ApplicationComponentStatus, settled bool) string {
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
