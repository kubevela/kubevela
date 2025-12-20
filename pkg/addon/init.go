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
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/encoding/gocode/gocodec"
	"github.com/fatih/color"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/utils"
)

const (
	// AddonNameRegex is the regex to validate addon names
	AddonNameRegex = `^[a-z\d]+(-[a-z\d]+)*$`
	// helmComponentDependency is the dependent addon of Helm Component
	helmComponentDependency = "fluxcd"
)

// InitCmd contains the options to initialize an addon scaffold
type InitCmd struct {
	AddonName        string
	NoSamples        bool
	HelmRepoURL      string
	HelmChartName    string
	HelmChartVersion string
	Path             string
	Overwrite        bool
	RefObjURLs       []string
	// EnableGoDef enables Go-based definition scaffolding in godef/ folder
	EnableGoDef bool
	// GoDefComponents is a comma-separated list of component names to scaffold
	GoDefComponents string
	// GoDefTraits is a comma-separated list of trait names to scaffold
	GoDefTraits string
	// GoDefPolicies is a comma-separated list of policy names to scaffold
	GoDefPolicies string
	// GoDefWorkflowSteps is a comma-separated list of workflow step names to scaffold
	GoDefWorkflowSteps string
	// We use string instead of v1beta1.Application is because
	// the cue formatter is having some problems: it will keep
	// TypeMeta (instead of inlined).
	AppTmpl     string
	Metadata    Meta
	Readme      string
	Resources   []ElementFile
	Schemas     []ElementFile
	Views       []ElementFile
	Definitions []ElementFile
	// GoDefFiles contains Go definition scaffolding files
	GoDefFiles []ElementFile
}

// CreateScaffold creates an addon scaffold
func (cmd *InitCmd) CreateScaffold() error {
	var err error

	if len(cmd.AddonName) == 0 || len(cmd.Path) == 0 {
		return fmt.Errorf("addon name and path should not be empty")
	}

	err = CheckAddonName(cmd.AddonName)
	if err != nil {
		return err
	}

	err = cmd.createDirs()
	if err != nil {
		return fmt.Errorf("cannot create addon structure: %w", err)
	}
	// Delete created files if an error occurred afterwards.
	defer func() {
		if err != nil {
			_ = os.RemoveAll(cmd.Path)
		}
	}()

	cmd.createRequiredFiles()

	if cmd.HelmChartName != "" && cmd.HelmChartVersion != "" && cmd.HelmRepoURL != "" {
		klog.Info("Creating Helm component...")
		err = cmd.createHelmComponent()
		if err != nil {
			return err
		}
	}

	if len(cmd.RefObjURLs) > 0 {
		klog.Info("Creating ref-objects URL component...")
		err = cmd.createURLComponent()
		if err != nil {
			return err
		}
	}

	if !cmd.NoSamples {
		cmd.createSamples()
	}

	if cmd.EnableGoDef {
		klog.Info("Creating Go definition module scaffolding...")
		cmd.createGoDefScaffold()
	}

	err = cmd.writeFiles()
	if err != nil {
		return err
	}

	// Print some instructions to get started.
	fmt.Println("\nScaffold created in directory " +
		color.New(color.Bold).Sprint(cmd.Path) + ". What to do next:\n" +
		"- Check out our guide on how to build your own addon: " +
		color.New(color.Bold, color.FgBlue).Sprint("https://kubevela.io/docs/platform-engineers/addon/intro") + "\n" +
		"- Review and edit what we have generated in " + color.New(color.Bold).Sprint(cmd.Path) + "\n" +
		"- To enable this addon, run: " +
		color.New(color.FgGreen).Sprint("vela") + color.GreenString(" addon enable ") + color.New(color.Bold, color.FgGreen).Sprint(cmd.Path))

	return nil
}

// CheckAddonName checks if an addon name is valid
func CheckAddonName(addonName string) error {
	if len(addonName) == 0 {
		return fmt.Errorf("addon name should not be empty")
	}

	// Make sure addonName only contains lowercase letters, dashes, and numbers, e.g. some-addon
	re := regexp.MustCompile(AddonNameRegex)
	if !re.MatchString(addonName) {
		return fmt.Errorf("addon name should only cocntain lowercase letters, dashes, and numbers, e.g. some-addon")
	}

	return nil
}

// createGoDefScaffold creates Go definition module scaffolding
func (cmd *InitCmd) createGoDefScaffold() {
	// module.yaml
	cmd.GoDefFiles = append(cmd.GoDefFiles, ElementFile{
		Name: path.Join(GoDefDirName, GoDefModuleFileName),
		Data: strings.ReplaceAll(godefModuleYAMLTemplate, "ADDON_NAME", cmd.AddonName),
	})

	// go.mod
	cmd.GoDefFiles = append(cmd.GoDefFiles, ElementFile{
		Name: path.Join(GoDefDirName, "go.mod"),
		Data: strings.ReplaceAll(godefGoModTemplate, "ADDON_NAME", cmd.AddonName),
	})

	// Always create doc.go files for each definition type folder
	cmd.GoDefFiles = append(cmd.GoDefFiles, ElementFile{
		Name: path.Join(GoDefDirName, "components", "doc.go"),
		Data: godefComponentsDocTemplate,
	})
	cmd.GoDefFiles = append(cmd.GoDefFiles, ElementFile{
		Name: path.Join(GoDefDirName, "traits", "doc.go"),
		Data: godefTraitsDocTemplate,
	})
	cmd.GoDefFiles = append(cmd.GoDefFiles, ElementFile{
		Name: path.Join(GoDefDirName, "policies", "doc.go"),
		Data: godefPoliciesDocTemplate,
	})
	cmd.GoDefFiles = append(cmd.GoDefFiles, ElementFile{
		Name: path.Join(GoDefDirName, "workflowsteps", "doc.go"),
		Data: godefWorkflowStepsDocTemplate,
	})

	// Create scaffold files for specified components
	for _, name := range parseCommaSeparated(cmd.GoDefComponents) {
		cmd.GoDefFiles = append(cmd.GoDefFiles, ElementFile{
			Name: path.Join(GoDefDirName, "components", name+".go"),
			Data: generateComponentScaffold(name),
		})
	}

	// Create scaffold files for specified traits
	for _, name := range parseCommaSeparated(cmd.GoDefTraits) {
		cmd.GoDefFiles = append(cmd.GoDefFiles, ElementFile{
			Name: path.Join(GoDefDirName, "traits", name+".go"),
			Data: generateTraitScaffold(name),
		})
	}

	// Create scaffold files for specified policies
	for _, name := range parseCommaSeparated(cmd.GoDefPolicies) {
		cmd.GoDefFiles = append(cmd.GoDefFiles, ElementFile{
			Name: path.Join(GoDefDirName, "policies", name+".go"),
			Data: generatePolicyScaffold(name),
		})
	}

	// Create scaffold files for specified workflow steps
	for _, name := range parseCommaSeparated(cmd.GoDefWorkflowSteps) {
		cmd.GoDefFiles = append(cmd.GoDefFiles, ElementFile{
			Name: path.Join(GoDefDirName, "workflowsteps", name+".go"),
			Data: generateWorkflowStepScaffold(name),
		})
	}
}

// parseCommaSeparated splits a comma-separated string into trimmed parts
func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// generateComponentScaffold generates a minimal component scaffold
func generateComponentScaffold(name string) string {
	return fmt.Sprintf(`package components

import (
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

func init() {
	defkit.Register(%sComponent())
}

// %sComponent creates the %s component definition
func %sComponent() *defkit.ComponentDefinition {
	return defkit.NewComponent("%s").
		Description("TODO: Add description for %s component").
		// Add parameters here
		// WithParameter("param", defkit.String().Description("...")).
		Template(func(tpl *defkit.Template) {
			// TODO: Implement template
		})
}
`, toCamelCase(name), toCamelCase(name), name, toCamelCase(name), name, name)
}

// generateTraitScaffold generates a minimal trait scaffold
func generateTraitScaffold(name string) string {
	return fmt.Sprintf(`package traits

import (
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

func init() {
	defkit.Register(%sTrait())
}

// %sTrait creates the %s trait definition
func %sTrait() *defkit.TraitDefinition {
	return defkit.NewTrait("%s").
		Description("TODO: Add description for %s trait").
		AppliesToWorkloads("deployments.apps").
		// Add parameters here
		// WithParameter("param", defkit.String().Description("...")).
		PatchTemplate(func(tpl *defkit.PatchTemplate) {
			// TODO: Implement patch template
		})
}
`, toCamelCase(name), toCamelCase(name), name, toCamelCase(name), name, name)
}

// generatePolicyScaffold generates a minimal policy scaffold
func generatePolicyScaffold(name string) string {
	return fmt.Sprintf(`package policies

import (
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

func init() {
	defkit.Register(%sPolicy())
}

// %sPolicy creates the %s policy definition
func %sPolicy() *defkit.PolicyDefinition {
	return defkit.NewPolicy("%s").
		Description("TODO: Add description for %s policy")
		// Add parameters here
		// WithParameter("param", defkit.String().Description("...")).
}
`, toCamelCase(name), toCamelCase(name), name, toCamelCase(name), name, name)
}

// generateWorkflowStepScaffold generates a minimal workflow step scaffold
func generateWorkflowStepScaffold(name string) string {
	return fmt.Sprintf(`package workflowsteps

import (
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

func init() {
	defkit.Register(%sStep())
}

// %sStep creates the %s workflow step definition
func %sStep() *defkit.WorkflowStepDefinition {
	return defkit.NewWorkflowStep("%s").
		Description("TODO: Add description for %s workflow step")
		// Add parameters here
		// WithParameter("param", defkit.String().Description("...")).
}
`, toCamelCase(name), toCamelCase(name), name, toCamelCase(name), name, name)
}

// toCamelCase converts a kebab-case string to CamelCase
func toCamelCase(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// createSamples creates sample files
func (cmd *InitCmd) createSamples() {
	// Sample Definition mytrait.cue
	cmd.Definitions = append(cmd.Definitions, ElementFile{
		Data: traitTemplate,
		Name: "mytrait.cue",
	})
	// Sample Resource
	cmd.Resources = append(cmd.Resources, ElementFile{
		Data: resourceTemplate,
		Name: "myresource.cue",
	})
	// Sample schema
	cmd.Schemas = append(cmd.Schemas, ElementFile{
		Data: schemaTemplate,
		Name: "myschema.yaml",
	})
	// Sample View
	cmd.Views = append(cmd.Views, ElementFile{
		Data: strings.ReplaceAll(viewTemplate, "ADDON_NAME", cmd.AddonName),
		Name: "my-view.cue",
	})
}

// createRequiredFiles creates README.md, template.yaml and metadata.yaml
func (cmd *InitCmd) createRequiredFiles() {
	// README.md
	cmd.Readme = strings.ReplaceAll(readmeTemplate, "ADDON_NAME", cmd.AddonName)

	// template.cue
	cmd.AppTmpl = appTemplate

	// metadata.yaml
	cmd.Metadata = Meta{
		Name:         cmd.AddonName,
		Version:      "1.0.0",
		Description:  "An addon for KubeVela.",
		Tags:         []string{"my-tag"},
		Dependencies: []*Dependency{},
		DeployTo:     nil,
	}
}

// createHelmComponent creates a <addon-name-helm>.cue in /resources
func (cmd *InitCmd) createHelmComponent() error {
	// Make fluxcd a dependency, since it uses a helm component
	cmd.Metadata.addDependency(helmComponentDependency)
	// Make addon version same as chart version
	cmd.Metadata.Version = cmd.HelmChartVersion

	// Create a <addon-name-helm>.cue in resources
	tmpl := helmComponentTmpl{}
	tmpl.Type = "helm"
	tmpl.Properties.RepoType = "helm"
	if strings.HasPrefix(cmd.HelmRepoURL, "oci") {
		tmpl.Properties.RepoType = "oci"
	}
	tmpl.Properties.URL = cmd.HelmRepoURL
	tmpl.Properties.Chart = cmd.HelmChartName
	tmpl.Properties.Version = cmd.HelmChartVersion
	tmpl.Name = "addon-" + cmd.AddonName

	str, err := toCUEResourceString(tmpl)
	if err != nil {
		return err
	}

	cmd.Resources = append(cmd.Resources, ElementFile{
		Name: "helm.cue",
		Data: str,
	})

	return nil
}

// createURLComponent creates a ref-object component containing URLs
func (cmd *InitCmd) createURLComponent() error {
	tmpl := refObjURLTmpl{Type: "ref-objects"}

	for _, url := range cmd.RefObjURLs {
		if !utils.IsValidURL(url) {
			return fmt.Errorf("%s is not a valid url", url)
		}

		tmpl.Properties.URLs = append(tmpl.Properties.URLs, url)
	}

	str, err := toCUEResourceString(tmpl)
	if err != nil {
		return err
	}

	cmd.Resources = append(cmd.Resources, ElementFile{
		Data: str,
		Name: "from-url.cue",
	})

	return nil
}

// toCUEResourceString formats object to CUE string used in addons
// nolint:staticcheck
func toCUEResourceString(obj interface{}) (string, error) {
	v, err := gocodec.New((*cue.Runtime)(cuecontext.New()), nil).Decode(obj)
	if err != nil {
		return "", err
	}

	bs, err := format.Node(v.Syntax())
	if err != nil {
		return "", err
	}

	// Append "output: " to the beginning of the string, like "output: {}"
	bs = append([]byte("output: "), bs...)

	return string(bs), nil
}

// addDependency adds a dependency into metadata.yaml
func (m *Meta) addDependency(dep string) {
	for _, d := range m.Dependencies {
		if d.Name == dep {
			return
		}
	}

	m.Dependencies = append(m.Dependencies, &Dependency{Name: dep})
}

// createDirs creates the directory structure for an addon
func (cmd *InitCmd) createDirs() error {
	// Make sure addonDir is pointing to an empty directory, or does not exist at all
	// so that we can create it later
	_, err := os.Stat(cmd.Path)
	if !os.IsNotExist(err) {
		emptyDir, err := utils.IsEmptyDir(cmd.Path)
		if err != nil {
			return fmt.Errorf("we can't create directory %s. Make sure the name has not already been taken and you have the proper rights to write to it", cmd.Path)
		}

		if !emptyDir {
			if !cmd.Overwrite {
				return fmt.Errorf("directory %s is not empty. To avoid any data loss, please manually delete it first or use -f, then try again", cmd.Path)
			}
			klog.Warningf("Overwriting non-empty directory %s", cmd.Path)
		}

		// Now we are sure addonPath is en empty dir, (or the user want to overwrite), delete it
		err = os.RemoveAll(cmd.Path)
		if err != nil {
			return err
		}
	}

	// nolint:gosec
	err = os.MkdirAll(cmd.Path, 0755)
	if err != nil {
		return err
	}

	dirs := []string{
		path.Join(cmd.Path, ResourcesDirName),
		path.Join(cmd.Path, DefinitionsDirName),
		path.Join(cmd.Path, DefSchemaName),
		path.Join(cmd.Path, ViewDirName),
	}

	// Add godef directories if enabled
	if cmd.EnableGoDef {
		dirs = append(dirs,
			path.Join(cmd.Path, GoDefDirName),
			path.Join(cmd.Path, GoDefDirName, "components"),
			path.Join(cmd.Path, GoDefDirName, "traits"),
			path.Join(cmd.Path, GoDefDirName, "policies"),
			path.Join(cmd.Path, GoDefDirName, "workflowsteps"),
		)
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

// writeFiles writes addon to disk
func (cmd *InitCmd) writeFiles() error {
	var files []ElementFile

	files = append(files, ElementFile{
		Name: ReadmeFileName,
		Data: cmd.Readme,
	}, ElementFile{
		Data: parameterTemplate,
		Name: GlobalParameterFileName,
	})

	for _, v := range cmd.Resources {
		files = append(files, ElementFile{
			Data: v.Data,
			Name: filepath.Join(ResourcesDirName, v.Name),
		})
	}
	for _, v := range cmd.Views {
		files = append(files, ElementFile{
			Data: v.Data,
			Name: filepath.Join(ViewDirName, v.Name),
		})
	}
	for _, v := range cmd.Definitions {
		files = append(files, ElementFile{
			Data: v.Data,
			Name: filepath.Join(DefinitionsDirName, v.Name),
		})
	}
	for _, v := range cmd.Schemas {
		files = append(files, ElementFile{
			Data: v.Data,
			Name: filepath.Join(DefSchemaName, v.Name),
		})
	}

	// Add godef files (paths already include godef/ prefix)
	for _, v := range cmd.GoDefFiles {
		files = append(files, ElementFile{
			Data: v.Data,
			Name: v.Name,
		})
	}

	// Prepare template.cue
	files = append(files, ElementFile{
		Data: cmd.AppTmpl,
		Name: AppTemplateCueFileName,
	})

	// Prepare metadata.yaml
	metaBytes, err := yaml.Marshal(cmd.Metadata)
	if err != nil {
		return err
	}
	files = append(files, ElementFile{
		Data: string(metaBytes),
		Name: MetadataFileName,
	})

	// Write files
	for _, f := range files {
		err := os.WriteFile(filepath.Join(cmd.Path, f.Name), []byte(f.Data), 0600)
		if err != nil {
			return err
		}
	}

	return nil
}

// helmComponentTmpl is a template for a helm component .cue in an addon
type helmComponentTmpl struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Properties struct {
		RepoType string `json:"repoType"`
		URL      string `json:"url"`
		Chart    string `json:"chart"`
		Version  string `json:"version"`
	} `json:"properties"`
}

// refObjURLTmpl is a template for ref-objects containing URLs in an addon
type refObjURLTmpl struct {
	Type       string `json:"type"`
	Properties struct {
		URLs []string `json:"urls"`
	} `json:"properties"`
}

const (
	readmeTemplate = "# ADDON_NAME\n" +
		"\n" +
		"This is an addon template. Check how to build your own addon: https://kubevela.net/docs/platform-engineers/addon/intro\n" +
		""
	viewTemplate = `// We put VelaQL views in views directory.
//
// VelaQL(Vela Query Language) is a resource query language for KubeVela, 
// used to query status of any extended resources in application-level.
// Reference: https://kubevela.net/docs/platform-engineers/system-operation/velaql
//
// This VelaQL View queries the status of this addon.
// Use this view to query by:
//     vela ql --query 'my-view{addonName:ADDON_NAME}.status'
// You should see 'running'.

import (
	"vela/ql"
)

app: ql.#Read & {
	value: {
		kind:       "Application"
		apiVersion: "core.oam.dev/v1beta1"
		metadata: {
			name:      "addon-" + parameter.addonName
			namespace: "vela-system"
		}
	}
}

parameter: {
	addonName: *"ADDON_NAME" | string
}

status: app.value.status.status
`
	traitTemplate = `// We put Definitions in definitions directory.
// References:
// - https://kubevela.net/docs/platform-engineers/cue/definition-edit
// - https://kubevela.net/docs/platform-engineers/addon/intro#definitions-directoryoptional
"mytrait": {
	alias: "mt"
	annotations: {}
	attributes: {
		appliesToWorkloads: [
			"deployments.apps",
			"replicasets.apps",
			"statefulsets.apps",
		]
		conflictsWith: []
		podDisruptive:   false
		workloadRefPath: ""
	}
	description: "My trait description."
	labels: {}
	type: "trait"
}
template: {
	parameter: {param: ""}
	outputs: {sample: {}}
}
`
	resourceTemplate = `// We put Components in resources directory.
// References:
// - https://kubevela.net/docs/end-user/components/references
// - https://kubevela.net/docs/platform-engineers/addon/intro#resources-directoryoptional
output: {
	type: "k8s-objects"
	properties: {
		objects: [
			{
				// This creates a plain old Kubernetes namespace
				apiVersion: "v1"
				kind:       "Namespace"
				// We can use the parameter defined in parameter.cue like this.
				metadata: name: parameter.myparam
			},
		]
	}
}
`
	parameterTemplate = `// parameter.cue is used to store addon parameters.
//
// You can use these parameters in template.cue or in resources/ by 'parameter.myparam'
//
// For example, you can use parameters to allow the user to customize
// container images, ports, and etc.
parameter: {
	// +usage=Custom parameter description
	myparam: *"myns" | string
}
`
	schemaTemplate = `# We put UI Schemas that correspond to Definitions in schemas directory.
# References:
# - https://kubevela.net/docs/platform-engineers/addon/intro#schemas-directoryoptional
# - https://kubevela.net/docs/reference/ui-schema
- jsonKey: myparam
  label: MyParam
  validate:
    required: true
`
	appTemplate = `package main
output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	spec: {
		components: []
		policies: []
	}
}
`

	// Go definition templates
	godefModuleYAMLTemplate = `apiVersion: defkit.oam.dev/v1
kind: DefinitionModule
metadata:
  name: ADDON_NAME
spec:
  description: Go-based definitions for ADDON_NAME addon
`

	godefGoModTemplate = `module github.com/my-org/ADDON_NAME/godef

go 1.23

require github.com/oam-dev/kubevela v1.10.0
`

	// doc.go templates for each definition type
	godefComponentsDocTemplate = `// Package components contains KubeVela ComponentDefinition implementations.
// Components define the types of workloads that can be deployed.
//
// To create a new component:
//
//	func init() {
//	    defkit.Register(myComponent())
//	}
//
//	func myComponent() *defkit.ComponentDefinition {
//	    return defkit.NewComponent("my-component").
//	        Description("My component description").
//	        WithParameter("image", defkit.String().Required()).
//	        Template(func(tpl *defkit.Template) {
//	            // Generate Kubernetes resources
//	        })
//	}
package components
`

	godefTraitsDocTemplate = `// Package traits contains KubeVela TraitDefinition implementations.
// Traits modify or enhance components with additional capabilities.
//
// To create a new trait:
//
//	func init() {
//	    defkit.Register(myTrait())
//	}
//
//	func myTrait() *defkit.TraitDefinition {
//	    return defkit.NewTrait("my-trait").
//	        Description("My trait description").
//	        AppliesToWorkloads("deployments.apps").
//	        WithParameter("replicas", defkit.Int().Required()).
//	        PatchTemplate(func(tpl *defkit.PatchTemplate) {
//	            // Patch the component output
//	        })
//	}
package traits
`

	godefPoliciesDocTemplate = `// Package policies contains KubeVela PolicyDefinition implementations.
// Policies define application-level behaviors like topology and override rules.
//
// To create a new policy:
//
//	func init() {
//	    defkit.Register(myPolicy())
//	}
//
//	func myPolicy() *defkit.PolicyDefinition {
//	    return defkit.NewPolicy("my-policy").
//	        Description("My policy description").
//	        WithParameter("clusters", defkit.StringArray().Required())
//	}
package policies
`

	godefWorkflowStepsDocTemplate = `// Package workflowsteps contains KubeVela WorkflowStepDefinition implementations.
// Workflow steps define actions in the application delivery workflow.
//
// To create a new workflow step:
//
//	func init() {
//	    defkit.Register(myStep())
//	}
//
//	func myStep() *defkit.WorkflowStepDefinition {
//	    return defkit.NewWorkflowStep("my-step").
//	        Description("My workflow step description").
//	        WithParameter("message", defkit.String().Required())
//	}
package workflowsteps
`
)
