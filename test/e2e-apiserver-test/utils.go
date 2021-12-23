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

package e2e_apiserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
)

// CreateRequest wraps request
func CreateRequest(method string, path string, body interface{}) (*http.Response, error) {
	if body == nil {
		body = map[string]string{}
	}
	bs, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, "http://127.0.0.1:8000/api/v1"+path, bytes.NewBuffer(bs))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

// HttpRequest will request for test
func HttpRequest(req interface{}, method string, path string, body interface{}) error {
	bodyByte, err := json.Marshal(req)
	if err != nil {
		return errors.Wrap(err, "marshal request err")
	}
	httpReq, err := http.NewRequest(method, "http://127.0.0.1:8000/api/v1"+path, bytes.NewBuffer(bodyByte))
	if err != nil {
		return errors.Wrap(err, "new request err")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return errors.Wrap(err, "do request err")
	}
	defer resp.Body.Close()
	bodyData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "read request body err")
	}
	if resp.StatusCode != 200 {
		return errors.Errorf("response code is %d", resp.StatusCode)
	}
	if body != nil {
		err = json.Unmarshal(bodyData, body)
		if err != nil {
			return errors.Wrap(err, "unmarshal body err")
		}
	}
	return nil
}

// DecodeResponseBody decode response and close response
func DecodeResponseBody(resp *http.Response, err error, dst interface{}) error {
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("response code is not 200: %d", resp.StatusCode)
	}
	if resp.Body == nil {
		return fmt.Errorf("response body is nil")
	}
	err = json.NewDecoder(resp.Body).Decode(dst)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
