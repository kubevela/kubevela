package apis

import "github.com/oam-dev/kubevela/pkg/apiserver/proto/model"

// CatalogType catalog for list capability
type CatalogType struct {
	Name     string `json:"name"`
	Desc     string `json:"desc,omitempty"`
	UpdateAt int64  `json:"updateAt,omitempty"`
	Type     string `json:"type,omitempty"`
	URL      string `json:"url,omitempty"`
	Token    string `json:"token,omitempty"`
}

// CatalogMeta catalog meta
type CatalogMeta struct {
	Catalog *model.Catalog `json:"catalog"`
}

// CatalogRequest catalog request
type CatalogRequest struct {
	CatalogType
	Method Action `json:"method"`
}
