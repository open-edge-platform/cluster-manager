// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package pagination

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func checkFilters(t *testing.T, filters []*Filter, wantedFieldList string, wantedValuesList string) {
	if len(filters) == 0 && wantedValuesList == "" {
		return
	}
	wantedFields := strings.Split(wantedFieldList, ",")
	wantedValues := strings.Split(wantedValuesList, ",")

	assert.Len(t, filters, len(wantedFields))
	for i := range wantedFields {
		assert.Equal(t, wantedFields[i], filters[i].Key)
		assert.Equal(t, wantedValues[i], filters[i].Value)
	}
}

func checkOrders(t *testing.T, orders []*OrderBy, wantedFieldList string, wantedOrderList []bool) {
	if len(orders) == 0 && len(wantedOrderList) == 0 {
		return
	}
	wantedFields := strings.Split(wantedFieldList, ",")

	assert.Len(t, orders, len(wantedFields))
	for i := range wantedFields {
		assert.Equal(t, wantedFields[i], orders[i].Name)
		assert.Equal(t, wantedOrderList[i], orders[i].IsDesc)
	}
}

func TestFiltersParsing(t *testing.T) {
	tests := map[string]struct {
		filter           string
		wantedFieldList  string
		wantedValuesList string
		expectedError    string
		useAnd           bool
	}{
		"none":             {filter: "", wantedValuesList: "", wantedFieldList: "", useAnd: false},
		"single":           {filter: "field1=value1", wantedFieldList: "field1", wantedValuesList: "value1", useAnd: false},
		"double":           {filter: "name=acme OR description=widget company", wantedFieldList: "name,description", wantedValuesList: "acme,widget company", useAnd: false},
		"triple":           {filter: "f1=v1 OR f2=v2 OR f3=v3", wantedFieldList: "f1,f2,f3", wantedValuesList: "v1,v2,v3", useAnd: false},
		"equals error":     {filter: "=", wantedFieldList: "", wantedValuesList: "", expectedError: "invalid filter request", useAnd: false},
		"two equals":       {filter: "= =", wantedFieldList: "", wantedValuesList: "", expectedError: "invalid filter request", useAnd: false},
		"no field":         {filter: "=v1", wantedFieldList: "", wantedValuesList: "", expectedError: "invalid filter request", useAnd: false},
		"no value":         {filter: "f1=", wantedFieldList: "", wantedValuesList: "", expectedError: "invalid filter request", useAnd: false},
		"no equals":        {filter: "f1 v1", wantedFieldList: "", wantedValuesList: "", expectedError: "invalid filter request", useAnd: false},
		"just OR":          {filter: "OR", wantedFieldList: "", wantedValuesList: "", expectedError: "invalid filter request", useAnd: false},
		"hanging OR":       {filter: "f1=v1 OR f2=v2 OR", wantedFieldList: "", wantedValuesList: "", expectedError: "invalid filter request", useAnd: false},
		"OR no left side":  {filter: "OR f2=v2", wantedFieldList: "", wantedValuesList: "", expectedError: "invalid filter request", useAnd: false},
		"single AND":       {filter: "field1=value1 AND field2=value2", wantedFieldList: "field1,field2", wantedValuesList: "value1,value2", useAnd: true},
		"AND and OR":       {filter: "field1=value1 AND field2=value2 OR field3=value3", wantedFieldList: "field1,field2,field3", wantedValuesList: "value1,value2,value3", useAnd: true},
		"AND with spaces":  {filter: "field1=value1 AND field2=value2 AND field3=value3", wantedFieldList: "field1,field2,field3", wantedValuesList: "value1,value2,value3", useAnd: true},
		"just AND":         {filter: "AND", wantedFieldList: "", wantedValuesList: "", expectedError: "invalid filter request", useAnd: false},
		"hanging AND":      {filter: "f1=v1 AND f2=v2 AND", wantedFieldList: "", wantedValuesList: "", expectedError: "invalid filter request", useAnd: true},
		"AND no left side": {filter: "AND f2=v2", wantedFieldList: "", wantedValuesList: "", expectedError: "invalid filter request", useAnd: false},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			resp, useAnd, err := parseFilter(testCase.filter)
			if testCase.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.useAnd, useAnd)
				checkFilters(t, resp, testCase.wantedFieldList, testCase.wantedValuesList)
			}
		})
	}
}

func TestParseOrderBy(t *testing.T) {
	tests := map[string]struct {
		orderBy         string
		wantedFieldList string
		WantedOrderList []bool
		expectedError   string
	}{
		"none": {orderBy: "", wantedFieldList: "", WantedOrderList: []bool{}},
		"single, no order specified, defaults to asc": {orderBy: "name", wantedFieldList: "name", WantedOrderList: []bool{false}},
		"single, desc order specified":                {orderBy: "name desc", wantedFieldList: "name", WantedOrderList: []bool{true}},
		"single, asc order specified":                 {orderBy: "name asc", wantedFieldList: "name", WantedOrderList: []bool{false}},
		"double, both asc":                            {orderBy: "name1 asc, name2 asc", wantedFieldList: "name1,name2", WantedOrderList: []bool{false, false}},
		"double, both desc":                           {orderBy: "name1 desc, name2 desc", wantedFieldList: "name1,name2", WantedOrderList: []bool{true, true}},
		"double, asc and desc":                        {orderBy: "name1 asc, name2 desc", wantedFieldList: "name1,name2", WantedOrderList: []bool{false, true}},
		"double, desc order and missing order":        {orderBy: "name1 desc, name2", wantedFieldList: "name1,name2", WantedOrderList: []bool{true, false}},
		"double, invalid order type1":                 {orderBy: "name1 something, name2=desc", wantedFieldList: "", WantedOrderList: []bool{}, expectedError: "invalid order by"},
		"single, multiple orders":                     {orderBy: "name1 asc desc", wantedFieldList: "", WantedOrderList: []bool{}, expectedError: "invalid order by"},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			resp, err := parseOrderBy(testCase.orderBy)
			if testCase.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.expectedError)
			} else {
				checkOrders(t, resp, testCase.wantedFieldList, testCase.WantedOrderList)
			}
		})
	}
}

func TestComputePageRange(t *testing.T) {
	tests := map[string]struct {
		pageSize      int32
		offset        int32
		totalCount    int
		expectedStart int
		expectedEnd   int
	}{
		"zeros":                           {pageSize: 0, offset: 0, totalCount: 0, expectedStart: 0, expectedEnd: -1},
		"whole array":                     {pageSize: 10, offset: 0, totalCount: 10, expectedStart: 0, expectedEnd: 10},
		"page larger than array":          {pageSize: 10, offset: 0, totalCount: 5, expectedStart: 0, expectedEnd: 5},
		"first page":                      {pageSize: 10, offset: 0, totalCount: 35, expectedStart: 0, expectedEnd: 10},
		"second page":                     {pageSize: 10, offset: 10, totalCount: 35, expectedStart: 10, expectedEnd: 20},
		"last page":                       {pageSize: 10, offset: 30, totalCount: 35, expectedStart: 30, expectedEnd: 35},
		"offset greater than total count": {pageSize: 10, offset: 40, totalCount: 35, expectedStart: 0, expectedEnd: -1},
		"total count zero with non-zero page size":  {pageSize: 10, offset: 0, totalCount: 0, expectedStart: 0, expectedEnd: -1},
		"large numbers - first page":                {pageSize: 1000, offset: 0, totalCount: 5000, expectedStart: 0, expectedEnd: 1000},
		"large numbers - middle page":               {pageSize: 1000, offset: 2000, totalCount: 5000, expectedStart: 2000, expectedEnd: 3000},
		"large numbers - last page":                 {pageSize: 1000, offset: 4000, totalCount: 5000, expectedStart: 4000, expectedEnd: 5000},
		"large numbers - offset beyond total count": {pageSize: 1000, offset: 6000, totalCount: 5000, expectedStart: 0, expectedEnd: -1},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			start, end := computePageRange(testCase.pageSize, testCase.offset, testCase.totalCount)
			assert.Equal(t, testCase.expectedStart, start)
			assert.Equal(t, testCase.expectedEnd, end)
		})
	}
}
