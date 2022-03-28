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
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/chart/loader"
)

// MemoryReader is async reader for memory data
type MemoryReader struct {
	Name     string
	Files    []*loader.BufferedFile
	fileData map[string]string
}

// ListAddonMeta list all metadata of helm repo registry
func (l *MemoryReader) ListAddonMeta() (map[string]SourceMeta, error) {
	metas := SourceMeta{Name: l.Name}
	for _, f := range l.Files {
		metas.Items = append(metas.Items, OSSItem{tp: "file", name: f.Name})
		if l.fileData == nil {
			l.fileData = make(map[string]string)
		}
		l.fileData[f.Name] = string(f.Data)
	}
	return map[string]SourceMeta{l.Name: metas}, nil
}

// ReadFile ready file from memory
func (l *MemoryReader) ReadFile(path string) (string, error) {
	if file, ok := l.fileData[path]; ok {
		return file, nil
	}
	return l.fileData[strings.TrimPrefix(path, l.Name+"/")], nil
}

// RelativePath calculate the relative path of one file
func (l *MemoryReader) RelativePath(item Item) string {
	if strings.HasPrefix(item.GetName(), l.Name) {
		return item.GetName()
	}
	return filepath.Join(l.Name, item.GetName())
}
