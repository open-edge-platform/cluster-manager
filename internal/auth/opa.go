// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	opa "github.com/open-edge-platform/orch-library/go/pkg/openpolicyagent"
)

const (
	regoPackage = "authz"
	regoQuery   = "allow"
)

// NewOpaClient returns a new OPA client
func NewOpaClient(port int) (opa.ClientWithResponsesInterface, error) {
	opaAddr := fmt.Sprintf("http://localhost:%d", port)
	client, err := opa.NewClientWithResponses(opaAddr)
	if err != nil {
		return nil, err
	}

	slog.Info("opa is enabled", "addr", opaAddr)

	return client, nil
}

// evaluatePolicy evaluates the policy using the OPA client
func evaluatePolicy(ctx context.Context, client opa.ClientWithResponsesInterface, roles []string, method, path, projectId string) error {
	input := opa.OpaInput{
		Input: map[string]any{
			"method":     method,
			"path":       path,
			"project_id": projectId,
			"roles":      roles,
		},
	}

	jsonInput, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal policy input: %w", err)
	}

	inputBytes := bytes.NewReader(jsonInput)
	resp, err := client.PostV1DataPackageRuleWithBodyWithResponse(
		ctx, regoPackage, regoQuery, &opa.PostV1DataPackageRuleParams{}, "application/json", inputBytes)
	if err != nil {
		return fmt.Errorf("failed to evaluate policy: %w", err)
	}

	allowed, err := resp.JSON200.Result.AsOpaResponseResult1()
	if err != nil {
		return fmt.Errorf("failed to parse policy result: %w", err)
	}
	if !allowed {
		return errors.New("authorization denied")
	}

	return nil
}
