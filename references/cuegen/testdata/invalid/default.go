package invalid

type Default struct {
	Field1 chan int `json:"field1"`
	Field2 int      `json:"field2" cue:"default:a"`
}
