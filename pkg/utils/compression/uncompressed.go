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

import "encoding/json"

type noCompressor struct{}

var _ compressor = &noCompressor{}

func (c *noCompressor) compress(obj interface{}) ([]byte, error) {
	return json.Marshal(obj)
}

func (c *noCompressor) decompress(compressed []byte, obj interface{}) error {
	return json.Unmarshal(compressed, obj)
}

func (c *noCompressor) init() {}
