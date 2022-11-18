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

package compression

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
)

type gzipCompressor struct{}

var _ compressor = &gzipCompressor{}

func (c *gzipCompressor) init() {}

func (c *gzipCompressor) compress(obj interface{}) ([]byte, error) {
	bs, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err = gz.Write(bs); err != nil {
		return nil, err
	}
	if err = gz.Flush(); err != nil {
		return nil, err
	}
	if err = gz.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (c *gzipCompressor) decompress(compressed []byte, obj interface{}) error {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return err
	}
	bs, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	return json.Unmarshal(bs, obj)
}
