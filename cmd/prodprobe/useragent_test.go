package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestProbeUserAgentCarriesTheRunID(t *testing.T) {
	t.Setenv("GITHUB_RUN_ID", "34617534443")
	got := ProbeUserAgent()
	if want := "k8flare-prodprobe/34617534443"; got != want {
		t.Fatalf("ProbeUserAgent() = %q, want %q", got, want)
	}
}

func TestProbeUserAgentIsStillDistinctWithoutGitHub(t *testing.T) {
	t.Setenv("GITHUB_RUN_ID", "")
	got := ProbeUserAgent()
	if !strings.HasPrefix(got, "k8flare-prodprobe/") {
		t.Fatalf("ProbeUserAgent() = %q, want the k8flare-prodprobe/ prefix", got)
	}
	if got == "k8flare-prodprobe/" {
		t.Fatal("ProbeUserAgent() has no run id at all; two local runs would be indistinguishable")
	}
}

func TestTheUserAgentReachesTheServer(t *testing.T) {
	t.Setenv("GITHUB_RUN_ID", "42")
	seen := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case seen <- r.Header.Get("User-Agent"):
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"kind":"NamespaceList","apiVersion":"v1","items":[]}`))
	}))
	defer srv.Close()

	cs, err := newClientset(srv.URL, "t")
	if err != nil {
		t.Fatalf("newClientset: %v", err)
	}
	if _, err := cs.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	got := <-seen
	if !strings.Contains(got, "k8flare-prodprobe/42") {
		t.Errorf("server saw User-Agent %q, want it to contain %q", got, "k8flare-prodprobe/42")
	}
}
