// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package auth_test

import (
	"github.com/stretchr/testify/mock"
)

type MockProvider struct {
	mock.Mock
}

func NewMockProvider(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockProvider {
	mock := &MockProvider{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}

func (m *MockProvider) GetSigningKey(kid string) (interface{}, error) {
	ret := m.Called(kid)

	if len(ret) == 0 {
		panic("no return value specified for GetSigningKey")
	}

	var r0 interface{}
	if rf, ok := ret.Get(0).(func(string) interface{}); ok {
		r0 = rf(kid)
	} else {
		r0 = ret.Get(0)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(kid)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
