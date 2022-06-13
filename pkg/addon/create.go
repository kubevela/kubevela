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
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/encoding/gocode/gocodec"
	"github.com/fatih/color"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// CreateAddonFromHelmChart create an addon scaffold from a Helm Chart
func CreateAddonFromHelmChart(addonPath, helmRepoURL, chartName, chartVersion string) error {
	if len(addonPath) == 0 || len(helmRepoURL) == 0 || len(chartName) == 0 || len(chartVersion) == 0 {
		return fmt.Errorf("addon path, helm URL, chart name, and chart verion should not be empty")
	}

	// Currently, we do not check whether the Helm Chart actually exists, because it is just a scaffold.
	// The user can still edit it after creation.
	// Also, if the user is offline, we cannot check whether the Helm Chart exists.

	// TODO(charlie0129): check if the Helm Chart exists (optional)

	// Extract addon name from path (using dir name)
	absPath, err := filepath.Abs(addonPath)
	if err != nil {
		return err
	}
	addonName := filepath.Base(absPath)

	// Make sure addon name is valid
	err = CheckAddonName(addonName)
	if err != nil {
		return err
	}

	// Make sure addonPath is pointing to an empty directory, or does not exist at all
	// so that we can create it later
	_, err = os.Stat(addonPath)
	if !os.IsNotExist(err) {
		emptyDir, err := utils.IsEmptyDir(addonPath)
		if err != nil {
			return fmt.Errorf("we can't create directory %s. Make sure the name has not already been taken and you have the proper rights to write to it ", addonPath)
		}

		if !emptyDir {
			return fmt.Errorf("directory %s is not empty. To avoid any data loss, manually delete it first", addonPath)
		}

		// Now we are sure addonPath is en empty dir, delete it
		err = os.Remove(addonPath)
		if err != nil {
			return err
		}
	}

	// Make sure url is valid
	isValidURL := utils.IsValidURL(helmRepoURL)
	if !isValidURL {
		return fmt.Errorf("invalid helm repo url: %w", err)
	}

	// Create dirs
	err = createAddonDirs(addonPath)
	if err != nil {
		return fmt.Errorf("cannot addon structure: %w", err)
	}

	// Create files like template.yaml, README.md, and etc.
	err = createAddonFiles(addonPath, addonName, helmRepoURL, chartName, chartVersion)
	if err != nil {
		return fmt.Errorf("cannot create addon files: %w", err)
	}

	fmt.Println("Scaffold created in directory " +
		color.New(color.Bold).Sprint(addonPath) + ". What to do next:\n" +
		"- Review and edit what we have generated in " + color.New(color.Bold).Sprint(addonPath) + "\n" +
		"- Check out our guide on how to build your own addon: " +
		color.BlueString("https://kubevela.io/docs/platform-engineers/addon/intro") + "\n" +
		"- To enable the addon, run: " +
		color.New(color.FgGreen).Sprint("vela") + color.GreenString(" addon enable ") + color.New(color.Bold, color.FgGreen).Sprint(addonPath))

	return nil
}

// CheckAddonName checks if an addon name is valid
func CheckAddonName(addonName string) error {
	if len(addonName) == 0 {
		return fmt.Errorf("addon name should not be empty")
	}

	// Make sure addonName only contains lowercase letters, dashes, and numbers, e.g. some-addon
	re := regexp.MustCompile(`^[a-z\d]+(-[a-z\d]+)*$`)
	if !re.MatchString(addonName) {
		return fmt.Errorf("addon name should only cocntain lowercase letters, dashes, and numbers, e.g. some-addon")
	}

	return nil
}

// writeHelmComponentTemplate writes a cue, with a helm component inside, intended as addon resource
func writeHelmComponentTemplate(tmpl HelmComponentTemplate, filePath string) error {
	r := cue.Runtime{}
	v, err := gocodec.New(&r, nil).Decode(tmpl)
	if err != nil {
		return err
	}

	// Use `output` value
	v = v.Lookup("output")
	// Format output
	bs, err := format.Node(v.Syntax())
	if err != nil {
		return err
	}

	// Append "output: " to the beginning of the string, like "output: {}"
	bs = append([]byte("output: "), bs...)
	err = os.WriteFile(filePath, bs, 0644)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", filePath, err)
	}

	return nil
}

// createAddonFiles creates the file structure for an addon,
// including template.yaml, readme.md, metadata.yaml, and <addon-nam>.cue.
func createAddonFiles(addonPath, addonName, helmRepoURL, chartName, chartVersion string) error {

	// TODO(charlie0129): fill out some basic info in the template for the user (read them from the chart), like chart logo, description, and readme, if the chart exists

	// Write README.md
	readmeFilePath := path.Join(addonPath, ReadmeFileName)
	err := os.WriteFile(readmeFilePath,
		[]byte(fmt.Sprintf("# %s\nInsert the README of your addon here.\n\nAlso check how to build your own addon: https://kubevela.net/docs/platform-engineers/addon/intro", addonName)),
		0644)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", readmeFilePath, err)
	}

	// Write template.yaml with an empty Application
	templateFilePath := path.Join(addonPath, TemplateFileName)
	applicationTemplate := v1beta1.Application{
		TypeMeta: v1.TypeMeta{
			APIVersion: v1beta1.SchemeGroupVersion.String(),
			Kind:       "Application",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      addonName,
			Namespace: types.DefaultKubeVelaNS,
		},
	}

	applicationTemplateBytes, err := yaml.Marshal(applicationTemplate)
	if err != nil {
		return err
	}
	err = os.WriteFile(templateFilePath,
		applicationTemplateBytes,
		0644)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", templateFilePath, err)
	}

	// Write metadata.yaml with `fluxcd` as a dependency because we are using helm.
	// However, this may change in the future, possibly with `argocd`.
	metadataFilePath := path.Join(addonPath, MetadataFileName)
	metadataTemplate := Meta{
		Name:        addonName,
		Version:     chartVersion,
		Description: "Insert the description of your addon here.",
		// Just use a dummy image with the addon name in it
		Icon:         fmt.Sprintf("https://dummyimage.com/400x400/aaaaaa/333333&text=%s", addonName), // no need to escape url
		URL:          "https://kubevela.io",
		Tags:         []string{chartVersion},
		Dependencies: []*Dependency{{Name: "fluxcd"}},
	}

	metadataTemplateBytes, err := yaml.Marshal(metadataTemplate)
	if err != nil {
		return err
	}
	err = os.WriteFile(metadataFilePath,
		metadataTemplateBytes,
		0644)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", metadataFilePath, err)
	}

	// Write addonName.cue, containing the helm chart
	addonResourcePath := path.Join(addonPath, ResourcesDirName, addonName+".cue")
	resourceTmpl := HelmComponentTemplate{}
	resourceTmpl.Output.Type = "helm"
	resourceTmpl.Output.Properties.RepoType = "helm"
	resourceTmpl.Output.Properties.URL = helmRepoURL
	resourceTmpl.Output.Properties.Chart = chartName
	resourceTmpl.Output.Properties.Version = chartVersion
	err = writeHelmComponentTemplate(resourceTmpl, addonResourcePath)
	if err != nil {
		return err
	}

	return nil
}

// createAddonDirs creates the directory structure for an addon
func createAddonDirs(addonDir string) error {
	// nolint:gosec
	err := os.MkdirAll(addonDir, 0755)
	if err != nil {
		return err
	}
	dirs := []string{
		path.Join(addonDir, ResourcesDirName),
		path.Join(addonDir, DefinitionsDirName),
		path.Join(addonDir, DefSchemaName),
	}
	for _, dir := range dirs {
		// nolint:gosec
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}

// HelmComponentTemplate is a template for a helm component .cue in an addon
type HelmComponentTemplate struct {
	Output struct {
		Type       string `json:"type"`
		Properties struct {
			RepoType string `json:"repoType"`
			URL      string `json:"url"`
			Chart    string `json:"chart"`
			Version  string `json:"version"`
		} `json:"properties"`
	} `json:"output"`
}
