package test

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
ReferenceField: field1: null | {
	field1: string
	field2: string
}
StructField: {
	field1: {
		field1: string
		field2: string
	}
	field2: null | {
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
}
EmptyStruct: {}
