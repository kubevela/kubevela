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

package mods

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/docgen"
)

const (
	// TraitDefRefPath is the target path for kubevela.io trait ref docs
	TraitDefRefPath = "../kubevela.io/docs/end-user/traits/references.md"
	// TraitDefRefPathZh is the target path for kubevela.io trait ref docs in Chinese
	TraitDefRefPathZh = "../kubevela.io/i18n/zh/docusaurus-plugin-content-docs/current/end-user/traits/references.md"
)

// TraitDefDirs store inner CUE definition
var TraitDefDirs = []string{"./vela-templates/definitions/internal/trait/"}

// CustomTraitHeaderEN .
var CustomTraitHeaderEN = `---
title: Built-in Trait Type
---

This documentation will walk through all the built-in trait types sorted alphabetically.

` + fmt.Sprintf("> It was generated automatically by [scripts](../../contributor/cli-ref-doc), please don't update manually, last updated at %s.\n\n", time.Now().Format(time.RFC3339))

// CustomTraitHeaderZH .
var CustomTraitHeaderZH = `---
title: 内置运维特征列表
---

本文档将**按字典序**展示所有内置运维特征的参数列表。

` + fmt.Sprintf("> 本文档由[脚本](../../contributor/cli-ref-doc)自动生成，请勿手动修改，上次更新于 %s。\n\n", time.Now().Format(time.RFC3339))

// TraitDef generate trait def reference doc
func TraitDef(ctx context.Context, c common.Args, opt Options) {
	if len(opt.DefDirs) == 0 {
		opt.DefDirs = TraitDefDirs
	}
	ref := &docgen.MarkdownReference{
		AllInOne:     true,
		ForceExample: opt.ForceExamples,
		Filter: func(capability types.Capability) bool {
			if capability.Type != types.TypeTrait || capability.Category != types.CUECategory {
				return false
			}
			if capability.Labels != nil && (capability.Labels[types.LabelDefinitionDeprecated] == "true") {
				return false
			}
			// only print capability which contained in cue def
			for _, dir := range opt.DefDirs {
				files, err := os.ReadDir(dir)
				if err != nil {
					fmt.Println("read dir err", opt.DefDirs, err)
					return false
				}
				for _, f := range files {
					if strings.Contains(f.Name(), capability.Name) {
						return true
					}
				}
			}
			return false
		},
		CustomDocHeader: CustomTraitHeaderEN,
	}
	ref.Local = &docgen.FromLocal{
		Paths: TraitDefDirs,
	}

	if opt.Path != "" {
		ref.I18N = &docgen.En
		if strings.Contains(opt.Location, "zh") || strings.Contains(opt.Location, "chinese") {
			ref.I18N = &docgen.Zh
			ref.CustomDocHeader = CustomTraitHeaderZH
		}
		if err := ref.GenerateReferenceDocs(ctx, c, opt.Path); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("trait reference docs (%s) successfully generated in %s \n", ref.I18N.Language(), opt.Path)
	} else {
		// Generate to default path depends on language
		if opt.Location == "" || opt.Location == "en" {
			ref.I18N = &docgen.En
			if err := ref.GenerateReferenceDocs(ctx, c, TraitDefRefPath); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Printf("trait reference docs (%s) successfully generated in %s \n", ref.I18N.Language(), TraitDefRefPath)
		}
		if opt.Location == "" || opt.Location == "zh" {
			ref.I18N = &docgen.Zh
			ref.CustomDocHeader = CustomTraitHeaderZH
			if err := ref.GenerateReferenceDocs(ctx, c, TraitDefRefPathZh); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Printf("trait reference docs (%s) successfully generated in %s \n", ref.I18N.Language(), TraitDefRefPathZh)
		}
	}
}
