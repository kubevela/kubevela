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

import "encoding/base64"

// CompressedBytes represents compressed data and which compression method is used.
// It stores compressed data in binary form.
type CompressedBytes struct {
	Type Type   `json:"type,omitempty"`
	Data []byte `json:"data,omitempty"`
}

// EncodeFrom encodes the given data using the given compression type. Note that
// it first JSON-marshals the object, then compresses it.
func (c *CompressedBytes) EncodeFrom(obj interface{}) (err error) {
	comp, ok := compressors[c.Type]
	if !ok {
		return NewUnsupportedCompressionTypeError(string(c.Type))
	}

	c.Data, err = comp.compress(obj)
	return
}

// DecodeTo decodes the compressed data and unmarshal it into the given object.
// obj must be a pointer.
func (c *CompressedBytes) DecodeTo(obj interface{}) error {
	comp, ok := compressors[c.Type]
	if !ok {
		return NewUnsupportedCompressionTypeError(string(c.Type))
	}

	return comp.decompress(c.Data, obj)
}

// SetType sets the compression type.
func (c *CompressedBytes) SetType(t Type) {
	c.Type = t
}

// CompressedText represents compressed data and which compression method is used.
// It stores compressed data in text form (base64), which can be used in JSON, yaml, etc.
type CompressedText struct {
	Type Type   `json:"type,omitempty"`
	Data string `json:"data,omitempty"`
}

// EncodeFrom encodes the given data using the given compression type. Note that
// it first JSON-marshals the object, compresses it, and encodes it in base64.
func (c *CompressedText) EncodeFrom(obj interface{}) error {
	cb := CompressedBytes{
		Type: c.Type,
	}

	err := cb.EncodeFrom(obj)
	if err != nil {
		return err
	}

	c.Data = base64.StdEncoding.EncodeToString(cb.Data)
	return nil
}

// DecodeTo decodes the compressed data and unmarshal it into the given object.
// obj must be a pointer.
func (c *CompressedText) DecodeTo(obj interface{}) error {
	cb := CompressedBytes{
		Type: c.Type,
	}

	var err error
	cb.Data, err = base64.StdEncoding.DecodeString(c.Data)
	if err != nil {
		return err
	}

	return cb.DecodeTo(obj)
}

// SetType sets the compression type.
func (c *CompressedText) SetType(t Type) {
	c.Type = t
}

// Clean clears the compressed data.
func (c *CompressedText) Clean() {
	c.Data = ""
}
