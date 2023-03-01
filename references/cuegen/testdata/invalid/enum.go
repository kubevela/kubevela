package invalid

type Enum struct {
	Field1 map[string]string `json:"field1" cue:"enum:1,2,3"`
	Field2 int               `json:"field2" cue:"enum:a,b,c"`
}
