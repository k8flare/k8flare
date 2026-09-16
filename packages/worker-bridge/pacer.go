package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	pollInterval = 2 * time.Second
	idlePolls    = 2
	quietPeriod  = 5 * time.Second
	minWake      = 15 * time.Second
	maxWake      = 5 * time.Minute
	maxWindow    = 5 * time.Minute
)

var lastPoke atomic.Int64

func Poked() {
	lastPoke.Store(time.Now().UnixNano())
}

func quiet() bool {
	return time.Since(time.Unix(0, lastPoke.Load())) >= quietPeriod
}

func Quiet() bool { return quiet() }


type Pacer struct {
	last   string
	streak int
}

func (p *Pacer) Reset() {
	p.last = ""
	p.streak = 0
}

func (p *Pacer) Next(pending string) time.Duration {
	if pending == "" {
		p.Reset()
		return 0
	}
	if pending == p.last {
		p.streak++
	} else {
		p.last = pending
		p.streak = 0
	}
	d := minWake << uint(p.streak)
	if d > maxWake || d <= 0 {
		return maxWake
	}
	return d
}

type HoldRequest struct {
	Window time.Duration
	Min    time.Duration
	Reset  bool
	Kick   bool
}

func ParseHold(r *http.Request, defaultWindow time.Duration) HoldRequest {
	return HoldRequest{
		Window: durationParam(r, "window", defaultWindow),
		Min:    durationParam(r, "min", 0),
		Reset:  r.URL.Query().Get("reset") == "1",
		Kick:   r.URL.Query().Get("kick") == "1",
	}
}

func durationParam(r *http.Request, name string, fallback time.Duration) time.Duration {
	ms, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil || ms < 0 {
		return fallback
	}
	if d := time.Duration(ms) * time.Millisecond; d < maxWindow {
		return d
	}
	return maxWindow
}

func Hold(ctx context.Context, h HoldRequest, idle func() bool) {
	start := time.Now()
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	idleFor := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		elapsed := time.Since(start)
		if elapsed >= h.Window {
			return
		}
		if elapsed >= h.Min && quiet() && idle() {
			idleFor++
		} else {
			idleFor = 0
		}
		if idleFor >= idlePolls {
			return
		}
	}
}

func WriteNext(w http.ResponseWriter, next time.Duration) {
	json.NewEncoder(w).Encode(map[string]int64{"next": next.Milliseconds()})
}
