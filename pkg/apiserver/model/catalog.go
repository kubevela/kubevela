package model

// Catalog defines the data model of a Catalog
type Catalog struct {
	Name string `json:"name,omitempty"`
	Desc string `json:"desc,omitempty"`
	// UpdatedAt is the unix time of the last time when the catalog is updated.
	UpdatedAt int64 `json:"updated_at,omitempty"`
	// Type of the Catalog, such as "github" for a github repo.
	Type string `json:"type,omitempty"`
	// URL of the Catalog.
	Url string `json:"url,omitempty"`
	// Auth token used to sync Catalog.
	Token string `json:"token,omitempty"`
}
