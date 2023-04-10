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

package testdata

import (
	"crypto"
	"net/http"

	"github.com/kubevela/pkg/cue/cuex/providers"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type BasicType struct {
	Field1  string      `json:"field1"`
	Field2  int         `json:"field2"`
	Field3  bool        `json:"field3"`
	Field4  float32     `json:"field4"`
	Field5  float64     `json:"field5"`
	Field6  int8        `json:"field6"`
	Field7  int16       `json:"field7"`
	Field8  int32       `json:"field8"`
	Field9  int64       `json:"field9"`
	Field10 uint        `json:"field10"`
	Field11 uint8       `json:"field11"`
	Field12 uint16      `json:"field12"`
	Field13 uint32      `json:"field13"`
	Field14 uint64      `json:"field14"`
	Field15 uintptr     `json:"field15"`
	Field16 byte        `json:"field16"`
	Field17 rune        `json:"field17"`
	Field18 interface{} `json:"field18"`
	Field19 any         `json:"field19"`
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
	Field1 map[string]string      `json:"field1"`
	Field2 map[string]int         `json:"field2"`
	Field3 map[string]interface{} `json:"field3"`
	Field4 map[string]SmallStruct `json:"field4"`
	Field5 map[string]any         `json:"field5"`
}

type EmptyStruct struct{}

type Interface interface {
	Foo()
}

// Comment is a test struct1
/* Struct is a test struct2 */
/*
	Struct is a test struct3
	Struct is a test struct4
*/
type Comment struct {
	// Field1 doc
	Field1 string `json:"field1"` // Field1 comment
	/* Field2 doc */
	Field2 string `json:"field2"` // Field2 comment
	/*
		Field3 doc
	*/
	Field3 string `json:"field3"` // Field3 comment
	Field4 string `json:"field4"` // Field4 comment

	// Field5 doc
	Field5 struct {
		Field1 string `json:"field1"` // Field5.Field1 comment
		// Field5.Field2 doc
		Field2 string `json:"field2"`
		/* Field5.Field3 doc */
		Field3 string `json:"field3"`
		/*
			Field5.Field4 doc
		*/
		Field4 string `json:"field4"`
		// Field5.Field5 doc
		Field5 struct {
			Field1 string `json:"field1"` // Field5.Field5.Field1 comment
			// Field5.Field5.Field2 doc
			Field2 string `json:"field2"`
			/* Field5.Field5.Field3 doc */
			Field3 string `json:"field3"`
			/*
				Field5.Field5.Field4 doc
			*/
			Field4 string `json:"field4"`
		} `json:"field5"`
	} `json:"field5"`

	// Field6 doc
	Field6 http.Header `json:"field6"`
	Field7 http.Header `json:"field7"` // Field7 comment
	// Field8 doc
	Field8 http.Header `json:"field8"` // Field8 comment
	Field9 http.Header `json:"field9"`

	/*
		Field10 doc1
		Field10 doc2
		Field10 doc3
	*/
	Field10 http.Header `json:"field10"` // Field10 comment

	// Field11 doc
	Field11 struct {
		Field1 string `json:"field1"` // Field11.Field1 comment
	} `json:"field11"`

	// Field12 doc
	Field12 struct {
		// Field12.Field1 doc
		Field1 string `json:"field1"`
	} `json:"field12"`

	// Field13 doc
	Field13 struct {
		Field1 string `json:"field1"`
	} `json:"field13"`

	// Field14 doc
	Field14 map[string]string `json:"field14"`
}

type Default struct {
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

type Unexported struct {
	Field1 string `json:"field1"`
	Field2 struct {
		Field1 string `json:"field1"`
		field2 string // unexported field will be ignored
		Field3 struct {
			Field1 string `json:"field1"`
			field2 string // unexported field will be ignored
			Field3 string `json:"field3"`
		} `json:"field3"`
	} `json:"field2"`
	field3 string // unexported field will be ignored
}

// RequestVars is the vars for http request
// TODO: support timeout & tls
type RequestVars struct {
	Method  string `json:"method"`
	URL     string `json:"url"`
	Request struct {
		Body    string      `json:"body"`
		Header  http.Header `json:"header"`
		Trailer http.Header `json:"trailer"`
	} `json:"request"`
}

// ResponseVars is the vars for http response
type ResponseVars struct {
	Body       string      `json:"body"`
	Header     http.Header `json:"header"`
	Trailer    http.Header `json:"trailer"`
	StatusCode int         `json:"statusCode"`
}

// DoParams is the params for http request
type DoParams providers.Params[RequestVars]

// DoReturns returned struct for http response
type DoReturns providers.Returns[ResponseVars]

type ResourceReturns providers.Returns[*unstructured.Unstructured]

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

// TypeFilter should be ignored
type TypeFilter http.Header

// TypeFilterStruct should be ignored
type TypeFilterStruct struct {
	Field1 string `json:"field1"`
	Field2 string `json:"field2"`
}

type SpecialFieldName struct {
	Field1 string `json:"field1-foo.bar+123<sa"`
	Field2 string `json:"field2:foo]bar[foo|bar"`
}
