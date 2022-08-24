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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/docgen"
)

const (
	// KubeVelaIOTerraformPath is the target path for kubevela.io terraform docs
	KubeVelaIOTerraformPath = "../kubevela.io/docs/end-user/components/cloud-services/terraform"
	// KubeVelaIOTerraformPathZh is the target path for kubevela.io terraform docs in Chinese
	KubeVelaIOTerraformPathZh = "../kubevela.io/i18n/zh/docusaurus-plugin-content-docs/current/end-user/components/cloud-services/terraform"
)

func main() {
	ref := &docgen.MarkdownReference{}
	ctx := context.Background()

	c, err := common.InitBaseRestConfig()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	ref.Remote = &docgen.FromCluster{Namespace: types.DefaultKubeVelaNS}
	ref.Filter = func(capability types.Capability) bool {
		if capability.Labels != nil && capability.Labels[types.LabelDefinitionHidden] == "true" {
			return false
		}
		return capability.Type == types.TypeComponentDefinition && capability.Category == types.TerraformCategory
	}

	path := flag.String("path", "", "path of output")
	location := flag.String("location", "", "path of output")
	i18nfile := flag.String("i18n", "../kubevela.io/static/reference-i18n.json", "file path of i18n data")
	flag.Parse()

	if *i18nfile != "" {
		docgen.LoadI18nData(*i18nfile)
	}

	if *path != "" {
		ref.I18N = &docgen.En
		if strings.Contains(*location, "zh") || strings.Contains(*location, "chinese") {
			ref.I18N = &docgen.Zh
		}
		if err := ref.GenerateReferenceDocs(ctx, c, *path); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("terraform reference docs (%s) successfully generated in %s \n", ref.I18N.Language(), *path)
	}
	ref.I18N = &docgen.En
	if err := ref.GenerateReferenceDocs(ctx, c, KubeVelaIOTerraformPath); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("terraform reference docs (%s) successfully generated in %s \n", ref.I18N.Language(), KubeVelaIOTerraformPath)
	ref.I18N = &docgen.Zh
	if err := ref.GenerateReferenceDocs(ctx, c, KubeVelaIOTerraformPathZh); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("terraform reference docs (%s) successfully generated in %s \n", ref.I18N.Language(), KubeVelaIOTerraformPathZh)
}
