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

import (
	"testing"

	"github.com/oam-dev/kubevela/pkg/definition"

	"github.com/stretchr/testify/assert"
)

func TestCompatibleTag(t *testing.T) {
	tg := map[string]string{
		definition.DefinitionAlias: "abc",
		definition.DefinitionType:  "image-registry",
		definition.ConfigCatalog:   "cata",
	}

	tgOld := map[string]string{
		definition.UserPrefix + definition.DefinitionAlias: "abc-2",
		definition.UserPrefix + definition.DefinitionType:  "image-registry-2",
		definition.UserPrefix + definition.ConfigCatalog:   "cata-2",
	}

	assert.Equal(t, DefinitionAlias(nil), "")
	assert.Equal(t, DefinitionAlias(tg), "abc")
	assert.Equal(t, DefinitionAlias(tgOld), "abc-2")

	assert.Equal(t, DefinitionType(nil), "")
	assert.Equal(t, DefinitionType(tg), "image-registry")
	assert.Equal(t, DefinitionType(tgOld), "image-registry-2")

	assert.Equal(t, ConfigCatalog(nil), "")
	assert.Equal(t, ConfigCatalog(tg), "cata")
	assert.Equal(t, ConfigCatalog(tgOld), "cata-2")

}
