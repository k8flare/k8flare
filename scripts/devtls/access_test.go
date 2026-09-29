package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func TestLogsAWatchStreamWithBytesAndLastWrite(t *testing.T) {
	logged := captureLog(t)
	h := accessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"type":"ADDED"}`)
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush through the access log: %v", err)
		}
	}), 5*time.Second)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/pods?watch=true&fieldSelector=spec.nodeName%3Dn1")
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()
	srv.Close()

	line := logged.String()
	for _, want := range []string{"GET /api/v1/pods?watch=true&fieldSelector=spec.nodeName%3Dn1", "status=200", "bytes=16", "last_byte="} {
		if !strings.Contains(line, want) {
			t.Fatalf("log %q is missing %q", line, want)
		}
	}
}

func TestSkipsShortNonWatchRequests(t *testing.T) {
	logged := captureLog(t)
	h := accessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}), 5*time.Second)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/namespaces")
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()
	srv.Close()

	if strings.Contains(logged.String(), "devtls: access") {
		t.Fatalf("a short request was logged: %q", logged.String())
	}
}
