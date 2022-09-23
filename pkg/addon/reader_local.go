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

package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type localReader struct {
	dir  string
	name string
}

func (l localReader) ListAddonMeta() (map[string]SourceMeta, error) {
	metas := SourceMeta{Name: l.name}
	if err := recursiveFetchFiles(l.dir, &metas); err != nil {
		return nil, err
	}
	return map[string]SourceMeta{l.name: metas}, nil
}

func (l localReader) ReadFile(path string) (string, error) {
	path = strings.TrimPrefix(path, l.name+"/")
	// for windows
	path = strings.TrimPrefix(path, l.name+"\\")
	b, err := os.ReadFile(filepath.Clean(filepath.Join(l.dir, path)))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (l localReader) RelativePath(item Item) string {
	file := strings.TrimPrefix(item.GetPath(), filepath.Clean(l.dir))
	return filepath.Join(l.name, file)
}

func recursiveFetchFiles(path string, metas *SourceMeta) error {
	files, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.IsDir() {
			if err := recursiveFetchFiles(fmt.Sprintf("%s/%s", path, file.Name()), metas); err != nil {
				return err
			}
		} else {
			metas.Items = append(metas.Items, OSSItem{tp: "file", path: filepath.Join(path, file.Name()), name: file.Name()})
		}
	}
	return nil
}
