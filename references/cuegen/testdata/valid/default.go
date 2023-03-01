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

type Default1 struct {
	A1 string  `json:"a1" cue:"default:abc"`
	A2 string  `json:"a2" cue:"default:"` // empty string
	B1 bool    `json:"b1" cue:"default:true"`
	B2 bool    `json:"b2" cue:"default:false"`
	C1 int     `json:"c1" cue:"default:123"`
	C2 int8    `json:"c2" cue:"default:123"`
	C3 int16   `json:"c3" cue:"default:123"`
	C4 int32   `json:"c4" cue:"default:123"`
	C5 int64   `json:"c5" cue:"default:123"`
	D1 uint    `json:"d1" cue:"default:123"`
	D2 uint8   `json:"d2" cue:"default:123"`
	D3 uint16  `json:"d3" cue:"default:123"`
	D4 uint32  `json:"d4" cue:"default:123"`
	D5 uint64  `json:"d5" cue:"default:123"`
	E1 float32 `json:"e1" cue:"default:123.456"`
	E2 float64 `json:"e2" cue:"default:123.456"`
}
