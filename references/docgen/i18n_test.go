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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"Outputs":{"Chinese":"输出"}}`)
	}))
	defer svr.Close()
	time.Sleep(time.Millisecond)
	assert.Equal(t, En.Language(), Language("English"))
	assert.Equal(t, En.Get("nihaoha"), "nihaoha")
	assert.Equal(t, En.Get("AlibabaCloud"), "Alibaba Cloud")
	var ni *I18n
	assert.Equal(t, ni.Get("AlibabaCloud"), "Alibaba Cloud")
	assert.Equal(t, ni.Get("AlibabaCloud."), "Alibaba Cloud")
	assert.Equal(t, ni.Get("AlibabaCloud。"), "Alibaba Cloud")
	assert.Equal(t, ni.Get("AlibabaCloud。 "), "Alibaba Cloud")
	assert.Equal(t, ni.Get("AlibabaCloud 。 "), "Alibaba Cloud")
	assert.Equal(t, ni.Get("AlibabaCloud \n "), "Alibaba Cloud")
	assert.Equal(t, ni.Get(" A\n "), "A")
	assert.Equal(t, ni.Get(" \n "), "")

	assert.Equal(t, Zh.Language(), Language("Chinese"))
	assert.Equal(t, Zh.Get("nihaoha"), "nihaoha")
	assert.Equal(t, Zh.Get("AlibabaCloud"), "阿里云")

	LoadI18nData(svr.URL)
	assert.Equal(t, Zh.Get("Outputs"), "输出")
}
