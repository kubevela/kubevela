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

package addon

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	helmregistry "helm.sh/helm/v3/pkg/registry"

	"github.com/oam-dev/kubevela/pkg/utils"
)

const (
	// EOFError is error returned by xml parse
	EOFError string = "EOF"
	// DirType means a directory
	DirType = "dir"
	// FileType means a file
	FileType = "file"
	// BlobType means a blob
	BlobType = "blob"
	// TreeType means a tree
	TreeType = "tree"

	bucketTmpl        = "%s://%s.%s"
	singleOSSFileTmpl = "%s/%s"
	listOSSFileTmpl   = "%s?max-keys=1000&prefix=%s"
)

// Source is where to get addons, Registry implement this interface
type Source interface {
	GetUIData(meta *SourceMeta, opt ListOptions) (*UIData, error)
	ListUIData(registryAddonMeta map[string]SourceMeta, opt ListOptions) ([]*UIData, error)
	GetInstallPackage(meta *SourceMeta, uiData *UIData) (*InstallPackage, error)
	ListAddonMeta() (map[string]SourceMeta, error)
}

// GitAddonSource defines the information about the Git as addon source
type GitAddonSource struct {
	URL            string `json:"url,omitempty" validate:"required"`
	Path           string `json:"path,omitempty"`
	Token          string `json:"token,omitempty"`
	TokenSecretRef string `json:"tokenSecretRef,omitempty"`
}

// GetToken returns the token of the source
func (g *GitAddonSource) GetToken() string {
	return g.Token
}

// SetToken set the token of the source
func (g *GitAddonSource) SetToken(token string) {
	g.Token = token
	g.TokenSecretRef = ""
}

// SetTokenSecretRef set the token secret ref to the source
func (g *GitAddonSource) SetTokenSecretRef(secretName string) {
	g.Token = ""
	g.TokenSecretRef = secretName
}

// GetTokenSecretRef return the token secret ref of the source
func (g *GitAddonSource) GetTokenSecretRef() string {
	return g.TokenSecretRef
}

// SafeCopy hides field Token
func (g *GitAddonSource) SafeCopy() *GitAddonSource {
	if g == nil {
		return nil
	}
	return &GitAddonSource{
		URL:            g.URL,
		Path:           g.Path,
		TokenSecretRef: g.TokenSecretRef,
	}
}

// GiteeAddonSource defines the information about the Gitee as addon source
type GiteeAddonSource struct {
	URL            string `json:"url,omitempty" validate:"required"`
	Path           string `json:"path,omitempty"`
	Token          string `json:"token,omitempty"`
	TokenSecretRef string `json:"tokenSecretRef,omitempty"`
}

// GetToken return the token of the source
func (g *GiteeAddonSource) GetToken() string {
	return g.Token
}

// SetToken set the token of the source
func (g *GiteeAddonSource) SetToken(token string) {
	g.Token = token
	g.TokenSecretRef = ""
}

// SetTokenSecretRef set the token secret ref to the source
func (g *GiteeAddonSource) SetTokenSecretRef(secretName string) {
	g.Token = ""
	g.TokenSecretRef = secretName
}

// GetTokenSecretRef return the token secret ref of the source
func (g *GiteeAddonSource) GetTokenSecretRef() string {
	return g.TokenSecretRef
}

// SafeCopy hides field Token
func (g *GiteeAddonSource) SafeCopy() *GiteeAddonSource {
	if g == nil {
		return nil
	}
	return &GiteeAddonSource{
		URL:            g.URL,
		Path:           g.Path,
		TokenSecretRef: g.TokenSecretRef,
	}
}

// GitlabAddonSource defines the information about Gitlab as an addon source
type GitlabAddonSource struct {
	URL            string `json:"url,omitempty" validate:"required"`
	Repo           string `json:"repo,omitempty" validate:"required"`
	Path           string `json:"path,omitempty"`
	Token          string `json:"token,omitempty"`
	TokenSecretRef string `json:"tokenSecretRef,omitempty"`
}

// GetToken return the token of the source
func (g *GitlabAddonSource) GetToken() string {
	return g.Token
}

// SetToken set the token of the source
func (g *GitlabAddonSource) SetToken(token string) {
	g.Token = token
	g.TokenSecretRef = ""
}

// SetTokenSecretRef set the token secret ref to the source
func (g *GitlabAddonSource) SetTokenSecretRef(secretName string) {
	g.Token = ""
	g.TokenSecretRef = secretName
}

// GetTokenSecretRef return the token secret ref of the source
func (g *GitlabAddonSource) GetTokenSecretRef() string {
	return g.TokenSecretRef
}

// SafeCopy hides field Token
func (g *GitlabAddonSource) SafeCopy() *GitlabAddonSource {
	if g == nil {
		return nil
	}
	return &GitlabAddonSource{
		URL:  g.URL,
		Repo: g.Repo,
		Path: g.Path,
	}
}

// HelmSource defines the information about a Helm chart repository as an addon
// source. The URL scheme decides the transport: an http(s):// URL is an indexed
// Helm repository, an oci:// URL is an OCI registry holding the addon as a Helm
// chart (pushed via `helm push oci://...`).
//
// The two transports authenticate through different fields. An http(s):// URL
// uses Password, which stays in the registry ConfigMap. An oci:// URL uses
// Token, which is moved into a Secret and referenced by TokenSecretRef. Setting
// the field belonging to the other scheme is a misconfiguration rather than a
// fallback, because it would otherwise reach the registry as anonymous access
// and fail as an opaque 401.
type HelmSource struct {
	URL             string `json:"url,omitempty" validate:"required"`
	InsecureSkipTLS bool   `json:"insecureSkipTLS,omitempty"`
	Username        string `json:"username,omitempty"`
	// Password authenticates an http(s):// Helm repository.
	Password string `json:"password,omitempty"`
	// Token authenticates an oci:// registry. For ECR the Username is "AWS" and
	// the Token is the output of `aws ecr get-login-password`.
	Token string `json:"token,omitempty"`
	// TokenSecretRef names the Secret holding Token once it has been moved out
	// of the ConfigMap.
	TokenSecretRef string `json:"tokenSecretRef,omitempty"`
}

// GetToken returns the token of the source
func (h *HelmSource) GetToken() string {
	return h.Token
}

// SetToken sets the token of the source and clears any secret ref
func (h *HelmSource) SetToken(token string) {
	h.Token = token
	h.TokenSecretRef = ""
}

// SetTokenSecretRef sets the token secret ref and clears the inline token
func (h *HelmSource) SetTokenSecretRef(secretName string) {
	h.Token = ""
	h.TokenSecretRef = secretName
}

// GetTokenSecretRef returns the token secret ref of the source
func (h *HelmSource) GetTokenSecretRef() string {
	return h.TokenSecretRef
}

// credential returns the username and secret the transport should authenticate
// with, chosen by URL scheme. Callers read credentials through this rather than
// reaching for Password or Token directly, so neither backend has to know which
// field the other one uses.
func (h *HelmSource) credential() (username, secret string) {
	if IsOCIURL(h.URL) {
		return h.Username, h.Token
	}
	return h.Username, h.Password
}

// validateCredential rejects a source whose credential fields do not match its
// URL scheme, and options the scheme cannot honour. Without this the mismatch
// surfaces far from its cause: the transport reads the field it knows about,
// finds it empty, and authenticates anonymously.
func (h *HelmSource) validateCredential() error {
	if IsOCIURL(h.URL) {
		if h.Password != "" {
			return errors.New("an oci:// addon registry authenticates with token, not password")
		}
		if h.InsecureSkipTLS {
			// The OCI client built by newOCIClientWithPlainHTTP has no seam for a
			// custom transport, so honouring this would be a lie.
			return errors.New("insecureSkipTLS is not supported for an oci:// addon registry")
		}
		return nil
	}
	if h.Token != "" {
		return errors.New("an http(s):// addon registry authenticates with password, not token")
	}
	if h.TokenSecretRef != "" {
		return errors.New("tokenSecretRef is only supported for an oci:// addon registry")
	}
	return nil
}

// IsOCIURL reports whether a repository URL addresses an OCI registry rather
// than an indexed HTTP Helm repository. The scheme is the whole signal: it
// decides which transport reads the chart, which credential field carries the
// password, and whether the credential is moved into a Secret.
//
// It parses rather than matching a prefix, so a repository merely hosted at
// oci.example.com over https is not mistaken for an OCI registry, and a URL
// that cannot be parsed classifies as not-OCI instead of guessing.
func IsOCIURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, helmregistry.OCIScheme)
}

// SafeCopier is an interface to copy struct without sensitive fields, such as Token, Username, Password
type SafeCopier interface {
	SafeCopy() interface{}
}

// SafeCopy hides field Username, Password
//
// This keeps only the URL, which is narrower than the OCI source it replaces:
// that one also carried Username and TokenSecretRef. Widening it would change
// released behaviour that TestSafeCopy pins, and no caller needs the identity
// fields, so the narrower contract stands. The push path builds its own source
// explicitly instead (see ociPushSource in push.go).
func (h *HelmSource) SafeCopy() *HelmSource {
	if h == nil {
		return nil
	}
	return &HelmSource{
		URL: h.URL,
	}
}

// Item is a partial interface for github.RepositoryContent
type Item interface {
	// GetType return "dir" or "file"
	GetType() string
	GetPath() string
	GetName() string
}

// SourceMeta record the whole metadata of an addon
type SourceMeta struct {
	Name  string
	Items []Item
}

// ClassifyItemByPattern will filter and classify addon data, data will be classified by pattern it meets
func ClassifyItemByPattern(meta *SourceMeta, r AsyncReader) map[string][]Item {
	var p = make(map[string][]Item)
	for _, it := range meta.Items {
		pt := GetPatternFromItem(it, r, meta.Name)
		if pt == "" {
			continue
		}
		items := p[pt]
		items = append(items, it)
		p[pt] = items
	}
	return p
}

// AsyncReader helps async read files of addon
type AsyncReader interface {
	// ListAddonMeta will return directory tree contain addon metadata only
	ListAddonMeta() (addonCandidates map[string]SourceMeta, err error)

	// ReadFile should accept relative path to github repo/path or OSS bucket, and report the file content
	ReadFile(path string) (content string, err error)

	// RelativePath return a relative path to GitHub repo/path or OSS bucket/path
	RelativePath(item Item) string
}

// ReaderType marks where to read addon files
type ReaderType string

const (
	gitType    ReaderType = "git"
	ossType    ReaderType = "oss"
	giteeType  ReaderType = "gitee"
	gitlabType ReaderType = "gitlab"
)

// NewAsyncReader create AsyncReader from
// 1. GitHub url and directory
// 2. OSS endpoint and bucket
func NewAsyncReader(baseURL, bucket, repo, subPath, token string, rdType ReaderType) (AsyncReader, error) {

	switch rdType {
	case gitType:
		baseURL = strings.TrimSuffix(baseURL, ".git")
		u, err := url.Parse(baseURL)
		if err != nil {
			return nil, errors.New("addon registry invalid")
		}
		u.Path = path.Join(u.Path, subPath)
		_, content, err := utils.Parse(u.String())
		if err != nil {
			return nil, err
		}
		gith := createGitHelper(content, token)
		return &gitReader{
			h: gith,
		}, nil
	case ossType:
		ossURL, err := url.Parse(baseURL)
		if err != nil {
			return nil, err
		}
		var bucketEndPoint string
		if bucket == "" {
			bucketEndPoint = ossURL.String()
		} else {
			if ossURL.Scheme == "" {
				ossURL.Scheme = "https"
			}
			bucketEndPoint = fmt.Sprintf(bucketTmpl, ossURL.Scheme, bucket, ossURL.Host)
		}
		return &ossReader{
			bucketEndPoint: bucketEndPoint,
			path:           subPath,
			client:         resty.New(),
		}, nil
	case giteeType:
		baseURL = strings.TrimSuffix(baseURL, ".git")
		u, err := url.Parse(baseURL)
		if err != nil {
			return nil, errors.New("addon registry invalid")
		}
		u.Path = path.Join(u.Path, subPath)
		_, content, err := utils.Parse(u.String())
		if err != nil {
			return nil, err
		}
		gitee := createGiteeHelper(content, token)
		return &giteeReader{
			h: gitee,
		}, nil
	case gitlabType:
		baseURL = strings.TrimSuffix(baseURL, ".git")
		u, err := url.Parse(baseURL)
		if err != nil {
			return nil, errors.New("addon registry invalid")
		}
		_, content, err := utils.ParseGitlab(u.String(), repo)
		if err != nil {
			return nil, err
		}
		content.GitlabContent.Path = subPath
		gitlabHelper, err := createGitlabHelper(content, token)
		if err != nil {
			return nil, errors.New("addon registry connect fail")
		}

		err = gitlabHelper.getGitlabProject(content)
		if err != nil {
			return nil, err
		}

		return &gitlabReader{
			h: gitlabHelper,
		}, nil
	}
	return nil, fmt.Errorf("invalid addon registry type '%s'", rdType)
}

// getGitlabProject get gitlab project , set project id
func (h *gitlabHelper) getGitlabProject(content *utils.Content) error {
	projectURL := content.GitlabContent.Owner + "/" + content.GitlabContent.Repo
	projects, _, err := h.Client.Projects.GetProject(projectURL, &gitlab.GetProjectOptions{})
	if err != nil {
		return err
	}
	content.GitlabContent.PId = projects.ID

	return nil
}

// BuildReader will build a AsyncReader from registry, AsyncReader are needed to read addon files
func (r *Registry) BuildReader() (AsyncReader, error) {
	if r.OSS != nil {
		o := r.OSS
		return NewAsyncReader(o.Endpoint, o.Bucket, "", o.Path, "", ossType)
	}
	if r.Git != nil {
		g := r.Git
		return NewAsyncReader(g.URL, "", "", g.Path, g.Token, gitType)
	}
	if r.Gitee != nil {
		g := r.Gitee
		return NewAsyncReader(g.URL, "", "", g.Path, g.Token, giteeType)
	}
	if r.Gitlab != nil {
		g := r.Gitlab
		return NewAsyncReader(g.URL, "", g.Repo, g.Path, g.Token, gitlabType)
	}
	return nil, errors.New("registry don't have enough info to build a reader")
}

// GetUIData get UIData of an addon
func (r *Registry) GetUIData(meta *SourceMeta, opt ListOptions) (*UIData, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	addon, err := GetUIDataFromReader(reader, meta, opt)
	if err != nil {
		return nil, err
	}
	if len(addon.GlobalParameters) != 0 {
		addon.Parameters = addon.GlobalParameters
	}
	addon.RegistryName = r.Name
	return addon, nil
}

// ListUIData list UI data from addon registry
func (r *Registry) ListUIData(registryAddonMeta map[string]SourceMeta, opt ListOptions) ([]*UIData, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	return ListAddonUIDataFromReader(reader, registryAddonMeta, r.Name, opt)
}

// GetInstallPackage get install package which is all needed to enable an addon from addon registry
func (r *Registry) GetInstallPackage(meta *SourceMeta, uiData *UIData) (*InstallPackage, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	return GetInstallPackageFromReader(reader, meta, uiData)
}

// ListAddonMeta list addon file meta(path and name) from a registry
func (r *Registry) ListAddonMeta() (map[string]SourceMeta, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	return reader.ListAddonMeta()
}

// ItemInfo contains summary information about an addon
type ItemInfo struct {
	Name              string
	Description       string
	AvailableVersions []string
}

type itemInfoMap map[string]ItemInfo

// ListAddonInfo lists addon info (name, versions, etc.) from a registry
func (r *Registry) ListAddonInfo() (map[string]ItemInfo, error) {
	addonInfoMap := make(map[string]ItemInfo)

	// local registry doesn't support listing addons
	if IsLocalRegistry(*r) {
		return addonInfoMap, nil
	}
	if IsVersionRegistry(*r) {
		versionedRegistry, err := ToVersionedRegistry(*r)
		if err != nil {
			return nil, err
		}
		addonList, err := versionedRegistry.ListAddon()
		if err != nil {
			return nil, err
		}
		for _, a := range addonList {
			addonInfoMap[a.Name] = ItemInfo{
				Name:              a.Name,
				Description:       a.Description,
				AvailableVersions: a.AvailableVersions,
			}
		}
	} else {
		meta, err := r.ListAddonMeta()
		if err != nil {
			return nil, err
		}
		addonList, err := r.ListUIData(meta, ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, a := range addonList {
			addonInfoMap[a.Name] = ItemInfo{
				Name:              a.Name,
				Description:       a.Description,
				AvailableVersions: a.AvailableVersions,
			}
		}
	}

	return addonInfoMap, nil
}
