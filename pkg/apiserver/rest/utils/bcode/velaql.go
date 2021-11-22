/*
 Copyright 2021. The KubeVela Authors.

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

// ErrParseVelaQL failed to parse velaQL
var ErrParseVelaQL = NewBcode(400, 60001, "fail to parse the velaQL")

// ErrViewQuery failed to query view
var ErrViewQuery = NewBcode(400, 60002, "view query failed")

// ErrParseQuery2Json failed to parse query result to response
var ErrParseQuery2Json = NewBcode(400, 60003, "fail to parse query result to json format")
