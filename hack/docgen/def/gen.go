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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/hack/docgen/def/mods"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/docgen"
)

func main() {

	ctx := context.Background()
	c, err := common.InitBaseRestConfig()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	path := flag.String("path", "", "specify the path of output")
	location := flag.String("location", "", "path of output")
	defdir := flag.String("def-dir", "", "path of definition dir")
	tp := flag.String("type", "", "choose one of the definition to print")
	i18nfile := flag.String("i18n", "../kubevela.io/static/reference-i18n.json", "file path of i18n data")
	forceExample := flag.Bool("force-example-doc", false, "example must be provided for definitions")
	flag.Parse()

	if *i18nfile != "" {
		docgen.LoadI18nData(*i18nfile)
	}

	if *tp == "" && (*defdir != "" || *path != "") {
		fmt.Println("you must specify a type with definition ref path specified ")
		os.Exit(1)
	}

	opt := mods.Options{
		Path:          *path,
		Location:      *location,
		DefDirs:       make([]string, 0),
		ForceExamples: *forceExample,
	}
	if *defdir != "" {
		opt.DefDirs = append(opt.DefDirs, *defdir)
	}

	fmt.Printf("creating docs with args path=%s, location=%s, defdir=%s, type=%s.\n", *path, *location, *defdir, *tp)
	switch types.CapType(*tp) {
	case types.TypeComponentDefinition, "component", "comp":
		mods.ComponentDef(ctx, c, opt)
	case types.TypeTrait:
		mods.TraitDef(ctx, c, opt)
	case types.TypePolicy:
		mods.PolicyDef(ctx, c, opt)
	case types.TypeWorkflowStep, "workflow", "wf":
		mods.WorkflowDef(ctx, c, opt)
	case "":
		mods.ComponentDef(ctx, c, opt)
		mods.TraitDef(ctx, c, opt)
		mods.PolicyDef(ctx, c, opt)
		mods.WorkflowDef(ctx, c, opt)
	default:
		fmt.Printf("type %s not supported\n", *tp)
		os.Exit(1)
	}

}
