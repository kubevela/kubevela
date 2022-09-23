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
	"context"
	j "encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// ReadRemoteOrLocalPath will read a path remote or locally
func ReadRemoteOrLocalPath(pathOrURL string, saveLocal bool) ([]byte, error) {
	var data []byte
	var err error
	fromLocalPath := false
	switch {
	case pathOrURL == "-":
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
	case IsValidURL(pathOrURL):
		data, err = common.HTTPGetWithOption(context.Background(), pathOrURL, nil)
		if err != nil {
			return nil, err
		}
	default:
		fromLocalPath = true
		data, err = os.ReadFile(filepath.Clean(pathOrURL))
		if err != nil {
			return nil, err
		}
	}
	if saveLocal && !fromLocalPath {
		if err = localSave(pathOrURL, data); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func localSave(url string, body []byte) error {
	var name string
	ext := filepath.Ext(url)
	switch ext {
	case ".json":
		name = "vela.json"
	case ".yaml", ".yml":
		name = "vela.yaml"
	default:
		if j.Valid(body) {
			name = "vela.json"
		} else {
			name = "vela.yaml"
		}
	}
	//nolint:gosec
	return os.WriteFile(name, body, 0644)
}
