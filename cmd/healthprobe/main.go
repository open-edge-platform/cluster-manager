// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

// healthprobe is a minimal HTTP health check binary for use in distroless
// container HEALTHCHECK instructions. It exits 0 if the target responds
// with HTTP 2xx, or 1 otherwise.
//
// Usage: healthprobe <url>
//
//	healthprobe http://localhost:8080/v2/healthz
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: healthprobe <url>")
		os.Exit(1)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "unhealthy: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}
}
