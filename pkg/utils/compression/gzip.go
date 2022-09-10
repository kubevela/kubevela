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
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
)

// GzipObjectToString marshal object into json, compress it with gzip, encode the result with base64
func GzipObjectToString(obj interface{}) (string, error) {
	bs, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err = gz.Write(bs); err != nil {
		return "", err
	}
	if err = gz.Flush(); err != nil {
		return "", err
	}
	if err = gz.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// GunzipStringToObject decode the compressed string with base64, decompress it with gzip, unmarshal it into obj
func GunzipStringToObject(compressed string, obj interface{}) error {
	bs, err := base64.StdEncoding.DecodeString(compressed)
	if err != nil {
		return err
	}
	reader, err := gzip.NewReader(bytes.NewReader(bs))
	if err != nil {
		return err
	}
	if bs, err = ioutil.ReadAll(reader); err != nil {
		return err
	}
	return json.Unmarshal(bs, obj)
}
