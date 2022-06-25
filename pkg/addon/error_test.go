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

package addon

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError(t *testing.T) {
	err := &VersionUnMatchError{}
	assert.False(t, strings.Contains(err.Error(), "which is the latest version that suits current version requirements"))
	err = &VersionUnMatchError{availableVersion: "1.0.0"}
	assert.Contains(t, err.Error(), "which is the latest version that suits current version requirements")
}

func TestGetAvailableVersionTip(t *testing.T) {
	err := errors.New("fail to install velaux version of v1.2.4, because .\nInstall velaux(v1.2.1) which is the latest version that suits current version requirements")
	version, err := GetAvailableVersionTip(err)
	assert.NoError(t, err)
	assert.Equal(t, version, "1.2.1")

	err = errors.New("fail to install velaux version of v1.2.4, because ")
	version, err = GetAvailableVersionTip(err)
	assert.Error(t, err)
	assert.Equal(t, version, "")

	err = errors.New("sssss")
	version, err = GetAvailableVersionTip(err)
	assert.Error(t, err)
	assert.Equal(t, version, "")
}
