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

package compression

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestZstdCompression(t *testing.T) {
	obj := v1.ConfigMap{
		Data: map[string]string{"1234": "5678"},
	}

	str, err := ZstdObjectToString(obj)
	assert.NoError(t, err)
	objOut := v1.ConfigMap{}
	err = UnZstdStringToObject(str, &objOut)
	assert.NoError(t, err)
	assert.Equal(t, obj, objOut)

	// Invalid obj
	_, err = ZstdObjectToString(math.Inf(1))
	assert.Error(t, err)

	// Invalid base64 string
	err = UnZstdStringToObject(".dew;.3234", &objOut)
	assert.Error(t, err)

	// Invalid zstd binary data
	err = UnZstdStringToObject("MTIzNDUK", &objOut)
	assert.Error(t, err)
}
