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
	"encoding/json"
	"log"
	"strings"

	"github.com/oam-dev/kubevela/pkg/utils"
)

// Language is used to define the language
type Language string

var (
	// En is english, the default language
	En = I18n{lang: LangEn}
	// Zh is Chinese
	Zh = I18n{lang: LangZh}
)

const (
	// LangEn is english, the default language
	LangEn Language = "English"
	// LangZh is Chinese
	LangZh Language = "Chinese"
)

// I18n will automatically get translated data
type I18n struct {
	lang Language
}

// LoadI18nData will load i18n data for the package
func LoadI18nData(path string) {

	log.Printf("loading i18n data from %s", path)
	data, err := utils.ReadRemoteOrLocalPath(path, false)
	if err != nil {
		log.Println("ignore using the i18n data", err)
		return
	}
	var dat = map[string]map[Language]string{}
	err = json.Unmarshal(data, &dat)
	if err != nil {
		log.Println("ignore using the i18n data", err)
		return
	}

	for k, v := range dat {
		if _, ok := v[LangEn]; !ok {
			v[LangEn] = k
		}
		k = strings.ToLower(k)
		ed, ok := i18nDoc[k]
		if !ok {
			ed = map[Language]string{}
		}
		for sk, sv := range v {
			sv = strings.TrimSpace(sv)
			if sv == "" {
				continue
			}
			ed[sk] = sv
		}
		i18nDoc[k] = ed
	}
}

// Language return the language used in i18n instance
func (i *I18n) Language() Language {
	if i == nil || i.lang == "" {
		return En.Language()
	}
	return i.lang
}

func (i *I18n) trans(str string) (string, bool) {
	dd, ok := i18nDoc[str]
	if !ok {
		return str, false
	}
	data := dd[i.lang]
	if data == "" {
		return str, true
	}
	return data, true
}

// Get translate for the string
func (i *I18n) Get(str string) string {
	if i == nil || i.lang == "" {
		return En.Get(str)
	}
	if data, ok := i.trans(str); ok {
		return data
	}
	str = strings.TrimSpace(str)
	if data, ok := i.trans(str); ok {
		return data
	}
	str = strings.TrimSuffix(str, ".")
	if data, ok := i.trans(str); ok {
		return data
	}
	str = strings.TrimSuffix(str, "。")
	if data, ok := i.trans(str); ok {
		return data
	}
	raw := str
	str = strings.TrimSpace(str)
	if data, ok := i.trans(str); ok {
		return data
	}
	str = strings.ToLower(str)
	if data, ok := i.trans(str); ok {
		return data
	}
	return raw
}

// Definitions are all the words and phrases for internationalization in cli and docs
var i18nDoc = map[string]map[Language]string{
	".": {
		LangZh: "。",
		LangEn: ".",
	},
	"Description": {
		LangZh: "描述",
		LangEn: "Description",
	},
	"Scope": {
		LangZh: "适用范围",
		LangEn: "Scope",
	},
	"Examples": {
		LangZh: "示例",
		LangEn: "Examples",
	},
	"Specification": {
		LangZh: "参数说明",
		LangEn: "Specification",
	},
	"AlibabaCloud": {
		LangZh: "阿里云",
		LangEn: "Alibaba Cloud",
	},
	"AWS": {
		LangZh: "AWS",
		LangEn: "AWS",
	},
	"Azure": {
		LangZh: "Azure",
		LangEn: "Azure",
	},
	"Name": {
		LangZh: "名称",
		LangEn: "Name",
	},
	"Type": {
		LangZh: "类型",
		LangEn: "Type",
	},
	"Required": {
		LangZh: "是否必须",
		LangEn: "Required",
	},
	"Default": {
		LangZh: "默认值",
		LangEn: "Default",
	},
	"Apply To Component Types": {
		LangZh: "适用于组件类型",
		LangEn: "Apply To Component Types",
	},
}
