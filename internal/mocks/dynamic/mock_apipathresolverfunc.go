// Code generated by mockery v2.53.0. DO NOT EDIT.

package dynamic

import (
	mock "github.com/stretchr/testify/mock"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
)

// MockAPIPathResolverFunc is an autogenerated mock type for the APIPathResolverFunc type
type MockAPIPathResolverFunc struct {
	mock.Mock
}

type MockAPIPathResolverFunc_Expecter struct {
	mock *mock.Mock
}

func (_m *MockAPIPathResolverFunc) EXPECT() *MockAPIPathResolverFunc_Expecter {
	return &MockAPIPathResolverFunc_Expecter{mock: &_m.Mock}
}

// Execute provides a mock function with given fields: kind
func (_m *MockAPIPathResolverFunc) Execute(kind schema.GroupVersionKind) string {
	ret := _m.Called(kind)

	if len(ret) == 0 {
		panic("no return value specified for Execute")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func(schema.GroupVersionKind) string); ok {
		r0 = rf(kind)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// MockAPIPathResolverFunc_Execute_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Execute'
type MockAPIPathResolverFunc_Execute_Call struct {
	*mock.Call
}

// Execute is a helper method to define mock.On call
//   - kind schema.GroupVersionKind
func (_e *MockAPIPathResolverFunc_Expecter) Execute(kind interface{}) *MockAPIPathResolverFunc_Execute_Call {
	return &MockAPIPathResolverFunc_Execute_Call{Call: _e.mock.On("Execute", kind)}
}

func (_c *MockAPIPathResolverFunc_Execute_Call) Run(run func(kind schema.GroupVersionKind)) *MockAPIPathResolverFunc_Execute_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(schema.GroupVersionKind))
	})
	return _c
}

func (_c *MockAPIPathResolverFunc_Execute_Call) Return(_a0 string) *MockAPIPathResolverFunc_Execute_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockAPIPathResolverFunc_Execute_Call) RunAndReturn(run func(schema.GroupVersionKind) string) *MockAPIPathResolverFunc_Execute_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockAPIPathResolverFunc creates a new instance of MockAPIPathResolverFunc. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockAPIPathResolverFunc(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockAPIPathResolverFunc {
	mock := &MockAPIPathResolverFunc{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
