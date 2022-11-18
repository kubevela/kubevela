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
	"encoding/json"

	"github.com/klauspost/compress/zstd"
)

type zstdCompressor struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

var _ compressor = &zstdCompressor{}

func (c *zstdCompressor) init() {
	// Create a writer that caches compressors. For this operation type we supply a nil Reader.
	c.encoder, _ = zstd.NewWriter(nil,
		// We use the default levels here because we got pretty good results.
		// It is almost as fast as no compression at all when the object is large enough.
		// Even with small objects, it is still very fast and efficient.
		//
		// Tests are here: /apis/core.oam.dev/v1beta1/resourcetracker_types_test.go
		//
		// Here are results:
		// zstd.SpeedFastest:
		//    Compressed Size:
		//      uncompressed: 2131455 bytes   100.00%
		//      gzip:         273057 bytes    12.81%
		//      zstd:         191737 bytes    9.00%
		//    Marshal Time:
		//      no compression: 37740514 ns   1.00x
		//      gzip:           97389702 ns   2.58x
		//      zstd:           39866808 ns   1.06x
		// zstd.SpeedDefault:
		//    Compressed Size:
		//      uncompressed: 2131455 bytes   100.00%
		//      gzip:         273057 bytes    12.81%
		//      zstd:         171577 bytes    8.05%
		//    Marshal Time:
		//      no compression: 42272142 ns   1.00x
		//      gzip:           90474722 ns   2.14x
		//      zstd:           39070416 ns   0.92x
		// zstd.SpeedBetterCompression:
		//    Compressed Size:
		//      uncompressed: 2131455 bytes   100.00%
		//      gzip:         273057 bytes    12.81%
		//      zstd:         149061 bytes    6.99%
		//    Marshal Time:
		//      no compression: 38826717 ns   1.00x
		//      gzip:           94855264 ns   2.44x
		//      zstd:           48524197 ns   1.25x
		zstd.WithEncoderLevel(zstd.SpeedDefault),
		// TODO(charlie0129): give a dictionary to compressor to get even more improvements.
		//
		// Since we are dealing with highly-specialized small JSON data, a dictionary will
		// give massive improvements, around 3x both (de)compression speed and size reduction,
		// according to Facebook https://github.com/facebook/zstd#the-case-for-small-data-compression.
		// zstd.WithEncoderDict()
	)
	c.decoder, _ = zstd.NewReader(nil)
}

func (c *zstdCompressor) compress(obj interface{}) ([]byte, error) {
	bs, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	compressed := c.encoder.EncodeAll(bs, make([]byte, 0, len(bs)))
	return compressed, nil
}

func (c *zstdCompressor) decompress(compressed []byte, obj interface{}) error {
	decompressed, err := c.decoder.DecodeAll(compressed, nil)
	if err != nil {
		return err
	}

	return json.Unmarshal(decompressed, obj)
}
