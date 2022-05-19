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
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// FileData data of a file at the path
type FileData struct {
	Path string
	Data []byte
}

// LoadDataFromPath load FileData from path
// If path is an url, fetch the data from the url
// If path is a file, fetch the data from the file
// If path is a dir, fetch the data from all the files inside the dir that passes the pathFilter
func LoadDataFromPath(ctx context.Context, path string, pathFilter func(string) bool) ([]FileData, error) {
	if IsValidURL(path) {
		bs, err := common.HTTPGetWithOption(ctx, path, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get data from %s: %w", path, err)
		}
		return []FileData{{Path: path, Data: bs}}, nil
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info for %s: %w", path, err)
	}
	if fileInfo.IsDir() {
		var data []FileData
		err = filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
			if pathFilter == nil || pathFilter(path) {
				bs, e := ioutil.ReadFile(filepath.Clean(path))
				if e != nil {
					return e
				}
				data = append(data, FileData{Path: path, Data: bs})
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to traverse directory %s: %w", path, err)
		}
		return data, nil
	}
	bs, e := ioutil.ReadFile(filepath.Clean(path))
	if e != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	return []FileData{{Path: path, Data: bs}}, nil
}

// IsJSONOrYAMLFile check if the path is a json or yaml file
func IsJSONOrYAMLFile(path string) bool {
	return strings.HasSuffix(path, ".json") ||
		strings.HasSuffix(path, ".yaml") ||
		strings.HasSuffix(path, ".yml")
}
