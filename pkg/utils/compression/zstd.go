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
	"encoding/base64"
	"encoding/json"

	"github.com/klauspost/compress/zstd"
)

// Create a writer that caches compressors.
// For this operation type we supply a nil Reader.
var encoder, _ = zstd.NewWriter(nil)

// Create a reader that caches decompressors.
var decoder, _ = zstd.NewReader(nil)

func compress(src []byte) []byte {
	return encoder.EncodeAll(src, make([]byte, 0, len(src)))
}

func decompress(src []byte) ([]byte, error) {
	return decoder.DecodeAll(src, nil)
}

// ZstdObjectToString marshals the object into json, compress it with zstd,
// encode the result with base64.
func ZstdObjectToString(obj interface{}) (string, error) {
	bs, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}

	compressedBytes := compress(bs)

	return base64.StdEncoding.EncodeToString(compressedBytes), nil
}

// UnZstdStringToObject decodes the compressed string with base64,
// decompresses it with zstd, and unmarshals it. obj must be a pointer so that
// it can be updated.
func UnZstdStringToObject(encoded string, obj interface{}) error {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}

	decompressed, err := decompress(decoded)
	if err != nil {
		return err
	}

	return json.Unmarshal(decompressed, obj)
}
