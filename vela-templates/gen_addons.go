/*
 Copyright 2021. The KubeVela Authors.

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

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	// InitializerTemplateName represents the Initializer template file of addons
	InitializerTemplateName = "template.yaml"

	// InitializerFileDir is where we store generated initializer & component definition
	InitializerFileDir = "auto-gen"

	// ComponentDefDir is where we store correspond componentDefinition for addon
	ComponentDefDir = "definitions"

	// ResourceDir is where we store correspond componentDefinition for addon
	ResourceDir = "resource"

	// DescAnnotation records the description of addon
	DescAnnotation = "addons.oam.dev/description"

	// MarkLabel is annotation key marks configMap as an addon
	MarkLabel = "addons.oam.dev/type"

	// ChartTemplateNamespace is placeholder for helm chart
	ChartTemplateNamespace = "{{.Values.systemDefinitionNamespace}}"
)

// DefaultEnableAddons is default enabled addons
var DefaultEnableAddons = []string{"terraform"}

type velaFile struct {
	RelativePath string
	Name         string
	Content      string
}

// AddonInfo records addon's metadata
type AddonInfo struct {
	ResourceFiles   []velaFile
	DefinitionFiles []velaFile
	HasDefs         bool
	Name            string
	Description     string
	TemplatePath    string
}

func walkAllAddons(path string) ([]string, error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	addons := make([]string, 0, len(files))
	for _, file := range files {
		if file.IsDir() && file.Name() != InitializerFileDir {
			addons = append(addons, file.Name())
		}
	}
	return addons, nil
}

func pathExist(path string) bool {
	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}

func newWalkFn(files *[]velaFile) filepath.WalkFunc {
	return func(path string, info fs.FileInfo, _ error) error {
		if info.IsDir() {
			return nil
		}
		content, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}

		obj := new(unstructured.Unstructured)
		if err = yaml.Unmarshal(content, obj); err != nil {
			return err
		}
		*files = append(*files, velaFile{
			RelativePath: path,
			Name:         obj.GetName(),
			Content:      string(content),
		})
		return nil
	}
}

func getAddonInfo(addon string, addonsPath string) (*AddonInfo, error) {
	addonRoot := filepath.Clean(addonsPath + "/" + addon)
	resourceRoot := filepath.Clean(addonRoot + "/" + ResourceDir)
	defRoot := filepath.Clean(addonRoot + "/" + ComponentDefDir)
	resourcesFiles := make([]velaFile, 0)
	defFiles := make([]velaFile, 0)
	addInfo := &AddonInfo{
		Name:         addon,
		TemplatePath: filepath.Join(addonRoot, InitializerTemplateName),
	}
	// raw resources directory
	if pathExist(resourceRoot) {
		if err := filepath.Walk(resourceRoot, newWalkFn(&resourcesFiles)); err != nil {
			return nil, err
		}
		addInfo.ResourceFiles = resourcesFiles
	}

	if pathExist(defRoot) {
		if err := filepath.Walk(defRoot, newWalkFn(&defFiles)); err != nil {
			return nil, err
		}
		addInfo.HasDefs = true
		addInfo.DefinitionFiles = defFiles
	}
	return addInfo, nil
}

// WriteToFile write files
func WriteToFile(filename string, data string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		if err = file.Close(); err != nil {
			fmt.Printf("Error closing file: %s\n", err)
		}
	}()

	_, err = io.WriteString(file, data)
	if err != nil {
		return err
	}
	return file.Sync()
}

func generateInitializer(addon *AddonInfo) (*v1beta1.Initializer, error) {
	templatePath := strings.Split(addon.TemplatePath, "/")
	templateName := templatePath[len(templatePath)-1]
	t, err := template.New(templateName).Funcs(sprig.TxtFuncMap()).ParseFiles(addon.TemplatePath)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = t.Execute(&buf, addon)
	if err != nil {
		return nil, errors.Wrapf(err, "generate Initializer %s fail", addon.Name)
	}

	init := new(v1beta1.Initializer)
	err = yaml.Unmarshal(buf.Bytes(), init)
	if err != nil {
		return nil, err
	}
	return init, err
}

func setConfigMapLabels(addonInfo *AddonInfo) map[string]string {
	return map[string]string{
		MarkLabel: addonInfo.Name,
	}
}
func setConfigMapAnnotations(addonInfo *AddonInfo) map[string]string {
	return map[string]string{
		DescAnnotation: addonInfo.Description,
	}
}
func removeTimestampInplace(s *string) {
	timeStampwithApptemplate := "appTemplate:\n(.*metadata:)?\n[ ]*creationTimestamp: null"
	re := regexp.MustCompile(timeStampwithApptemplate)
	*s = re.ReplaceAllString(*s, "appTemplate:")

	pureTimeStamp := "\n[ ]*creationTimestamp: null"
	re = regexp.MustCompile(pureTimeStamp)
	*s = re.ReplaceAllString(*s, "")
}

// storeConfigMap store configMap in helm chart
func storeConfigMap(addonInfo *AddonInfo, initializer *v1beta1.Initializer, storePath string) error {
	configMap := &corev1.ConfigMap{
		TypeMeta: v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
	}
	addonInfo.Description = initializer.GetAnnotations()[DescAnnotation]
	configMap.SetName(addonInfo.Name)
	configMap.SetNamespace(ChartTemplateNamespace)
	configMap.SetAnnotations(setConfigMapAnnotations(addonInfo))
	configMap.SetLabels(setConfigMapLabels(addonInfo))

	data := make(map[string]string, 1)
	initContent, err := yaml.Marshal(initializer)
	if err != nil {
		return err
	}
	data["initializer"] = string(initContent)
	configMap.Data = data
	content, err := yaml.Marshal(configMap)
	if err != nil {
		return err
	}
	raw := string(content)
	removeTimestampInplace(&raw)
	raw = strings.ReplaceAll(raw, fmt.Sprintf("'%s'", ChartTemplateNamespace), ChartTemplateNamespace)
	filename := storePath + "/" + addonInfo.Name + ".yaml"
	return WriteToFile(filename, raw)
}

// storeInitializer store init in one file for apply directly
func storeInitializer(init *v1beta1.Initializer, addonPath string, addonName string) error {
	initContent, err := yaml.Marshal(init)
	if err != nil {
		return err
	}

	filename := path.Join(addonPath, InitializerFileDir, addonName+".yaml")
	contents := string(initContent)
	removeTimestampInplace(&contents)
	return WriteToFile(filename, contents)
}

func storeDefaultAddon(init *v1beta1.Initializer, storePath, addonName string) error {
	init.SetNamespace(ChartTemplateNamespace)

	init.SetAnnotations(util.MergeMapOverrideWithDst(init.Annotations, map[string]string{
		"helm.sh/hook": "post-install, post-upgrade, pre-delete",
	}))

	initContent, err := yaml.Marshal(init)
	if err != nil {
		return err
	}

	filename := path.Join(storePath, addonName+".yaml")
	raw := string(initContent)
	raw = strings.ReplaceAll(raw, fmt.Sprintf("'%s'", ChartTemplateNamespace), ChartTemplateNamespace)
	removeTimestampInplace(&raw)
	return WriteToFile(filename, raw)
}
func main() {
	var addonsPath string
	var configMapStorePath string
	var initStorePath string

	flag.StringVar(&addonsPath, "addons-path", "./vela-templates/addons", "addons path")
	flag.StringVar(&configMapStorePath, "store-path", "./charts/vela-core/templates/addons", "path store configMap")
	flag.StringVar(&initStorePath, "init-path", "./charts/vela-core/templates/addons-default", "path to store default addon")

	addons, err := walkAllAddons(addonsPath)
	dealErr := func(addonName string, err error) {
		if err != nil {
			fmt.Printf("%s gen_addon err:%e", addonName, err)
			os.Exit(1)
		}
	}
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for _, addon := range addons {
		addInfo, err := getAddonInfo(addon, addonsPath)
		dealErr(addon, err)
		init, err := generateInitializer(addInfo)
		dealErr(addon, err)
		err = storeInitializer(init, addonsPath, addInfo.Name)
		dealErr(addon, err)
		err = storeConfigMap(addInfo, init, configMapStorePath)
		dealErr(addon, err)
		if slices.Contains(DefaultEnableAddons, addon) {
			err = storeDefaultAddon(init, initStorePath, addon)
			dealErr(addon, err)
		}
	}
}
