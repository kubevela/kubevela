package test

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
