package kine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const StaleAfter = 5 * time.Minute

type PassHealth struct {
	Target    string `json:"target"`
	PendingMs int64  `json:"pendingMs"`
}

type Health struct {
	Outbox         int          `json:"outbox"`
	FlushFailingMs int64        `json:"flushFailingMs"`
	Passes         []PassHealth `json:"passes"`
}

func (c *Client) Health(ctx context.Context) (*Health, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kineBase+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kine GET /health: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kine GET /health: status %d", resp.StatusCode)
	}
	var out Health
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("kine GET /health: %w", err)
	}
	return &out, nil
}

func stale(ms int64) bool { return time.Duration(ms)*time.Millisecond > StaleAfter }

func age(ms int64) time.Duration { return (time.Duration(ms) * time.Millisecond).Round(time.Second) }

func (h *Health) Queues() error {
	if stale(h.FlushFailingMs) {
		return fmt.Errorf("outbox flush failing for %s with %d messages undelivered", age(h.FlushFailingMs), h.Outbox)
	}
	return nil
}

func (h *Health) Controllers(include func(target string) bool) error {
	var stuck []string
	for _, p := range h.Passes {
		if (include == nil || include(p.Target)) && stale(p.PendingMs) {
			stuck = append(stuck, fmt.Sprintf("%s pending for %s", p.Target, age(p.PendingMs)))
		}
	}
	if len(stuck) > 0 {
		return fmt.Errorf("%s", strings.Join(stuck, ", "))
	}
	return nil
}
