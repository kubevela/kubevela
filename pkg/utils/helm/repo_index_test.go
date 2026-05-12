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

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/version"
)

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

func TestLoadRepo(t *testing.T) {

	u := "https://kubevela.github.io/charts"

	ctx := context.Background()
	index, err := LoadRepoIndex(ctx, u, &RepoCredential{})
	if err != nil {
		t.Errorf("load repo failed, err: %s", err)
		t.Failed()
		return
	}

	for _, entry := range index.Entries {
		chartUrl := entry[0].URLs[0]

		if !(strings.HasPrefix(chartUrl, "https://") || strings.HasPrefix(chartUrl, "http://")) {
			chartUrl = fmt.Sprintf("%s/%s", u, chartUrl)
		}
		chartData, err := loadData(chartUrl, &RepoCredential{})
		if err != nil {
			t.Errorf("load chart data failed, err: %s", err)
			t.Failed()
		}
		_ = chartData
		break
	}

}
