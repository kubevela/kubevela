package test

type A struct {
	Field1 string `json:"field1"`
	field2 string
	Field3 string `json:"field3"`
	field4 string
}

type B struct {
	Field1 string `json:"field1"`
	Field2 struct {
		Field1 string `json:"field1"`
		field2 string
		Field3 struct {
			Field1 string `json:"field1"`
			field2 string
			Field3 string `json:"field3"`
		} `json:"field3"`
	} `json:"field2"`
	field3 string
}
