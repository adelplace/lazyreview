package http

import (
	"fmt"
	"net/url"
	"strconv"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// pageParams are the pagination query parameters: ?page=2&size=50.
type pageParams struct {
	Page int
	Size int
}

func (p pageParams) offset() int { return (p.Page - 1) * p.Size }

// page is the JSON envelope of a paginated list.
type page[T any] struct {
	Items []T `json:"items"`
	Page  int `json:"page"`
	Size  int `json:"size"`
	Total int `json:"total"`
}

func parsePage(q url.Values) (pageParams, error) {
	p := pageParams{Page: 1, Size: defaultPageSize}
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return p, fmt.Errorf("page must be a positive integer")
		}
		p.Page = n
	}
	if v := q.Get("size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxPageSize {
			return p, fmt.Errorf("size must be between 1 and %d", maxPageSize)
		}
		p.Size = n
	}
	return p, nil
}
