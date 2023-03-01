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

type InlineStruct1 struct {
	// Field1 doc
	Field1 string `json:"field11"` // Field1 comment
	Field2 string `json:"field12"`
	// Field3 doc
	Field3 struct {
		// Field3.Field1 doc
		Field1 string `json:"field11"` // Field3.Field1 comment
		Field2 string `json:"field12"` // Field3.Field2 comment
	} `json:"field13"`
}

type InlineStruct2 struct {
	// Field1 doc
	Field1        string           `json:"field21"`
	InlineStruct1 `json:",inline"` // Field1 comment
}

type InlineStruct3 struct {
	Field1        string `json:"field31"`
	InlineStruct2 `json:",inline"`
}

type Optional struct {
	Field1 string `json:"field1,omitempty"`
	Field2 string `json:"field2"`
	Field3 string `json:"field3,omitempty"`
	Field4 string `json:"field4"`
	Field5 struct {
		Field1 string `json:"field1,omitempty"`
		Field2 string `json:"field2"`
	} `json:"field5"`
	Field6 struct {
		Field1 string `json:"field1,omitempty"`
		Field2 string `json:"field2"`
		Field3 struct {
			Field1 string `json:"field1,omitempty"`
			Field2 string `json:"field2"`
		} `json:"field3,omitempty"`
	} `json:"field6,omitempty"`
}

type Skip struct {
	Field1 string `json:"-"`
	Field2 string `json:"field2"`
	Field3 string `json:"-"`
	Field4 string `json:"field4"`
}
