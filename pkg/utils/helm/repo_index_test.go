/*
Copyright 2023 The KubeVela Authors.

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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/utils/ptr"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/version"
)

func TestHelmIndexUserAgent(t *testing.T) {
	want := types.KubeVelaName + "/" + version.GitRevision
	if got := helmIndexUserAgent(); got != want {
		t.Fatalf("helmIndexUserAgent() = %q, want %q", got, want)
	}
}

func TestLoadIndex(t *testing.T) {
	t.Run("valid index", func(t *testing.T) {
		idx, err := loadIndex([]byte("apiVersion: v1\nentries: {}\n"))
		if err != nil {
			t.Fatalf("loadIndex: %v", err)
		}
		if idx.APIVersion != "v1" {
			t.Fatalf("APIVersion = %q, want v1", idx.APIVersion)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		_, err := loadIndex([]byte("not: [valid: yaml"))
		if err == nil {
			t.Fatal("expected unmarshal error")
		}
	})

	t.Run("missing apiVersion", func(t *testing.T) {
		_, err := loadIndex([]byte("entries: {}\n"))
		if err == nil {
			t.Fatal("expected ErrNoAPIVersion")
		}
	})
}

func TestLoadData(t *testing.T) {
	t.Run("invalid url", func(t *testing.T) {
		_, err := loadData("://bad-url", &RepoCredential{})
		if err == nil {
			t.Fatal("expected error for invalid URL")
		}
	})

	t.Run("http error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "fail", http.StatusInternalServerError)
		}))
		defer ts.Close()

		_, err := loadData(ts.URL, &RepoCredential{})
		if err == nil {
			t.Fatal("expected error for non-200 response")
		}
	})

	t.Run("insecure skip tls verify false", func(t *testing.T) {
		var gotUA string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
			_, _ = w.Write([]byte("ok"))
		}))
		defer ts.Close()

		_, err := loadData(ts.URL, &RepoCredential{InsecureSkipTLSVerify: ptr.To(false)})
		if err != nil {
			t.Fatalf("loadData: %v", err)
		}
		want := types.KubeVelaName + "/" + version.GitRevision
		if gotUA != want {
			t.Fatalf("User-Agent = %q, want %q", gotUA, want)
		}
	})
}

func TestLoadRepoIndex_SetsUserAgent(t *testing.T) {
	var gotUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+IndexYaml {
			http.NotFound(w, r)
			return
		}
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte("apiVersion: v1\nentries: {}\n"))
	}))
	defer ts.Close()

	_, err := LoadRepoIndex(context.Background(), ts.URL+"/", &RepoCredential{})
	if err != nil {
		t.Fatalf("LoadRepoIndex: %v", err)
	}
	want := types.KubeVelaName + "/" + version.GitRevision
	if gotUA != want {
		t.Fatalf("User-Agent header = %q, want %q", gotUA, want)
	}
}

func TestLoadRepoIndex_URLWithoutTrailingSlash(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+IndexYaml {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("apiVersion: v1\nentries: {}\n"))
	}))
	defer ts.Close()

	// URL without trailing slash exercises the branch that appends /index.yaml.
	_, err := LoadRepoIndex(context.Background(), ts.URL, &RepoCredential{})
	if err != nil {
		t.Fatalf("LoadRepoIndex: %v", err)
	}
}

func TestLoadRepoIndex_relativeChartURL(t *testing.T) {
	var chartUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + IndexYaml:
			_, _ = w.Write([]byte(`apiVersion: v1
entries:
  demo:
  - urls:
    - charts/demo-1.0.0.tgz
`))
		case "/charts/demo-1.0.0.tgz":
			chartUA = r.Header.Get("User-Agent")
			_, _ = w.Write([]byte("chart-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	base := ts.URL + "/"
	index, err := LoadRepoIndex(context.Background(), base, &RepoCredential{})
	if err != nil {
		t.Fatalf("LoadRepoIndex: %v", err)
	}

	for _, entry := range index.Entries {
		chartURL := entry[0].URLs[0]
		if !(strings.HasPrefix(chartURL, "https://") || strings.HasPrefix(chartURL, "http://")) {
			chartURL = fmt.Sprintf("%s/%s", strings.TrimSuffix(base, "/"), chartURL)
		}
		if _, err := loadData(chartURL, &RepoCredential{}); err != nil {
			t.Fatalf("loadData chart: %v", err)
		}
		want := types.KubeVelaName + "/" + version.GitRevision
		if chartUA != want {
			t.Fatalf("chart User-Agent = %q, want %q", chartUA, want)
		}
		return
	}
	t.Fatal("expected at least one chart entry in index")
}

func TestLoadRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in short mode")
	}

	u := "https://kubevela.github.io/charts"

	ctx := context.Background()
	index, err := LoadRepoIndex(ctx, u, &RepoCredential{})
	if err != nil {
		t.Errorf("load repo failed, err: %s", err)
		t.Failed()
		return
	}

	for _, entry := range index.Entries {
		chartURL := entry[0].URLs[0]

		if !(strings.HasPrefix(chartURL, "https://") || strings.HasPrefix(chartURL, "http://")) {
			chartURL = fmt.Sprintf("%s/%s", u, chartURL)
		}
		chartData, err := loadData(chartURL, &RepoCredential{})
		if err != nil {
			t.Errorf("load chart data failed, err: %s", err)
			t.Failed()
		}
		_ = chartData
		break
	}
}
