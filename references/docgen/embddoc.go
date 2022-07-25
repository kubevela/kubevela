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
	"embed"
	"io/fs"
	"strings"

	"k8s.io/klog/v2"
)

//go:embed def-doc
var defdoc embed.FS

// DefinitionDocSamples stores the configuration yaml sample for capabilities
var DefinitionDocSamples = map[string]string{}

// DefinitionDocDescription stores the description for capabilities
var DefinitionDocDescription = map[string]string{}

// DefinitionDocParameters stores the parameters for capabilities, it will override the generated one
var DefinitionDocParameters = map[string]string{}

const (
	suffixSample      = ".eg.md"
	suffixParameter   = ".param.md"
	suffixDescription = ".desc.md"
)

func init() {
	err := fs.WalkDir(defdoc, "def-doc", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		switch {
		case strings.HasSuffix(d.Name(), suffixSample):
			data, err := defdoc.ReadFile(path)
			if err != nil {
				klog.ErrorS(err, "ignore this embed built-in definition sample", "path", path)
				return nil
			}
			DefinitionDocSamples[strings.TrimSuffix(d.Name(), suffixSample)] = strings.TrimSpace(string(data))
		case strings.HasSuffix(d.Name(), suffixDescription):
			data, err := defdoc.ReadFile(path)
			if err != nil {
				klog.ErrorS(err, "ignore this embed built-in definition description", "path", path)
				return nil
			}
			DefinitionDocDescription[strings.TrimSuffix(d.Name(), suffixDescription)] = strings.TrimSpace(string(data))
		case strings.HasSuffix(d.Name(), suffixParameter):
			data, err := defdoc.ReadFile(path)
			if err != nil {
				klog.ErrorS(err, "ignore this embed built-in definition parameter", "path", path)
				return nil
			}
			DefinitionDocParameters[strings.TrimSuffix(d.Name(), suffixParameter)] = strings.TrimSpace(string(data))
		}
		return nil
	})
	if err != nil {
		klog.ErrorS(err, "unable to read embed built-in definition documentation")
		return
	}
}
