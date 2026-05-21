// SPDX-FileCopyrightText: (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package tenancyclient

import (
	"net"
	"net/url"
	"strings"
	"time"
)

// PickReachableURL returns the first URL from candidates that responds on its TCP
// port within a short timeout. If none are reachable, it returns the first
// candidate (if any).
func PickReachableURL(candidates []string) string {
	timeout := 2 * time.Second
	for _, c := range candidates {
		if c == "" {
			continue
		}

		u, err := url.Parse(c)
		if err != nil || u.Host == "" {
			continue
		}

		host := u.Host
		if !strings.Contains(host, ":") {
			if u.Scheme == "https" {
				host += ":443"
			} else {
				host += ":80"
			}
		}

		conn, err := net.DialTimeout("tcp", host, timeout)
		if err == nil {
			_ = conn.Close()
			return c
		}
	}

	if len(candidates) > 0 {
		return candidates[0]
	}

	return ""
}
