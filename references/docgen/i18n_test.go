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

	"github.com/stretchr/testify/assert"
)

func TestI18n(t *testing.T) {
	t.Run("English defaults", func(t *testing.T) {
		assert.Equal(t, En.Language(), Language("English"))
		assert.Equal(t, En.Get("nihaoha"), "nihaoha")
		assert.Equal(t, En.Get("AlibabaCloud"), "Alibaba Cloud")
	})

	t.Run("Chinese defaults", func(t *testing.T) {
		assert.Equal(t, Zh.Language(), Language("Chinese"))
		assert.Equal(t, Zh.Get("nihaoha"), "nihaoha")
		assert.Equal(t, Zh.Get("AlibabaCloud"), "阿里云")
	})

	t.Run("nil receiver", func(t *testing.T) {
		var ni *I18n
		assert.Equal(t, ni.Get("AlibabaCloud"), "Alibaba Cloud")
		assert.Equal(t, ni.Get("AlibabaCloud."), "Alibaba Cloud")
		assert.Equal(t, ni.Get("AlibabaCloud。"), "Alibaba Cloud")
		assert.Equal(t, ni.Get("AlibabaCloud。 "), "Alibaba Cloud")
		assert.Equal(t, ni.Get("AlibabaCloud 。 "), "Alibaba Cloud")
		assert.Equal(t, ni.Get("AlibabaCloud \n "), "Alibaba Cloud")
		assert.Equal(t, ni.Get(" A\n "), "A")
		assert.Equal(t, ni.Get(" \n "), "")
	})

	t.Run("Get with fallback logic", func(t *testing.T) {
		// Test suffix trimming
		assert.Equal(t, "Description", En.Get("Description."))
		assert.Equal(t, "描述", Zh.Get("描述。"))

		// Test lowercase fallback (Note: this reveals a bug, as it doesn't find the capitalized key)
		assert.Equal(t, "description", En.Get("description"))
		assert.Equal(t, "description", Zh.Get("description"))
	})
}

func TestLoadI18nData(t *testing.T) {
	t.Run("Load external data", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintf(w, `{"Outputs":{"Chinese":"输出"}}`)
		}))
		defer svr.Close()
		LoadI18nData(svr.URL)
		assert.Equal(t, "输出", Zh.Get("Outputs"))
	})
}

func TestLoadI18nDataErrors(t *testing.T) {
	t.Run("http error", func(t *testing.T) {
		// Check that a non-existent key is not translated before the call
		assert.Equal(t, "TestKey", Zh.Get("TestKey"))

		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer svr.Close()
		LoadI18nData(svr.URL)

		// Assert that the key is still not translated
		assert.Equal(t, "TestKey", Zh.Get("TestKey"))
	})

	t.Run("malformed json", func(t *testing.T) {
		// Check that another non-existent key is not translated
		assert.Equal(t, "AnotherKey", Zh.Get("AnotherKey"))

		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `this-is-not-json`)
		}))
		defer svr.Close()
		LoadI18nData(svr.URL)

		// Assert that the key is still not translated
		assert.Equal(t, "AnotherKey", Zh.Get("AnotherKey"))
	})
}
