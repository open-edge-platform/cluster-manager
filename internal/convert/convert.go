// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package convert

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// ToUnstructured converts any type to an unstructured object
func ToUnstructured[T any](obj T) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}

	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&obj)
	if err != nil {
		return nil, err
	}

	u.Object = m
	return u, nil
}

// FromUnstructured converts an unstructured object to any type
func FromUnstructured[T any](u unstructured.Unstructured, obj T) error {
	return runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
}

// ToUnstructuredList converts a list of any type to an unstructured object
func ToUnstructuredList[T any](list []T) (*unstructured.UnstructuredList, error) {
	u := &unstructured.UnstructuredList{}

	for _, item := range list {
		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&item)
		if err != nil {
			return nil, err
		}
		u.Items = append(u.Items, unstructured.Unstructured{Object: m})
	}
	return u, nil
}

func MapStringToAny(original map[string]string) map[string]any {
	converted := make(map[string]any)
	for key, value := range original {
		converted[key] = value
	}
	return converted
}

func MapAnyToString(original map[string]any) map[string]string {
	converted := make(map[string]string)
	for key, value := range original {
		converted[key] = value.(string)
	}
	return converted
}

func Int32Ptr(num int32) *int32 {
	return &num
}

// Ptr returns a pointer to the value passed in
func Ptr[T any](v T) *T {
	return &v
}
