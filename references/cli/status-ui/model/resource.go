package model

type ResourceList interface {
	Header() []string
	Body() [][]string
}
