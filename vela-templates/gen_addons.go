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
	"github.com/oam-dev/kubevela/references/cli"
)

const (
	// TemplateName represents the Application template file of addons
	TemplateName = "template.yaml"

	// ApplicationFileDir is where we store generated application & component definition
	ApplicationFileDir = "auto-gen"

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

	// NameAnnotation marked the addon's name if exist, or application's name
	NameAnnotation = "addons.oam.dev/name"
)

// DefaultEnableAddons is default enabled addons
var DefaultEnableAddons = []string{""}

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
	StoreName       string
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
		if file.IsDir() && file.Name() != ApplicationFileDir {
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
		TemplatePath: filepath.Join(addonRoot, TemplateName),
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

func generateApplication(addon *AddonInfo) (*v1beta1.Application, error) {
	templatePath := strings.Split(addon.TemplatePath, "/")
	templateName := templatePath[len(templatePath)-1]
	t, err := template.New(templateName).Funcs(sprig.TxtFuncMap()).ParseFiles(addon.TemplatePath)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = t.Execute(&buf, addon)
	if err != nil {
		fmt.Println(err)
		return nil, errors.Wrapf(err, "generate Application %s fail", addon.TemplatePath)
	}

	app := new(v1beta1.Application)
	err = yaml.Unmarshal(buf.Bytes(), app)
	if err != nil {
		return nil, err
	}
	return app, err
}

func setConfigMapLabels(addonInfo *AddonInfo) map[string]string {
	return map[string]string{
		MarkLabel: addonInfo.StoreName,
	}
}
func setConfigMapAnnotations(addonInfo *AddonInfo) map[string]string {
	return map[string]string{
		NameAnnotation: addonInfo.Name,
		DescAnnotation: addonInfo.Description,
	}
}

func removeUselessInplace(s *string) {
	timeStampwithApptemplate := "appTemplate:\n(.*metadata:)?\n[ ]*creationTimestamp: null"
	re := regexp.MustCompile(timeStampwithApptemplate)
	*s = re.ReplaceAllString(*s, "appTemplate:")

	pureTimeStamp := "\n[ ]*creationTimestamp: null"
	re = regexp.MustCompile(pureTimeStamp)
	*s = re.ReplaceAllString(*s, "")

	nullProperties := "\n[ ]*properties: null"
	re = regexp.MustCompile(nullProperties)
	*s = re.ReplaceAllString(*s, "")
}

func addHelmTemplating(s *string, addonInfo *AddonInfo) {
	annotationPattern := regexp.MustCompile(`\n\s{2}annotations:\n`)
	index := annotationPattern.FindStringIndex(*s)
	camelCaseSub := regexp.MustCompile(`-([a-zA-Z0-9])`)
	camelCaseStoreName := camelCaseSub.ReplaceAllStringFunc(addonInfo.StoreName, func(s string) string { return strings.ToUpper(strings.TrimLeft(s, "-")) })
	helmTemplateSnippet := "{{- with .Values.addons." + camelCaseStoreName + ".configMapAnnotations }}\n{{- toYaml . | nindent 4 }}\n{{- end }}\n"

	newstring := *s
	newstring = newstring[:index[1]] + helmTemplateSnippet + newstring[index[1]:]

	*s = newstring
}

// storeConfigMap store configMap in helm chart
func storeConfigMap(addonInfo *AddonInfo, application *v1beta1.Application, storePath string) error {
	configMap := &corev1.ConfigMap{
		TypeMeta: v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
	}
	addonInfo.Description = application.GetAnnotations()[DescAnnotation]
	configMap.SetName(addonInfo.StoreName)
	configMap.SetNamespace(ChartTemplateNamespace)
	configMap.SetAnnotations(setConfigMapAnnotations(addonInfo))
	configMap.SetLabels(setConfigMapLabels(addonInfo))

	data := make(map[string]string, 1)
	initContent, err := yaml.Marshal(application)
	if err != nil {
		return err
	}
	data["application"] = string(initContent)
	configMap.Data = data
	content, err := yaml.Marshal(configMap)
	if err != nil {
		return err
	}
	raw := string(content)
	removeUselessInplace(&raw)
	raw = strings.ReplaceAll(raw, fmt.Sprintf("'%s'", ChartTemplateNamespace), ChartTemplateNamespace)
	addHelmTemplating(&raw, addonInfo)

	filename := storePath + "/" + addonInfo.StoreName + ".yaml"
	return WriteToFile(filename, raw)
}

// storeApplication store app in one file for apply directly
func storeApplication(app *v1beta1.Application, addonPath string, addonName string) error {
	initContent, err := yaml.Marshal(app)
	if err != nil {
		return err
	}

	filename := path.Join(addonPath, ApplicationFileDir, addonName+".yaml")
	contents := string(initContent)
	removeUselessInplace(&contents)
	return WriteToFile(filename, contents)
}

func storeDefaultAddon(app *v1beta1.Application, storePath, addonName string) error {
	app.SetNamespace(ChartTemplateNamespace)

	app.SetAnnotations(util.MergeMapOverrideWithDst(app.Annotations, map[string]string{
		"helm.sh/hook": "post-install, post-upgrade, pre-delete",
	}))

	initContent, err := yaml.Marshal(app)
	if err != nil {
		return err
	}

	filename := path.Join(storePath, addonName+".yaml")
	raw := string(initContent)
	raw = strings.ReplaceAll(raw, fmt.Sprintf("'%s'", ChartTemplateNamespace), ChartTemplateNamespace)
	removeUselessInplace(&raw)
	return WriteToFile(filename, raw)
}

func main() {
	var addonsPath string
	var configMapStorePath string
	var initStorePath string

	flag.StringVar(&addonsPath, "addons-path", "./vela-templates/addons", "addons path")
	flag.StringVar(&configMapStorePath, "store-path", "./charts/vela-core/templates/addons", "path store configMap")
	flag.StringVar(&initStorePath, "app-path", "./charts/vela-core/templates/addons-default", "path to store default addon")

	addons, err := walkAllAddons(addonsPath)
	dealErr := func(addonName string, err error) {
		if err != nil {
			fmt.Printf("%s gen_addon err:%+v", addonName, err)
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
		app, err := generateApplication(addInfo)
		dealErr(addon, err)
		setAddonName(addInfo, app)
		err = storeApplication(app, addonsPath, addInfo.StoreName)
		dealErr(addon, err)
		err = storeConfigMap(addInfo, app, configMapStorePath)
		dealErr(addon, err)
		if slices.Contains(DefaultEnableAddons, addon) {
			err = storeDefaultAddon(app, initStorePath, addon)
			dealErr(addon, err)
		}
	}
}

func setAddonName(addInfo *AddonInfo, app *v1beta1.Application) {
	var name string
	if val, ok := app.Annotations[NameAnnotation]; ok {
		name = val
	} else {
		name = app.Name
	}
	addInfo.Name = name
	addInfo.StoreName = cli.TransAddonName(name)
}
