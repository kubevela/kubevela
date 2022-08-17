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

// Create a writer that caches compressors. For this operation type we supply a nil Reader.
var encoder, _ = zstd.NewWriter(nil,
	// We use the fastest level here because we are dealing with highly-compressible
	// JSON string. We would not gain much compression ratio when going for the
	// slower levels. Instead, we will almost get double the performance comparing
	// Fastest and Default.
	//
	// file                        level   insize      outsize     millis  mb/s
	// github-june-2days-2019.json     1   6273951764  697439532   9789    611.17
	// github-june-2days-2019.json     2   6273951764  610876538   18553   322.49
	// github-june-2days-2019.json     3   6273951764  517662858   44186   135.41
	// github-june-2days-2019.json     4   6273951764  464617114   165373  36.18
	zstd.WithEncoderLevel(zstd.SpeedFastest),
	// TODO(charlie0129): give a dictionary to compressor to get even more improvements.
	//
	// Since we are dealing with highly-specialized small JSON data, a dictionary will
	// give massive improvements, around 3x both (de)compression speed and size reduction,
	// according to Facebook https://github.com/facebook/zstd#the-case-for-small-data-compression.
	// zstd.WithEncoderDict(),
)

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
