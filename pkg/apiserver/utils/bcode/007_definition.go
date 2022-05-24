/*
Copyright 2021 The KubeVela Authors.

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

package bcode

// ErrDefinitionNotFound definition is not exist
var ErrDefinitionNotFound = NewBcode(404, 70001, "definition is not exist")

// ErrDefinitionNoSchema definition not have schema
var ErrDefinitionNoSchema = NewBcode(400, 70002, "definition not have schema")

// ErrDefinitionTypeNotSupport definition type not support
var ErrDefinitionTypeNotSupport = NewBcode(400, 70003, "definition type not support")

// ErrInvalidDefinitionUISchema invalid custom definition ui schema
var ErrInvalidDefinitionUISchema = NewBcode(400, 70004, "invalid custom defnition ui schema")
