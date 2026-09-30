package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"sync"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

type memoryKine struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (m *memoryKine) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	reply := func(status int, body any) (*http.Response, error) {
		raw, _ := json.Marshal(body)
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(raw)), Header: http.Header{}, Request: req}, nil
	}
	switch req.Method {
	case http.MethodGet:
		key := req.URL.Query().Get("key")
		value, ok := m.data[key]
		if !ok {
			return reply(http.StatusNotFound, map[string]any{})
		}
		return reply(http.StatusOK, map[string]any{"revision": 1, "kv": map[string]any{"key": key, "value": base64.StdEncoding.EncodeToString(value), "modRevision": 1}})
	case http.MethodPut:
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		if _, ok := m.data[body.Key]; ok {
			return reply(http.StatusConflict, map[string]any{})
		}
		value, _ := base64.StdEncoding.DecodeString(body.Value)
		m.data[body.Key] = value
		return reply(http.StatusOK, map[string]any{"revision": 2})
	}
	return reply(http.StatusMethodNotAllowed, map[string]any{})
}

func newMemoryKine() (*kine.Client, *memoryKine) {
	m := &memoryKine{data: map[string][]byte{}}
	return &kine.Client{HTTP: &http.Client{Transport: m}}, m
}

func pemKey(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func TestInstallServiceAccountKeyGeneratesRandomKeyOnFirstUse(t *testing.T) {
	secret := []byte("same-admin-token")
	var installed []*rsa.PrivateKey
	for i := 0; i < 2; i++ {
		store, memory := newMemoryKine()
		saKeys.Delete(string(secret))
		if err := InstallServiceAccountKey(context.Background(), store, secret); err != nil {
			t.Fatal(err)
		}
		key, err := saPrivateKey(secret)
		if err != nil {
			t.Fatal(err)
		}
		persisted, ok := memory.data[saSigningKey]
		if !ok || !bytes.Equal(persisted, pemKey(key)) {
			t.Fatal("key not persisted")
		}
		installed = append(installed, key)
	}
	if installed[0].Equal(installed[1]) {
		t.Fatal("key is determined by the secret")
	}
}

func TestInstallServiceAccountKeyKeepsPersistedKey(t *testing.T) {
	secret := []byte("existing-cluster-token")
	existing, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	store, memory := newMemoryKine()
	memory.data[saSigningKey] = pemKey(existing)
	saKeys.Delete(string(secret))
	if err := InstallServiceAccountKey(context.Background(), store, secret); err != nil {
		t.Fatal(err)
	}
	key, err := saPrivateKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	if !key.Equal(existing) {
		t.Fatal("persisted key was replaced")
	}
}
