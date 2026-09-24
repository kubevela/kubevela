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
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	pkgcomponent "github.com/oam-dev/kubevela/pkg/registry/component"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	moduleRegistryTypeFlag          = "type"
	moduleRegistryUsernameFlag      = "username"
	moduleRegistryPasswordFlag      = "password"
	moduleRegistryPasswordStdinFlag = "password-stdin"

	moduleOCIType = "oci"
)

// NewModuleCommand returns the vela module command group. The publish and deploy
// subcommands attach to this same group.
func NewModuleCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "module",
		Short: "Manage KubeVela modules.",
		Long:  "Manage KubeVela modules and the registries they are published to and fetched from.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeExtension,
		},
	}
	cmd.AddCommand(NewModuleRegistryCommand(c, ioStreams), NewModulePublishCommand(c, ioStreams), NewModuleDeployCommand(c, ioStreams), NewModuleInitCommand(c, ioStreams))
	return cmd
}

// NewModuleRegistryCommand returns the vela module registry command group.
func NewModuleRegistryCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage module registries.",
		Long:  "Manage the OCI registries that modules are published to and fetched from.",
	}
	cmd.AddCommand(
		NewAddModuleRegistryCommand(c, ioStreams),
		NewUpdateModuleRegistryCommand(c, ioStreams),
		NewListModuleRegistryCommand(c, ioStreams),
		NewGetModuleRegistryCommand(c, ioStreams),
		NewDeleteModuleRegistryCommand(c, ioStreams),
	)
	return cmd
}

// NewAddModuleRegistryCommand returns the vela module registry add command.
func NewAddModuleRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a module registry.",
		Long:  "Add a named OCI registry that modules are published to and fetched from.",
		Example: `  Add an OCI registry:
	vela module registry add catalog oci://ghcr.io/kubevela/modules

  Add an OCI registry, reading the password from stdin:
	printf '%s' "$PASSWORD" | vela module registry add ghcr oci://ghcr.io/org/modules --username robot --password-stdin

  Re-adding an existing name overwrites it in place, same as vela addon registry add:
	vela module registry add catalog oci://ghcr.io/org/fork`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setRegistryPasswordFromStdin(cmd); err != nil {
				return err
			}
			registry, err := moduleRegistryFromArgs(cmd, args)
			if err != nil {
				return err
			}
			return addModuleRegistry(cmd.Context(), c, *registry, cmd.OutOrStdout())
		},
	}
	addModuleRegistryFlags(cmd)
	return cmd
}

// NewUpdateModuleRegistryCommand returns the vela module registry update command.
func NewUpdateModuleRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a module registry.",
		Long:  "Update an existing OCI module registry. Unlike \"add\", this fails if the registry does not already exist.",
		Example: `  Update a registry's URL:
	vela module registry update catalog oci://ghcr.io/org/fork

  Update an OCI registry's credentials, reading the password from stdin:
	printf '%s' "$PASSWORD" | vela module registry update ghcr oci://ghcr.io/org/modules --username robot --password-stdin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setRegistryPasswordFromStdin(cmd); err != nil {
				return err
			}
			registry, err := moduleRegistryFromArgs(cmd, args)
			if err != nil {
				return err
			}
			return updateModuleRegistry(cmd.Context(), c, *registry, cmd.OutOrStdout())
		},
	}
	addModuleRegistryFlags(cmd)
	return cmd
}

// addModuleRegistryFlags registers the flags the add command accepts.
func addModuleRegistryFlags(cmd *cobra.Command) {
	cmd.Flags().String(moduleRegistryTypeFlag, "", "registry type; only \"oci\" is supported, and it is inferred from the URL when omitted")
	cmd.Flags().String(moduleRegistryUsernameFlag, "", "username for an OCI registry")
	cmd.Flags().String(moduleRegistryPasswordFlag, "", "password for an OCI registry")
	cmd.Flags().Bool(moduleRegistryPasswordStdinFlag, false, "read the OCI registry password from stdin")
}

// moduleRegistryFromArgs builds a Registry from the positional name and URL plus
// the add command's flags.
func moduleRegistryFromArgs(cmd *cobra.Command, args []string) (*pkgaddon.Registry, error) {
	if len(args) != 2 {
		return nil, errors.New("must specify the registry name and URL, for example: " +
			"vela module registry add catalog oci://ghcr.io/kubevela/modules")
	}
	name, rawURL := args[0], args[1]
	if err := validateModuleRegistryName(name); err != nil {
		return nil, err
	}

	registryType, err := cmd.Flags().GetString(moduleRegistryTypeFlag)
	if err != nil {
		return nil, err
	}
	if registryType == "" {
		if registryType, err = inferModuleRegistryType(rawURL); err != nil {
			return nil, err
		}
	}

	r := &pkgaddon.Registry{Name: name}
	switch strings.ToLower(registryType) {
	case moduleOCIType:
		username, err := cmd.Flags().GetString(moduleRegistryUsernameFlag)
		if err != nil {
			return nil, err
		}
		password, err := cmd.Flags().GetString(moduleRegistryPasswordFlag)
		if err != nil {
			return nil, err
		}
		if (username == "") != (password == "") {
			return nil, errors.New("OCI registry username and password must be supplied together; omit both for anonymous access")
		}
		// An OCI registry is stored as a Helm source carrying the oci:// scheme,
		// the same record `vela addon registry add --type oci` writes. Asserting
		// the scheme here keeps a mistyped URL from being stored as a plain helm
		// entry that ResolveRegistry would later reject as an unsupported source.
		if !pkgaddon.IsOCIURL(rawURL) && !strings.HasPrefix(strings.ToLower(rawURL), "http://") {
			return nil, errors.New("an OCI module registry URL must use the oci:// scheme (or http:// for a registry served without TLS)")
		}
		r.Helm = &pkgaddon.HelmSource{URL: rawURL, Username: username, Token: password}
	default:
		return nil, fmt.Errorf("unsupported registry type %q, must be %q", registryType, moduleOCIType)
	}
	return r, nil
}

// validateModuleRegistryName rejects a name that cannot be used as a Kubernetes
// Secret name. The registry's token secret is named
// ModuleRegistrySecretPrefix + name, so both the name itself and that derived
// name must be a lowercase RFC 1123 DNS subdomain -- IsDNS1123Subdomain alone
// allows up to 253 characters, but the derived name can then exceed the limit
// and fail later, at the API server, only once a token is supplied.
func validateModuleRegistryName(name string) error {
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("invalid registry name %q: %s; the name is used for the registry's "+
			"token Secret, so it must be a lowercase RFC 1123 DNS subdomain",
			name, strings.Join(errs, "; "))
	}
	secretName := pkgmodule.ModuleRegistrySecretPrefix + name
	if errs := validation.IsDNS1123Subdomain(secretName); len(errs) > 0 {
		return fmt.Errorf("registry name %q is too long: its token Secret name %q is invalid: %s",
			name, secretName, strings.Join(errs, "; "))
	}
	return nil
}

// requireModuleRegistryName extracts the single positional registry name
// argument taken by get, update, and delete, rejecting an empty or
// whitespace-only value. Without this, an unset shell variable in ordinary
// scripting (e.g. `vela module registry delete "$REG"`) passes an empty
// string through the len(args) != 1 check alone, and ResolveRegistry's
// default-resolution rules then treat the empty name as "no name given" and
// silently substitute the default registry.
func requireModuleRegistryName(args []string) (string, error) {
	if len(args) != 1 {
		return "", errors.New("must specify the registry name")
	}
	name := strings.TrimSpace(args[0])
	if name == "" {
		return "", errors.New("registry name must not be empty")
	}
	return name, nil
}

// inferModuleRegistryType reads the registry type off the URL. OCI is the only
// type modules support, so the work here is recognising the spellings that mean
// something else and saying so.
//
// The scheme is decisive and is checked first: a repository path can end in
// .git legitimately -- mirroring a repository keeps its name -- so
// oci://host/mirrors/catalog.git is an OCI reference, and the .git heuristic
// only applies where no scheme has answered. http:// is OCI too, a registry
// served without TLS. A schemeless reference is refused rather than guessed,
// since moduleRegistryFromArgs requires one of those schemes anyway.
func inferModuleRegistryType(rawURL string) (string, error) {
	lower := strings.ToLower(rawURL)
	switch {
	case strings.HasPrefix(lower, "oci://"), strings.HasPrefix(lower, "http://"):
		return moduleOCIType, nil
	case strings.HasPrefix(lower, "https://"):
		// Do not assert this is a chart repository. Now that https:// no longer
		// means git, the likeliest reason to type it is an OCI registry with
		// the wrong scheme, and telling the author their URL is something it
		// plainly is not sends them the wrong way. Name both and show the
		// rewrite.
		return "", fmt.Errorf("modules cannot read a registry over https; %q is either a Helm chart repository, "+
			"which modules do not support, or an OCI registry addressed with the wrong scheme -- if it is the "+
			"latter, use %q", rawURL, "oci://"+rawURL[len("https://"):])
	case strings.HasSuffix(lower, ".git"):
		return "", fmt.Errorf("%q names a git repository, and git registries are not supported for modules; %s",
			rawURL, pkgcomponent.ModuleGitRemedy)
	default:
		return "", fmt.Errorf("cannot infer the registry type from %q; an OCI module registry URL must use the oci:// scheme (or http:// for a registry served without TLS)", rawURL)
	}
}

// addModuleRegistry persists a registry, overwriting a duplicate name in
// place -- matching vela addon registry add, which never required a --force
// flag for this either. The store's own AddRegistry already upserts (see
// component/registry.go), so there is no existence check to gate here; the
// only reason to look up the existing entry first is to preserve its token
// when the new payload does not carry one.
func addModuleRegistry(ctx context.Context, c common.Args, registry pkgaddon.Registry, out io.Writer) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return err
	}
	store := pkgmodule.NewStore(k8sClient)

	switch existing, err := store.GetRegistry(ctx, registry.Name); {
	case err == nil:
		preserveTokenSecretRef(&registry, existing)
	case !apierrors.IsNotFound(err):
		return err
	}

	if err := store.AddRegistry(ctx, registry); err != nil {
		return err
	}
	fmt.Fprintf(out, "Successfully added module registry %s\n", registry.Name)
	return nil
}

// updateModuleRegistry updates an existing registry, rejecting a name that is
// not already configured -- unlike add, there is nothing to create here.
func updateModuleRegistry(ctx context.Context, c common.Args, registry pkgaddon.Registry, out io.Writer) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return err
	}
	store := pkgmodule.NewStore(k8sClient)

	existing, err := store.GetRegistry(ctx, registry.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return pkgmodule.NotFoundError(ctx, store, registry.Name)
		}
		return err
	}
	preserveTokenSecretRef(&registry, existing)
	if err := store.UpdateRegistry(ctx, registry); err != nil {
		return err
	}
	fmt.Fprintf(out, "Successfully updated module registry %s\n", registry.Name)
	return nil
}

// preserveTokenSecretRef carries the stored credential forward onto registry
// when this invocation supplied no new token. Both add's overwrite path and
// update hand the store a Registry built fresh from flags; if the invocation
// omitted --password, that source's Token is empty, so the store's
// own token handling (pkg/registry/component/registry.go) never migrates a
// token to a secret, and the entry would be rewritten with an empty
// TokenSecretRef -- silently dropping the credential and orphaning the
// secret it used to point to, since delete skips a secret it has no ref to.
//
// existing comes from store.GetRegistry, which already loaded a configured
// secret's value into its Token field -- and, as a side effect of
// HelmSource's SetToken, cleared TokenSecretRef in memory
// while doing so. So the credential to carry forward is existing's Token, not
// its TokenSecretRef: handing that Token to UpdateRegistry re-migrates it to
// the same secret name, which restores the ref. Only if the secret could not
// be loaded at all (e.g. deleted out of band, so Token is still empty but the
// ref was never cleared) is the stale ref itself carried forward, so the
// entry does not silently lose the pointer.
func preserveTokenSecretRef(registry *pkgaddon.Registry, existing pkgaddon.Registry) {
	src := registry.GetTokenSource()
	if src == nil || src.GetToken() != "" {
		return
	}
	// Only carry a credential between two entries of the same source kind. A
	// credential is scoped to the host and protocol it was issued for, so
	// carrying one across kinds would send the old kind's token to the new
	// kind's host -- leaking it, and breaking a registry that wanted none. An
	// operator changing the kind supplies the new credential, or no credential.
	if pkgmodule.SourceTypeName(*registry) != pkgmodule.SourceTypeName(existing) {
		return
	}
	old := existing.GetTokenSource()
	if old == nil {
		return
	}
	if token := old.GetToken(); token != "" {
		src.SetToken(token)
		return
	}
	if ref := old.GetTokenSecretRef(); ref != "" {
		src.SetTokenSecretRef(ref)
	}
}

// NewListModuleRegistryCommand returns the vela module registry list command.
func NewListModuleRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List module registries.",
		Long:    "List every configured module registry.",
		Example: "vela module registry list",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listModuleRegistry(cmd.Context(), c, cmd.OutOrStdout())
		},
	}
}

// NewGetModuleRegistryCommand returns the vela module registry get command.
func NewGetModuleRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "get",
		Short:   "Get a module registry.",
		Long:    "Show one module registry by name.",
		Example: "vela module registry get catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := requireModuleRegistryName(args)
			if err != nil {
				return err
			}
			return getModuleRegistry(cmd.Context(), c, name, cmd.OutOrStdout())
		},
	}
}

// NewDeleteModuleRegistryCommand returns the vela module registry delete command.
func NewDeleteModuleRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "delete",
		Short:   "Delete a module registry.",
		Long:    "Remove one module registry and its token secret.",
		Example: "vela module registry delete catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := requireModuleRegistryName(args)
			if err != nil {
				return err
			}
			return deleteModuleRegistry(cmd.Context(), c, name, cmd.OutOrStdout())
		},
	}
}

// listModuleRegistry prints every configured module registry. An absent ConfigMap
// yields an empty table rather than an error.
func listModuleRegistry(ctx context.Context, c common.Args, out io.Writer) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return err
	}
	registries, err := pkgmodule.NewStore(k8sClient).ListRegistries(ctx)
	if err != nil {
		return err
	}
	// ListRegistries iterates a map, so sort for stable output.
	sort.Slice(registries, func(i, j int) bool { return registries[i].Name < registries[j].Name })

	table := uitable.New()
	table.AddRow("NAME", "TYPE", "URL", "PATH")
	for _, registry := range registries {
		table.AddRow(registry.Name, pkgmodule.SourceTypeName(registry),
			moduleRegistrySourceURL(registry), moduleRegistrySourcePath(registry))
	}
	fmt.Fprintln(out, table.String())
	return nil
}

// moduleRegistrySourceURL renders a display URL for any source the shared
// addon ConfigMap format can hold. Modules only support OCI, but list must
// still show a git, helm, OSS, gitee, or gitlab entry -- an operator has to be
// able to see a bad entry in order to remove it.
func moduleRegistrySourceURL(registry pkgaddon.Registry) string {
	switch {
	case registry.Git != nil:
		return registry.Git.URL
	case registry.Helm != nil:
		// Covers both spellings: an oci:// Helm source and a plain helm entry
		// render the same URL field.
		return registry.Helm.URL
	case registry.OSS != nil:
		return registry.OSS.Endpoint
	case registry.Gitee != nil:
		return registry.Gitee.URL
	case registry.Gitlab != nil:
		return registry.Gitlab.URL
	default:
		return ""
	}
}

// moduleRegistrySourcePath renders the subpath for the source kinds that have
// one; git, gitee, gitlab, and OSS all carry a Path field, OCI and helm do not.
// Modules cannot resolve the Path-carrying kinds any more, but list still
// renders them so an operator can recognise an entry that needs removing.
func moduleRegistrySourcePath(registry pkgaddon.Registry) string {
	switch {
	case registry.Git != nil:
		return registry.Git.Path
	case registry.Gitee != nil:
		return registry.Gitee.Path
	case registry.Gitlab != nil:
		return registry.Gitlab.Path
	case registry.OSS != nil:
		return registry.OSS.Path
	default:
		return ""
	}
}

// getModuleRegistry prints one registry. Resolution goes through ResolveRegistry so
// an unknown name produces the same error, listing what is configured, as the
// other verbs.
func getModuleRegistry(ctx context.Context, c common.Args, name string, out io.Writer) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return err
	}
	registry, err := pkgmodule.ResolveRegistry(ctx, pkgmodule.NewStore(k8sClient), name)
	if err != nil {
		return err
	}
	table := uitable.New()
	switch {
	case registry.OCIChartSource() != nil:
		oci := registry.OCIChartSource()
		table.AddRow("NAME", "TYPE", "URL", "USERNAME")
		table.AddRow(registry.Name, moduleOCIType, oci.URL, oci.Username)
	default:
		table.AddRow("NAME")
		table.AddRow(registry.Name)
	}
	fmt.Fprintln(out, table.String())
	return nil
}

// deleteModuleRegistry removes a registry and its token secret. Existence is
// checked with a plain store.GetRegistry rather than the strict
// ResolveRegistry, so an entry ResolveRegistry would reject as unsupported
// (git, helm, OSS, gitee, gitlab) can still be found and removed -- cleanup must
// always work, even for a bad entry. This also sidesteps ResolveRegistry's
// default-resolution rules for an empty name; delete removes the entry's own
// resolved name rather than whatever raw string was passed in, so the two
// cannot diverge.
func deleteModuleRegistry(ctx context.Context, c common.Args, name string, out io.Writer) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return err
	}
	store := pkgmodule.NewStore(k8sClient)
	existing, err := store.GetRegistry(ctx, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return pkgmodule.NotFoundError(ctx, store, name)
		}
		return err
	}
	if err := store.DeleteRegistry(ctx, existing.Name); err != nil {
		return err
	}
	fmt.Fprintf(out, "Successfully deleted module registry %s\n", existing.Name)
	return nil
}
