package testdata

BasicType: {
	field1:  string
	field2:  int
	field3:  bool
	field4:  float32
	field5:  float64
	field6:  int8
	field7:  int16
	field8:  int32
	field9:  int64
	field10: uint
	field11: uint8
	field12: uint16
	field13: uint32
	field14: uint64
	field15: uint64
	field16: uint8
	field17: rune
	field18: {
		...
	}
	field19: {
		...
	}
}
TagName: {
	f1: string
	f2: string
	f3: string
}
SliceAndArray: {
	field1: [...string]
	field2: 3 * [string]
	field3: [...int]
	field4: 3 * [int]
	field5: [...bool]
	field6: 3 * [bool]
	field7: [...float32]
	field8: 3 * [float32]
	field9: [...float64]
	field10: 3 * [float64]
	field11: bytes
	field12: bytes
}
SmallStruct: {
	field1: string
	field2: string
}
AnonymousField: SmallStruct: {
	field1: string
	field2: string
}
ReferenceField: field1: {
	field1: string
	field2: string
}
StructField: {
	field1: {
		field1: string
		field2: string
	}
	field2: {
		field1: string
		field2: string
	}
}
EmbedStruct: {
	field1: {
		field1: string
		field2: string
	}
	field2: {
		field1: string
		field2: string
		field3: {
			field1: string
			field2: string
			field3: {
				field1: string
				field2: string
				field3: {
					field1: string
					field2: string
				}
			}
		}
	}
	field3: [string]: [...string]
	field4: uint
}
MapField: {
	field1: [string]: string
	field2: [string]: int
	field3: {
		...
	}
	field4: [string]: {
		field1: string
		field2: string
	}
	field5: {
		...
	}
}
EmptyStruct: {}
// Comment is a test struct1
// Struct is a test struct2 
// Struct is a test struct3
// Struct is a test struct4
Comment: {
	// Field1 comment
	// Field1 doc
	field1: string
	// Field2 comment
	// Field2 doc 
	field2: string
	// Field3 comment
	// Field3 doc
	field3: string
	// Field4 comment
	field4: string
	// Field5 doc
	field5: {
		// Field5.Field1 comment
		field1: string
		// Field5.Field2 doc
		field2: string
		// Field5.Field3 doc 
		field3: string
		// Field5.Field4 doc
		field4: string
		// Field5.Field5 doc
		field5: {
			// Field5.Field5.Field1 comment
			field1: string
			// Field5.Field5.Field2 doc
			field2: string
			// Field5.Field5.Field3 doc 
			field3: string
			// Field5.Field5.Field4 doc
			field4: string
		}
	}
	// Field6 doc
	field6: [string]: [...string]
	// Field7 comment
	field7: [string]: [...string]
	// Field8 comment
	// Field8 doc
	field8: [string]: [...string]
	field9: [string]: [...string]
	// Field10 comment
	// Field10 doc1
	// Field10 doc2
	// Field10 doc3
	field10: [string]: [...string]
	// Field11 doc
	field11: {
		// Field11.Field1 comment
		field1: string
	}
	// Field12 doc
	field12: {
		// Field12.Field1 doc
		field1: string
	}
	// Field13 doc
	field13: field1: string
	// Field14 doc
	field14: [string]: string
}
Default: {
	a1: *"abc" | string
	// empty string
	a2: *"" | string
	b1: *true | bool
	b2: *false | bool
	c1: *123 | int
	c2: *123 | int8
	c3: *123 | int16
	c4: *123 | int32
	c5: *123 | int64
	d1: *123 | uint
	d2: *123 | uint8
	d3: *123 | uint16
	d4: *123 | uint32
	d5: *123 | uint64
	e1: *123.456 | float32
	e2: *123.456 | float64
}
Enum: {
	a: "abc" | "def" | "ghi"
	b: 1 | 2 | 3
	c: true | false
	d: 1.1 | 2.2 | 3.3
	e: "abc" | "def" | *"ghi"
	f: 1 | *2 | 3
	g: true | *false
	// if default value is first enum, '*' will not be added
	h: 1.1 | 2.2 | 3.3
	i: "abc"
}
Unexported: {
	field1: string
	field2: {
		field1: string
		field3: {
			field1: string
			field3: string
		}
	}
}
// RequestVars is the vars for http request
// TODO: support timeout & tls
RequestVars: {
	method: string
	url:    string
	request: {
		body: string
		header: [string]: [...string]
		trailer: [string]: [...string]
	}
}
// ResponseVars is the vars for http response
ResponseVars: {
	body: string
	header: [string]: [...string]
	trailer: [string]: [...string]
	statusCode: int
}
// DoParams is the params for http request
DoParams: $params: {
	method: string
	url:    string
	request: {
		body: string
		header: [string]: [...string]
		trailer: [string]: [...string]
	}
}
// DoReturns returned struct for http response
DoReturns: $returns: {
	body: string
	header: [string]: [...string]
	trailer: [string]: [...string]
	statusCode: int
}
ResourceReturns: $returns: {
	...
}
InlineStruct1: {
	// Field1 comment
	// Field1 doc
	field11: string
	field12: string
	// Field3 doc
	field13: {
		// Field3.Field1 comment
		// Field3.Field1 doc
		field11: string
		// Field3.Field2 comment
		field12: string
	}
}
InlineStruct2: {
	// Field1 doc
	field21: string
	// Field1 comment
	// Field1 doc
	field11: string
	field12: string
	// Field3 doc
	field13: {
		// Field3.Field1 comment
		// Field3.Field1 doc
		field11: string
		// Field3.Field2 comment
		field12: string
	}
}
InlineStruct3: {
	field31: string
	// Field1 doc
	field21: string
	// Field1 comment
	// Field1 doc
	field11: string
	field12: string
	// Field3 doc
	field13: {
		// Field3.Field1 comment
		// Field3.Field1 doc
		field11: string
		// Field3.Field2 comment
		field12: string
	}
}
Optional: {
	field1?: string
	field2:  string
	field3?: string
	field4:  string
	field5: {
		field1?: string
		field2:  string
	}
	field6?: {
		field1?: string
		field2:  string
		field3?: {
			field1?: string
			field2:  string
		}
	}
}
Skip: {
	field2: string
	field4: string
}
SpecialFieldName: {
	"field1-foo.bar+123<sa":  string
	"field2:foo]bar[foo|bar": string
}
