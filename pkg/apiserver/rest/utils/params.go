/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"strconv"

	"github.com/emicklei/go-restful/v3"
	"github.com/pkg/errors"
)

// ExtractPagingParams extract `page` and `pageSize` params from request
func ExtractPagingParams(req *restful.Request, minPageSize int, maxPageSize int, defaultPageSize int) (int, int, error) {
	pageStr := req.QueryParameter("page")
	pageSizeStr := req.QueryParameter("pageSize")
	if pageStr == "" {
		pageStr = "0"
	}
	if pageSizeStr == "" {
		pageSizeStr = strconv.Itoa(defaultPageSize)
	}
	page64, err := strconv.ParseInt(pageStr, 10, 32)
	if err != nil {
		return 0, 0, errors.Errorf("invalid page %s: %v", pageStr, err)
	}
	pageSize64, err := strconv.ParseInt(pageSizeStr, 10, 32)
	if err != nil {
		return 0, 0, errors.Errorf("invalid pageSize %s: %v", pageSizeStr, err)
	}
	page := int(page64)
	pageSize := int(pageSize64)
	if page < 0 {
		page = 0
	}
	if pageSize < minPageSize {
		pageSize = minPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize, nil
}
