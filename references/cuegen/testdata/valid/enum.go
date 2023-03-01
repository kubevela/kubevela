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

type Enum struct {
	A string  `json:"a" cue:"enum:abc,def,ghi"`
	B int     `json:"b" cue:"enum:1,2,3"`
	C bool    `json:"c" cue:"enum:true,false"`
	D float64 `json:"d" cue:"enum:1.1,2.2,3.3"`
	E string  `json:"e" cue:"enum:abc,def,ghi;default:ghi"`
	F int     `json:"f" cue:"enum:1,2,3;default:2"`
	G bool    `json:"g" cue:"enum:true,false;default:false"`
	H float64 `json:"h" cue:"enum:1.1,2.2,3.3;default:1.1"` // if default value is first enum, '*' will not be added
	I string  `json:"i" cue:"enum:abc"`
}
