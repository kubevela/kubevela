/*
Copyright 2022 The KubeVela Authors.

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	cm "github.com/chartmuseum/helm-push/pkg/chartmuseum"
	cmhelm "github.com/chartmuseum/helm-push/pkg/helm"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart/loader"
	helmrepo "helm.sh/helm/v3/pkg/repo"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var chartMuseumURLPattern = regexp.MustCompile(`^https?://`)

// PushCmd is the command object to initiate a push command to ChartMuseum or an OCI registry.
type PushCmd struct {
	ChartName          string
	AppVersion         string
	ChartVersion       string
	RepoName           string
	Username           string
	Password           string
	PasswordStdin      bool
	AccessToken        string
	AuthHeader         string
	ContextPath        string
	ForceUpload        bool
	UseHTTP            bool
	CaFile             string
	CertFile           string
	KeyFile            string
	InsecureSkipVerify bool
	Out                io.Writer
	Timeout            int64
	KeepChartMetadata  bool
	// We need it to search in addon registries.
	// If you use URL, instead of registry names, then it is not needed.
	Client client.Client

	// ociPushFn is a test seam for verifying target and credential resolution
	// without contacting an OCI registry.
	ociPushFn func(context.Context, *OCIAddonSource, bool) error
}

// IsDirectAddonPushTarget reports whether target can be used without resolving
// a configured addon registry from Kubernetes.
func IsDirectAddonPushTarget(target string) bool {
	return strings.HasPrefix(target, "oci://") || chartMuseumURLPattern.MatchString(target)
}

// Push pushes addons (i.e. Helm Charts) to ChartMuseum or an OCI registry.
// It will package the addon into a Helm Chart if necessary.
func (p *PushCmd) Push(ctx context.Context) error {
	if strings.HasPrefix(p.RepoName, "oci://") {
		return p.pushToOCI(ctx, &OCIAddonSource{
			URL:      p.RepoName,
			Username: p.Username,
			Token:    p.Password,
		})
	}
	if p.Client != nil {
		reg, lookupErr := NewRegistryDataStore(p.Client).GetRegistry(ctx, p.RepoName)
		if lookupErr == nil && IsOCIRegistry(reg) {
			source := reg.OCI.SafeCopy()
			source.Token = reg.OCI.Token
			if p.Username != "" {
				source.Username = p.Username
			}
			if p.Password != "" {
				source.Token = p.Password
			}
			return p.pushToOCI(ctx, source)
		}
	}

	var repo *cmhelm.Repo
	var err error

	// Get the user specified Helm repo
	repo, err = GetHelmRepo(ctx, p.Client, p.RepoName)
	if err != nil {
		return err
	}

	// Make the addon dir a Helm Chart
	// The user can decide if they want Chart.yaml be in sync with addon metadata.yaml
	// By default, it will recreate Chart.yaml according to addon metadata.yaml
	err = MakeChartCompatible(p.ChartName, !p.KeepChartMetadata)
	// `Not a directory` errors are ignored, that's fine,
	// since .tgz files are also supported.
	if err != nil && !strings.Contains(err.Error(), "is not a directory") {
		return err
	}

	// Get chart from a directory or .tgz package
	chart, err := cmhelm.GetChartByName(p.ChartName)
	if err != nil {
		return err
	}

	// Override chart version using specified version
	if p.ChartVersion != "" {
		chart.SetVersion(p.ChartVersion)
	}

	// Override app version using specified version
	if p.AppVersion != "" {
		chart.SetAppVersion(p.AppVersion)
	}

	// Override username and password using specified values
	username := repo.Config.Username
	password := repo.Config.Password
	if p.Username != "" {
		username = p.Username
	}
	if p.Password != "" {
		password = p.Password
	}

	// Unset accessToken if repo credentials are provided
	if username != "" && password != "" {
		p.AccessToken = ""
	}

	// In case the repo is stored with cm:// protocol,
	// (if that's somehow possible with KubeVela addon registries)
	// use http instead,
	// otherwise keep as it-is.
	var url string
	if p.UseHTTP {
		url = strings.Replace(repo.Config.URL, "cm://", "http://", 1)
	} else {
		url = strings.Replace(repo.Config.URL, "cm://", "https://", 1)
	}

	cmClient, err := cm.NewClient(
		cm.URL(url),
		cm.Username(username),
		cm.Password(password),
		cm.AccessToken(p.AccessToken),
		cm.AuthHeader(p.AuthHeader),
		cm.ContextPath(p.ContextPath),
		cm.CAFile(p.CaFile),
		cm.CertFile(p.CertFile),
		cm.KeyFile(p.KeyFile),
		cm.InsecureSkipVerify(p.InsecureSkipVerify),
		cm.Timeout(p.Timeout),
	)

	if err != nil {
		return err
	}

	// Use a temporary dir to hold packaged .tgz Charts
	tmp, err := os.MkdirTemp("", "helm-push-")
	if err != nil {
		return err
	}
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tmp)

	// Package Chart into .tgz packages for uploading to ChartMuseum
	chartPackagePath, err := cmhelm.CreateChartPackage(chart, tmp)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stderr, "Pushing %s to %s... ",
		color.New(color.Bold).Sprintf("%s", filepath.Base(chartPackagePath)),
		formatRepoNameAndURL(p.RepoName, repo.Config.URL),
	)

	// Push Chart to ChartMuseum
	resp, err := cmClient.UploadChartPackage(chartPackagePath, p.ForceUpload)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return handlePushResponse(resp)
}

func (p *PushCmd) pushToOCI(ctx context.Context, source *OCIAddonSource) error {
	if p.AccessToken != "" {
		return errors.New("--access-token is only supported for ChartMuseum; use --password or --password-stdin for OCI registries")
	}
	if (source.Username == "") != (source.Token == "") {
		return errors.New("OCI registry username and password must be supplied together; omit both to use anonymous access or configured Helm/Docker credentials")
	}
	if p.ociPushFn != nil {
		return p.ociPushFn(ctx, source, p.UseHTTP)
	}
	return p.pushOCI(source)
}

func (p *PushCmd) pushOCI(source *OCIAddonSource) error {
	if err := MakeChartCompatible(p.ChartName, !p.KeepChartMetadata); err != nil && !strings.Contains(err.Error(), "is not a directory") {
		return err
	}
	addonChart, err := cmhelm.GetChartByName(p.ChartName)
	if err != nil {
		return err
	}
	if p.ChartVersion != "" {
		addonChart.SetVersion(p.ChartVersion)
	}
	if p.AppVersion != "" {
		addonChart.SetAppVersion(p.AppVersion)
	}

	tmp, err := os.MkdirTemp("", "helm-push-oci-")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tmp)
	}()
	archivePath, err := cmhelm.CreateChartPackage(addonChart, tmp)
	if err != nil {
		return err
	}
	archive, err := os.ReadFile(filepath.Clean(archivePath))
	if err != nil {
		return err
	}
	loadedChart, err := loader.LoadArchive(bytes.NewReader(archive))
	if err != nil {
		return err
	}

	repoRef, host := ociRepoRef(source.URL, loadedChart.Metadata.Name)
	ociClient, err := newOCIClientWithPlainHTTP(host, source.Username, source.Token, p.UseHTTP)
	if err != nil {
		return err
	}
	ref := repoRef + ":" + loadedChart.Metadata.Version
	// Out is optional: the existing Push API works with a zero-value PushCmd, so
	// writing status must not panic when a caller left it unset.
	if p.Out != nil {
		_, _ = fmt.Fprintf(p.Out, "Pushing %s to %s\n", filepath.Base(archivePath), ref)
	}
	if _, err := ociClient.Push(archive, ref); err != nil {
		return errors.Wrapf(err, "failed to push OCI addon %s", ref)
	}
	// The chart is published at this point. The portable catalog is a discovery
	// convenience on top of that, and plenty of registries cannot support it (no
	// /v2/_catalog, no tag listing, or no permission to push the catalog repo).
	// Reporting a hard error here would tell the user their push failed when it
	// did not, inviting a retry that publishes a duplicate. Warn instead.
	if err := updateOCIAddonCatalog(ociClient, source, loadedChart.Metadata, p.UseHTTP); err != nil {
		klog.Warningf("addon %s:%s was pushed successfully, but the portable OCI catalog was not updated: %v",
			loadedChart.Metadata.Name, loadedChart.Metadata.Version, err)
		if p.Out != nil {
			_, _ = fmt.Fprintf(p.Out, "Warning: %s was pushed, but the portable addon catalog was not updated: %v\n", ref, err)
		}
	}
	return nil
}

// GetHelmRepo searches for a Helm repo by name.
// By saying name, it can actually be a URL or a name.
// If a URL is provided, a temp repo object is returned.
// If a name is provided, we will try to find it in local addon registries (only Helm type).
func GetHelmRepo(ctx context.Context, c client.Client, repoName string) (*cmhelm.Repo, error) {
	var repo *cmhelm.Repo
	var err error

	// If RepoName looks like a URL (https / http), just create a temp repo object.
	// We do not look for it in local addon registries.
	if chartMuseumURLPattern.MatchString(repoName) {
		repo, err = cmhelm.TempRepoFromURL(repoName)
		if err != nil {
			return nil, err
		}
		return repo, nil
	}

	// Otherwise, search for in it in the local addon registries.
	ds := NewRegistryDataStore(c)
	registries, err := ds.ListRegistries(ctx)
	if err != nil {
		return nil, err
	}

	var matchedEntry *helmrepo.Entry

	// Search for the target repo name in addon registries
	for _, reg := range registries {
		// We are only interested in Helm registries.
		if reg.Helm == nil {
			continue
		}

		if reg.Name == repoName {
			matchedEntry = &helmrepo.Entry{
				Name:     reg.Name,
				URL:      reg.Helm.URL,
				Username: reg.Helm.Username,
				Password: reg.Helm.Password,
			}
			break
		}
	}

	if matchedEntry == nil {
		return nil, fmt.Errorf("we cannot find Helm repository %s. Make sure you hava added it using `vela addon registry add` and it is a Helm repository", repoName)
	}

	// Use the repo found locally.
	repo = &cmhelm.Repo{ChartRepository: &helmrepo.ChartRepository{Config: matchedEntry}}

	return repo, nil
}

// SetFieldsFromEnv sets fields in PushCmd from environment variables
func (p *PushCmd) SetFieldsFromEnv() {
	if v, ok := os.LookupEnv("HELM_REPO_USERNAME"); ok && p.Username == "" {
		p.Username = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_PASSWORD"); ok && p.Password == "" {
		p.Password = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_ACCESS_TOKEN"); ok && p.AccessToken == "" {
		p.AccessToken = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_AUTH_HEADER"); ok && p.AuthHeader == "" {
		p.AuthHeader = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_CONTEXT_PATH"); ok && p.ContextPath == "" {
		p.ContextPath = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_USE_HTTP"); ok {
		p.UseHTTP, _ = strconv.ParseBool(v)
	}
	if v, ok := os.LookupEnv("HELM_REPO_CA_FILE"); ok && p.CaFile == "" {
		p.CaFile = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_CERT_FILE"); ok && p.CertFile == "" {
		p.CertFile = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_KEY_FILE"); ok && p.KeyFile == "" {
		p.KeyFile = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_INSECURE"); ok {
		p.InsecureSkipVerify, _ = strconv.ParseBool(v)
	}
}

// handlePushResponse checks response from ChartMuseum
func handlePushResponse(resp *http.Response) error {
	if resp.StatusCode != 201 && resp.StatusCode != 202 {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", color.RedString("Failed"))
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return getChartMuseumError(b, resp.StatusCode)
	}
	_, _ = fmt.Fprintf(os.Stderr, "%s\n", color.GreenString("Done"))
	return nil
}

// getChartMuseumError checks error messages from the response
func getChartMuseumError(b []byte, code int) error {
	var er struct {
		Error string `json:"error"`
	}
	err := json.Unmarshal(b, &er)
	if err != nil || er.Error == "" {
		return fmt.Errorf("%d: could not properly parse response JSON: %s", code, string(b))
	}
	return fmt.Errorf("%d: %s", code, er.Error)
}

func formatRepoNameAndURL(name, url string) string {
	if name == "" || chartMuseumURLPattern.MatchString(name) {
		return color.BlueString(url)
	}

	return fmt.Sprintf("%s(%s)",
		color.New(color.Bold).Sprintf("%s", name),
		color.BlueString(url),
	)
}
