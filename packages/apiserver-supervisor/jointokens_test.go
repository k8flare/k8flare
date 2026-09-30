package supervisor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

func fakeKine(t *testing.T) *kine.Client {
	t.Helper()
	var mu sync.Mutex
	store := map[string][]byte{}
	revs := map[string]int64{}
	var rev int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/kv" && r.Method == http.MethodGet:
			k := r.URL.Query().Get("key")
			if v, ok := store[k]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": rev, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": revs[k]}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": rev})
		case r.URL.Path == "/kv" && r.Method == http.MethodPut:
			var body struct {
				Key      string `json:"key"`
				Value    string `json:"value"`
				Revision int64  `json:"revision"`
			}
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &body)
			_, exists := store[body.Key]
			if body.Revision == 0 && exists || body.Revision != 0 && revs[body.Key] != body.Revision {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": rev})
				return
			}
			raw, _ := base64.StdEncoding.DecodeString(body.Value)
			rev++
			store[body.Key] = raw
			revs[body.Key] = rev
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": rev})
		case r.URL.Path == "/kv" && r.Method == http.MethodDelete:
			var body struct {
				Key string `json:"key"`
			}
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &body)
			if _, ok := store[body.Key]; !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": rev})
				return
			}
			delete(store, body.Key)
			rev++
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": rev})
		case r.URL.Path == "/list":
			prefix := r.URL.Query().Get("prefix")
			var names []string
			for k := range store {
				if strings.HasPrefix(k, prefix) {
					names = append(names, k)
				}
			}
			sort.Strings(names)
			kvs := []map[string]any{}
			for _, k := range names {
				kvs = append(kvs, map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(store[k]), "modRevision": revs[k]})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": rev, "kvs": kvs})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &kine.Client{HTTP: &http.Client{Transport: rewrite{base: srv.URL, next: srv.Client().Transport}}}
}

func joinRequest(mux *http.ServeMux, token string) int {
	req := httptest.NewRequest(http.MethodGet, "/v1-k3s/readyz", nil)
	req.SetBasicAuth("node", token)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr.Code
}

func TestSupervisorAcceptsCreatedJoinTokenAndRejectsDeleted(t *testing.T) {
	ctx := context.Background()
	v := NewVault(fakeKine(t))
	mux := http.NewServeMux()
	New(v, "static").Register(mux)

	created, secret, err := v.CreateJoinToken(ctx, "ci", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, created.ID+".") || created.ExpiresAt == nil {
		t.Fatalf("token %q record %+v", secret, created)
	}
	if code := joinRequest(mux, secret); code != http.StatusOK {
		t.Fatalf("created token: %d", code)
	}
	if code := joinRequest(mux, "static"); code != http.StatusOK {
		t.Fatalf("static token: %d", code)
	}
	if code := joinRequest(mux, created.ID+".wrongwrongwrong00"); code != http.StatusUnauthorized {
		t.Fatalf("wrong secret: %d", code)
	}
	if err := v.DeleteJoinToken(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if code := joinRequest(mux, secret); code != http.StatusUnauthorized {
		t.Fatalf("deleted token: %d", code)
	}
	if err := v.DeleteJoinToken(ctx, created.ID); err != ErrJoinTokenNotFound {
		t.Fatalf("delete twice: %v", err)
	}
}

func TestSupervisorRejectsExpiredJoinToken(t *testing.T) {
	ctx := context.Background()
	v := NewVault(fakeKine(t))
	mux := http.NewServeMux()
	New(v, "").Register(mux)
	_, secret, err := v.CreateJoinToken(ctx, "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if code := joinRequest(mux, secret); code != http.StatusOK {
		t.Fatalf("fresh: %d", code)
	}
	v.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if code := joinRequest(mux, secret); code != http.StatusUnauthorized {
		t.Fatalf("expired: %d", code)
	}
}

func TestJoinTokenWithoutTTLNeverExpires(t *testing.T) {
	ctx := context.Background()
	v := NewVault(fakeKine(t))
	created, secret, err := v.CreateJoinToken(ctx, "forever", 0)
	if err != nil || created.ExpiresAt != nil {
		t.Fatalf("%+v %v", created, err)
	}
	v.now = func() time.Time { return time.Now().Add(100 * 365 * 24 * time.Hour) }
	if err := v.CheckJoinToken(ctx, secret); err != nil {
		t.Fatal(err)
	}
}

func TestListJoinTokensHidesSecrets(t *testing.T) {
	ctx := context.Background()
	v := NewVault(fakeKine(t))
	a, secretA, _ := v.CreateJoinToken(ctx, "a", time.Hour)
	if _, _, err := v.CreateJoinToken(ctx, "b", 0); err != nil {
		t.Fatal(err)
	}
	listed, err := v.ListJoinTokens(ctx)
	if err != nil || len(listed) != 2 {
		t.Fatalf("%+v %v", listed, err)
	}
	raw, _ := json.Marshal(listed)
	if strings.Contains(string(raw), strings.TrimPrefix(secretA, a.ID+".")) {
		t.Fatalf("list leaks the secret: %s", raw)
	}
}

func TestRotateJoinTokenInvalidatesTheOldSecret(t *testing.T) {
	ctx := context.Background()
	v := NewVault(fakeKine(t))
	created, oldSecret, _ := v.CreateJoinToken(ctx, "rot", time.Hour)
	rotated, newSecret, err := v.RotateJoinToken(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ID != created.ID || newSecret == oldSecret || !strings.HasPrefix(newSecret, created.ID+".") {
		t.Fatalf("%+v %q", rotated, newSecret)
	}
	if err := v.CheckJoinToken(ctx, oldSecret); err == nil {
		t.Fatal("old secret still valid")
	}
	if err := v.CheckJoinToken(ctx, newSecret); err != nil {
		t.Fatal(err)
	}
	if _, _, err := v.RotateJoinToken(ctx, "nope00"); err != ErrJoinTokenNotFound {
		t.Fatalf("rotate unknown: %v", err)
	}
}
