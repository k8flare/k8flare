package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const droppedBody = "Error: Network connection lost.\n    at async Object.fetch (file:///miniflare/dist/src/workers/core/entry.worker.js:1:1)"

func TestRetriesAWriteDroppedByTheProxyWorker(t *testing.T) {
	var calls atomic.Int32
	var bodies []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if calls.Add(1) == 1 {
			http.Error(w, droppedBody, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, "created")
	}))
	defer upstream.Close()

	req, _ := http.NewRequest(http.MethodPost, upstream.URL+"/api/v1/namespaces/ns/events", strings.NewReader(`{"kind":"Event"}`))
	resp, err := retryDropped{base: http.DefaultTransport}.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated || string(got) != "created" {
		t.Fatalf("got %d %q, want the retried 201", resp.StatusCode, got)
	}
	if calls.Load() != 2 || bodies[1] != `{"kind":"Event"}` {
		t.Fatalf("calls=%d bodies=%q, want the same body sent twice", calls.Load(), bodies)
	}
}

func TestPassesThroughOtherServerErrors(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "etcdserver: request timed out", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	req, _ := http.NewRequest(http.MethodPut, upstream.URL+"/api/v1/namespaces/ns/configmaps/c", strings.NewReader("{}"))
	resp, err := retryDropped{base: http.DefaultTransport}.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(string(got), "etcdserver") || calls.Load() != 1 {
		t.Fatalf("got %d %q after %d calls, want the original 500 untouched", resp.StatusCode, got, calls.Load())
	}
}

func TestGivesUpAfterRepeatedDrops(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, droppedBody, http.StatusInternalServerError)
	}))
	defer upstream.Close()

	req, _ := http.NewRequest(http.MethodPatch, upstream.URL+"/api/v1/nodes/n/status", strings.NewReader("{}"))
	resp, err := retryDropped{base: http.DefaultTransport}.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusInternalServerError || !strings.HasPrefix(string(got), "Error: Network connection lost.") {
		t.Fatalf("got %d %q, want the last dropped response", resp.StatusCode, got)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d, want three attempts", calls.Load())
	}
}

func TestReplaysARequestTheBackendClosedWithoutAnswering(t *testing.T) {
	logged := captureLog(t)
	var calls atomic.Int32
	var bodies []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if calls.Add(1) == 1 {
			conn, _, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	req, _ := http.NewRequest(http.MethodPatch, upstream.URL+"/api/v1/nodes/n", strings.NewReader("[]"))
	resp, err := retryDropped{base: http.DefaultTransport}.RoundTrip(req)
	if err != nil {
		t.Fatalf("PATCH after the backend closed the first attempt: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want the replayed 200", resp.StatusCode)
	}
	if calls.Load() != 2 || bodies[1] != "[]" {
		t.Fatalf("calls=%d bodies=%q, want the same body sent twice", calls.Load(), bodies)
	}
	if !strings.Contains(logged.String(), "PATCH /api/v1/nodes/n lost its connection") {
		t.Fatalf("log %q does not record the replay", logged.String())
	}
}

func TestReplaysARequestTheBackendResetWithoutReading(t *testing.T) {
	logged := captureLog(t)
	var calls atomic.Int32
	var bodies []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			conn, _, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			conn.(*net.TCPConn).SetLinger(0)
			conn.Close()
			return
		}
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.WriteHeader(http.StatusCreated)
	}))
	defer upstream.Close()

	req, _ := http.NewRequest(http.MethodPost, upstream.URL+"/api/v1/namespaces", strings.NewReader(`{"kind":"Namespace"}`))
	resp, err := retryDropped{base: http.DefaultTransport}.RoundTrip(req)
	if err != nil {
		t.Fatalf("POST after the backend reset the first attempt: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want the replayed 201", resp.StatusCode)
	}
	if calls.Load() != 2 || bodies[0] != `{"kind":"Namespace"}` {
		t.Fatalf("calls=%d bodies=%q, want the body replayed once", calls.Load(), bodies)
	}
	if !strings.Contains(logged.String(), "POST /api/v1/namespaces lost its connection") {
		t.Fatalf("log %q does not record the replay", logged.String())
	}
}

func TestDoesNotReplayAfterResponseBytesArrived(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		conn, buf, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		buf.WriteString("HTTP/1.1 201 Created\r\nContent-Length: 100\r\n\r\npartial")
		buf.Flush()
		conn.(*net.TCPConn).SetLinger(0)
		conn.Close()
	}))
	defer upstream.Close()

	req, _ := http.NewRequest(http.MethodPost, upstream.URL+"/api/v1/namespaces", strings.NewReader(`{"kind":"Namespace"}`))
	resp, err := retryDropped{base: http.DefaultTransport}.RoundTrip(req)
	if err != nil {
		t.Fatalf("the started response was not returned: %v", err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Fatal("the truncated body read without an error")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d, want no replay once the response started", calls.Load())
	}
}
