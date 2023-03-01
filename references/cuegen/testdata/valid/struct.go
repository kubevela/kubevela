/*
Copyright 2023 The KubeVela Authors.

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

package test

import (
	"crypto"
	"net/http"
)

type BasicType struct {
	Field1  string  `json:"field1"`
	Field2  int     `json:"field2"`
	Field3  bool    `json:"field3"`
	Field4  float32 `json:"field4"`
	Field5  float64 `json:"field5"`
	Field6  int8    `json:"field6"`
	Field7  int16   `json:"field7"`
	Field8  int32   `json:"field8"`
	Field9  int64   `json:"field9"`
	Field10 uint    `json:"field10"`
	Field11 uint8   `json:"field11"`
	Field12 uint16  `json:"field12"`
	Field13 uint32  `json:"field13"`
	Field14 uint64  `json:"field14"`
	Field15 uintptr `json:"field15"`
	Field16 byte    `json:"field16"`
	Field17 rune    `json:"field17"`
}

type TagName struct {
	Field1 string `json:"f1"`
	Field2 string `json:"f2"`
	Field3 string `json:"f3"`
}

type SliceAndArray struct {
	Field1  []string   `json:"field1"`
	Field2  [3]string  `json:"field2"`
	Field3  []int      `json:"field3"`
	Field4  [3]int     `json:"field4"`
	Field5  []bool     `json:"field5"`
	Field6  [3]bool    `json:"field6"`
	Field7  []float32  `json:"field7"`
	Field8  [3]float32 `json:"field8"`
	Field9  []float64  `json:"field9"`
	Field10 [3]float64 `json:"field10"`
	Field11 [3]byte    `json:"field11"`
	Field12 []byte     `json:"field12"`
}

type SmallStruct struct {
	Field1 string `json:"field1"`
	Field2 string `json:"field2"`
}

type AnonymousField struct {
	SmallStruct
}

type ReferenceField struct {
	Field1 *SmallStruct `json:"field1"`
}

type StructField struct {
	Field1 SmallStruct  `json:"field1"`
	Field2 *SmallStruct `json:"field2"`
}

type EmbedStruct struct {
	Field1 struct {
		Field1 string `json:"field1"`
		Field2 string `json:"field2"`
	} `json:"field1"`
	Field2 struct {
		Field1 string `json:"field1"`
		Field2 string `json:"field2"`
		Field3 struct {
			Field1 string `json:"field1"`
			Field2 string `json:"field2"`
			Field3 struct {
				Field1 string `json:"field1"`
				Field2 string `json:"field2"`
				Field3 struct {
					Field1 string `json:"field1"`
					Field2 string `json:"field2"`
				} `json:"field3"`
			} `json:"field3"`
		} `json:"field3"`
	} `json:"field2"`
	Field3 http.Header `json:"field3"`
	Field4 crypto.Hash `json:"field4"`
}

type MapField struct {
	Field1 map[string]string `json:"field1"`
	Field2 map[string]int    `json:"field2"`
}

type EmptyStruct struct{}

type Interface interface {
	Foo()
}
