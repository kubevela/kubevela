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

package applicationconfiguration

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

var RevisionHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	fmt.Println("got request from", req.Comp.Name)

	if len(req.RelatedApps) != 1 {
		var abc []string
		for _, v := range req.RelatedApps {
			abc = append(abc, v.Name)
		}
		// we can add a check here for real world handler
		fmt.Printf("we should have only one relatedApps, but now %d: %s\n", len(req.RelatedApps), strings.Join(abc, ", "))
	}
	if req.Comp.Annotations == nil {
		req.Comp.Annotations = make(map[string]string)
	}
	if len(req.RelatedApps) > 0 {
		req.Comp.Annotations["app-name"] = req.RelatedApps[0].Name
		req.Comp.Annotations["app-namespace"] = req.RelatedApps[0].Namespace
	}
	a := &unstructured.Unstructured{}
	err = json.Unmarshal(req.Comp.Spec.Workload.Raw, a)
	fmt.Println("XX:", err)
	a.SetAnnotations(map[string]string{"time": time.Now().Format(time.RFC3339Nano)})
	data, _ = json.Marshal(a)
	req.Comp.Spec.Workload.Raw = data
	newdata, err := json.Marshal(req.Comp)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(200)
	w.Write(newdata)
})

func TestCustomRevisionHook(t *testing.T) {
	srv := httptest.NewServer(RevisionHandler)
	defer srv.Close()
	compHandler := ComponentHandler{
		CustomRevisionHookURL: srv.URL,
	}
	comp := &v1alpha2.Component{}
	err := compHandler.customComponentRevisionHook([]reconcile.Request{{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default1"}}}, comp)
	assert.NoError(t, err)
	assert.Equal(t, "app1", comp.Annotations["app-name"])
	assert.Equal(t, "default1", comp.Annotations["app-namespace"])
}
