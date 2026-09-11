//go:build js && wasm

package cloudflare

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"syscall/js"
	"testing"
	"time"
)

func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	}()
	fn()
	return buf.String()
}

func TestPumpTraceIsSilentWhenTheVarIsUnset(t *testing.T) {
	if PumpTraceEnabled() {
		t.Fatal("tracing reported on with no PUMP_TRACE set; the default must cost nothing")
	}
	out := captureLog(t, func() { PumpTrace("observed", "kcm", "default/p", "747") })
	if out != "" {
		t.Errorf("PumpTrace wrote %q with tracing off, want nothing", out)
	}
}

func TestPumpTraceNamesTheWindowItHappenedIn(t *testing.T) {
	id := OpenPumpWindow(js.ValueOf(map[string]any{}), 5_000)
	defer ClosePumpWindow(id)

	line := formatPumpTrace("observed", "kcm", "default/p", "747", CurrentWindowID(), time.UnixMilli(1757600000000))
	var got struct {
		B  string `json:"b"`
		C  string `json:"c"`
		W  int    `json:"w"`
		O  string `json:"o"`
		RV string `json:"rv"`
		T  int64  `json:"t"`
	}
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("trace line is not JSON: %v (%s)", err, line)
	}
	if got.W != id {
		t.Errorf("window = %d, want %d -- an observation that cannot be attributed to a window measures nothing", got.W, id)
	}
	if got.B != "observed" || got.C != "kcm" || got.O != "default/p" || got.RV != "747" {
		t.Errorf("trace line lost a field: %+v", got)
	}
	if got.T != 1757600000000 {
		t.Errorf("t = %d, want 1757600000000", got.T)
	}
}

func TestCurrentWindowIDNeverBlocks(t *testing.T) {
	// CurrentWindow blocks until a window opens; labelling an observation
	// must not. Other tests in this package leave windows open, so this
	// asserts the timing property rather than a particular id.
	done := make(chan int, 1)
	go func() { done <- CurrentWindowID() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CurrentWindowID blocked; a trace observation must never wait for a window")
	}
}

func TestCurrentWindowIDIsZeroOnceEveryWindowIsClosed(t *testing.T) {
	windowsMu.Lock()
	open := append([]*Window(nil), windows...)
	windowsMu.Unlock()
	for _, w := range open {
		ClosePumpWindow(w.ID())
	}
	if got := CurrentWindowID(); got != 0 {
		t.Errorf("CurrentWindowID() = %d with every window closed, want 0", got)
	}
	id := OpenPumpWindow(js.ValueOf(map[string]any{}), 5_000)
	t.Cleanup(func() { ClosePumpWindow(id) })
	if got := CurrentWindowID(); got != id {
		t.Errorf("CurrentWindowID() = %d after opening window %d", got, id)
	}
}

func TestPumpTraceLineIsPrefixedForLogSearch(t *testing.T) {
	line := "pumptrace " + formatPumpTrace("commit", "storage", "k/v", "1", 0, time.Now())
	if !strings.HasPrefix(line, "pumptrace {") {
		t.Errorf("trace line = %q, want a stable prefix a log search can filter on", line)
	}
}
