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

package helm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	sourceTypeOCI  = "oci"
	sourceTypeURL  = "url"
	sourceTypeRepo = "repo"
)

// secretRef identifies a Kubernetes Secret carrying chart-repository credentials.
type secretRef struct {
	Name      string
	Namespace string // empty -> resolver defaults to releaseNamespace
}

// authResolveOptions controls cross-namespace policy and host matching.
type authResolveOptions struct {
	AppNamespace     string // Application's own namespace
	ReleaseNamespace string // chart release's target namespace
	RegistryHost     string // for dockerconfigjson entry lookup
	SourceScheme     string // "https", "http", "oci"; drives RFC 6750 guards
	SourceType       string // sourceTypeOCI | sourceTypeURL | sourceTypeRepo
}

// extractRegistryHost picks the host for dockerconfigjson matching and for the
// synthesized OCI Docker config.json's auths.<host>.* entry.
//
//	OCI sources:  trim "oci://" and take the first path segment up to '/'.
//	URL sources:  url.Parse(source).Host.
//	Repo sources: url.Parse(repoURL).Host.
//
// Returns empty string when neither input is a recognisable URL.
func extractRegistryHost(source, repoURL string) string {
	if strings.HasPrefix(source, "oci://") {
		rest := strings.TrimPrefix(source, "oci://")
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			return rest[:i]
		}
		return rest
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		if u, err := neturl.Parse(source); err == nil {
			return u.Host
		}
	}
	if repoURL != "" {
		if u, err := neturl.Parse(repoURL); err == nil {
			return u.Host
		}
	}
	return ""
}

// resolveAuthSecretNamespace mirrors the valuesFrom cross-NS policy
// (resolveValuesFromNamespace at helm.go:650-660): the secret namespace
// MUST equal either the release namespace or the Application namespace.
// An empty namespace defaults to the release namespace.
func resolveAuthSecretNamespace(ref secretRef, opts authResolveOptions) (string, error) {
	ns := ref.Namespace
	if ns == "" {
		return opts.ReleaseNamespace, nil
	}
	if ns == opts.ReleaseNamespace || ns == opts.AppNamespace {
		return ns, nil
	}
	return "", fmt.Errorf(
		`auth secret reference "%s/%s" rejected: namespace MUST equal the release namespace %q or the Application namespace %q`,
		ns, ref.Name, opts.ReleaseNamespace, opts.AppNamespace)
}

// validateBasicCredentials enforces RFC 7617 §2 wire-format constraints.
// The username MUST NOT contain ':' (the field separator). Neither value
// MUST contain control characters (CTL = 0x00-0x1F, 0x7F).
func validateBasicCredentials(username, password, ns, name string) error {
	if strings.ContainsRune(username, ':') {
		return fmt.Errorf(`auth secret "%s/%s" username MUST NOT contain ':' (RFC 7617 §2)`, ns, name)
	}
	if hasControlChar(username) || hasControlChar(password) {
		return fmt.Errorf(`auth secret "%s/%s" credentials MUST NOT contain control characters (RFC 7617 §2)`, ns, name)
	}
	return nil
}

func hasControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// validateBearerToken enforces the RFC 6750 §2.1 grammar:
//
//	b64token = 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
func validateBearerToken(token, ns, name string) error {
	if token == "" {
		return fmt.Errorf(`auth secret "%s/%s" token MUST NOT be empty (RFC 6750 §2.1)`, ns, name)
	}
	seenEquals := false
	for _, r := range token {
		if r == '=' {
			seenEquals = true
			continue
		}
		if seenEquals {
			// trailing '=' padding only; any further non-'=' char is invalid
			return fmt.Errorf(`auth secret "%s/%s" token contains characters outside the b64token charset (RFC 6750 §2.1)`, ns, name)
		}
		if !isB64TokenChar(r) {
			return fmt.Errorf(`auth secret "%s/%s" token contains characters outside the b64token charset (RFC 6750 §2.1)`, ns, name)
		}
	}
	return nil
}

func isB64TokenChar(r rune) bool {
	switch {
	case r >= 'A' && r <= 'Z':
		return true
	case r >= 'a' && r <= 'z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '-' || r == '.' || r == '_' || r == '~' || r == '+' || r == '/':
		return true
	}
	return false
}

// validateBearerTransport enforces RFC 6750 §2: bearer tokens MUST only be
// sent over TLS-protected transport. Plain HTTP and insecureSkipTLS are
// both rejected.
func validateBearerTransport(scheme string, insecureSkipTLS bool, sourceURL, ns, name string) error {
	if scheme == "http" {
		return fmt.Errorf(
			`chart source %q uses scheme "http" but auth secret "%s/%s" supplies a bearer token: bearer tokens MUST be sent only over HTTPS or OCI (RFC 6750 §2)`,
			sourceURL, ns, name)
	}
	if insecureSkipTLS {
		return fmt.Errorf(
			`auth secret "%s/%s" sets insecureSkipTLS together with a bearer token: bearer tokens MUST NOT be sent with TLS verification disabled (RFC 6750 §2)`,
			ns, name)
	}
	return nil
}

// dispatchBasicAuthSecret handles secrets of type kubernetes.io/basic-auth.
// Only the dockerconfigjson dispatcher returns a non-nil raw config blob; the
// other Secret-type dispatchers communicate everything via *common.HTTPOption.
func dispatchBasicAuthSecret(s *corev1.Secret, _ authResolveOptions) (*common.HTTPOption, error) {
	user, ok := s.Data[corev1.BasicAuthUsernameKey]
	if !ok {
		return nil, fmt.Errorf(`auth secret "%s/%s" of type %q MUST contain key "username"`,
			s.Namespace, s.Name, s.Type)
	}
	pass, ok := s.Data[corev1.BasicAuthPasswordKey]
	if !ok {
		return nil, fmt.Errorf(`auth secret "%s/%s" of type %q MUST contain key "password"`,
			s.Namespace, s.Name, s.Type)
	}
	if err := validateBasicCredentials(string(user), string(pass), s.Namespace, s.Name); err != nil {
		return nil, err
	}
	return &common.HTTPOption{Username: string(user), Password: string(pass)}, nil
}

type dockerConfigJSON struct {
	Auths map[string]struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Auth     string `json:"auth"`
		Email    string `json:"email"`
	} `json:"auths"`
}

func dispatchDockerConfigJSONSecret(s *corev1.Secret, opts authResolveOptions) (*common.HTTPOption, []byte, error) {
	raw, ok := s.Data[corev1.DockerConfigJsonKey]
	if !ok {
		return nil, nil, fmt.Errorf(
			`auth secret "%s/%s" of type %q MUST contain key %q`,
			s.Namespace, s.Name, s.Type, corev1.DockerConfigJsonKey)
	}
	var cfg dockerConfigJSON
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.Auths == nil {
		return nil, nil, fmt.Errorf(
			`auth secret "%s/%s" of type kubernetes.io/dockerconfigjson MUST contain a valid Docker configuration`,
			s.Namespace, s.Name)
	}
	entry, found := cfg.Auths[opts.RegistryHost]
	if !found {
		return nil, nil, fmt.Errorf(
			`auth secret "%s/%s" of type kubernetes.io/dockerconfigjson has no entry matching registry host %q`,
			s.Namespace, s.Name, opts.RegistryHost)
	}
	user, pass := entry.Username, entry.Password
	// `docker login` stores credentials as a base64-encoded "username:password"
	// string in the `auth` field, without populating `username`/`password`
	// separately. Decode it when the explicit fields are absent.
	if user == "" && pass == "" && entry.Auth != "" {
		decoded, derr := base64.StdEncoding.DecodeString(entry.Auth)
		if derr != nil {
			return nil, nil, fmt.Errorf(
				`auth secret "%s/%s" of type kubernetes.io/dockerconfigjson has an "auth" field that is not valid base64 for host %q`,
				s.Namespace, s.Name, opts.RegistryHost)
		}
		i := strings.IndexByte(string(decoded), ':')
		if i < 0 {
			return nil, nil, fmt.Errorf(
				`auth secret "%s/%s" of type kubernetes.io/dockerconfigjson "auth" field for host %q MUST decode to "username:password"`,
				s.Namespace, s.Name, opts.RegistryHost)
		}
		user, pass = string(decoded[:i]), string(decoded[i+1:])
	}
	if err := validateBasicCredentials(user, pass, s.Namespace, s.Name); err != nil {
		return nil, nil, err
	}
	httpOpt := &common.HTTPOption{Username: user, Password: pass}
	if opts.SourceType == sourceTypeOCI {
		return httpOpt, raw, nil
	}
	return httpOpt, nil, nil
}

func dispatchTLSSecret(s *corev1.Secret, _ authResolveOptions) (*common.HTTPOption, error) {
	crt, ok := s.Data[corev1.TLSCertKey]
	if !ok {
		return nil, fmt.Errorf(
			`auth secret "%s/%s" of type %q MUST contain key %q`,
			s.Namespace, s.Name, s.Type, corev1.TLSCertKey)
	}
	key, ok := s.Data[corev1.TLSPrivateKeyKey]
	if !ok {
		return nil, fmt.Errorf(
			`auth secret "%s/%s" of type %q MUST contain key %q`,
			s.Namespace, s.Name, s.Type, corev1.TLSPrivateKeyKey)
	}
	opt := &common.HTTPOption{CertFile: string(crt), KeyFile: string(key)}
	if ca, ok := s.Data["ca.crt"]; ok {
		opt.CaFile = string(ca)
	}
	return opt, nil
}

func dispatchOpaqueSecret(s *corev1.Secret, _ authResolveOptions) (*common.HTTPOption, error) {
	opt := &common.HTTPOption{}

	hasUser := len(s.Data["username"]) > 0
	hasPass := len(s.Data["password"]) > 0
	hasToken := len(s.Data["token"]) > 0

	if hasToken && (hasUser || hasPass) {
		return nil, fmt.Errorf(
			`auth secret "%s/%s" sets both basic-auth keys and a token: at most one credential method MUST be configured per Secret (RFC 6750 §2)`,
			s.Namespace, s.Name)
	}

	if hasUser || hasPass {
		u := string(s.Data["username"])
		p := string(s.Data["password"])
		if err := validateBasicCredentials(u, p, s.Namespace, s.Name); err != nil {
			return nil, err
		}
		opt.Username = u
		opt.Password = p
	}
	if hasToken {
		t := string(s.Data["token"])
		if err := validateBearerToken(t, s.Namespace, s.Name); err != nil {
			return nil, err
		}
		opt.BearerToken = t
	}

	// TLS material (Opaque accepts both shorthand "caFile/certFile/keyFile" and
	// K8s-conventional "ca.crt/tls.crt/tls.key" key names).
	if ca, ok := firstNonEmpty(s.Data, "caFile", "ca.crt"); ok {
		opt.CaFile = ca
	}
	if crt, ok := firstNonEmpty(s.Data, "certFile", "tls.crt"); ok {
		opt.CertFile = crt
	}
	if key, ok := firstNonEmpty(s.Data, "keyFile", "tls.key"); ok {
		opt.KeyFile = key
	}
	if v, ok := s.Data["insecureSkipTLS"]; ok && string(v) == "true" {
		opt.InsecureSkipTLS = true
	}
	// insecurePlainHTTP opts the OCI fetcher into plain HTTP instead of TLS.
	// Honored only on the OCI fetch path; ignored elsewhere. Insecure by
	// design (no transport encryption); MUST be set explicitly by the user.
	if v, ok := s.Data["insecurePlainHTTP"]; ok && string(v) == "true" {
		opt.PlainHTTP = true
	}
	return opt, nil
}

func firstNonEmpty(m map[string][]byte, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok && len(v) > 0 {
			return string(v), true
		}
	}
	return "", false
}

// resolveAuthOptions reads the referenced Secret and returns *common.HTTPOption.
// Returns (nil, nil, nil) when ref is nil. The second return value is the raw
// .dockerconfigjson bytes when the Secret is kubernetes.io/dockerconfigjson AND
// the source is OCI.
func resolveAuthOptions(ctx context.Context, k8s client.Client, ref *secretRef, opts authResolveOptions) (*common.HTTPOption, []byte, error) {
	if ref == nil {
		return nil, nil, nil
	}
	ns, err := resolveAuthSecretNamespace(*ref, opts)
	if err != nil {
		return nil, nil, err
	}
	s := &corev1.Secret{}
	if err := k8s.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, s); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, fmt.Errorf(
				`auth secret "%s/%s" not found: it MUST exist in the release namespace %q or the Application namespace %q`,
				ns, ref.Name, opts.ReleaseNamespace, opts.AppNamespace)
		}
		return nil, nil, fmt.Errorf(`reading auth secret "%s/%s": %w`, ns, ref.Name, err)
	}

	var (
		httpOpt *common.HTTPOption
		rawCfg  []byte
	)
	switch s.Type {
	case corev1.SecretTypeBasicAuth:
		httpOpt, err = dispatchBasicAuthSecret(s, opts)
	case corev1.SecretTypeDockerConfigJson:
		httpOpt, rawCfg, err = dispatchDockerConfigJSONSecret(s, opts)
	case corev1.SecretTypeTLS:
		httpOpt, err = dispatchTLSSecret(s, opts)
	case corev1.SecretTypeOpaque, "":
		httpOpt, err = dispatchOpaqueSecret(s, opts)
	default:
		return nil, nil, fmt.Errorf(
			`auth secret "%s/%s" has unsupported type %q: it MUST be one of kubernetes.io/basic-auth, kubernetes.io/dockerconfigjson, kubernetes.io/tls, or Opaque`,
			s.Namespace, s.Name, s.Type)
	}
	if err != nil {
		return nil, nil, err
	}

	// RFC 6750 §3 / OCI Distribution Spec: user-supplied bearer tokens have no
	// effect on OCI sources; the registry runs its own Basic->Bearer flow.
	if httpOpt != nil && httpOpt.BearerToken != "" && opts.SourceType == sourceTypeOCI {
		return nil, nil, fmt.Errorf(
			`chart.auth: user-supplied bearer tokens MUST NOT be used with OCI sources; the registry performs its own Basic->Bearer exchange (RFC 6750 §3, OCI Distribution Spec §authentication)`)
	}

	// RFC 6750 §2: TLS mandate for bearer tokens.
	if httpOpt != nil && httpOpt.BearerToken != "" && opts.SourceType != sourceTypeOCI {
		sourceURL := ""
		if opts.SourceType == sourceTypeRepo {
			sourceURL = opts.SourceScheme + "://"
		}
		if err := validateBearerTransport(opts.SourceScheme, httpOpt.InsecureSkipTLS, sourceURL, s.Namespace, s.Name); err != nil {
			return nil, nil, err
		}
	}

	return httpOpt, rawCfg, nil
}

// writeOCIRegistryConfigFile materializes the temp file passed to
// registry.ClientOptCredentialsFile. When dockerCfgJSON is non-nil, the
// bytes are written verbatim (preserves multi-host configurations from
// kubernetes.io/dockerconfigjson Secrets). Otherwise a one-entry config
// is synthesized from opts.Username/Password keyed by host.
//
// Caller MUST defer cleanup to remove the file.
func writeOCIRegistryConfigFile(opts *common.HTTPOption, dockerCfgJSON []byte, host string) (string, func(), error) {
	f, err := os.CreateTemp("", "kubevela-helm-auth-*.json")
	if err != nil {
		return "", func() {}, fmt.Errorf("creating temp credentials file: %w", err)
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }

	var content []byte
	switch {
	case dockerCfgJSON != nil:
		// Apply the same Docker Hub alias normalization to verbatim
		// kubernetes.io/dockerconfigjson Secrets that the synthesized
		// path below applies. Without this, a user-supplied dockerconfig
		// keyed by "registry-1.docker.io" (the pull host) would not be
		// found by ORAS/Helm, which looks up under the canonical
		// "https://index.docker.io/v1/" key.
		content = normalizeDockerHubAliases(dockerCfgJSON, host)
	case opts != nil && (opts.Username != "" || opts.Password != ""):
		b64 := base64.StdEncoding.EncodeToString([]byte(opts.Username + ":" + opts.Password))
		type authEntry struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Auth     string `json:"auth"`
			Email    string `json:"email"`
		}
		entry := authEntry{Username: opts.Username, Password: opts.Password, Auth: b64}
		auths := map[string]authEntry{host: entry}
		// Docker Hub quirk: the ORAS/Helm auth resolver normalises three host
		// strings ("registry-1.docker.io", "index.docker.io", "docker.io") to
		// the canonical credential key "https://index.docker.io/v1/", not to
		// any of the pull hosts. See oras-go/pkg/auth/docker/resolver.go
		// resolveHostname(). Register the entry under both the pull host and
		// the canonical v1 key so the resolver finds it regardless of which
		// lookup path it takes. No other public OCI registry (GHCR, Quay,
		// ECR, GAR, ACR, Harbor) has this alias quirk.
		if host == "registry-1.docker.io" || host == "index.docker.io" || host == "docker.io" {
			auths["https://index.docker.io/v1/"] = entry
		}
		cfg := struct {
			Auths map[string]authEntry `json:"auths"`
		}{Auths: auths}
		content, err = json.Marshal(cfg)
		if err != nil {
			_ = f.Close()
			cleanup()
			return "", func() {}, fmt.Errorf("marshalling OCI credentials: %w", err)
		}
	default:
		_ = f.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("writeOCIRegistryConfigFile: no credentials provided")
	}

	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("writing OCI credentials: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("closing OCI credentials file: %w", err)
	}
	return path, cleanup, nil
}

// normalizeDockerHubAliases rewrites a verbatim dockerconfigjson blob so it
// works regardless of which Docker Hub host the user keyed it under. ORAS/Helm
// normalizes "registry-1.docker.io", "index.docker.io", and "docker.io" to
// the canonical "https://index.docker.io/v1/" key when looking up credentials,
// but a user-supplied Secret may be keyed under any of those aliases. This
// helper parses the JSON, and if any Docker Hub alias is present (or if the
// pull host is one of them and matches an existing entry), copies the entry
// under the canonical key so the lookup succeeds. If parsing fails or no
// Docker Hub alias is involved, returns the original bytes unchanged.
func normalizeDockerHubAliases(cfgJSON []byte, host string) []byte {
	const canonical = "https://index.docker.io/v1/"
	dockerHubHosts := []string{"registry-1.docker.io", "index.docker.io", "docker.io"}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(cfgJSON, &raw); err != nil {
		return cfgJSON
	}
	authsRaw, ok := raw["auths"]
	if !ok {
		return cfgJSON
	}
	var auths map[string]json.RawMessage
	if err := json.Unmarshal(authsRaw, &auths); err != nil {
		return cfgJSON
	}
	if _, exists := auths[canonical]; exists {
		return cfgJSON
	}
	pickSource := func() (json.RawMessage, bool) {
		for _, h := range dockerHubHosts {
			if v, ok := auths[h]; ok {
				return v, true
			}
		}
		for _, h := range dockerHubHosts {
			if host == h {
				if v, ok := auths[host]; ok {
					return v, true
				}
			}
		}
		return nil, false
	}
	src, ok := pickSource()
	if !ok {
		return cfgJSON
	}
	auths[canonical] = src
	newAuths, err := json.Marshal(auths)
	if err != nil {
		return cfgJSON
	}
	raw["auths"] = newAuths
	out, err := json.Marshal(raw)
	if err != nil {
		return cfgJSON
	}
	return out
}

// resolveHTTPOptions is the provider-side wrapper called from each fetcher.
// Builds authResolveOptions from ChartSourceParams + namespace context, calls
// resolveAuthOptions, and wraps errors with chart-source context.
func resolveHTTPOptions(ctx context.Context, params *ChartSourceParams, appNamespace, releaseNamespace, sourceType string) (*common.HTTPOption, []byte, error) {
	if params == nil || params.Auth == nil || params.Auth.SecretRef == nil {
		return nil, nil, nil
	}
	opts := authResolveOptions{
		AppNamespace:     appNamespace,
		ReleaseNamespace: releaseNamespace,
		RegistryHost:     extractRegistryHost(params.Source, params.RepoURL),
		SourceScheme:     detectSourceScheme(params.Source, params.RepoURL),
		SourceType:       sourceType,
	}
	ref := &secretRef{Name: params.Auth.SecretRef.Name, Namespace: params.Auth.SecretRef.Namespace}
	httpOpt, raw, err := resolveAuthOptions(ctx, singleton.KubeClient.Get(), ref, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("chart source %q: %w", chartSourceLabel(params), err)
	}
	return httpOpt, raw, nil
}

// detectSourceScheme returns "oci", "http", "https", or "" based on the
// chart source or repoURL.
func detectSourceScheme(source, repoURL string) string {
	switch {
	case strings.HasPrefix(source, "oci://"):
		return "oci"
	case strings.HasPrefix(source, "https://"):
		return "https"
	case strings.HasPrefix(source, "http://"):
		return "http"
	case strings.HasPrefix(repoURL, "https://"):
		return "https"
	case strings.HasPrefix(repoURL, "http://"):
		return "http"
	}
	return ""
}

// chartSourceLabel returns a human-readable label for error messages:
// the Source URL if set, otherwise the RepoURL.
func chartSourceLabel(params *ChartSourceParams) string {
	if params.Source != "" {
		return params.Source
	}
	return params.RepoURL
}
