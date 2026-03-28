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
	"strings"
	"testing"
)

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

func TestIsSSHURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"ssh scheme", "ssh://git@github.enterprise.com:org/charts.git", true},
		{"git+ssh scheme", "git+ssh://git@github.enterprise.com:org/charts.git", true},
		{"scp-like git URL", "git@github.com:org/charts.git", true},
		{"https URL", "https://charts.example.com", false},
		{"http URL", "http://charts.example.com", false},
		{"oci URL", "oci://registry.example.com/charts", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSSHURL(tt.url)
			if result != tt.expected {
				t.Errorf("IsSSHURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestLoadDataViaSSH_InvalidKey(t *testing.T) {
	// Test that loadDataViaSSH returns an error with an invalid SSH key
	cred := &RepoCredential{
		SSHPrivateKey: "not-a-valid-key",
	}
	_, err := loadDataViaSSH("ssh://git@github.com:org/charts.git/index.yaml", cred)
	if err == nil {
		t.Error("expected error with invalid SSH key, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create SSH public keys") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}
