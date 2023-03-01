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

type A struct {
	Field1 string `json:"field1"`
	field2 string
	Field3 string `json:"field3"`
	field4 string
}

type B struct {
	Field1 string `json:"field1"`
	Field2 struct {
		Field1 string `json:"field1"`
		field2 string
		Field3 struct {
			Field1 string `json:"field1"`
			field2 string
			Field3 string `json:"field3"`
		} `json:"field3"`
	} `json:"field2"`
	field3 string
}
