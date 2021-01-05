/*
Copyright 2020 The KubeVela Authors.

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

package applicationconfiguration

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

func TestCustomRevisionHook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req RevisionHookRequest
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(400)
			return
		}
		err = json.Unmarshal(data, &req)
		if err != nil {
			w.WriteHeader(401)
			return
		}
		if len(req.RelatedApps) != 1 {
			w.WriteHeader(400)
			w.Write([]byte("we should have only one relatedApps"))
			return
		}
		if req.Comp.Annotations == nil {
			req.Comp.Annotations = make(map[string]string)
		}
		req.Comp.Annotations["app-name"] = req.RelatedApps[0].Name
		req.Comp.Annotations["app-namespace"] = req.RelatedApps[0].Namespace

		newdata, err := json.Marshal(req.Comp)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
		w.Write(newdata)
	}))
	defer srv.Close()
	compHandler := ComponentHandler{
		CustomRevisionHookURL: srv.URL,
	}
	comp := &v1alpha2.Component{}
	err := compHandler.customComponentRevisionHook([]reconcile.Request{{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default1"}}}, comp)
	assert.NoError(t, err)
	assert.Equal(t, "app1", comp.Annotations["app-name"])
	assert.Equal(t, "default1", comp.Annotations["app-namespace"])

	err = compHandler.customComponentRevisionHook([]reconcile.Request{{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default1"}}, {NamespacedName: types.NamespacedName{Name: "app2", Namespace: "default2"}}}, comp)
	assert.Equal(t, err.Error(), "httpcode(400) err: we should have only one relatedApps")
}
