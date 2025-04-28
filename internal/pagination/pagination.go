// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package pagination

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

var (
	// validOrderByFields is a map of valid order by fields
	validOrderByFields = map[string]bool{
		"name":              true,
		"kubernetesVersion": true,
		"providerStatus":    true,
		"lifecyclePhase":    true,
		"version":           true,
	}

	// validFilterFields is a map of valid filter fields
	validFilterFields = map[string]bool{
		"name":              true,
		"kubernetesVersion": true,
		"providerStatus":    true,
		"lifecyclePhase":    true,
		"version":           true,
	}
)

type Filter struct {
	Name  string
	Value string
}

type OrderBy struct {
	Name   string
	IsDesc bool
}

type orderFunc[T any] func(item1, item2 T, orderBy *OrderBy) bool

var normalizeEqualsRe = regexp.MustCompile(`[ \t]*=[ \t]*`)

// ParseFilter parses the given filter string and returns a list of Filter
// If any error is encountered, an nil Filter slice and non-nil error is returned
func parseFilter(filterParameter string) ([]*Filter, bool, error) {
	if filterParameter == "" {
		return nil, false, nil
	}

	// Replace the matched pattern in regexp 'normalizeEqualsRe' with just '=' (basically the spaces and tabs are removed)
	normalizedFilterParameter := normalizeEqualsRe.ReplaceAllString(filterParameter, "=")

	// Now split the string with space as delimiter. Note that there could be a 'OR' predicate with on or
	// more space on either side of it
	// Consider this example 'f1=v1 OR f2=v2 OR f3=v3'.
	// After below step, the 'elements' contains ["f1=v1", "OR",  "f2=v2", "OR", "f3=v3"]
	elements := strings.Split(normalizedFilterParameter, " ")

	var filters []*Filter
	var currentFilter *Filter
	useAnd := false

	// Now parse each element and make a list of all 'name=value' filters
	for index, element := range elements {
		switch {
		case strings.Contains(element, "="):
			selectors := strings.Split(element, "=")
			if currentFilter != nil || len(selectors) != 2 || selectors[0] == "" || selectors[1] == "" {
				// Error condition - too many equals
				return nil, false, fmt.Errorf("filter: invalid filter request (=): %s", elements)
			}
			currentFilter = &Filter{
				Name:  selectors[0],
				Value: selectors[1],
			}
		case element == "OR":
			if currentFilter == nil || index == len(elements)-1 {
				return nil, false, fmt.Errorf("filter: invalid filter request (OR): %s", elements)
			}
			filters = append(filters, currentFilter)
			currentFilter = nil
		case element == "AND":
			if currentFilter == nil || index == len(elements)-1 {
				return nil, false, fmt.Errorf("filter: invalid filter request (AND): %s", elements)
			}
			filters = append(filters, currentFilter)
			currentFilter = nil
			useAnd = true
		default:
			if currentFilter == nil {
				// Error condition - missing an =
				return nil, false, fmt.Errorf("filter: invalid filter request: %s", elements)
			}
			currentFilter.Value = currentFilter.Value + " " + element
		}
	}
	if currentFilter != nil {
		filters = append(filters, currentFilter)
	}

	return filters, useAnd, nil
}

// ParseOrderBy parses the incoming orderBy query string
// Below is a sample orderBy query specifying that the results should be sorted
// that name is ascending and create_time should be descending
//
//	/books?orderBy="name asc, create_time desc"
func parseOrderBy(orderByParameter string) ([]*OrderBy, error) {
	if orderByParameter == "" {
		return nil, nil
	}

	// orderBy commands should be separated by ',' if there are more than one.
	// Split them by ',' delimiter.
	elements := strings.Split(orderByParameter, ",")
	var orderBys []*OrderBy
	for _, element := range elements {
		descending := false
		// Parse each orderBy command to extract the field name and the command (asc or desc)
		direction := strings.Split(strings.Trim(element, " "), " ")
		// Do some validations to ensure we have the right format and right command
		if len(direction) == 0 || len(direction) > 2 {
			return nil, errors.New("invalid order by: " + element)
		}

		if len(direction) == 2 {
			switch direction[1] {
			case "asc":
				descending = false
			case "desc":
				descending = true
			default:
				return nil, errors.New("invalid order by direction: " + element)
			}
		}
		orderBys = append(orderBys, &OrderBy{
			Name:   direction[0],
			IsDesc: descending,
		})
	}
	return orderBys, nil
}

// computePageRange computes the startIndex and endIndex based on the provided pageSize, offset and totalCount.
// It returns -1 for the endIndex if there are no items to paginate. Note that this version has the 'end' index inclusive.
func computePageRange(pageSize int32, offset int32, totalCount int) (int, int) {
	if totalCount <= 0 || // Invalid totalCount
		totalCount > math.MaxInt32 || // totalCount exceeds int32 range
		offset >= int32(totalCount) { // Offset out of bounds
		return 0, -1 // -1 to indicate that there are no items to paginate
	}

	startIndex := int(offset)
	endIndex := startIndex + int(pageSize)
	if endIndex > totalCount {
		endIndex = totalCount
	}

	return startIndex, endIndex
}

func PaginateItems[T any](items []T, pageSize, offset int) (*[]T, error) {
	paginatedItems, err := applyPagination(items, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to apply pagination: %w", err)
	}

	return &paginatedItems, nil
}

func applyPagination[T any](items []T, pageSize, offset int) ([]T, error) {
	start, end := computePageRange(int32(pageSize), int32(offset), len(items))
	if end == -1 {
		return nil, fmt.Errorf("no items to paginate")
	}

	return items[start:end], nil
}

func FilterItems[T any](items []T, filter string, filterFunc func(T, *Filter) bool) ([]T, error) {
	filters, useAnd, err := parseFilter(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to parse filter: %w", err)
	}

	var filteredItems []T
	for _, item := range items {
		if useAnd {
			// all required filters should match
			matchesAll := true
			for _, filter := range filters {
				if !filterFunc(item, filter) {
					matchesAll = false
					break
				}
			}
			if matchesAll {
				filteredItems = append(filteredItems, item)
			}
		} else {
			// at least one filter match
			for _, filter := range filters {
				if filterFunc(item, filter) {
					filteredItems = append(filteredItems, item)
					break
				}
			}
		}
	}

	return filteredItems, nil
}

func OrderItems[T any](items []T, orderBy string, orderFunc func(T, T, *OrderBy) bool) ([]T, error) {
	orderBys, err := parseOrderBy(orderBy)
	if err != nil {
		return nil, fmt.Errorf("failed to parse order by: %w", err)
	}

	return applyOrderBy(items, orderBys, orderFunc), nil
}

func applyOrderBy[T any](items []T, orderBys []*OrderBy, orderFunc orderFunc[T]) []T {
	sort.SliceStable(items, func(i, j int) bool {
		for _, orderBy := range orderBys {
			if orderFunc(items[i], items[j], orderBy) {
				return true
			}
		}
		return false
	})

	return items
}

func extractParamsFields(params any) (pageSize, offset *int, orderBy, filter *string, err error) {
	switch p := params.(type) {
	case api.GetV2ClustersParams:
		return p.PageSize, p.Offset, p.OrderBy, p.Filter, nil
	case api.GetV2TemplatesParams:
		return p.PageSize, p.Offset, p.OrderBy, p.Filter, nil
	default:
		return nil, nil, nil, nil, fmt.Errorf("unsupported params type: %v (%v)", p, params)
	}
}

// ValidateParams validates the incoming parameters for pagination
func ValidateParams(params any) (pageSize, offset *int, orderBy, filter *string, err error) {
	pageSize, offset, orderBy, filter, err = extractParamsFields(params)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if pageSize == nil || *pageSize <= 0 {
		return nil, nil, nil, nil, fmt.Errorf("invalid pageSize: must be greater than 0")
	}
	if offset == nil || *offset < 0 {
		return nil, nil, nil, nil, fmt.Errorf("invalid offset: must be non-negative")
	}

	if orderBy != nil {
		if *orderBy == "" {
			return nil, nil, nil, nil, fmt.Errorf("invalid orderBy field")
		}

		orderByParts := strings.Split(*orderBy, " ")
		if len(orderByParts) == 1 {
			orderBy = convert.Ptr(orderByParts[0] + " asc")
		} else if len(orderByParts) == 2 {
			if !validOrderByFields[orderByParts[0]] || (orderByParts[1] != "asc" && orderByParts[1] != "desc") {
				return nil, nil, nil, nil, fmt.Errorf("invalid orderBy field")
			}
		}
	}

	if filter != nil {
		if *filter == "" {
			return nil, nil, nil, nil, fmt.Errorf("invalid filter: cannot be empty")
		}

		filterParts := strings.FieldsFunc(*filter, func(r rune) bool {
			return r == ' ' || r == 'O' || r == 'R' || r == 'A' || r == 'N' || r == 'D'
		})
		for _, part := range filterParts {
			subParts := strings.Split(part, "=")
			if len(subParts) != 2 || !validFilterFields[subParts[0]] {
				return nil, nil, nil, nil, fmt.Errorf("invalid filter field")
			}
		}
	}

	return pageSize, offset, orderBy, filter, nil
}

// MatchWildcard checks if the target string starts with the prefix derived from the pattern.
func MatchWildcard(target *string, pattern string) bool {
	if target == nil {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(*target, prefix)
	}
	return *target == pattern
}
