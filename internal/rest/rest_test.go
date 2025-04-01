// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"

	"github.com/getkin/kin-openapi/openapi3filter"
)

type noopAuthenticator struct{}

func (auth noopAuthenticator) Authenticate(ctx context.Context, input *openapi3filter.AuthenticationInput) error {
	return openapi3filter.NoopAuthenticationFunc(ctx, input)
}
