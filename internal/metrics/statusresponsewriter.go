// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"net/http"
	"strconv"
)

type StatusResponseWriter struct {
	wr     http.ResponseWriter
	status int
}

func NewStatusResponseWriter(wr http.ResponseWriter) *StatusResponseWriter {
	return &StatusResponseWriter{
		wr: wr,
	}
}

func (srw *StatusResponseWriter) WriteHeader(code int) {
	srw.status = code
	srw.wr.WriteHeader(code)
}

func (srw *StatusResponseWriter) Write(b []byte) (int, error) {
	return srw.wr.Write(b)
}

func (srw *StatusResponseWriter) Header() http.Header {
	return srw.wr.Header()
}

func (srw *StatusResponseWriter) Status() string {
	return strconv.Itoa(srw.status)
}
