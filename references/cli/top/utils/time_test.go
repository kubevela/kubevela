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

package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeFormat(t *testing.T) {
	t1, err1 := time.ParseDuration("1.5h")
	assert.NoError(t, err1)
	assert.Equal(t, TimeFormat(t1), "1h30m0s")
	t2, err2 := time.ParseDuration("25h")
	assert.NoError(t, err2)
	assert.Equal(t, TimeFormat(t2), "1d1h0m0s")
	t3, err3 := time.ParseDuration("0.1h")
	assert.NoError(t, err3)
	assert.Equal(t, TimeFormat(t3), "6m0s")
	t4, err4 := time.ParseDuration("0.001h")
	assert.NoError(t, err4)
	assert.Equal(t, TimeFormat(t4), "3s")
	t5, err5 := time.ParseDuration("0.00001h")
	assert.NoError(t, err5)
	assert.Equal(t, TimeFormat(t5), "36ms")
}
