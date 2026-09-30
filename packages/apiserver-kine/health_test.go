package kine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func healthClient(t *testing.T, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Client{HTTP: rewriteListClient(srv)}
}

func TestHealthQueues(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"idle", `{"outbox":0,"flushFailingMs":0,"passes":[]}`, ""},
		{"backlog draining", `{"outbox":40,"flushFailingMs":1000,"passes":[]}`, ""},
		{"stuck", `{"outbox":40,"flushFailingMs":600000,"passes":[]}`, "outbox flush failing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, err := healthClient(t, c.body).Health(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			err = h.Queues()
			if c.want == "" && err != nil {
				t.Fatalf("unexpected %v", err)
			}
			if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
}

func TestHealthControllers(t *testing.T) {
	body := `{"outbox":0,"flushFailingMs":0,"passes":[{"target":"scheduler","pendingMs":0},{"target":"workloads","pendingMs":400000},{"target":"gc","pendingMs":1000}]}`
	h, err := healthClient(t, body).Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Controllers(func(target string) bool { return target == "scheduler" }); err != nil {
		t.Fatalf("scheduler: %v", err)
	}
	err = h.Controllers(func(target string) bool { return target != "scheduler" })
	if err == nil || !strings.Contains(err.Error(), "workloads") || strings.Contains(err.Error(), "gc") {
		t.Fatalf("controllers: %v", err)
	}
	if err := h.Controllers(nil); err == nil {
		t.Fatal("nil filter should include every pass")
	}
}

func TestHealthUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer srv.Close()
	if _, err := (&Client{HTTP: rewriteListClient(srv)}).Health(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
