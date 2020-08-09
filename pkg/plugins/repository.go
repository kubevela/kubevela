package plugins

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"
	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

//Used to store addon center config in file
type RepoConfig struct {
	Name    string `json:"repoName"`
	Address string `json:"repoAddress"`
}

var (
	RepoConfigFile = ".vela/addon_config"
	DefaultRepo    = "local"
)

type RemoteAddon struct {
	// Name MUST be xxx.yaml
	Name string `json:"name"`
	Url  string `json:"download_url"`
	Sha  string `json:"sha"`
	// Type MUST be file
	Type string `json:"type"`
}

type RemoteAddons []RemoteAddon

type Plugin struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Definition string `json:"definition"`
	Status     string `json:"status"`
	ApplesTo   string `json:"applies_to"`
}

//TODO(wonderflow): we can make default(built-in) repo configurable, then we should make default inside the answer
func LoadRepos() ([]RepoConfig, error) {
	config, err := system.GetRepoConfig()
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(config)
	if err != nil {
		if os.IsNotExist(err) {
			return []RepoConfig{}, nil
		}
		return nil, err
	}
	var repos []RepoConfig
	if err = yaml.Unmarshal(data, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

func StoreRepos(repos []RepoConfig) error {
	config, err := system.GetRepoConfig()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(repos)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(config, data, 0644)
}

func GetReposFromRemote(r RepoConfig) (RemoteAddons, error) {
	resp, err := http.Get(r.Address)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var repos RemoteAddons
	if err = json.Unmarshal(data, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

func GetDefinitionFromURL(address, syncDir string) (types.Template, error) {
	resp, err := http.Get(address)
	if err != nil {
		return types.Template{}, err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return types.Template{}, err
	}
	var obj = unstructured.Unstructured{Object: make(map[string]interface{})}
	err = yaml.Unmarshal(data, &obj.Object)
	if err != nil {
		return types.Template{}, err
	}
	switch obj.GetKind() {
	case "WorkloadDefinition":
		var rd v1alpha2.WorkloadDefinition
		err = yaml.Unmarshal(data, &rd)
		if err != nil {
			return types.Template{}, err
		}
		return HandleDefinition(rd.Name, syncDir, rd.Spec.Reference.Name, rd.Spec.Extension, types.TypeWorkload, nil)
	case "TraitDefinition":
		var td v1alpha2.TraitDefinition
		err = yaml.Unmarshal(data, &td)
		if err != nil {
			return types.Template{}, err
		}
		return HandleDefinition(td.Name, syncDir, td.Spec.Reference.Name, td.Spec.Extension, types.TypeTrait, td.Spec.AppliesToWorkloads)
	case "ScopeDefinition":
		//TODO(wonderflow): support scope definition here.
	}
	return types.Template{}, fmt.Errorf("unknown definition Type %s", obj.GetKind())
}

//TODO(wonderflow): currently we only sync by create, we also need to delete which not exist remotely.
func SyncRemoteAddons(r RepoConfig, addons RemoteAddons) error {
	dir, err := system.GetRepoDir()
	if err != nil {
		return err
	}
	repoDir := filepath.Join(dir, r.Name)
	system.StatAndCreate(repoDir)
	var tmps []types.Template
	for _, addon := range addons {
		tmp, err := GetDefinitionFromURL(addon.Url, repoDir)
		if err != nil {
			return err
		}
		tmps = append(tmps, tmp)
	}
	success := SinkTemp2Local(tmps, repoDir)
	fmt.Printf("successfully sync %d remote addons\n", success)
	return nil
}
