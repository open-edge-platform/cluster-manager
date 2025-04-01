// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package config

import (
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	t.Skip("Test doesn't work in CI yet. It needs to be fixed.")

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		expect  string
	}{
		{
			name: "Valid config",
			cfg: Config{
				LogLevel:  0,
				LogFormat: "json",
			},
			wantErr: false,
		},
		{
			name: "Empty KubeConfig default path",
			cfg: Config{
				LogLevel:  0,
				LogFormat: "json",
			},
			wantErr: false,
		},
		{
			name: "Invalid LogLevel",
			cfg: Config{
				LogLevel:  10,
				LogFormat: "json",
			},
			wantErr: true,
		},
		{
			name: "Invalid Format",
			cfg: Config{
				LogLevel:  -4,
				LogFormat: "xml",
			},
			wantErr: true,
		},
		{
			name: "Invalid path KubeConfig",
			cfg: Config{
				LogLevel:  8,
				LogFormat: "human",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v - wantErr %v", err, tt.wantErr)
			}

		})
	}
}
