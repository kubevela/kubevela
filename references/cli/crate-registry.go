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
	"net/url"
	"sort"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	pkgcrate "github.com/oam-dev/kubevela/pkg/crate"
	pkgcomponent "github.com/oam-dev/kubevela/pkg/registry/component"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	crateRegistryTypeFlag          = "type"
	crateRegistryUsernameFlag      = "username"
	crateRegistryPasswordFlag      = "password"
	crateRegistryPasswordStdinFlag = "password-stdin"

	crateOCIType = "oci"
)

// NewCrateCommand returns the vela crate command group. The publish and deploy
// subcommands attach to this same group.
func NewCrateCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "crate",
		Short: "Manage KubeVela crates.",
		Long:  "Manage KubeVela crates and the registries they are published to and fetched from.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeExtension,
		},
	}
	cmd.AddCommand(NewCrateRegistryCommand(c, ioStreams), NewCratePublishCommand(c, ioStreams), NewCrateDeployCommand(c, ioStreams), NewCrateInitCommand(c, ioStreams))
	return cmd
}

// NewCrateRegistryCommand returns the vela crate registry command group.
func NewCrateRegistryCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage crate registries.",
		Long:  "Manage the OCI registries that crates are published to and fetched from.",
	}
	cmd.AddCommand(
		NewAddCrateRegistryCommand(c, ioStreams),
		NewUpdateCrateRegistryCommand(c, ioStreams),
		NewListCrateRegistryCommand(c, ioStreams),
		NewGetCrateRegistryCommand(c, ioStreams),
		NewDeleteCrateRegistryCommand(c, ioStreams),
	)
	return cmd
}

// NewAddCrateRegistryCommand returns the vela crate registry add command.
func NewAddCrateRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add NAME OCI_URL",
		Short: "Add a crate registry.",
		Long:  "Add a named OCI registry that crates are published to and fetched from.",
		Example: `  Add an OCI registry:
	vela crate registry add catalog oci://ghcr.io/kubevela/crates

  Add an OCI registry, reading the password from stdin:
	printf '%s' "$PASSWORD" | vela crate registry add ghcr oci://ghcr.io/org/crates --username robot --password-stdin

  Re-adding an existing name overwrites it in place, same as vela addon registry add:
	vela crate registry add catalog oci://ghcr.io/org/fork`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setCrateRegistryPasswordFromStdin(cmd); err != nil {
				return err
			}
			registry, err := crateRegistryFromArgs(cmd, args)
			if err != nil {
				return err
			}
			return addCrateRegistry(cmd.Context(), c, *registry, cmd.OutOrStdout())
		},
	}
	addCrateRegistryFlags(cmd)
	return cmd
}

// NewUpdateCrateRegistryCommand returns the vela crate registry update command.
func NewUpdateCrateRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update NAME OCI_URL",
		Short: "Update a crate registry.",
		Long:  "Update an existing OCI crate registry. Unlike \"add\", this fails if the registry does not already exist.",
		Example: `  Update a registry's URL:
	vela crate registry update catalog oci://ghcr.io/org/fork

  Update an OCI registry's credentials, reading the password from stdin:
	printf '%s' "$PASSWORD" | vela crate registry update ghcr oci://ghcr.io/org/crates --username robot --password-stdin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setCrateRegistryPasswordFromStdin(cmd); err != nil {
				return err
			}
			registry, err := crateRegistryFromArgs(cmd, args)
			if err != nil {
				return err
			}
			return updateCrateRegistry(cmd.Context(), c, *registry, cmd.OutOrStdout())
		},
	}
	addCrateRegistryFlags(cmd)
	return cmd
}

// addCrateRegistryFlags registers the flags the add command accepts.
func addCrateRegistryFlags(cmd *cobra.Command) {
	cmd.Flags().String(crateRegistryTypeFlag, "", "registry type; only \"oci\" is supported, and it is inferred from the URL when omitted")
	cmd.Flags().String(crateRegistryUsernameFlag, "", "username for an OCI registry")
	cmd.Flags().String(crateRegistryPasswordFlag, "", "password for an OCI registry")
	cmd.Flags().Bool(crateRegistryPasswordStdinFlag, false, "read the OCI registry password from stdin")
}

func setCrateRegistryPasswordFromStdin(cmd *cobra.Command) error {
	passwordStdin, err := cmd.Flags().GetBool(crateRegistryPasswordStdinFlag)
	if err != nil {
		return err
	}
	if !passwordStdin {
		return nil
	}
	value, err := readPasswordFromStdin(cmd, cmd.Flags().Changed(crateRegistryPasswordFlag), "registry")
	if err != nil {
		return err
	}
	return cmd.Flags().Set(crateRegistryPasswordFlag, value)
}

// crateRegistryFromArgs builds a Registry from the positional name and URL plus
// the add command's flags.
func crateRegistryFromArgs(cmd *cobra.Command, args []string) (*pkgaddon.Registry, error) {
	if len(args) != 2 {
		return nil, errors.New("must specify the registry name and URL, for example: " +
			"vela crate registry add catalog oci://ghcr.io/kubevela/crates")
	}
	name, rawURL := args[0], args[1]
	if err := validateCrateRegistryName(name); err != nil {
		return nil, err
	}

	registryType, err := cmd.Flags().GetString(crateRegistryTypeFlag)
	if err != nil {
		return nil, err
	}
	if registryType == "" {
		if registryType, err = inferCrateRegistryType(rawURL); err != nil {
			return nil, err
		}
	}

	r := &pkgaddon.Registry{Name: name}
	switch strings.ToLower(registryType) {
	case crateOCIType:
		username, err := cmd.Flags().GetString(crateRegistryUsernameFlag)
		if err != nil {
			return nil, err
		}
		password, err := cmd.Flags().GetString(crateRegistryPasswordFlag)
		if err != nil {
			return nil, err
		}
		if (username == "") != (password == "") {
			return nil, errors.New("OCI registry username and password must be supplied together; omit both for anonymous access")
		}
		if strings.HasPrefix(strings.ToLower(rawURL), "http://") && password != "" {
			return nil, errors.New("credentials are not supported for http:// crate registries; use oci:// for authenticated registries")
		}
		// An OCI registry is stored as a Helm source carrying the oci:// scheme,
		// the same record `vela addon registry add --type oci` writes. Asserting
		// the scheme here keeps a mistyped URL from being stored as a plain helm
		// entry that ResolveRegistry would later reject as an unsupported source.
		if !pkgaddon.IsOCIURL(rawURL) && !strings.HasPrefix(strings.ToLower(rawURL), "http://") {
			return nil, errors.New("an OCI crate registry URL must use the oci:// scheme (or http:// for a registry served without TLS)")
		}
		if strings.HasPrefix(strings.ToLower(rawURL), "http://") {
			u, err := url.Parse(rawURL)
			if err != nil || u.Host == "" {
				return nil, errors.New("http:// crate registry URL must include a registry host (for example: http://127.0.0.1:5000/crates)")
			}
		}
		r.Helm = &pkgaddon.HelmSource{URL: rawURL, Username: username, Token: password}
	default:
		return nil, fmt.Errorf("unsupported registry type %q, must be %q", registryType, crateOCIType)
	}
	return r, nil
}

// validateCrateRegistryName rejects a name that cannot be used as a Kubernetes
// Secret name. The registry's token secret is named
// CrateRegistrySecretPrefix + name, so both the name itself and that derived
// name must be a lowercase RFC 1123 DNS subdomain -- IsDNS1123Subdomain alone
// allows up to 253 characters, but the derived name can then exceed the limit
// and fail later, at the API server, only once a token is supplied.
func validateCrateRegistryName(name string) error {
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("invalid registry name %q: %s; the name is used for the registry's "+
			"token Secret, so it must be a lowercase RFC 1123 DNS subdomain",
			name, strings.Join(errs, "; "))
	}
	secretName := pkgcrate.CrateRegistrySecretPrefix + name
	if errs := validation.IsDNS1123Subdomain(secretName); len(errs) > 0 {
		return fmt.Errorf("registry name %q is too long: its token Secret name %q is invalid: %s",
			name, secretName, strings.Join(errs, "; "))
	}
	return nil
}

// requireCrateRegistryName extracts the single positional registry name
// argument taken by get, update, and delete, rejecting an empty or
// whitespace-only value. Without this, an unset shell variable in ordinary
// scripting (e.g. `vela crate registry delete "$REG"`) passes an empty
// string through the len(args) != 1 check alone, and ResolveRegistry's
// default-resolution rules then treat the empty name as "no name given" and
// silently substitute the default registry.
func requireCrateRegistryName(args []string) (string, error) {
	if len(args) != 1 {
		return "", errors.New("must specify the registry name")
	}
	name := strings.TrimSpace(args[0])
	if name == "" {
		return "", errors.New("registry name must not be empty")
	}
	return name, nil
}

// inferCrateRegistryType reads the registry type off the URL. OCI is the only
// type crates support, so the work here is recognising the spellings that mean
// something else and saying so.
//
// The scheme is decisive and is checked first: a repository path can end in
// .git legitimately -- mirroring a repository keeps its name -- so
// oci://host/mirrors/catalog.git is an OCI reference, and the .git heuristic
// only applies where no scheme has answered. http:// is OCI too, a registry
// served without TLS. A schemeless reference is refused rather than guessed,
// since crateRegistryFromArgs requires one of those schemes anyway.
func inferCrateRegistryType(rawURL string) (string, error) {
	lower := strings.ToLower(rawURL)
	switch {
	case strings.HasPrefix(lower, "oci://"), strings.HasPrefix(lower, "http://"):
		return crateOCIType, nil
	case strings.HasPrefix(lower, "https://"):
		// Do not assert this is a chart repository. Now that https:// no longer
		// means git, the likeliest reason to type it is an OCI registry with
		// the wrong scheme, and telling the author their URL is something it
		// plainly is not sends them the wrong way. Name both and show the
		// rewrite.
		return "", fmt.Errorf("crates cannot read a registry over https; %q is either a Helm chart repository, "+
			"which crates do not support, or an OCI registry addressed with the wrong scheme -- if it is the "+
			"latter, use %q", rawURL, "oci://"+rawURL[len("https://"):])
	case strings.HasSuffix(lower, ".git"):
		return "", fmt.Errorf("%q names a git repository, and git registries are not supported for crates; %s",
			rawURL, pkgcomponent.CrateGitRemedy)
	default:
		return "", fmt.Errorf("cannot infer the registry type from %q; an OCI crate registry URL must use the oci:// scheme (or http:// for a registry served without TLS)", rawURL)
	}
}

// addCrateRegistry persists a registry, overwriting a duplicate name in
// place -- matching vela addon registry add, which never required a --force
// flag for this either. The store's own AddRegistry already upserts (see
// component/registry.go), so there is no existence check to gate here; the
// only reason to look up the existing entry first is to preserve its token
// when the new payload does not carry one.
func addCrateRegistry(ctx context.Context, c common.Args, registry pkgaddon.Registry, out io.Writer) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return err
	}
	store := pkgcrate.NewStore(k8sClient)

	switch existing, err := store.GetRegistry(ctx, registry.Name); {
	case err == nil:
		preserveTokenSecretRef(&registry, existing)
	case !apierrors.IsNotFound(err):
		return err
	}

	if err := store.AddRegistry(ctx, registry); err != nil {
		return err
	}
	fmt.Fprintf(out, "Successfully added crate registry %s\n", registry.Name)
	return nil
}

// updateCrateRegistry updates an existing registry, rejecting a name that is
// not already configured -- unlike add, there is nothing to create here.
func updateCrateRegistry(ctx context.Context, c common.Args, registry pkgaddon.Registry, out io.Writer) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return err
	}
	store := pkgcrate.NewStore(k8sClient)

	existing, err := store.GetRegistry(ctx, registry.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return pkgcrate.NotFoundError(ctx, store, registry.Name)
		}
		return err
	}
	preserveTokenSecretRef(&registry, existing)
	if err := store.UpdateRegistry(ctx, registry); err != nil {
		return err
	}
	if droppedCredentialAcrossOrigins(existing, registry) {
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name:      pkgcrate.CrateRegistrySecretPrefix + registry.Name,
			Namespace: types.DefaultKubeVelaNS,
		}}
		if err := k8sClient.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("updated registry but failed to remove stale credential secret %q: %w", secret.Name, err)
		}
	}
	fmt.Fprintf(out, "Successfully updated crate registry %s\n", registry.Name)
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
	if pkgcrate.SourceTypeName(*registry) != pkgcrate.SourceTypeName(existing) {
		return
	}
	// Keep a credential only when the target origin is unchanged. Reusing a
	// token across hosts can leak credentials to the new endpoint.
	if !sameRegistryOrigin(*registry, existing) {
		return
	}
	old := existing.GetTokenSource()
	if old == nil {
		return
	}
	if token := old.GetToken(); token != "" {
		if nextOCI := registry.OCIChartSource(); nextOCI != nil && nextOCI.Username == "" {
			if prevOCI := existing.OCIChartSource(); prevOCI != nil {
				nextOCI.Username = prevOCI.Username
			}
		}
		src.SetToken(token)
		return
	}
	if ref := old.GetTokenSecretRef(); ref != "" {
		src.SetTokenSecretRef(ref)
	}
}

func sameRegistryOrigin(next, prev pkgaddon.Registry) bool {
	n := next.OCIChartSource()
	p := prev.OCIChartSource()
	if n == nil || p == nil {
		return false
	}
	nh, _ := pkgcomponent.OCIRegistryLocation(n.URL)
	ph, _ := pkgcomponent.OCIRegistryLocation(p.URL)
	return strings.EqualFold(nh, ph)
}

func droppedCredentialAcrossOrigins(existing, updated pkgaddon.Registry) bool {
	if sameRegistryOrigin(updated, existing) {
		return false
	}
	oldSrc := existing.GetTokenSource()
	if oldSrc == nil {
		return false
	}
	oldHadCredential := oldSrc.GetToken() != "" || oldSrc.GetTokenSecretRef() != ""
	if !oldHadCredential {
		return false
	}
	newSrc := updated.GetTokenSource()
	if newSrc == nil {
		return false
	}
	return newSrc.GetToken() == "" && newSrc.GetTokenSecretRef() == ""
}

// NewListCrateRegistryCommand returns the vela crate registry list command.
func NewListCrateRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List crate registries.",
		Long:    "List every configured crate registry.",
		Example: "vela crate registry list",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listCrateRegistry(cmd.Context(), c, cmd.OutOrStdout())
		},
	}
}

// NewGetCrateRegistryCommand returns the vela crate registry get command.
func NewGetCrateRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "get NAME",
		Short:   "Get a crate registry.",
		Long:    "Show one crate registry by name.",
		Example: "vela crate registry get catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := requireCrateRegistryName(args)
			if err != nil {
				return err
			}
			return getCrateRegistry(cmd.Context(), c, name, cmd.OutOrStdout())
		},
	}
}

// NewDeleteCrateRegistryCommand returns the vela crate registry delete command.
func NewDeleteCrateRegistryCommand(c common.Args, _ cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "delete NAME",
		Short:   "Delete a crate registry.",
		Long:    "Remove one crate registry and its token secret.",
		Example: "vela crate registry delete catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := requireCrateRegistryName(args)
			if err != nil {
				return err
			}
			return deleteCrateRegistry(cmd.Context(), c, name, cmd.OutOrStdout())
		},
	}
}

// listCrateRegistry prints every configured crate registry. An absent ConfigMap
// yields an empty table rather than an error.
func listCrateRegistry(ctx context.Context, c common.Args, out io.Writer) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return err
	}
	registries, err := pkgcrate.NewStore(k8sClient).ListRegistries(ctx)
	if err != nil {
		return err
	}
	// ListRegistries iterates a map, so sort for stable output.
	sort.Slice(registries, func(i, j int) bool { return registries[i].Name < registries[j].Name })

	table := uitable.New()
	table.AddRow("NAME", "TYPE", "URL", "PATH")
	for _, registry := range registries {
		table.AddRow(registry.Name, pkgcrate.SourceTypeName(registry),
			crateRegistrySourceURL(registry), crateRegistrySourcePath(registry))
	}
	fmt.Fprintln(out, table.String())
	return nil
}

// crateRegistrySourceURL renders a display URL for any source the shared
// addon ConfigMap format can hold. Crates only support OCI, but list must
// still show a git, helm, OSS, gitee, or gitlab entry -- an operator has to be
// able to see a bad entry in order to remove it.
func crateRegistrySourceURL(registry pkgaddon.Registry) string {
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

// crateRegistrySourcePath renders the subpath for the source kinds that have
// one; git, gitee, gitlab, and OSS all carry a Path field, OCI and helm do not.
// Crates cannot resolve the Path-carrying kinds any more, but list still
// renders them so an operator can recognise an entry that needs removing.
func crateRegistrySourcePath(registry pkgaddon.Registry) string {
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

// getCrateRegistry prints one registry. Resolution goes through ResolveRegistry so
// an unknown name produces the same error, listing what is configured, as the
// other verbs.
func getCrateRegistry(ctx context.Context, c common.Args, name string, out io.Writer) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return err
	}
	registry, err := pkgcrate.ResolveRegistry(ctx, pkgcrate.NewStore(k8sClient), name)
	if err != nil {
		return err
	}
	table := uitable.New()
	switch {
	case registry.OCIChartSource() != nil:
		oci := registry.OCIChartSource()
		table.AddRow("NAME", "TYPE", "URL", "USERNAME")
		table.AddRow(registry.Name, crateOCIType, oci.URL, oci.Username)
	default:
		table.AddRow("NAME")
		table.AddRow(registry.Name)
	}
	fmt.Fprintln(out, table.String())
	return nil
}

// deleteCrateRegistry removes a registry and its token secret. Existence is
// checked with a plain store.GetRegistry rather than the strict
// ResolveRegistry, so an entry ResolveRegistry would reject as unsupported
// (git, helm, OSS, gitee, gitlab) can still be found and removed -- cleanup must
// always work, even for a bad entry. This also sidesteps ResolveRegistry's
// default-resolution rules for an empty name; delete removes the entry's own
// resolved name rather than whatever raw string was passed in, so the two
// cannot diverge.
func deleteCrateRegistry(ctx context.Context, c common.Args, name string, out io.Writer) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return err
	}
	store := pkgcrate.NewStore(k8sClient)
	existing, err := store.GetRegistry(ctx, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return pkgcrate.NotFoundError(ctx, store, name)
		}
		return err
	}
	if err := store.DeleteRegistry(ctx, existing.Name); err != nil {
		return err
	}
	fmt.Fprintf(out, "Successfully deleted crate registry %s\n", existing.Name)
	return nil
}
