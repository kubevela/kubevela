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
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

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
	if path == "-" {
		bs, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to get data from stdin: %w", err)
		}
		return []FileData{{Path: path, Data: bs}}, nil
	}

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
				bs, e := os.ReadFile(filepath.Clean(path))
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
	bs, e := os.ReadFile(filepath.Clean(path))
	if e != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	return []FileData{{Path: path, Data: bs}}, nil
}

// IsJSONYAMLorCUEFile check if the path is a json or yaml file
func IsJSONYAMLorCUEFile(path string) bool {
	return strings.HasSuffix(path, ".json") ||
		strings.HasSuffix(path, ".yaml") ||
		strings.HasSuffix(path, ".yml") ||
		strings.HasSuffix(path, ".cue")
}

// IsCUEFile check if the path is a cue file
func IsCUEFile(path string) bool {
	return strings.HasSuffix(path, ".cue")
}

// IsEmptyDir checks if a given path is an empty directory
func IsEmptyDir(path string) (bool, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return false, err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	// Read just one file in the dir (just read names, which is faster)
	_, err = f.Readdirnames(1)
	// If the error is EOF, the dir is empty
	if errors.Is(err, io.EOF) {
		return true, nil
	}

	return false, err
}

// GetFilenameFromLocalOrRemote returns the filename of a local path or a URL.
// It doesn't guarantee that the file or URL actually exists.
func GetFilenameFromLocalOrRemote(path string) (string, error) {
	if !IsValidURL(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		return strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs)), nil
	}

	u, err := url.Parse(path)
	if err != nil {
		return "", err
	}

	return strings.TrimSuffix(filepath.Base(u.Path), filepath.Ext(u.Path)), nil
}
