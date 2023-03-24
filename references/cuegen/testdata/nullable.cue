package testdata

Nullable: {
	field1?: null | string
	field2?: null | int
	field3?: null | bool
	Field4:  null | {
		field1: null | string
		field2: null | int
		field3: null | bool
	}
	field5?: null | [...string]
	field6?: null | [...int]
}
