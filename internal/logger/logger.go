// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/open-edge-platform/cluster-manager/internal/config"
)

// initializeLogger initializes the logger with the given config
func InitializeLogger(cfg *config.Config) {
	replaceAttributes := func(groups []string, a slog.Attr) slog.Attr {
		switch a.Value.Kind() {
		case slog.KindTime:
			a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02T15:04:05.000Z07:00"))
		case slog.KindAny:
			if a.Key == slog.SourceKey {
				v := a.Value.Any().(*slog.Source)
				a.Value = slog.StringValue(fmt.Sprintf("%s:%d %s",
					v.File[strings.LastIndex(v.File, "/")+1:],
					v.Line,
					v.Function[strings.LastIndex(v.Function, ".")+1:]))
			}
		}

		return a
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       slog.Level(cfg.LogLevel),
		ReplaceAttr: replaceAttributes,
		AddSource:   true,
	})))

	if cfg.LogFormat == "human" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:       slog.Level(cfg.LogLevel),
			ReplaceAttr: replaceAttributes,
			AddSource:   true,
		})))
	}
}
