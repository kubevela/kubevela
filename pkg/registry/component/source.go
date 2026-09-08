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

// Package component holds the package-source machinery shared by the addon and
// module implementations: the Registry model and its data store, the readers
// that speak git, OSS and OCI, and the OCI chart helpers.
package component

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/oauth2"
	helmregistry "helm.sh/helm/v3/pkg/registry"

	"github.com/oam-dev/kubevela/pkg/utils"
)

const (
	// EOFError is error returned by xml parse
	EOFError string = "EOF"
	// MetadataFileName is the package metadata.yaml file name, which is how a
	// reader recognises a package directory in a flat listing.
	MetadataFileName string = "metadata.yaml"
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
func (h *HelmSource) Credential() (username, secret string) {
	if IsOCIURL(h.URL) {
		return h.Username, h.Token
	}
	return h.Username, h.Password
}

// validateCredential rejects a source whose credential fields do not match its
// URL scheme, and options the scheme cannot honour. Without this the mismatch
// surfaces far from its cause: the transport reads the field it knows about,
// finds it empty, and authenticates anonymously.
func (h *HelmSource) ValidateCredential() error {
	if IsOCIURL(h.URL) {
		if h.Password != "" {
			return errors.New("an oci:// addon registry authenticates with token, not password")
		}
		if h.InsecureSkipTLS {
			// The OCI client built by newOCIClientWithPlainHTTP has no seam for a
			// custom transport, so honouring this would be a lie.
			return errors.New("insecureSkipTLS is not supported for an oci:// addon registry")
		}
		// The token may not be inline: once a registry is saved, the token is
		// moved into a Secret and only TokenSecretRef remains, so either field
		// counts as the credential being configured.
		hasToken := h.Token != "" || h.TokenSecretRef != ""
		if (h.Username != "") != hasToken {
			// Otherwise the registry client attempts BasicAuth with an empty
			// counterpart, which registries answer with a 401 that names neither
			// field. Anonymous access is spelled by leaving both empty.
			return errors.New("an oci:// addon registry needs username and token together; omit both for anonymous access")
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
	// GitType reads from a GitHub repository.
	GitType ReaderType = "git"
	// OSSType reads from an object-storage bucket.
	OSSType ReaderType = "oss"
	// GiteeType reads from a Gitee repository.
	GiteeType ReaderType = "gitee"
	// GitlabType reads from a GitLab repository.
	GitlabType ReaderType = "gitlab"
)

// NewAsyncReader create AsyncReader from
// 1. GitHub url and directory
// 2. OSS endpoint and bucket
func NewAsyncReader(baseURL, bucket, repo, subPath, token string, rdType ReaderType) (AsyncReader, error) {

	switch rdType {
	case GitType:
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
	case OSSType:
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
	case GiteeType:
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
	case GitlabType:
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
		return NewAsyncReader(o.Endpoint, o.Bucket, "", o.Path, "", OSSType)
	}
	if r.Git != nil {
		g := r.Git
		return NewAsyncReader(g.URL, "", "", g.Path, g.Token, GitType)
	}
	if r.Gitee != nil {
		g := r.Gitee
		return NewAsyncReader(g.URL, "", "", g.Path, g.Token, GiteeType)
	}
	if r.Gitlab != nil {
		g := r.Gitlab
		return NewAsyncReader(g.URL, "", g.Repo, g.Path, g.Token, GitlabType)
	}
	return nil, errors.New("registry don't have enough info to build a reader")
}

// ListAddonMeta list addon file meta(path and name) from a registry
func (r *Registry) ListAddonMeta() (map[string]SourceMeta, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	return reader.ListAddonMeta()
}

func createGitHelper(content *utils.Content, token string) *gitHelper {
	var ts oauth2.TokenSource
	if token != "" {
		ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	}
	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = time.Second * 20
	cli := github.NewClient(tc)
	return &gitHelper{
		Client: cli,
		Meta:   content,
	}
}

func createGiteeHelper(content *utils.Content, token string) *giteeHelper {
	var ts oauth2.TokenSource
	if token != "" {
		ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	}
	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = time.Second * 20
	cli := NewGiteeClient(tc, nil)
	return &giteeHelper{
		Client: cli,
		Meta:   content,
	}
}

func createGitlabHelper(content *utils.Content, token string) (*gitlabHelper, error) {
	newClient, err := gitlab.NewClient(token, gitlab.WithBaseURL(content.GitlabContent.Host))

	return &gitlabHelper{
		Client: newClient,
		Meta:   content,
	}, err
}
