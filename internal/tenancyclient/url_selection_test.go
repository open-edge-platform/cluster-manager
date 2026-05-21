// SPDX-FileCopyrightText: (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package tenancyclient

import (
	"fmt"
	"net"
	"testing"
)

func TestPickReachableURLReturnsReachableCandidate(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	reachable := fmt.Sprintf("http://%s", ln.Addr().String())
	got := PickReachableURL([]string{"http://127.0.0.1:1", reachable})
	if got != reachable {
		t.Fatalf("PickReachableURL() = %q, want %q", got, reachable)
	}
}

func TestPickReachableURLFallsBackToFirstCandidate(t *testing.T) {
	first := "http://127.0.0.1:1"
	got := PickReachableURL([]string{first, "http://127.0.0.1:2"})
	if got != first {
		t.Fatalf("PickReachableURL() = %q, want %q", got, first)
	}
}

func TestPickReachableURLReturnsEmptyForNoCandidates(t *testing.T) {
	if got := PickReachableURL(nil); got != "" {
		t.Fatalf("PickReachableURL() = %q, want empty", got)
	}
}
