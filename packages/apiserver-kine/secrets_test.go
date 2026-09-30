package kine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
)

type memKine struct {
	mu   sync.Mutex
	rev  int64
	data map[string][]byte
	revs map[string]int64
}

func newMemKine(t *testing.T) (*memKine, *http.Client) {
	m := &memKine{data: map[string][]byte{}, revs: map[string]int64{}}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return m, rewriteListClient(srv)
}

func (m *memKine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	reply := func(code int, body any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /kv":
		key := r.URL.Query().Get("key")
		v, ok := m.data[key]
		if !ok {
			reply(http.StatusNotFound, map[string]any{"revision": m.rev})
			return
		}
		reply(http.StatusOK, map[string]any{"revision": m.rev, "kv": KV{Key: key, Value: base64.StdEncoding.EncodeToString(v), ModRevision: m.revs[key]}})
	case "PUT /kv":
		var body struct {
			Key      string `json:"key"`
			Value    string `json:"value"`
			Revision int64  `json:"revision"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, exists := m.data[body.Key]; body.Revision != m.revs[body.Key] || (body.Revision == 0 && exists) {
			reply(http.StatusConflict, map[string]any{"revision": m.rev})
			return
		}
		raw, _ := base64.StdEncoding.DecodeString(body.Value)
		m.rev++
		m.data[body.Key] = raw
		m.revs[body.Key] = m.rev
		reply(http.StatusOK, map[string]any{"revision": m.rev})
	case "GET /list":
		q := r.URL.Query()
		var kvs []KV
		for key, v := range m.data {
			if strings.HasPrefix(key, q.Get("prefix")) && key >= q.Get("from") {
				kvs = append(kvs, KV{Key: key, Value: base64.StdEncoding.EncodeToString(v), ModRevision: m.revs[key]})
			}
		}
		sort.Slice(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })
		reply(http.StatusOK, map[string]any{"revision": m.rev, "kvs": kvs})
	default:
		reply(http.StatusNotFound, map[string]any{})
	}
}

const (
	keyA = "a:MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	keyB = "b:ZmVkY2JhOTg3NjU0MzIxMGZlZGNiYTk4NzY1NDMyMTA="
)

func mustCipher(t *testing.T, spec string) *SecretCipher {
	t.Helper()
	c, err := ParseSecretKeys(spec)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func secretStorage(client *Client) *Storage {
	codec := scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
	return NewStorage(client, codec, func() runtime.Object { return &corev1.Secret{} })
}

func newSecret(name, value string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: name}, Data: map[string][]byte{"k": []byte(value)}}
}

func TestSecretsRoundTripStoresUpstreamPrefixedCiphertext(t *testing.T) {
	mem, hc := newMemKine(t)
	s := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, keyA)})
	ctx := context.Background()
	if err := s.Create(ctx, "/secrets/ns/s1", newSecret("s1", "hunter2"), &corev1.Secret{}, 0); err != nil {
		t.Fatal(err)
	}
	stored := mem.data["/registry/secrets/ns/s1"]
	if !bytes.HasPrefix(stored, []byte("k8s:enc:aesgcm:v1:a:")) || bytes.Contains(stored, []byte("hunter2")) {
		t.Fatalf("stored value is not upstream aesgcm ciphertext: %q", stored)
	}
	var got corev1.Secret
	if err := s.Get(ctx, "/secrets/ns/s1", storage.GetOptions{}, &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Data["k"]) != "hunter2" {
		t.Fatalf("got %q", got.Data["k"])
	}
}

func TestSecretsOnlyEncryptsTheSecretsPrefix(t *testing.T) {
	mem, hc := newMemKine(t)
	client := &Client{HTTP: hc, Secrets: mustCipher(t, keyA)}
	if _, err := client.Put(context.Background(), "/registry/configmaps/ns/c", []byte("plain"), 0); err != nil {
		t.Fatal(err)
	}
	if string(mem.data["/registry/configmaps/ns/c"]) != "plain" {
		t.Fatalf("configmap was transformed: %q", mem.data["/registry/configmaps/ns/c"])
	}
}

func TestSecretsPlaintextStillReadsAfterEnablingEncryption(t *testing.T) {
	_, hc := newMemKine(t)
	ctx := context.Background()
	before := secretStorage(&Client{HTTP: hc})
	if err := before.Create(ctx, "/secrets/ns/old", newSecret("old", "legacy"), nil, 0); err != nil {
		t.Fatal(err)
	}
	after := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, keyA)})
	var got corev1.Secret
	if err := after.Get(ctx, "/secrets/ns/old", storage.GetOptions{}, &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Data["k"]) != "legacy" {
		t.Fatalf("got %q", got.Data["k"])
	}
}

func TestSecretsRotationDecryptsWithAnyKeyAndWritesWithTheFirst(t *testing.T) {
	mem, hc := newMemKine(t)
	ctx := context.Background()
	old := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, keyA)})
	if err := old.Create(ctx, "/secrets/ns/s1", newSecret("s1", "v1"), nil, 0); err != nil {
		t.Fatal(err)
	}
	rotated := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, keyB+","+keyA)})
	var got corev1.Secret
	if err := rotated.Get(ctx, "/secrets/ns/s1", storage.GetOptions{}, &got); err != nil || string(got.Data["k"]) != "v1" {
		t.Fatalf("read with old key: %v %q", err, got.Data["k"])
	}
	if err := rotated.Create(ctx, "/secrets/ns/s2", newSecret("s2", "v2"), nil, 0); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(mem.data["/registry/secrets/ns/s2"], []byte("k8s:enc:aesgcm:v1:b:")) {
		t.Fatalf("new write not under the first key: %q", mem.data["/registry/secrets/ns/s2"])
	}
	dropped := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, keyB)})
	if err := dropped.Get(ctx, "/secrets/ns/s1", storage.GetOptions{}, &got); err == nil {
		t.Fatal("secret encrypted with a removed key must not be readable")
	}
}

func TestSecretsEncryptedValueWithoutKeysIsAnErrorNotGarbage(t *testing.T) {
	_, hc := newMemKine(t)
	ctx := context.Background()
	enc := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, keyA)})
	if err := enc.Create(ctx, "/secrets/ns/s1", newSecret("s1", "v1"), nil, 0); err != nil {
		t.Fatal(err)
	}
	var got corev1.Secret
	if err := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, "")}).Get(ctx, "/secrets/ns/s1", storage.GetOptions{}, &got); err == nil {
		t.Fatal("expected an error reading ciphertext without keys")
	}
}

func TestSecretsListDecryptsEveryItem(t *testing.T) {
	_, hc := newMemKine(t)
	ctx := context.Background()
	s := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, keyA)})
	for _, n := range []string{"a", "b", "c"} {
		if err := s.Create(ctx, "/secrets/ns/"+n, newSecret(n, "val-"+n), nil, 0); err != nil {
			t.Fatal(err)
		}
	}
	var list corev1.SecretList
	pred := storage.SelectionPredicate{Label: labels.Everything(), Field: fields.Everything(), GetAttrs: storage.DefaultNamespaceScopedAttr}
	if err := s.GetList(ctx, "/secrets/ns", storage.ListOptions{Recursive: true, Predicate: pred}, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 3 {
		t.Fatalf("items = %d", len(list.Items))
	}
	for _, it := range list.Items {
		if string(it.Data["k"]) != "val-"+it.Name {
			t.Fatalf("%s = %q", it.Name, it.Data["k"])
		}
	}
}

func TestSecretsWatchEventsDecrypt(t *testing.T) {
	_, hc := newMemKine(t)
	client := &Client{HTTP: hc, Secrets: mustCipher(t, keyA)}
	s := secretStorage(client)
	plain, err := s.encode(newSecret("s1", "watched"))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := client.Secrets.Seal("/registry/secrets/ns/s1", plain)
	if err != nil {
		t.Fatal(err)
	}
	ev := kineEvent{Rev: 5, Type: "created", Key: "/registry/secrets/ns/s1", Value: base64.StdEncoding.EncodeToString(sealed)}
	out, ok, err := s.watchEvent(ev, storage.Everything)
	if err != nil || !ok {
		t.Fatal("event dropped")
	}
	if got := out.Object.(*corev1.Secret); string(got.Data["k"]) != "watched" {
		t.Fatalf("got %q", got.Data["k"])
	}
}

func TestReencryptRewritesEverySecretUnderTheFirstKey(t *testing.T) {
	mem, hc := newMemKine(t)
	ctx := context.Background()
	plain := secretStorage(&Client{HTTP: hc})
	if err := plain.Create(ctx, "/secrets/ns/plain", newSecret("plain", "p"), nil, 0); err != nil {
		t.Fatal(err)
	}
	old := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, keyA)})
	if err := old.Create(ctx, "/secrets/ns/old", newSecret("old", "o"), nil, 0); err != nil {
		t.Fatal(err)
	}
	client := &Client{HTTP: hc, Secrets: mustCipher(t, keyB+","+keyA)}
	if err := secretStorage(client).Create(ctx, "/secrets/ns/current", newSecret("current", "c"), nil, 0); err != nil {
		t.Fatal(err)
	}
	before, err := client.SecretsStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.Total != 3 || before.Current != 1 || before.Stale != 2 {
		t.Fatalf("before: %+v", before)
	}
	n, err := client.ReencryptSecrets(ctx)
	if err != nil || n != 2 {
		t.Fatalf("reencrypted %d, err %v", n, err)
	}
	for _, name := range []string{"plain", "old", "current"} {
		if !bytes.HasPrefix(mem.data["/registry/secrets/ns/"+name], []byte("k8s:enc:aesgcm:v1:b:")) {
			t.Fatalf("%s not under key b: %q", name, mem.data["/registry/secrets/ns/"+name])
		}
	}
	after, err := client.SecretsStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Stale != 0 || after.Current != 3 || after.ActiveKey != "aesgcm b" || len(after.InactiveKeys) != 1 || after.InactiveKeys[0] != "aesgcm a" || !after.Enabled {
		t.Fatalf("after: %+v", after)
	}
	trimmed := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, keyB)})
	var got corev1.Secret
	if err := trimmed.Get(ctx, "/secrets/ns/old", storage.GetOptions{}, &got); err != nil || string(got.Data["k"]) != "o" {
		t.Fatalf("after dropping key a: %v %q", err, got.Data["k"])
	}
}

func TestParseSecretKeysRejectsBadInput(t *testing.T) {
	for name, spec := range map[string]string{
		"missing secret": "a",
		"bad base64":     "a:!!!",
		"short key":      "a:c2hvcnQ=",
		"duplicate name": keyA + "," + keyA,
		"empty entry":    keyA + ",",
	} {
		if _, err := ParseSecretKeys(spec); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if c, err := ParseSecretKeys(""); err != nil || c == nil {
		t.Fatalf("empty spec: %v", err)
	}
}

func TestSecretsWriteWithoutKeysIsRejected(t *testing.T) {
	mem, hc := newMemKine(t)
	ctx := context.Background()
	client := &Client{HTTP: hc, Secrets: mustCipher(t, "")}
	s := secretStorage(client)
	err := s.Create(ctx, "/secrets/ns/s1", newSecret("s1", "v1"), nil, 0)
	if err == nil || !strings.Contains(err.Error(), "SECRETS_ENCRYPTION_KEYS") {
		t.Fatalf("create: %v", err)
	}
	if len(mem.data) != 0 {
		t.Fatalf("plaintext was stored: %v", mem.data)
	}
	if _, err := client.Put(ctx, "/registry/configmaps/ns/c", []byte("ok"), 0); err != nil {
		t.Fatalf("non-secret write: %v", err)
	}
}

func TestSecretsPlaintextReadsWithoutKeys(t *testing.T) {
	_, hc := newMemKine(t)
	ctx := context.Background()
	if err := secretStorage(&Client{HTTP: hc}).Create(ctx, "/secrets/ns/old", newSecret("old", "legacy"), nil, 0); err != nil {
		t.Fatal(err)
	}
	var got corev1.Secret
	if err := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, "")}).Get(ctx, "/secrets/ns/old", storage.GetOptions{}, &got); err != nil || string(got.Data["k"]) != "legacy" {
		t.Fatalf("%v %q", err, got.Data["k"])
	}
}

func TestReencryptWithoutKeysIsRejected(t *testing.T) {
	_, hc := newMemKine(t)
	client := &Client{HTTP: hc, Secrets: mustCipher(t, "")}
	if _, err := client.ReencryptSecrets(context.Background()); err == nil || !strings.Contains(err.Error(), "SECRETS_ENCRYPTION_KEYS") {
		t.Fatalf("reencrypt: %v", err)
	}
}

func TestSecretsWatchDecryptFailureSendsAnErrorEvent(t *testing.T) {
	_, hc := newMemKine(t)
	previous := WatchDialer
	defer func() { WatchDialer = previous }()
	msgs := make(chan []byte, 1)
	msgs <- []byte(`{"rev":5,"type":"created","key":"/registry/secrets/ns/s1","value":"` + base64.StdEncoding.EncodeToString([]byte("k8s:enc:aesgcm:v1:a:garbage")) + `"}`)
	WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) {
		return msgs, func() {}, nil
	}
	s := secretStorage(&Client{HTTP: hc, Secrets: mustCipher(t, keyA)})
	w, err := s.Watch(context.Background(), "/secrets/ns/", storage.ListOptions{Recursive: true, ResourceVersion: "1", Predicate: storage.Everything})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	select {
	case ev := <-w.ResultChan():
		if ev.Type != watch.Error {
			t.Fatalf("event %v", ev.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
	}
}
