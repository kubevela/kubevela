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
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/server/util"
)

// RevisionHookRequest is request body for custom component revision hook
type RevisionHookRequest struct {
	RelatedApps []reconcile.Request `json:"relatedApps"`
	Comp        *v1alpha2.Component `json:"component"`
}

func (c *ComponentHandler) customComponentRevisionHook(relatedApps []reconcile.Request, comp *v1alpha2.Component) error {
	if c.CustomWebHookURL == "" {
		return nil
	}
	req := RevisionHookRequest{
		RelatedApps: relatedApps,
		Comp:        comp.DeepCopy(),
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpRequest, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.CustomWebHookURL, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", util.ContentTypeJSON)
	resp, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return err
	}
	//nolint:errcheck
	defer resp.Body.Close()
	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(respData, comp)
}
