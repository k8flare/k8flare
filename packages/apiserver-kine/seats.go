package kine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const seatReleaseTimeout = 5 * time.Second

type SeatRequest struct {
	Level            string `json:"level"`
	Limit            int    `json:"limit"`
	Queues           int    `json:"queues"`
	QueueLengthLimit int    `json:"queueLengthLimit"`
	Hand             []int  `json:"hand"`
	WaitMs           int64  `json:"waitMs"`
	User             string `json:"user,omitempty"`
}

func (c *Client) AcquireSeat(ctx context.Context, seat SeatRequest) (release func(), retryAfter int, err error) {
	data, err := json.Marshal(seat)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kineBase+"/apf/seat", bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("kine POST /apf/seat: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Seat int64 `json:"seat"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, 0, fmt.Errorf("kine POST /apf/seat: %w", err)
		}
		var once sync.Once
		return func() { once.Do(func() { c.releaseSeat(context.WithoutCancel(ctx), out.Seat) }) }, 0, nil
	case http.StatusTooManyRequests:
		var out struct {
			RetryAfter int `json:"retryAfter"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, 0, fmt.Errorf("kine POST /apf/seat: status 429: %w", err)
		}
		return nil, max(out.RetryAfter, 1), nil
	}
	return nil, 0, fmt.Errorf("kine POST /apf/seat: status %d", resp.StatusCode)
}

func (c *Client) releaseSeat(ctx context.Context, seat int64) {
	ctx, cancel := context.WithTimeout(ctx, seatReleaseTimeout)
	defer cancel()
	data, _ := json.Marshal(map[string]int64{"seat": seat})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kineBase+"/apf/release", bytes.NewReader(data))
	if err != nil {
		println("kine POST /apf/release:", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		println("kine POST /apf/release:", err.Error())
		return
	}
	resp.Body.Close()
}
