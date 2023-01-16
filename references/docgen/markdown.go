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

package docgen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kubevela/workflow/pkg/cue/packages"
	"github.com/pkg/errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// AllComponentTypes means trait can be applied to all component types
const AllComponentTypes = "*"

// MarkdownReference is the struct for capability information in
type MarkdownReference struct {
	Filter          func(types.Capability) bool
	AllInOne        bool
	ForceExample    bool
	CustomDocHeader string
	DiscoveryMapper discoverymapper.DiscoveryMapper
	ParseReference
}

// GenerateReferenceDocs generates reference docs
func (ref *MarkdownReference) GenerateReferenceDocs(ctx context.Context, c common.Args, baseRefPath string) error {
	caps, err := ref.getCapabilities(ctx, c)
	if err != nil {
		return err
	}
	var pd *packages.PackageDiscover
	if ref.Remote != nil {
		pd = ref.Remote.PD
	}
	if pd == nil {
		pd = func() *packages.PackageDiscover {
			rpd, err := c.GetPackageDiscover()
			if err != nil {
				klog.Error("fail to build package discover", err)
				return nil
			}
			return rpd
		}()
	}
	return ref.CreateMarkdown(ctx, caps, baseRefPath, false, pd)
}

// CreateMarkdown creates markdown based on capabilities
func (ref *MarkdownReference) CreateMarkdown(ctx context.Context, caps []types.Capability, baseRefPath string, catalog bool, pd *packages.PackageDiscover) error {

	sort.Slice(caps, func(i, j int) bool {
		return caps[i].Name < caps[j].Name
	})

	var all string
	ref.DisplayFormat = Markdown
	for _, c := range caps {
		if ref.Filter != nil && !ref.Filter(c) {
			continue
		}
		capDoc, err := ref.GenerateMarkdownForCap(ctx, c, pd, ref.AllInOne)
		if err != nil {
			return err
		}
		if baseRefPath == "" {
			fmt.Println(capDoc)
			continue
		}
		if ref.AllInOne {
			all += capDoc + "\n\n"
			continue
		}

		refPath := baseRefPath
		if catalog {
			// catalog by capability type with folder
			refPath = filepath.Join(baseRefPath, string(c.Type))
		}

		if _, err := os.Stat(refPath); err != nil && os.IsNotExist(err) {
			if err := os.MkdirAll(refPath, 0750); err != nil {
				return err
			}
		}

		refPath = strings.TrimSuffix(refPath, "/")
		fileName := fmt.Sprintf("%s.md", c.Name)
		markdownFile := filepath.Join(refPath, fileName)
		f, err := os.OpenFile(filepath.Clean(markdownFile), os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", markdownFile, err)
		}
		if err = os.Truncate(markdownFile, 0); err != nil {
			return fmt.Errorf("failed to truncate file %s: %w", markdownFile, err)
		}

		if _, err := f.WriteString(capDoc); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	if !ref.AllInOne {
		return nil
	}
	all = ref.CustomDocHeader + all
	if baseRefPath != "" {
		return os.WriteFile(baseRefPath, []byte(all), 0600)
	}
	fmt.Println(all)
	return nil
}

// GenerateMarkdownForCap will generate markdown for one capability
// nolint:gocyclo
func (ref *MarkdownReference) GenerateMarkdownForCap(ctx context.Context, c types.Capability, pd *packages.PackageDiscover, containSuffix bool) (string, error) {
	var (
		description   string
		base          string
		sample        string
		specification string
		generatedDoc  string
		baseDoc       string
		err           error
	)
	switch c.Type {
	case types.TypeWorkload, types.TypeComponentDefinition, types.TypeTrait, types.TypeWorkflowStep, types.TypePolicy:
	default:
		return "", fmt.Errorf("type(%s) of the capability(%s) is not supported for now", c.Type, c.Name)
	}

	capName := c.Name
	lang := ref.I18N
	capNameInTitle := ref.makeReadableTitle(capName)
	switch c.Category {
	case types.CUECategory:
		cueValue, err := common.GetCUEParameterValue(c.CueTemplate, pd)
		if err != nil && !errors.Is(err, cue.ErrParameterNotExist) {
			return "", fmt.Errorf("failed to retrieve `parameters` value from %s with err: %w", c.Name, err)
		}
		var defaultDepth = 0
		generatedDoc, _, err = ref.parseParameters(capName, cueValue, Specification, defaultDepth, containSuffix)
		if err != nil {
			return "", err
		}
		if c.Type == types.TypeComponentDefinition {
			var warnErr error
			baseDoc, warnErr = GetBaseResourceKinds(c.CueTemplate, pd, ref.DiscoveryMapper)
			if warnErr != nil {
				klog.Warningf("failed to get base resource kinds for %s: %v", c.Name, warnErr)
			}
		}
	case types.HelmCategory, types.KubeCategory:
		properties, _, err := ref.GenerateHelmAndKubeProperties(ctx, &c)
		if err != nil {
			return "", fmt.Errorf("failed to retrieve `parameters` value from %s with err: %w", c.Name, err)
		}
		for _, property := range properties {
			generatedDoc += ref.getParameterString("###"+property.Name, property.Parameters, types.HelmCategory)
		}
	case types.TerraformCategory:
		generatedDoc, err = ref.GenerateTerraformCapabilityPropertiesAndOutputs(c)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupport category %s from capability %s", c.Category, capName)
	}
	title := fmt.Sprintf("---\ntitle:  %s\n---", capNameInTitle)
	if ref.AllInOne {
		title = fmt.Sprintf("## %s", capNameInTitle)
	}
	sampleContent := c.Example
	if sampleContent == "" {
		sampleContent = DefinitionDocSamples[capName]
	}
	descriptionI18N := DefinitionDocDescription[capName]
	if descriptionI18N == "" {
		descriptionI18N = c.Description
	}

	parameterDoc := DefinitionDocParameters[capName]
	if parameterDoc == "" {
		if strings.TrimSpace(generatedDoc) == "" {
			generatedDoc = "This capability has no arguments."
		}
		parameterDoc = generatedDoc
	}

	var sharp = "##"
	exampleTitle := lang.Get(Examples)
	baseTitle := lang.Get(Base)
	specificationTitle := lang.Get(Specification)
	if ref.AllInOne {
		sharp = "###"
		exampleTitle += " (" + capName + ")"
		specificationTitle += " (" + capName + ")"
		baseTitle += " (" + capName + ")"
	}
	description = fmt.Sprintf("\n\n%s %s\n\n%s", sharp, lang.Get(Description), strings.TrimSpace(lang.Get(descriptionI18N)))
	if !strings.HasSuffix(description, lang.Get(".")) {
		description += lang.Get(".")
	}

	if c.Type == types.TypeWorkflowStep {
		var scopeI18N string
		switch c.Labels["custom.definition.oam.dev/scope"] {
		case "":
			scopeI18N = "This step type is valid in both Application and WorkflowRun"
		case "Application":
			scopeI18N = "This step type is only valid in Application"
		case "WorkflowRun":
			scopeI18N = "This step type is only valid in WorkflowRun"
		}
		scope := fmt.Sprintf("\n\n%s %s\n\n%s%s", sharp, lang.Get(Scope), strings.TrimSpace(lang.Get(scopeI18N)), lang.Get("."))
		description += scope
	}

	if c.Type == types.TypeTrait {

		if c.Labels[types.LabelDefinitionHidden] == "true" {
			description += "\n\n> " + lang.Get("For now this trait is hidden from the VelaUX. Available when using CLI.")
		}
		description += "\n\n### " + lang.Get("Apply To Component Types") + "\n\n"
		var applyto string
		if len(c.AppliesTo) == 1 && c.AppliesTo[0] == AllComponentTypes {
			applyto += lang.Get("All Component Types.")
		} else {
			applyto += lang.Get("Component based on the following kinds of resources:") + "\n"
			for _, ap := range c.AppliesTo {
				applyto += "- " + ap + "\n"
			}
		}
		if applyto == "" {
			applyto = lang.Get("All Component Types.")
		}
		description += applyto + "\n"
	}

	if sampleContent != "" {
		sample = fmt.Sprintf("\n\n%s %s\n\n%s", sharp, exampleTitle, sampleContent)
	} else if ref.ForceExample {
		fmt.Printf("You must provide example doc for the new added definition \"%s\", place the example doc in the /refereces/docgen/def-doc folders, for more details refer to https://kubevela.io/docs/contributor/cli-ref-doc#how-the-docs-generated", capName)
		os.Exit(1)
	}
	if c.Category == types.CUECategory && baseDoc != "" {
		base = fmt.Sprintf("\n\n%s %s\n\n%s", sharp, baseTitle, baseDoc)
	}
	specification = fmt.Sprintf("\n\n%s %s\n%s", sharp, specificationTitle, parameterDoc)

	return title + description + base + sample + specification, nil
}

func (ref *MarkdownReference) makeReadableTitle(title string) string {
	if !strings.Contains(title, "-") {
		return cases.Title(language.Und).String(title)
	}
	var name string
	provider := strings.Split(title, "-")[0]
	switch provider {
	case "alibaba":
		name = "AlibabaCloud"
	case "aws":
		name = "AWS"
	case "azure":
		name = "Azure"
	default:
		return cases.Title(language.Und).String(title)
	}
	cloudResource := strings.Replace(title, provider+"-", "", 1)
	return fmt.Sprintf("%s %s", ref.I18N.Get(name), strings.ToUpper(cloudResource))
}

// getParameterString prepares the table content for each property
func (ref *MarkdownReference) getParameterString(tableName string, parameterList []ReferenceParameter, category types.CapabilityCategory) string {
	tab := fmt.Sprintf("\n\n%s\n\n", tableName)
	if tableName == "" || tableName == Specification {
		tab = "\n\n"
	}
	tab += fmt.Sprintf(" %s | %s | %s | %s | %s \n", ref.I18N.Get("Name"), ref.I18N.Get("Description"), ref.I18N.Get("Type"), ref.I18N.Get("Required"), ref.I18N.Get("Default"))
	tab += fmt.Sprintf(" %s | %s | %s | %s | %s \n",
		strings.Repeat("-", len(ref.I18N.Get("Name"))),
		strings.Repeat("-", len(ref.I18N.Get("Description"))),
		strings.Repeat("-", len(ref.I18N.Get("Type"))),
		strings.Repeat("-", len(ref.I18N.Get("Required"))),
		strings.Repeat("-", len(ref.I18N.Get("Default"))))

	switch category {
	case types.CUECategory:
		for _, p := range parameterList {
			if !p.Ignore {
				printableDefaultValue := ref.getCUEPrintableDefaultValue(p.Default)
				tab += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, ref.prettySentence(p.Usage), ref.formatTableString(p.PrintableType), p.Required, printableDefaultValue)
			}
		}
	case types.HelmCategory:
		for _, p := range parameterList {
			printableDefaultValue := ref.getJSONPrintableDefaultValue(p.JSONType, p.Default)
			tab += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, ref.prettySentence(p.Usage), ref.formatTableString(p.PrintableType), p.Required, printableDefaultValue)
		}
	case types.KubeCategory:
		for _, p := range parameterList {
			// Kube parameter doesn't have default value
			tab += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, ref.prettySentence(p.Usage), ref.formatTableString(p.PrintableType), p.Required, "")
		}
	case types.TerraformCategory:
		// Terraform doesn't have default value
		for _, p := range parameterList {
			tab += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, ref.prettySentence(p.Usage), ref.formatTableString(p.PrintableType), p.Required, "")
		}
	default:
	}
	return tab
}

// GenerateTerraformCapabilityPropertiesAndOutputs generates Capability properties and outputs for Terraform ComponentDefinition
func (ref *MarkdownReference) GenerateTerraformCapabilityPropertiesAndOutputs(capability types.Capability) (string, error) {
	var references string

	variableTables, outputsTable, err := ref.parseTerraformCapabilityParameters(capability)
	if err != nil {
		return "", err
	}
	for _, t := range variableTables {
		references += ref.getParameterString(t.Name, t.Parameters, types.CUECategory)
	}
	for _, t := range outputsTable {
		references += ref.prepareTerraformOutputs(t.Name, t.Parameters)
	}
	return references, nil
}

// getParameterString prepares the table content for each property
func (ref *MarkdownReference) prepareTerraformOutputs(tableName string, parameterList []ReferenceParameter) string {
	if len(parameterList) == 0 {
		return ""
	}
	tfdoc := fmt.Sprintf("\n\n%s\n\n", tableName)
	if tableName == "" {
		tfdoc = "\n\n"
	}
	tfdoc += fmt.Sprintf(" %s | %s \n", ref.I18N.Get("Name"), ref.I18N.Get("Description"))
	tfdoc += " ------------ | ------------- \n"

	for _, p := range parameterList {
		tfdoc += fmt.Sprintf(" %s | %s\n", p.Name, p.Usage)
	}

	return tfdoc
}
