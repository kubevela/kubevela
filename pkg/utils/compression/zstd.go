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
