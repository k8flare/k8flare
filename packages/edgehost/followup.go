package edgehost

import (
	"encoding/json"
	"net/http"
)

const (
	refusedRetryS = 5
	gcSettleS     = 2
	metricsDelayS = 15
)

type followIn struct {
	Target          string   `json:"target"`
	PlanOK          bool     `json:"planOK"`
	ProvisionFailed bool     `json:"provisionFailed"`
	Changed         []string `json:"changed"`
	OK              bool     `json:"ok"`
	Skip            bool     `json:"skip"`
	Attempt         int      `json:"attempt"`
	RetryAfterS     int      `json:"retryAfterS"`
	Names           []string `json:"names"`
	Node            string   `json:"node"`
	NextMs          int64    `json:"nextMs"`
	Drained         bool     `json:"drained"`
	HasResult       bool     `json:"hasResult"`
	Deleted         int      `json:"deleted"`
	Patched         int      `json:"patched"`
	Pending         int      `json:"pending"`
	Status          int      `json:"status"`
	HasWork         bool     `json:"hasWork"`
	Remaining       int      `json:"remaining"`
	Terminating     int      `json:"terminating"`
	Phase           string   `json:"phase"`
}

type followSend struct {
	Queue        string   `json:"queue"`
	DelaySeconds int      `json:"delaySeconds,omitempty"`
	Kind         string   `json:"kind"`
	Attempt      *int     `json:"attempt,omitempty"`
	Changed      []string `json:"changed,omitempty"`
	Names        []string `json:"names,omitempty"`
	Node         string   `json:"node,omitempty"`
}

type followResult struct {
	Sends []followSend `json:"sends"`
	Stop  bool         `json:"stop"`
}

func FollowUp(w http.ResponseWriter, r *http.Request) {
	var in followIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(followUp(in))
}

func followUp(in followIn) followResult {
	out := followResult{Sends: []followSend{}}
	switch in.Target {
	case "scheduler":
		if !in.OK {
			out.Sends = append(out.Sends, retry("sched", intPtr(0), nil, nil, "", refusedRetryS))
			return out
		}
		if !in.Skip && in.RetryAfterS > 0 {
			out.Sends = append(out.Sends, retry("sched", intPtr(in.Attempt), nil, nil, "", in.RetryAfterS))
		}
	case "workloads":
		if !in.PlanOK {
			out.Sends = append(out.Sends, retry("wl", nil, nil, nil, "", refusedRetryS))
			return out
		}
		if in.ProvisionFailed {
			out.Sends = append(out.Sends, retry("wl", nil, in.Changed, nil, "", refusedRetryS))
		}
		if len(in.Changed) == 0 {
			return out
		}
		if !in.HasResult {
			out.Sends = append(out.Sends, retry("wl", nil, in.Changed, nil, "", refusedRetryS))
			return out
		}
		delayMs := in.NextMs
		if !in.Drained {
			delayMs = refusedRetryS * 1000
		}
		if delayMs > 0 {
			out.Sends = append(out.Sends, retry("wl", nil, in.Changed, nil, "", ceilSeconds(delayMs)))
		}
	case "crds":
		if in.Status != http.StatusOK || in.Pending > 0 {
			out.Sends = append(out.Sends, retry("crd", nil, nil, nil, "", refusedRetryS))
		}
	case "gc":
		if !in.HasResult || in.Deleted > 0 || in.Patched > 0 || in.Pending > 0 {
			delay := refusedRetryS
			if in.HasResult {
				delay = gcSettleS
			}
			out.Sends = append(out.Sends, retry("gc", nil, nil, nil, "", delay))
		}
	case "accounts":
		if in.Phase == "sync" {
			if !in.HasResult || !in.Drained {
				out.Sends = append(out.Sends, retry("acct", nil, nil, nil, "", refusedRetryS))
			}
			return out
		}
		delayMs := in.NextMs
		if !in.HasResult {
			delayMs = refusedRetryS * 1000
		}
		if delayMs > 0 {
			out.Sends = append(out.Sends, retry("acct", nil, nil, in.Names, "", atLeastOne(ceilSeconds(delayMs))))
		}
		out.Stop = in.HasResult && (in.Remaining > 0 || in.Terminating > 0)
	case "extensions":
		if !in.OK {
			out.Sends = append(out.Sends, retry("ext", nil, nil, nil, "", refusedRetryS))
		}
	case "leases":
		delayMs := in.NextMs
		if !in.HasResult {
			delayMs = refusedRetryS * 1000
		}
		if delayMs > 0 {
			out.Sends = append(out.Sends, retry("ctrl", nil, nil, nil, in.Node, ceilSeconds(delayMs)))
		}
	case "metrics":
		out.Sends = append(out.Sends, retry("hpa", nil, nil, nil, "", 0), retry("metrics", nil, nil, nil, "", metricsDelayS))
	case "containers":
		if in.HasWork {
			out.Sends = append(out.Sends, retry("containers", nil, nil, nil, "", refusedRetryS))
		}
	}
	return out
}

func retry(queue string, attempt *int, changed, names []string, node string, delay int) followSend {
	kind := "retry"
	if queue == "ctrl" {
		kind = "lease-check"
	}
	if len(changed) == 0 {
		changed = nil
	}
	if len(names) == 0 {
		names = nil
	}
	return followSend{Queue: queue, DelaySeconds: delay, Kind: kind, Attempt: attempt, Changed: changed, Names: names, Node: node}
}

func intPtr(v int) *int { return &v }

func ceilSeconds(ms int64) int {
	if ms <= 0 {
		return 0
	}
	return int((ms + 999) / 1000)
}

func atLeastOne(s int) int {
	if s < 1 {
		return 1
	}
	return s
}
