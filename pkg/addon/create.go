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
	"github.com/fatih/color"
	"os"
	"path"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/encoding/gocode/gocodec"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// CreateAddonFromHelmChart creates an addon scaffold from a Helm Chart, with a Helm component inside
func CreateAddonFromHelmChart(addonName, addonPath, helmRepoURL, chartName, chartVersion string) error {
	if len(addonName) == 0 || len(helmRepoURL) == 0 || len(chartName) == 0 || len(chartVersion) == 0 {
		return fmt.Errorf("addon addonPath, helm URL, chart name, and chart verion should not be empty")
	}

	// Currently, we do not check whether the Helm Chart actually exists, because it is just a scaffold.
	// The user can still edit it after creation.
	// Also, if the user is offline, we cannot check whether the Helm Chart exists.
	// TODO(charlie0129): check whether the Helm Chart exists (if the user wants)

	// Make sure url is valid
	isValidURL := utils.IsValidURL(helmRepoURL)
	if !isValidURL {
		return fmt.Errorf("invalid helm repo url")
	}

	err := preAddonCreation(addonName, addonPath)
	if err != nil {
		return err
	}

	// Create files like template.yaml, README.md, and etc.
	err = createFilesFromHelmChart(addonName, addonPath, helmRepoURL, chartName, chartVersion)
	if err != nil {
		return fmt.Errorf("cannot create addon files: %w", err)
	}

	postAddonCreation(addonName, addonPath)

	return nil
}

// CreateAddonSample creates an empty addon scaffold, with some required files
func CreateAddonSample(addonName, addonPath string) error {
	if len(addonName) == 0 || len(addonPath) == 0 {
		return fmt.Errorf("addon name and addon path should not be empty")
	}

	err := preAddonCreation(addonName, addonPath)
	if err != nil {
		return err
	}

	err = createSampleFiles(addonName, addonPath)
	if err != nil {
		return err
	}

	postAddonCreation(addonName, addonPath)

	return nil
}

// preAddonCreation is executed before creating an addon scaffold
// It makes sure that user-provided info is valid.
func preAddonCreation(addonName, addonPath string) error {
	if len(addonName) == 0 || len(addonPath) == 0 {
		return fmt.Errorf("addon name and addonPath should not be empty")
	}

	// Make sure addon name is valid
	err := CheckAddonName(addonName)
	if err != nil {
		return err
	}

	// Create dirs
	err = createAddonDirs(addonPath)
	if err != nil {
		return fmt.Errorf("cannot create addon structure: %w", err)
	}

	return nil
}

// postAddonCreation is after before creating an addon scaffold
// It prints some instructions to get started.
func postAddonCreation(addonName, addonPath string) {
	fmt.Println("Scaffold created in directory " +
		color.New(color.Bold).Sprint(addonPath) + ". What to do next:\n" +
		"- Check out our guide on how to build your own addon: " +
		color.BlueString("https://kubevela.io/docs/platform-engineers/addon/intro") + "\n" +
		"- Review and edit what we have generated in " + color.New(color.Bold).Sprint(addonPath) + "\n" +
		"- To enable the addon, run: " +
		color.New(color.FgGreen).Sprint("vela") + color.GreenString(" addon enable ") + color.New(color.Bold, color.FgGreen).Sprint(addonPath))
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

// createFilesFromHelmChart creates the file structure for a Helm Chart addon,
// including template.yaml, readme.md, metadata.yaml, and <addon-nam>.cue.
func createFilesFromHelmChart(addonName, addonPath, helmRepoURL, chartName, chartVersion string) error {
	// Generate template.yaml with an empty Application
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

	// Generate metadata.yaml with `fluxcd` as a dependency because we are using helm.
	// However, this may change in the future, possibly with `argocd`.
	metadataTemplate := Meta{
		Name:         addonName,
		Version:      chartVersion,
		Description:  "An addon for KubeVela.",
		Tags:         []string{chartVersion},
		Dependencies: []*Dependency{{Name: "fluxcd"}},
	}
	metadataTemplateBytes, err := yaml.Marshal(metadataTemplate)
	if err != nil {
		return err
	}

	// Write template.yaml, readme.md, and metadata.yaml
	err = writeRequiredFiles(addonPath,
		applicationTemplateBytes,
		[]byte(strings.ReplaceAll(readmeTemplate, "ADDON_NAME", addonName)),
		metadataTemplateBytes)
	if err != nil {
		return err
	}

	// Write addonName.cue, containing the helm chart
	addonResourcePath := path.Join(addonPath, ResourcesDirName, addonName+".cue")
	resourceTmpl := HelmCUETemplate{}
	resourceTmpl.Output.Type = "helm"
	resourceTmpl.Output.Properties.RepoType = "helm"
	resourceTmpl.Output.Properties.URL = helmRepoURL
	resourceTmpl.Output.Properties.Chart = chartName
	resourceTmpl.Output.Properties.Version = chartVersion
	err = writeHelmCUETemplate(resourceTmpl, addonResourcePath)
	if err != nil {
		return err
	}

	return nil
}

// createSampleFiles creates the file structure for an empty addon
func createSampleFiles(addonName, addonPath string) error {
	// Generate metadata.yaml
	metadataTemplate := Meta{
		Name:         addonName,
		Version:      "1.0.0",
		Description:  "An addon for KubeVela.",
		Tags:         []string{},
		Dependencies: []*Dependency{},
	}
	metadataTemplateBytes, err := yaml.Marshal(metadataTemplate)
	if err != nil {
		return err
	}

	// Generate template.yaml
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

	err = writeRequiredFiles(addonPath,
		applicationTemplateBytes,
		[]byte(strings.ReplaceAll(readmeTemplate, "ADDON_NAME", addonName)),
		metadataTemplateBytes)
	if err != nil {
		return err
	}

	return nil
}

// writeRequiredFiles creates required files for an addon,
// including template.yaml, readme.md, and metadata.yaml
func writeRequiredFiles(addonPath string, tmplContent, readmeContent, metadataContent []byte) error {
	// Write template.yaml
	templateFilePath := path.Join(addonPath, TemplateFileName)
	err := os.WriteFile(templateFilePath,
		tmplContent,
		0644)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", templateFilePath, err)
	}

	// Write README.md
	readmeFilePath := path.Join(addonPath, ReadmeFileName)
	err = os.WriteFile(readmeFilePath,
		readmeContent,
		0644)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", readmeFilePath, err)
	}

	// Write metadata.yaml
	metadataFilePath := path.Join(addonPath, MetadataFileName)
	err = os.WriteFile(metadataFilePath,
		metadataContent,
		0644)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", metadataFilePath, err)
	}

	return nil
}

// createAddonDirs creates the directory structure for an addon
func createAddonDirs(addonDir string) error {
	// Make sure addonDir is pointing to an empty directory, or does not exist at all
	// so that we can create it later
	_, err := os.Stat(addonDir)
	if !os.IsNotExist(err) {
		emptyDir, err := utils.IsEmptyDir(addonDir)
		if err != nil {
			return fmt.Errorf("we can't create directory %s. Make sure the name has not already been taken and you have the proper rights to write to it", addonDir)
		}

		if !emptyDir {
			return fmt.Errorf("directory %s is not empty. To avoid any data loss, please manually delete it first, then try again", addonDir)
		}

		// Now we are sure addonPath is en empty dir, delete it
		err = os.Remove(addonDir)
		if err != nil {
			return err
		}
	}

	// nolint:gosec
	err = os.MkdirAll(addonDir, 0755)
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

// writeHelmCUETemplate writes a cue, with a helm component inside, intended as addon resource
func writeHelmCUETemplate(tmpl HelmCUETemplate, filePath string) error {
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

// HelmCUETemplate is a template for a helm component .cue in an addon
type HelmCUETemplate struct {
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

const (
	readmeTemplate = "# ADDON_NAME\n" +
		"\n" +
		"This is an addon template. Check how to build your own addon: https://kubevela.net/docs/platform-engineers/addon/intro\n" +
		"\n" +
		"## Directory Structure\n" +
		"\n" +
		"- `template.yaml`: contains the basic app, you can add some component and workflow to meet your requirements. Other files in `resources/` and `definitions/` will be rendered as Components and appended in `spec.components`\n" +
		"- `metadata.yaml`: contains addon metadata information.\n" +
		"- `definitions/`: contains the X-Definition yaml/cue files. These file will be rendered as KubeVela Component in `template.yaml`\n" +
		"- `resources/`:\n" +
		"  - `parameter.cue` to expose parameters. It will be converted to JSON schema and rendered in UI forms.\n" +
		"  - All other files will be rendered as KubeVela Components. It can be one of the two types:\n" +
		"    - YAML file that contains only one resource. This will be rendered as a `raw` component\n" +
		"    - CUE template file that can read user input as `parameter.XXX` as defined `parameter.cue`.\n" +
		"      Basically the CUE template file will be combined with `parameter.cue` to render a resource.\n" +
		"      **You can specify the type and trait in this format**\n" +
		""
)
