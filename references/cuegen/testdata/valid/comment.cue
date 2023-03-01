package test

// Struct is a test struct1
// Struct is a test struct2 
// Struct is a test struct3
// Struct is a test struct4
Struct: {
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
