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

package rollout

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
)

// issue an http call to the an end ponit
func makeHTTPRequest(ctx context.Context, webhookEndPoint, method string, payload interface{}) ([]byte, int, error) {
	payloadBin, err := json.Marshal(payload)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	hook, err := url.Parse(webhookEndPoint)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	req, err := http.NewRequestWithContext(context.Background(), method, hook.String(), bytes.NewBuffer(payloadBin))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	req.Header.Set("Content-Type", "application/json")

	// issue request with retry
	var r *http.Response
	var body []byte
	err = retry.OnError(retry.DefaultBackoff,
		func(error) bool {
			// not sure what not to retry on
			return true
		}, func() error {
			var requestErr error
			r, requestErr = http.DefaultClient.Do(req.WithContext(ctx))
			defer func() {
				if r != nil {
					_ = r.Body.Close()
				}
			}()
			if requestErr != nil {
				return requestErr
			}
			body, requestErr = ioutil.ReadAll(r.Body)
			if requestErr != nil {
				return requestErr
			}
			if r.StatusCode >= http.StatusInternalServerError {
				requestErr = fmt.Errorf("internal server error, status code = %d", r.StatusCode)
			}
			return requestErr
		})

	// failed even with retry
	if err != nil {
		if r != nil {
			return nil, r.StatusCode, err
		}
		return nil, -1, err
	}
	return body, r.StatusCode, nil
}

// callWebhook does a HTTP POST to an external service and
// returns an error if the response status code is non-2xx
func callWebhook(ctx context.Context, resource klog.KMetadata, phase string, rw v1alpha1.RolloutWebhook) error {
	payload := v1alpha1.RolloutWebhookPayload{
		Name:      resource.GetName(),
		Namespace: resource.GetNamespace(),
		Phase:     phase,
	}

	if rw.Metadata != nil {
		payload.Metadata = *rw.Metadata
	}
	// make the http request
	if len(rw.Method) == 0 {
		rw.Method = http.MethodPost
	}
	_, status, err := makeHTTPRequest(ctx, rw.URL, rw.Method, payload)
	if err != nil {
		return err
	}
	if len(rw.ExpectedStatus) == 0 {
		if status > http.StatusAccepted {
			err := fmt.Errorf("we fail the webhook request based on status, http status = %d", status)
			return err
		}
		return nil
	}
	// check if the returned status is expected
	accepted := false
	for _, es := range rw.ExpectedStatus {
		if es == status {
			accepted = true
			break
		}
	}
	if !accepted {
		err := fmt.Errorf("http request to the webhook not accepeted, http status = %d", status)
		klog.V(common.LogDebug).InfoS("the status is not expected", "expected status", rw.ExpectedStatus)
		return err
	}
	return nil
}
