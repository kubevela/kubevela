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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

var i18nDoc = map[string]map[string]string{}

const cnComp = "../kubevela.io/i18n/zh/docusaurus-plugin-content-docs/current/end-user/components/references.md"
const enComp = "../kubevela.io/docs/end-user/components/references.md"

/*
const enTrait = "../kubevela.io/docs/end-user/traits/references.md"
const cnTrait = "../kubevela.io/i18n/zh/docusaurus-plugin-content-docs/current/end-user/traits/references.md"
const enPolicy = "../kubevela.io/docs/end-user/policies/references.md"
const cnPolicy = "../kubevela.io/i18n/zh/docusaurus-plugin-content-docs/current/end-user/policies/references.md"
const cnWorkflow = "../kubevela.io/i18n/zh/docusaurus-plugin-content-docs/current/end-user/workflow/built-in-workflow-defs.md"
const enWorkflow = "../kubevela.io/docs/end-user/workflow/built-in-workflow-defs.md"
*/

func main() {

	pathCN := flag.String("path-cn", cnComp, "specify the path of chinese reference doc.")
	pathEN := flag.String("path-en", enComp, "specify the path of english reference doc.")
	path := flag.String("path", "", "path of existing i18n json data, if specified, it will read the file and keep the old data with append only.")

	flag.Parse()

	if *path != "" {
		data, err := os.ReadFile(*path)
		if err == nil {
			err = json.Unmarshal(data, &i18nDoc)
			if err != nil {
				log.Fatalln(err)
			}
		}
	}

	paths := strings.Split(*pathEN, ";")
	var enbuff string
	for _, v := range paths {
		if strings.TrimSpace(v) == "" {
			continue
		}
		data, err := os.ReadFile(v)
		if err != nil {
			log.Fatalln(err)
		}
		enbuff += string(data) + "\n"
	}

	cnpaths := strings.Split(*pathCN, ";")
	var cnbuff string
	for _, v := range cnpaths {
		if strings.TrimSpace(v) == "" {
			continue
		}
		data, err := os.ReadFile(v)
		if err != nil {
			log.Fatalln(err)
		}
		cnbuff += string(data) + "\n"
	}

	var entable, cntable = map[string]string{}, map[string]string{}
	ens := strings.Split(enbuff, "\n")
	for _, v := range ens {
		values := strings.Split(v, "|")
		if len(values) < 4 {
			continue
		}
		var a, b = 0, 1
		if values[0] == "" {
			a, b = 1, 2
		}
		key := strings.TrimSpace(values[a])
		desc := strings.Trim(strings.TrimSpace(values[b]), ".")
		if strings.Contains(key, "----") {
			continue
		}
		if strings.TrimSpace(desc) == "" {
			continue
		}
		if len(entable[key]) > len(desc) {
			continue
		}
		entable[key] = desc
	}

	cns := strings.Split(cnbuff, "\n")
	for _, v := range cns {
		values := strings.Split(v, "|")
		if len(values) < 5 {
			continue
		}
		var a, b = 0, 1
		if values[0] == "" {
			a, b = 1, 2
		}
		key := strings.TrimSpace(values[a])
		desc := strings.Trim(strings.TrimSpace(values[b]), ".")
		if strings.Contains(key, "----") {
			continue
		}
		if strings.TrimSpace(desc) == "" {
			continue
		}
		if len(cntable[key]) > len(desc) {
			continue
		}
		cntable[key] = desc
	}

	for k, v := range entable {

		trans := i18nDoc[v]
		if trans == nil {
			trans = map[string]string{}
		}
		trans["Chinese"] = cntable[k]
		// fmt.Println("Key=", k, " | ", v, " | ", cntable[k])
		i18nDoc[v] = trans
	}
	output, err := json.MarshalIndent(i18nDoc, "", "\t")
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(string(output))
}
