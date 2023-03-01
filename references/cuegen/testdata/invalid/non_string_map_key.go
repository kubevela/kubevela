package invalid

type NonStringMapKey struct {
	Field1 map[int]string `json:"field1"`
}
