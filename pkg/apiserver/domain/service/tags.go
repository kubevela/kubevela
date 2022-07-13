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

package service

import "github.com/oam-dev/kubevela/pkg/definition"

const (
	definitionAlias = "alias.config.oam.dev"
	definitionType  = "type.config.oam.dev"
	configCatalog   = "catalog.config.oam.dev"
)

// DefinitionAlias will get definitionAlias value from tags
func DefinitionAlias(tags map[string]string) string {
	if tags == nil {
		return ""
	}
	val := tags[definitionAlias]
	if val != "" {
		return val
	}
	return tags[definition.UserPrefix+definitionAlias]
}

// DefinitionType will get definitionType value from tags
func DefinitionType(tags map[string]string) string {
	if tags == nil {
		return ""
	}
	val := tags[definitionType]
	if val != "" {
		return val
	}
	return tags[definition.UserPrefix+definitionType]
}

// ConfigCatalog will get configCatalog value from tags
func ConfigCatalog(tags map[string]string) string {
	if tags == nil {
		return ""
	}
	val := tags[configCatalog]
	if val != "" {
		return val
	}
	return tags[definition.UserPrefix+configCatalog]
}
