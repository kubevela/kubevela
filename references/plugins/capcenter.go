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

package plugins

import (
	"context"
	"errors"
	"fmt"

	"net/http"
	"os"
	"path/filepath"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/utils"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

// CapCenterConfig is used to store cap center config in file
type CapCenterConfig struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Token   string `json:"token"`
}

// CenterClient defines an interface for cap center client
type CenterClient interface {
	SyncCapabilityFromCenter() error
}

// NewCenterClient create a client from type
func NewCenterClient(ctx context.Context, name, address, token string) (CenterClient, error) {
	Type, cfg, err := utils.Parse(address)
	if err != nil {
		return nil, err
	}
	switch Type {
	case utils.TypeGithub:
		return NewGithubCenter(ctx, token, name, &cfg.GithubContent)
	case utils.TypeOss:
		return NewOssCenter(fmt.Sprintf("https://%s/", cfg.BucketURL), name), nil
	default:
	}
	return nil, errors.New("we only support github as repository now")
}

// LoadRepos will load all cap center repos
func LoadRepos() ([]CapCenterConfig, error) {
	defaultRepo := CapCenterConfig{
		Name:    "default-cap-center",
		Address: "oss://registry.kubevela.net/",
	}
	config, err := system.GetRepoConfig()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Clean(config))
	if err != nil {
		if os.IsNotExist(err) {
			return []CapCenterConfig{defaultRepo}, nil
		}
		return nil, err
	}
	var repos []CapCenterConfig
	if err = yaml.Unmarshal(data, &repos); err != nil {
		return nil, err
	}
	haveDefault := false
	for _, repo := range repos {
		if repo.Address == defaultRepo.Address {
			haveDefault = true
			break
		}
	}
	if !haveDefault {
		repos = append(repos, defaultRepo)
	}
	return repos, nil
}

// StoreRepos will store cap center repo locally
func StoreRepos(repos []CapCenterConfig) error {
	config, err := system.GetRepoConfig()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(repos)
	if err != nil {
		return err
	}
	//nolint:gosec
	return os.WriteFile(config, data, 0644)
}

// ParseCapability will convert config from remote center to capability
func ParseCapability(mapper discoverymapper.DiscoveryMapper, data []byte) (types.Capability, error) {
	var obj = unstructured.Unstructured{Object: make(map[string]interface{})}
	err := yaml.Unmarshal(data, &obj.Object)
	if err != nil {
		return types.Capability{}, err
	}
	switch obj.GetKind() {
	case "ComponentDefinition":
		var cd v1beta1.ComponentDefinition
		err = yaml.Unmarshal(data, &cd)
		if err != nil {
			return types.Capability{}, err
		}
		ref, err := util.ConvertWorkloadGVK2Definition(mapper, cd.Spec.Workload.Definition)
		if err != nil {
			return types.Capability{}, err
		}
		return HandleDefinition(cd.Name, ref.Name, cd.Annotations, cd.Labels, cd.Spec.Extension, types.TypeComponentDefinition, nil, cd.Spec.Schematic)
	case "TraitDefinition":
		var td v1beta1.TraitDefinition
		err = yaml.Unmarshal(data, &td)
		if err != nil {
			return types.Capability{}, err
		}
		return HandleDefinition(td.Name, td.Spec.Reference.Name, td.Annotations, td.Labels, td.Spec.Extension, types.TypeTrait, td.Spec.AppliesToWorkloads, td.Spec.Schematic)
	case "ScopeDefinition":
		// TODO(wonderflow): support scope definition here.
	}
	return types.Capability{}, fmt.Errorf("unknown definition Type %s", obj.GetKind())
}

// NewGithubCenter will create client by github center implementation
func NewGithubCenter(ctx context.Context, token, centerName string, r *utils.GithubContent) (*GithubRegistry, error) {
	var tc *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc = oauth2.NewClient(ctx, ts)
	}
	return &GithubRegistry{client: github.NewClient(tc), cfg: r, centerName: centerName, ctx: ctx}, nil
}

// SyncCapabilityFromCenter will sync capability from github registry
// TODO(wonderflow): currently we only sync by create, we also need to delete which not exist remotely.
func (g *GithubRegistry) SyncCapabilityFromCenter() error {
	dir, err := system.GetCapCenterDir()
	if err != nil {
		return err
	}
	repoDir := filepath.Join(dir, g.centerName)
	_, _ = system.CreateIfNotExist(repoDir)
	var success int
	items, err := g.getRepoFile()
	if err != nil {
		return err
	}
	for _, item := range items {
		addon, err := item.toAddon()
		if err != nil {
			fmt.Printf("[INFO] CRD for %s not found\n", item.name)
			continue
		}
		//nolint:gosec
		err = os.WriteFile(filepath.Join(repoDir, addon.Name+".yaml"), item.data, 0644)
		if err != nil {
			fmt.Printf("write definition %s to %s err %v\n", addon.Name+".yaml", repoDir, err)
			continue
		}
		success++
	}
	fmt.Printf("successfully sync %d from %s remote center\n", success, g.centerName)
	return nil
}

// NewOssCenter will create OSS center implementation
func NewOssCenter(bucketURL string, centerName string) *OssRegistry {
	var tc http.Client
	return &OssRegistry{
		Client:     &tc,
		bucketURL:  bucketURL,
		centerName: centerName,
	}
}

// SyncCapabilityFromCenter will sync capability from oss registry
func (o *OssRegistry) SyncCapabilityFromCenter() error {
	dir, err := system.GetCapCenterDir()
	if err != nil {
		return err
	}
	repoDir := filepath.Join(dir, o.centerName)
	_, _ = system.CreateIfNotExist(repoDir)
	var success int
	items, err := o.getRegFiles()
	if err != nil {
		return err
	}
	for _, item := range items {
		addon, err := item.toAddon()
		if err != nil {
			fmt.Printf("[INFO] CRD for %s not found\n", item.name)
			continue
		}
		//nolint:gosec
		err = os.WriteFile(filepath.Join(repoDir, addon.Name+".yaml"), item.data, 0644)
		if err != nil {
			fmt.Printf("write definition %s to %s err %v\n", addon.Name+".yaml", repoDir, err)
			continue
		}
		success++
	}
	fmt.Printf("successfully sync %d from %s remote center\n", success, o.centerName)
	return nil
}
