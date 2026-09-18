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

package service

import (
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// mapFS is a minimal read-only in-memory fs.FS built from files keyed by
// slash-separated path relative to the module root. It synthesizes the
// directories implied by those keys, which is all ParseModule needs
// (fs.ReadFile + fs.ReadDir). Both fetch adapters (git and OCI) build one.
type mapFS map[string][]byte

var (
	_ fs.FS         = mapFS(nil)
	_ fs.ReadFileFS = mapFS(nil)
	_ fs.ReadDirFS  = mapFS(nil)
)

func (m mapFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrInvalid}
	}
	data, ok := m[name]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

func (m mapFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	prefix := ""
	if name != "." {
		prefix = name + "/"
	}
	seen := map[string]bool{}
	var entries []fs.DirEntry
	for p := range m {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := p[len(prefix):]
		if rest == "" {
			continue
		}
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			child := rest[:i]
			if !seen[child] {
				seen[child] = true
				entries = append(entries, mapDirEntry{name: child, dir: true})
			}
			continue
		}
		if !seen[rest] {
			seen[rest] = true
			entries = append(entries, mapDirEntry{name: rest, dir: false})
		}
	}
	if len(entries) == 0 && name != "." {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (m mapFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	data, ok := m[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &openMapFile{name: path.Base(name), data: data}, nil
}

type mapDirEntry struct {
	name string
	dir  bool
}

func (e mapDirEntry) Name() string { return e.name }
func (e mapDirEntry) IsDir() bool  { return e.dir }
func (e mapDirEntry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return 0
}
func (e mapDirEntry) Info() (fs.FileInfo, error) {
	return mapFileInfo{name: e.name, dir: e.dir}, nil
}

type openMapFile struct {
	name string
	data []byte
	off  int
}

func (f *openMapFile) Stat() (fs.FileInfo, error) {
	return mapFileInfo{name: f.name, size: int64(len(f.data))}, nil
}
func (f *openMapFile) Read(b []byte) (int, error) {
	if f.off >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(b, f.data[f.off:])
	f.off += n
	return n, nil
}
func (f *openMapFile) Close() error { return nil }

type mapFileInfo struct {
	name string
	size int64
	dir  bool
}

func (i mapFileInfo) Name() string { return i.name }
func (i mapFileInfo) Size() int64  { return i.size }
func (i mapFileInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir
	}
	return 0
}
func (i mapFileInfo) ModTime() time.Time { return time.Time{} }
func (i mapFileInfo) IsDir() bool        { return i.dir }
func (i mapFileInfo) Sys() interface{}   { return nil }
