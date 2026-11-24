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
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	pkgcrate "github.com/oam-dev/kubevela/pkg/crate"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	cratePublishRegistryFlag = "registry"
	cratePublishVersionFlag  = "version"
	cratePublishForceFlag    = "force"
	cratePublishDryRunFlag   = "dry-run"
	cratePublishUsernameFlag = "username"
)

// cratePublishOptions holds the resolved inputs of vela crate publish. The
// push and tagExists seams are wired to pkg/addon in production and replaced
// in tests, so no test needs a registry.
type cratePublishOptions struct {
	// dir is the crate source tree to package and publish.
	dir string
	// ociRef is the positional OCI or ECR reference to publish to, taking
	// precedence over registry when set.
	ociRef string
	// registry is the name of a configured crate registry to publish to.
	// Empty means the configured default registry.
	registry string
	// version overrides the artifact tag. It never bypasses version
	// immutability: the existence check runs against this tag too.
	version string
	// username overrides the registry username when non-empty.
	username string
	// password overrides the registry password when non-empty.
	password string
	// force skips the tag-existence check and publishes unconditionally.
	force bool
	// dryRun prints the target reference and annotations without pushing.
	dryRun bool

	// push publishes the packaged archive. Wired to pkgaddon.PushOCIChart in
	// production.
	push func(ctx context.Context, reg pkgaddon.Registry, name, version string, archive []byte) error
	// tagExists reports whether a tag is already published. Wired to
	// pkgaddon.OCIChartTagExists in production.
	tagExists func(ctx context.Context, reg pkgaddon.Registry, name, tag string) (bool, error)
}

// NewCratePublishCommand returns the vela crate publish command.
func NewCratePublishCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	o := &cratePublishOptions{push: pkgaddon.PushOCIChart, tagExists: pkgaddon.OCIChartTagExists}
	cmd := &cobra.Command{
		Use:   "publish <dir> [oci-ref]",
		Short: "Publish a crate to an OCI or ECR registry.",
		Long:  "Validate a crate source tree and publish it as an OCI artifact tagged from its own version. A published version is immutable: change the crate, bump version in _crate.cue, publish again.",
		Example: `  Publish to the configured default registry:
	vela crate publish ./crates/s3

  Publish to a named registry:
	vela crate publish ./crates/s3 --registry ecr

  Publish straight to an ECR repository prefix:
	vela crate publish ./crates/s3 123456789012.dkr.ecr.us-west-2.amazonaws.com/crates

  Check the target and the annotations without pushing:
	vela crate publish ./crates/s3 --registry ecr --dry-run

  Republish a release candidate over itself while iterating:
	vela crate publish ./crates/s3 --registry ecr --version 1.1.0-rc1 --force`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setCratePublishPasswordFromStdin(cmd); err != nil {
				return err
			}
			o.dir = args[0]
			if len(args) == 2 {
				o.ociRef = args[1]
			}
			var err error
			if o.registry, err = cmd.Flags().GetString(cratePublishRegistryFlag); err != nil {
				return err
			}
			if o.version, err = cmd.Flags().GetString(cratePublishVersionFlag); err != nil {
				return err
			}
			if o.username, err = cmd.Flags().GetString(cratePublishUsernameFlag); err != nil {
				return err
			}
			if o.password, err = cmd.Flags().GetString(addonPassword); err != nil {
				return err
			}
			if o.force, err = cmd.Flags().GetBool(cratePublishForceFlag); err != nil {
				return err
			}
			if o.dryRun, err = cmd.Flags().GetBool(cratePublishDryRunFlag); err != nil {
				return err
			}

			var cli client.Client
			if o.ociRef == "" {
				if cli, err = c.GetClient(); err != nil {
					return err
				}
			}
			return o.run(cmd.Context(), cli, cmd.OutOrStdout())
		},
	}
	cmd.Flags().String(cratePublishRegistryFlag, "", "The configured crate registry to publish to. Empty means the configured default.")
	cmd.Flags().String(cratePublishVersionFlag, "", "Override the artifact tag. Must be semver, and does not bypass version immutability. The crate's own version in _crate.cue is unchanged inside the artifact.")
	cmd.Flags().Bool(cratePublishForceFlag, false, "Overwrite an already-published version.")
	cmd.Flags().Bool(cratePublishDryRunFlag, false, "Print the target reference, tag, and annotations without pushing.")
	cmd.Flags().String(cratePublishUsernameFlag, "", "Registry username. Empty uses the docker credential chain.")
	cmd.Flags().String(addonPassword, "", "Registry password. Empty uses the docker credential chain.")
	cmd.Flags().Bool(addonPasswordStdin, false, "Read the registry password from stdin.")
	cmd.SetIn(ioStreams.In)
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func setCratePublishPasswordFromStdin(cmd *cobra.Command) error {
	passwordStdin, err := cmd.Flags().GetBool(addonPasswordStdin)
	if err != nil {
		return err
	}
	if !passwordStdin {
		return nil
	}
	value, err := readPasswordFromStdin(cmd, cmd.Flags().Changed(addonPassword), "registry")
	if err != nil {
		return err
	}
	return cmd.Flags().Set(addonPassword, value)
}

// run validates the tree, resolves the target registry, and publishes.
// Nothing reaches a registry until the crate parses and the target is known
// to be OCI.
func (o *cratePublishOptions) run(ctx context.Context, cli client.Client, out io.Writer) error {
	if o.registry != "" && o.ociRef != "" {
		return fmt.Errorf("--%s cannot be combined with a positional OCI reference; pass one or the other", cratePublishRegistryFlag)
	}

	artifact, err := pkgcrate.PackageCrate(o.dir, o.version)
	if err != nil {
		return err
	}
	if artifact.Tag != artifact.Crate.Version {
		fmt.Fprintf(out, "Warning: publishing tag %s while %s/_crate.cue still declares version %s; the crate's declared version is unchanged inside the artifact\n",
			artifact.Tag, o.dir, artifact.Crate.Version)
	}
	for _, w := range artifact.Crate.Warnings {
		fmt.Fprintf(out, "Warning: %s\n", w)
	}

	reg, err := o.resolveTarget(ctx, cli)
	if err != nil {
		return err
	}

	ref, err := pkgaddon.OCIChartRef(reg, artifact.Crate.Name, artifact.Tag)
	if err != nil {
		return err
	}

	if o.dryRun {
		fmt.Fprintf(out, "Would publish %s\n", ref)
		for _, key := range sortedAnnotationKeys(artifact.Annotations) {
			fmt.Fprintf(out, "  %s: %s\n", key, artifact.Annotations[key])
		}
		return nil
	}

	if !o.force {
		exists, err := o.tagExists(ctx, reg, artifact.Crate.Name, artifact.Tag)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("%s is already published; bump version in %s/_crate.cue and publish again, or pass --%s to overwrite",
				ref, o.dir, cratePublishForceFlag)
		}
	}

	if err := o.push(ctx, reg, artifact.Crate.Name, artifact.Tag, artifact.Archive); err != nil {
		return publishError(ref, err)
	}
	fmt.Fprintf(out, "Published %s\n", ref)
	return nil
}

// resolveTarget returns the registry to publish to: the positional OCI
// reference when given, otherwise the named or default configured registry,
// which requires a cluster. A non-OCI registry is rejected here, before any
// network call.
func (o *cratePublishOptions) resolveTarget(ctx context.Context, cli client.Client) (pkgaddon.Registry, error) {
	if o.ociRef != "" {
		if (o.username == "") != (o.password == "") {
			return pkgaddon.Registry{}, fmt.Errorf("OCI registry username and password must be supplied together; omit both for anonymous access")
		}
		return pkgaddon.Registry{
			Name: o.ociRef,
			// A positional reference is by definition an OCI target, stored as
			// the Helm source every registry record now uses. It is kept exactly
			// as typed -- oci://, http:// for a registry without TLS, or a bare
			// ECR host -- all three of which OCIChartSource accepts.
			Helm: &pkgaddon.HelmSource{URL: o.ociRef, Username: o.username, Token: o.password},
		}, nil
	}
	if cli == nil {
		return pkgaddon.Registry{}, fmt.Errorf("publishing to a configured registry needs cluster access; pass an OCI reference to publish without a cluster")
	}
	reg, err := pkgcrate.ResolveRegistry(ctx, pkgcrate.NewStore(cli), o.registry)
	if err != nil {
		return pkgaddon.Registry{}, err
	}
	oci := reg.OCIChartSource()
	if oci == nil {
		return pkgaddon.Registry{}, fmt.Errorf("crate registry %q is a %s source; vela crate publish supports OCI/ECR only",
			reg.Name, pkgcrate.SourceTypeName(reg))
	}
	if o.username != "" {
		oci.Username = o.username
	}
	if o.password != "" {
		oci.Token = o.password
	}
	return reg, nil
}

// publishError turns a registry rejection into a message naming the fix. ECR
// creates no repository on push, and an IMMUTABLE repository refuses a tag
// move no matter what the client asks for.
func publishError(ref string, err error) error {
	switch {
	case pkgaddon.IsOCIRepositoryNotFound(err):
		return fmt.Errorf("the repository for %s does not exist; create it in the registry first (ECR does not create repositories on push): %w", ref, err)
	case pkgaddon.IsOCITagImmutable(err):
		return fmt.Errorf("%s cannot be overwritten because the repository rejects tag changes; bump version in _crate.cue and publish a new version: %w", ref, err)
	default:
		return fmt.Errorf("failed to publish %s: %w", ref, err)
	}
}

// sortedAnnotationKeys returns the annotation keys in sorted order so dry-run
// output is stable.
func sortedAnnotationKeys(annotations map[string]string) []string {
	keys := make([]string, 0, len(annotations))
	for key := range annotations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
