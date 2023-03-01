package test

import "net/http"

// Struct is a test struct1
/* Struct is a test struct2 */
/*
	Struct is a test struct3
	Struct is a test struct4
*/
type Struct struct {
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
