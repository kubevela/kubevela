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
	"context"
	"encoding/json"
	"fmt"
	cm "github.com/chartmuseum/helm-push/pkg/chartmuseum"
	cmhelm "github.com/chartmuseum/helm-push/pkg/helm"
	helmrepo "helm.sh/helm/v3/pkg/repo"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"strings"
)

// PushCmd is TBD
type PushCmd struct {
	ChartName          string
	AppVersion         string
	ChartVersion       string
	RepoName           string
	Username           string
	Password           string
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
	Client             client.Client
}

// Push pushes addons (Helm Charts) to ChartMuseum.
// Will package the addon into a Helm Chart if necessary.
func (p *PushCmd) Push(ctx context.Context) error {
	var repo *cmhelm.Repo
	var err error

	// If RepoName looks like a URL (https / http), just create a temp repo object.
	// We do not look for it in local addon registries.
	if regexp.MustCompile(`^https?://`).MatchString(p.RepoName) {
		repo, err = cmhelm.TempRepoFromURL(p.RepoName)
		p.RepoName = repo.Config.URL
		if err != nil {
			return err
		}
	} else {
		// Otherwise, search for in it in the local registries
		ds := NewRegistryDataStore(p.Client)
		registries, err := ds.ListRegistries(ctx)
		if err != nil {
			return err
		}
		var matchedEntry *helmrepo.Entry
		for _, reg := range registries {
			// We only keep Helm registries
			if reg.Helm == nil {
				continue
			}

			if reg.Name == p.RepoName {
				matchedEntry = &helmrepo.Entry{
					Name:     reg.Name,
					URL:      reg.Helm.URL,
					Username: reg.Helm.Username,
					Password: reg.Helm.Password,
				}
			}
		}
		if matchedEntry == nil {
			return fmt.Errorf("we cannot fing repository %s. Make sure you hava added it using `vela addon registry add`", p.RepoName)
		}
		// Use the repo found locally
		repo = &cmhelm.Repo{ChartRepository: &helmrepo.ChartRepository{Config: matchedEntry}}
	}

	// Make the addon dir a Helm Chart
	err = MakeChart(p.ChartName)
	// Not a directory errors are ignored, since .tgz files are also supported.
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

	// In case the repo is stored with cm:// protocol, remove it,
	// otherwise keep as it-is.
	var url string
	if p.UseHTTP {
		url = strings.Replace(repo.Config.URL, "cm://", "http://", 1)
	} else {
		url = strings.Replace(repo.Config.URL, "cm://", "https://", 1)
	}

	client, err := cm.NewClient(
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

	tmp, err := ioutil.TempDir("", "helm-push-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	chartPackagePath, err := cmhelm.CreateChartPackage(chart, tmp)
	if err != nil {
		return err
	}

	fmt.Printf("Pushing %s to %s...\n", filepath.Base(chartPackagePath), p.RepoName)
	resp, err := client.UploadChartPackage(chartPackagePath, p.ForceUpload)
	if err != nil {
		return err
	}

	return handlePushResponse(resp)
}

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

func handlePushResponse(resp *http.Response) error {
	if resp.StatusCode != 201 && resp.StatusCode != 202 {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return getChartmuseumError(b, resp.StatusCode)
	}
	fmt.Println("Done.")
	return nil
}

func getChartmuseumError(b []byte, code int) error {
	var er struct {
		Error string `json:"error"`
	}
	err := json.Unmarshal(b, &er)
	if err != nil || er.Error == "" {
		return fmt.Errorf("%d: could not properly parse response JSON: %s", code, string(b))
	}
	return fmt.Errorf("%d: %s", code, er.Error)
}
