// SPDX-FileCopyrightText: (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

// keeping here for now just to test but move to orch-library if we want to reuse in other places, 
// or remove if we decide to keep this logic in the manager  instead of the client
package tenancyclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	libauth "github.com/open-edge-platform/orch-library/go/pkg/auth"
	libtenancy "github.com/open-edge-platform/orch-library/go/pkg/tenancy"
)

const (
    activeProjectIDHeader              = "Activeprojectid"
    tenancyManagerActiveProjectIDEnv   = "TENANCY_MANAGER_ACTIVE_PROJECT_ID"
    tenancyManagerEventsProjectPathEnv = "TENANCY_MANAGER_EVENTS_PROJECT_PATH"
)

// Minimal PollerConfig compatible with orch-library's tenancy PollerConfig
type PollerConfig struct {
    PollInterval   time.Duration
    PollLimit      int
    InitialBackoff time.Duration
    MaxBackoff     time.Duration
    HTTPTimeout    time.Duration
    OnError        func(err error, msg string)
}

func DefaultPollerConfig() PollerConfig {
    return PollerConfig{
        PollInterval:   5 * time.Second,
        PollLimit:      100,
        InitialBackoff: 1 * time.Second,
        MaxBackoff:     30 * time.Second,
        HTTPTimeout:    30 * time.Second,
    }
}

type Poller struct {
    tenantManagerURL string
    controllerName   string
    handler          libtenancy.Handler
    cfg              PollerConfig
    client           *http.Client
    tokenProvider    func(ctx context.Context) (string, error)
}

// NewAuthPoller creates a Poller that injects Authorization header using tokenProvider.
func NewAuthPoller(tenantManagerURL, controllerName string, handler libtenancy.Handler, tokenProvider func(ctx context.Context) (string, error), opts ...func(*PollerConfig)) (*Poller, error) {
    if tenantManagerURL == "" {
        return nil, fmt.Errorf("tenantManagerURL must not be empty")
    }
    if controllerName == "" {
        return nil, fmt.Errorf("controllerName must not be empty")
    }
    if handler == nil {
        return nil, fmt.Errorf("handler must not be nil")
    }
    if tokenProvider == nil {
        return nil, fmt.Errorf("tokenProvider must not be nil")
    }

    cfg := DefaultPollerConfig()
    for _, opt := range opts {
        opt(&cfg)
    }

    client := &http.Client{Timeout: cfg.HTTPTimeout}
    return &Poller{tenantManagerURL: tenantManagerURL, controllerName: controllerName, handler: handler, cfg: cfg, client: client, tokenProvider: tokenProvider}, nil
}

type eventsResponse struct {
    Events      []libtenancy.Event `json:"events"`
    LastEventID int64               `json:"lastEventId"`
}

func (p *Poller) Run(ctx context.Context) error {
    // replay
    last, err := p.replayWithRetry(ctx)
    if err != nil {
        return err
    }
    // steady-state
    ticker := time.NewTicker(p.cfg.PollInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            newLast, err := p.poll(ctx, last)
            if err != nil {
                p.logError(err, "poll failed, will retry next interval")
                continue
            }
            last = newLast
        }
    }
}

func (p *Poller) logError(err error, msg string) {
    if p.cfg.OnError != nil {
        p.cfg.OnError(err, msg)
    }
}

func (p *Poller) replayWithRetry(ctx context.Context) (int64, error) {
    backoff := p.cfg.InitialBackoff
    for {
        last, err := p.replay(ctx)
        if err == nil {
            return last, nil
        }
        p.logError(err, fmt.Sprintf("replay failed (controller=%s), retrying in %s", p.controllerName, backoff))
        select {
        case <-ctx.Done():
            return 0, ctx.Err()
        case <-time.After(backoff):
        }
        backoff *= 2
        if backoff > p.cfg.MaxBackoff {
            backoff = p.cfg.MaxBackoff
        }
    }
}

func (p *Poller) replay(ctx context.Context) (int64, error) {
    reqURL := p.buildEventsURL(true, 0)
    resp, err := p.doGet(ctx, reqURL)
    if err != nil {
        return 0, err
    }
    for _, ev := range resp.Events {
        if err := p.handler.HandleEvent(ctx, ev); err != nil {
            return 0, fmt.Errorf("replay event %s %s/%s failed (controller=%s): %w", ev.EventType, ev.ResourceType, ev.ResourceName, p.controllerName, err)
        }
    }
    return resp.LastEventID, nil
}

func (p *Poller) poll(ctx context.Context, lastEventID int64) (int64, error) {
    reqURL := p.buildEventsURL(false, lastEventID)
    resp, err := p.doGet(ctx, reqURL)
    if err != nil {
        return lastEventID, err
    }
    processed := lastEventID
    for _, ev := range resp.Events {
        if err := p.handler.HandleEvent(ctx, ev); err != nil {
            p.logError(err, fmt.Sprintf("event %s %s/%s failed (controller=%s), will retry on next poll", ev.EventType, ev.ResourceType, ev.ResourceName, p.controllerName))
            return processed, nil
        }
        processed = ev.ID
    }
    return processed, nil
}

func (p *Poller) doGet(ctx context.Context, reqURL string) (*eventsResponse, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
    if err != nil {
        return nil, err
    }
    // add auth header
    if p.tokenProvider != nil {
        if tok, err := p.tokenProvider(ctx); err == nil && tok != "" {
            req.Header.Set("Authorization", "Bearer "+tok)
        }
    }

    if projectID, source := resolveActiveProjectID(req.Header.Get("Authorization")); projectID != "" {
        req.Header.Set(activeProjectIDHeader, projectID)
        slog.Info("doGet request has active project id header", "url", reqURL, "source", source, "project_id", projectID)
    } else {
        slog.Info("doGet request missing active project id header", "url", reqURL)
    }

    // debug: surface whether Authorization header is set on the outgoing request
    if ah := req.Header.Get("Authorization"); ah == "" {
        slog.Info("doGet request missing Authorization header", "url", reqURL)
    } else {
        prefix := ah
        if len(prefix) > 64 {
            prefix = prefix[:64]
        }
        slog.Info("doGet request has Authorization header", "url", reqURL, "auth_prefix", prefix)
    }

    resp, err := p.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("tenant manager returned %d: %s", resp.StatusCode, string(body))
    }
    var er eventsResponse
    if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
        return nil, fmt.Errorf("decode events response: %w", err)
    }
    return &er, nil
}

func (p *Poller) buildEventsURL(replay bool, lastEventID int64) string {
    endpoint := fmt.Sprintf("%s/v1/events", strings.TrimRight(p.tenantManagerURL, "/"))
    if projectPath := strings.TrimSpace(os.Getenv(tenancyManagerEventsProjectPathEnv)); projectPath != "" {
        endpoint = fmt.Sprintf("%s/v1/projects/%s/events", strings.TrimRight(p.tenantManagerURL, "/"), url.PathEscape(projectPath))
    }

    values := url.Values{}
    values.Set("controller", p.controllerName)
    if replay {
        values.Set("replay", "true")
    } else {
        values.Set("after", fmt.Sprintf("%d", lastEventID))
        values.Set("limit", fmt.Sprintf("%d", p.cfg.PollLimit))
    }

    return endpoint + "?" + values.Encode()
}

func resolveActiveProjectID(authHeader string) (string, string) {
    if projectID := strings.TrimSpace(os.Getenv(tenancyManagerActiveProjectIDEnv)); projectID != "" {
        return projectID, "env"
    }

    if authHeader == "" {
        return "", ""
    }

    projectIDs, err := libauth.ExtractAllProjectIDsFromJWT(authHeader)
    if err != nil || len(projectIDs) != 1 {
        return "", ""
    }

    for projectID := range projectIDs {
        return projectID, "jwt"
    }

    return "", ""
}
