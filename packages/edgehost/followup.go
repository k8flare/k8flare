package edgehost

import (
	"encoding/json"
	"net/http"
)

const (
	refusedRetryS                     = 5
	failedRetryMaxS                   = 1000
	gcSettleS                         = 2
	horizontalPodAutoscalerSyncPeriod = 15

	storageMigrateOnly = "storage-migrate"
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
	HPAs            int      `json:"hpas"`
	Failed          int      `json:"failed"`
	Pending         int      `json:"pending"`
	Status          int      `json:"status"`
	Remaining       int      `json:"remaining"`
	Terminating     int      `json:"terminating"`
	Phase           string   `json:"phase"`
	Failures        int      `json:"failures"`
	Busy            bool     `json:"busy"`
	MigrateFailed   bool     `json:"migrateFailed"`
}

type followSend struct {
	Queue        string   `json:"queue"`
	DelaySeconds int      `json:"delaySeconds,omitempty"`
	Kind         string   `json:"kind"`
	Attempt      *int     `json:"attempt,omitempty"`
	Changed      []string `json:"changed,omitempty"`
	Names        []string `json:"names,omitempty"`
	Node         string   `json:"node,omitempty"`
	Failures     int      `json:"failures,omitempty"`
}

type followResult struct {
	Sends  []followSend `json:"sends"`
	Cancel []string     `json:"cancel,omitempty"`
	Stop   bool         `json:"stop"`
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
			out.Sends = append(out.Sends, backoff("sched", intPtr(0), nil, nil, "", in.Failures))
			return out
		}
		if !in.Skip && in.RetryAfterS > 0 {
			out.Sends = append(out.Sends, retry("sched", intPtr(in.Attempt), nil, nil, "", in.RetryAfterS))
		}
	case "workloads":
		if !in.PlanOK {
			out.Sends = append(out.Sends, backoff("wl", nil, nil, nil, "", in.Failures))
			return out
		}
		if in.ProvisionFailed {
			out.Sends = append(out.Sends, backoff("wl", nil, in.Changed, nil, "", in.Failures))
		}
		if len(in.Changed) == 0 {
			return out
		}
		switch {
		case !in.HasResult || (!in.Drained && !in.Busy):
			out.Sends = append(out.Sends, backoff("wl", nil, in.Changed, nil, "", in.Failures))
		case in.Busy:
			out.Sends = append(out.Sends, busyRetry("wl", in.Changed, in.Failures))
		case in.NextMs > 0:
			out.Sends = append(out.Sends, retry("wl", nil, in.Changed, nil, "", ceilSeconds(in.NextMs)))
		}
	case "crds":
		if in.Status != http.StatusOK || in.Pending > 0 {
			out.Sends = append(out.Sends, backoff("crd", nil, nil, nil, "", in.Failures))
		}
	case "gc":
		if !in.HasResult || in.Failed > 0 {
			out.Sends = append(out.Sends, backoff("gc", nil, nil, nil, "", in.Failures))
		} else if in.Remaining > 0 {
			out.Sends = append(out.Sends, retry("gc", nil, nil, nil, "", gcSettleS))
		}
	case "accounts":
		if in.Phase == "sync" {
			switch {
			case !in.HasResult || (!in.Drained && !in.Busy):
				out.Sends = append(out.Sends, backoff("acct", nil, nil, nil, "", in.Failures))
			case in.Busy:
				out.Sends = append(out.Sends, busyRetry("acct", nil, in.Failures))
			}
			return out
		}
		if !in.HasResult {
			out.Sends = append(out.Sends, backoff("acct", nil, nil, in.Names, "", in.Failures))
		} else if in.NextMs > 0 {
			out.Sends = append(out.Sends, retry("acct", nil, nil, in.Names, "", atLeastOne(ceilSeconds(in.NextMs))))
		}
		out.Stop = in.HasResult && (in.Remaining > 0 || in.Terminating > 0)
	case "extensions":
		if !in.OK {
			out.Sends = append(out.Sends, backoff("ext", nil, nil, nil, "", in.Failures))
		}
	case "addons":
		if !in.OK {
			out.Sends = append(out.Sends, backoff("addons", nil, nil, nil, "", in.Failures))
		} else if in.MigrateFailed {
			out.Sends = append(out.Sends, backoff("addons", nil, nil, []string{storageMigrateOnly}, "", in.Failures))
		} else if in.Pending > 0 {
			out.Sends = append(out.Sends, retry("addons", nil, nil, []string{storageMigrateOnly}, "", refusedRetryS))
		}
		if in.OK && in.NextMs > 0 {
			out.Sends = append(out.Sends, retry("addons", nil, nil, nil, "", ceilSeconds(in.NextMs)))
		} else if in.OK && !in.Skip {
			out.Cancel = append(out.Cancel, "addons")
		}
	case "leases":
		if !in.HasResult {
			out.Sends = append(out.Sends, backoff("ctrl", nil, nil, nil, in.Node, in.Failures))
		} else if in.NextMs > 0 {
			out.Sends = append(out.Sends, retry("ctrl", nil, nil, nil, in.Node, ceilSeconds(in.NextMs)))
		}
	case "hpa":
		if !in.HasResult {
			out.Sends = append(out.Sends, backoff("hpa", nil, nil, nil, "", in.Failures))
		} else if in.HPAs > 0 {
			out.Sends = append(out.Sends, retry("hpa", nil, nil, nil, "", horizontalPodAutoscalerSyncPeriod))
		}
	case "containers":
		if !in.OK {
			out.Sends = append(out.Sends, backoff("containers", nil, nil, nil, "", in.Failures))
		} else if in.NextMs > 0 {
			out.Sends = append(out.Sends, retry("containers", nil, nil, nil, "", ceilSeconds(in.NextMs)))
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

func backoff(queue string, attempt *int, changed, names []string, node string, failures int) followSend {
	send := retry(queue, attempt, changed, names, node, failedRetryDelayS(failures))
	send.Failures = failures + 1
	return send
}

func failedRetryDelayS(failures int) int {
	delay := refusedRetryS
	for range failures {
		if delay >= failedRetryMaxS {
			break
		}
		delay *= 2
	}
	return min(delay, failedRetryMaxS)
}

func busyRetry(queue string, changed []string, failures int) followSend {
	send := retry(queue, nil, changed, nil, "", refusedRetryS)
	send.Failures = failures
	return send
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
