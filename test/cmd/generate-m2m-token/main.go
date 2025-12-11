// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/open-edge-platform/cluster-manager/v2/test/helpers"
)

func main() {
	// Generate a long-lived M2M token (24 hours) with admin roles
	roles := []string{"uma_authorization", "admin"}
	token := helpers.CreateTestJWT(time.Now().Add(24*time.Hour), roles)
	fmt.Print(token)
	os.Exit(0)
}
