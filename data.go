package main

import (
	"math"
	"time"
)

// NewsItem represents news item
type NewsItem struct {
	ID          int
	Title       string
	Link        string
	Published   time.Time
	Description string
	Image       string
}

type Metadata struct {
	CurrentPage  int `json:"current_page,omitempty"`
	PageSize     int `json:"page_size,omitempty"`
	FirstPage    int `json:"first_page,omitempty"`
	LastPage     int `json:"last_page,omitempty"`
	TotalRecords int `json:"total_records,omitempty"`
}

// calculateMetadata calculates metadata for pagination
func calculateMetadata(totalRecords, page, pageSize int) Metadata {
	if totalRecords == 0 {
		return Metadata{}
	}

	return Metadata{
		CurrentPage:  page,
		PageSize:     pageSize,
		FirstPage:    1,
		LastPage:     int(math.Ceil(float64(totalRecords) / float64(pageSize))),
		TotalRecords: totalRecords,
	}
}

// Filters represents filters for news items
// ?page=1&pagesize=5
type Filters struct {
	Page     int
	PageSize int
}

var defaultFilters = Filters{
	Page:     1,
	PageSize: 5,
}

// validate validates filters and sets default values if needed
func (f *Filters) validate(defaultFilters Filters) {
	if f.Page < 1 {
		f.Page = defaultFilters.Page
	}
	if f.PageSize < 1 {
		f.PageSize = defaultFilters.PageSize
	}
}

// limit returns limit for SQL query
func (f Filters) limit() int {
	return f.PageSize
}

// offset returns offset for SQL query
func (f Filters) offset() int {
	offset := (f.Page - 1) * f.PageSize
	return min(offset, math.MaxInt)
}
