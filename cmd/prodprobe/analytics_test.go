package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseInvocations(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    invocations
		wantErr string
	}{
		{
			name: "sums every node",
			body: `{"data":{"viewer":{"accounts":[{
				"workersInvocationsAdaptive":[{"sum":{"requests":40,"errors":2}},{"sum":{"requests":2,"errors":0}}],
				"durableObjectsInvocationsAdaptiveGroups":[{"sum":{"requests":17,"errors":1}}]}]}}}`,
			want: invocations{WorkerRequests: 42, WorkerErrors: 2, DORequests: 17, DOErrors: 1},
		},
		{
			name: "a genuinely quiet window is zero, not an error",
			body: `{"data":{"viewer":{"accounts":[{"workersInvocationsAdaptive":[],"durableObjectsInvocationsAdaptiveGroups":[]}]}}}`,
			want: invocations{},
		},
		{
			name:    "GraphQL errors are never read as zero",
			body:    `{"data":null,"errors":[{"message":"Unknown field 'sum'"}]}`,
			wantErr: "Unknown field 'sum'",
		},
		{
			name:    "no matching account is an error, not zero",
			body:    `{"data":{"viewer":{"accounts":[]}}}`,
			wantErr: "no account",
		},
		{
			name:    "a null data block is an error, not zero",
			body:    `{"data":null}`,
			wantErr: "no data",
		},
		{
			name:    "garbage is an error",
			body:    `<html>502 Bad Gateway</html>`,
			wantErr: "decode GraphQL response",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInvocations([]byte(tt.body))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestInvocationsSendsTokenAndWindow(t *testing.T) {
	var gotAuth string
	var gotVars map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("server got undecodable body: %v", err)
		}
		gotVars = payload.Variables
		io.WriteString(w, `{"data":{"viewer":{"accounts":[{"workersInvocationsAdaptive":[{"sum":{"requests":3,"errors":0}}],"durableObjectsInvocationsAdaptiveGroups":[{"sum":{"requests":4,"errors":0}}]}]}}}`)
	}))
	defer srv.Close()

	c := newAnalyticsClient("acct123", "tok456", "k8flare")
	c.endpoint = srv.URL
	start := time.Date(2026, 9, 11, 1, 2, 3, 0, time.UTC)
	got, err := c.invocations(context.Background(), start, start.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("invocations: %v", err)
	}
	if got.WorkerRequests != 3 || got.DORequests != 4 {
		t.Fatalf("got %+v", got)
	}
	if gotAuth != "Bearer tok456" {
		t.Fatalf("Authorization header was %q", gotAuth)
	}
	if gotVars["accountTag"] != "acct123" || gotVars["scriptName"] != "k8flare" {
		t.Fatalf("variables were %+v", gotVars)
	}
	if gotVars["start"] != "2026-09-11T01:02:03Z" || gotVars["end"] != "2026-09-11T01:12:03Z" {
		t.Fatalf("window variables were %+v", gotVars)
	}
}

func TestInvocationsRejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"errors":[{"message":"Authentication error"}]}`)
	}))
	defer srv.Close()

	c := newAnalyticsClient("acct", "bad", "k8flare")
	c.endpoint = srv.URL
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.invocations(ctx, time.Now(), time.Now()); err == nil {
		t.Fatal("want an error for HTTP 403, got nil")
	}
}
