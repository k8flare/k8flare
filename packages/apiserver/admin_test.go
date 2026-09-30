package apiserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

type recordedCall struct {
	method, path, query, body string
}

func adminMux(t *testing.T, respond func(r *http.Request) (int, string)) (*http.ServeMux, *[]recordedCall) {
	t.Helper()
	var mu sync.Mutex
	calls := &[]recordedCall{}
	client := &kine.Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
		}
		mu.Lock()
		*calls = append(*calls, recordedCall{r.Method, r.URL.Path, r.URL.RawQuery, string(body)})
		mu.Unlock()
		code, out := respond(r)
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(out)), Header: http.Header{}, Request: r}, nil
	})}}
	mux := http.NewServeMux()
	installSnapshots(mux, client)
	return mux, calls
}

func callAdmin(mux http.Handler, method, target, body string, groups ...string) (int, string) {
	req := httptest.NewRequest(method, target, bytes.NewReader([]byte(body)))
	req = req.WithContext(genericapirequest.WithUser(req.Context(), &user.DefaultInfo{Name: "someone", Groups: groups}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestSnapshotRoutesAreAdminOnly(t *testing.T) {
	mux, calls := adminMux(t, func(*http.Request) (int, string) { return 200, "{}" })
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/internal/snapshots"},
		{http.MethodGet, "/internal/snapshots"},
		{http.MethodPost, "/internal/snapshots/restore"},
		{http.MethodPost, "/internal/restore?to=2026-01-01T00:00:00Z"},
	} {
		if code, _ := callAdmin(mux, c.method, c.path, "{}", user.AllAuthenticated); code != http.StatusForbidden {
			t.Fatalf("%s %s as non-admin: %d", c.method, c.path, code)
		}
	}
	if len(*calls) != 0 {
		t.Fatalf("non-admin reached the store: %v", *calls)
	}
}

func TestSnapshotRoutesForwardToTheClusterStore(t *testing.T) {
	mux, calls := adminMux(t, func(r *http.Request) (int, string) {
		if r.URL.Path == "/snapshot/restore" {
			return http.StatusConflict, `{"error":"cluster is not empty"}`
		}
		return http.StatusOK, `{"ok":true}`
	})
	if code, body := callAdmin(mux, http.MethodPost, "/internal/snapshots", "", user.SystemPrivilegedGroup); code != 200 || body != `{"ok":true}` {
		t.Fatalf("save: %d %s", code, body)
	}
	if code, _ := callAdmin(mux, http.MethodGet, "/internal/snapshots", "", user.SystemPrivilegedGroup); code != 200 {
		t.Fatalf("list: %d", code)
	}
	code, body := callAdmin(mux, http.MethodPost, "/internal/snapshots/restore", `{"key":"k","force":false}`, user.SystemPrivilegedGroup)
	if code != http.StatusConflict || !strings.Contains(body, "not empty") {
		t.Fatalf("restore keeps the store status: %d %s", code, body)
	}
	want := []recordedCall{
		{"POST", "/snapshot", "", ""},
		{"GET", "/snapshots", "", ""},
		{"POST", "/snapshot/restore", "", `{"key":"k","force":false}`},
	}
	if len(*calls) != len(want) {
		t.Fatalf("calls: %v", *calls)
	}
	for i, c := range want {
		if (*calls)[i] != c {
			t.Fatalf("call %d: %v want %v", i, (*calls)[i], c)
		}
	}
}

func TestPointInTimeRestorePreparesThenApplies(t *testing.T) {
	mux, calls := adminMux(t, func(r *http.Request) (int, string) {
		if r.URL.Path == "/restore/apply" {
			return http.StatusInternalServerError, "Durable Object reset"
		}
		return http.StatusOK, `{"ok":true,"bookmark":"b1"}`
	})
	code, body := callAdmin(mux, http.MethodPost, "/internal/restore?to=2026-01-01T00%3A00%3A00Z", "", user.SystemPrivilegedGroup)
	if code != http.StatusOK || !strings.Contains(body, `"bookmark":"b1"`) {
		t.Fatalf("restore: %d %s", code, body)
	}
	if len(*calls) != 2 || (*calls)[0].path != "/restore" || (*calls)[0].query != "to=2026-01-01T00%3A00%3A00Z" || (*calls)[1].path != "/restore/apply" {
		t.Fatalf("calls: %v", *calls)
	}
}

func TestPointInTimeRestoreReportsPrepareFailure(t *testing.T) {
	mux, calls := adminMux(t, func(*http.Request) (int, string) { return http.StatusBadRequest, "invalid to" })
	code, body := callAdmin(mux, http.MethodPost, "/internal/restore?to=nonsense", "", user.SystemPrivilegedGroup)
	if code != http.StatusBadRequest || !strings.Contains(body, "invalid to") {
		t.Fatalf("restore: %d %s", code, body)
	}
	if len(*calls) != 1 {
		t.Fatalf("apply ran after a failed prepare: %v", *calls)
	}
}

func fakeVaultStore() *kine.Client {
	var mu sync.Mutex
	store := map[string][]byte{}
	revs := map[string]int64{}
	var rev int64
	return &kine.Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		reply := func(code int, v any) (*http.Response, error) {
			data, _ := json.Marshal(v)
			return &http.Response{StatusCode: code, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}, Request: r}, nil
		}
		switch {
		case r.URL.Path == "/kv" && r.Method == http.MethodGet:
			k := r.URL.Query().Get("key")
			if v, ok := store[k]; ok {
				return reply(200, map[string]any{"revision": rev, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": revs[k]}})
			}
			return reply(200, map[string]any{"revision": rev})
		case r.URL.Path == "/kv" && r.Method == http.MethodPut:
			var body struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &body)
			raw, _ := base64.StdEncoding.DecodeString(body.Value)
			rev++
			store[body.Key] = raw
			revs[body.Key] = rev
			return reply(201, map[string]any{"revision": rev})
		case r.URL.Path == "/kv" && r.Method == http.MethodDelete:
			var body struct {
				Key string `json:"key"`
			}
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &body)
			if _, ok := store[body.Key]; !ok {
				return reply(404, map[string]any{"revision": rev})
			}
			delete(store, body.Key)
			return reply(200, map[string]any{"revision": rev})
		case r.URL.Path == "/list":
			prefix := r.URL.Query().Get("prefix")
			kvs := []map[string]any{}
			for k, v := range store {
				if strings.HasPrefix(k, prefix) {
					kvs = append(kvs, map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": revs[k]})
				}
			}
			return reply(200, map[string]any{"revision": rev, "kvs": kvs})
		}
		return reply(404, map[string]any{})
	})}}
}

func tokensMux() *http.ServeMux {
	mux := http.NewServeMux()
	installTokens(mux, supervisor.NewVault(fakeVaultStore()))
	return mux
}

func TestTokenRoutesAreAdminOnly(t *testing.T) {
	mux := tokensMux()
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/internal/tokens"},
		{http.MethodGet, "/internal/tokens"},
		{http.MethodDelete, "/internal/tokens/abc123"},
		{http.MethodPost, "/internal/tokens/abc123/rotate"},
	} {
		if code, _ := callAdmin(mux, c.method, c.path, "{}", user.AllAuthenticated); code != http.StatusForbidden {
			t.Fatalf("%s %s as non-admin: %d", c.method, c.path, code)
		}
	}
}

func TestTokenLifecycleThroughTheRoutes(t *testing.T) {
	mux := tokensMux()
	admin := user.SystemPrivilegedGroup
	code, body := callAdmin(mux, http.MethodPost, "/internal/tokens", `{"description":"ci","ttlSeconds":3600}`, admin)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, body)
	}
	var created struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || !strings.HasPrefix(created.Token, created.ID+".") {
		t.Fatalf("create body: %s", body)
	}
	code, body = callAdmin(mux, http.MethodGet, "/internal/tokens", "", admin)
	if code != http.StatusOK || !strings.Contains(body, created.ID) || !strings.Contains(body, `"description":"ci"`) || strings.Contains(body, created.Token) {
		t.Fatalf("list: %d %s", code, body)
	}
	code, body = callAdmin(mux, http.MethodPost, "/internal/tokens/"+created.ID+"/rotate", "", admin)
	var rotated struct {
		Token string `json:"token"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &rotated) != nil || rotated.Token == created.Token || !strings.HasPrefix(rotated.Token, created.ID+".") {
		t.Fatalf("rotate: %d %s", code, body)
	}
	if code, _ := callAdmin(mux, http.MethodDelete, "/internal/tokens/"+created.ID, "", admin); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := callAdmin(mux, http.MethodDelete, "/internal/tokens/"+created.ID, "", admin); code != http.StatusNotFound {
		t.Fatalf("delete again: %d", code)
	}
	if code, _ := callAdmin(mux, http.MethodPost, "/internal/tokens/"+created.ID+"/rotate", "", admin); code != http.StatusNotFound {
		t.Fatalf("rotate deleted: %d", code)
	}
}

func TestTokenCreateRejectsNegativeTTL(t *testing.T) {
	if code, _ := callAdmin(tokensMux(), http.MethodPost, "/internal/tokens", `{"ttlSeconds":-1}`, user.SystemPrivilegedGroup); code != http.StatusBadRequest {
		t.Fatalf("negative ttl: %d", code)
	}
}
