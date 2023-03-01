package invalid

type RecursiveStruct struct {
	Field1 *RecursiveStruct `json:"field1"`
}
