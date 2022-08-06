package model

// ResourceList an abstract kinds of resource list which can convert it to data of view in the form of table
type ResourceList interface {
	// Header generate header of table in resource view
	Header() []string
	// Body generate body of table in resource view
	Body() [][]string
}
