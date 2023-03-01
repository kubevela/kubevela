package invalid

type InlineStruct struct {
	Field1 string `json:"field1"`
}

type SameNameInlined struct {
	Field1       string `json:"field1"`
	InlineStruct `json:",inline"`
}
