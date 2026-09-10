//go:build js && wasm

package clusterop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	cffetch "github.com/k8flare/k8flare/pkg/cfruntime/cloudflare/fetch"
)

// bridge is the operator's platform-operations channel: the handful of
// actions that are NOT Kubernetes object writes -- minting into a tenant
// cluster's token vault, upserting the /c/<id> resolution cache, and
// running the teardown cascade over Durable Objects and Containers.
//
// It exists because a Loader-loaded dynamic worker cannot be handed a
// Durable Object namespace at all (docs/platform-verification.md S2: only
// plain values and Fetchers survive the env clone), so none of those
// actions can happen in Go. They live in the shell Worker behind
// /internal/clusters/* (packages/k8flare-worker/src/clusters/internalapi.ts)
// and this type is the typed caller. Same binding-backed transport as
// pkg/controllers/restconfig -- the GATEWAY Fetcher, not a real socket.
type bridge struct {
	http  *http.Client
	base  string
	token string
}

func newBridge(bindingName, token string) *bridge {
	client := cffetch.NewClient(cffetch.WithLiveBinding(bindingName))
	return &bridge{
		http:  client.HTTPClient(cffetch.RedirectModeFollow),
		base:  "https://" + bindingName + ".k8flare.internal",
		token: token,
	}
}

// vaultResult is the shell Worker's answer for both vault endpoints: the
// cluster's currently-distributable credentials, plus (for a rotation)
// the token ids this new one supersedes.
type vaultResult struct {
	TokenID    string   `json:"tokenId"`
	Token      string   `json:"token"`
	Kubeconfig string   `json:"kubeconfig"`
	Endpoint   string   `json:"endpoint"`
	Superseded []string `json:"superseded"`
}

func (b *bridge) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, string(raw))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// EnsureVault initializes doName's token vault and returns its first
// token, or -- if the vault already has one -- returns that existing
// token unchanged. Idempotent by design: a reconcile that crashed after
// minting but before writing the Secret must be able to resume without
// stranding a token nobody holds.
func (b *bridge) EnsureVault(ctx context.Context, doName string) (*vaultResult, error) {
	var out vaultResult
	if err := b.do(ctx, http.MethodPost, "/internal/clusters/vault/"+doName, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MintToken adds a NEW token to doName's vault under the caller-chosen
// tokenID, leaving the existing ones valid, and reports which ids it
// supersedes so the caller can revoke them once the replacement is
// distributed.
//
// tokenID is deterministic (see rotationTokenID), which is what makes a
// rotation replayable: a reconcile that crashed after minting but before
// finishing re-requests the SAME id and gets the SAME token back instead
// of stranding one extra valid token per attempt.
func (b *bridge) MintToken(ctx context.Context, doName, tokenID string) (*vaultResult, error) {
	var out vaultResult
	body := map[string]string{"tokenId": tokenID}
	if err := b.do(ctx, http.MethodPost, "/internal/clusters/vault/"+doName+"/tokens", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeToken drops one token from doName's vault.
func (b *bridge) RevokeToken(ctx context.Context, doName, tokenID string) error {
	return b.do(ctx, http.MethodDelete, "/internal/clusters/vault/"+doName+"/tokens/"+tokenID, nil, nil)
}

// UpsertRegistry writes the /c/<id> resolution cache entry. The Cluster
// object is the truth and this registry is a pure cache the operator can
// rebuild in full (docs/cluster-api-design.md, review point #5), so uid is
// passed in rather than allocated there -- the operator is the only
// doName allocator. The shell Worker refuses to overwrite a record whose
// uid differs, which would silently repoint a live cluster at another
// DO tree.
func (b *bridge) UpsertRegistry(ctx context.Context, id, uid string) error {
	return b.do(ctx, http.MethodPut, "/internal/clusters/registry/"+id, map[string]string{"uid": uid}, nil)
}

// DeleteRegistry drops the resolution cache entry for id.
func (b *bridge) DeleteRegistry(ctx context.Context, id string) error {
	return b.do(ctx, http.MethodDelete, "/internal/clusters/registry/"+id, nil, nil)
}

// Teardown runs the ordered DO/Containers cascade (Scheduler destroy
// first -- live Containers are wall-clock billed -- then Controllers,
// WatchHub, Cluster, registry). Awaited, not fire-and-forget: the
// operator only removes the Cluster's finalizer once this returns, so a
// half-finished teardown is retried by the next reconcile rather than
// leaving orphaned DO state behind an object that is already gone.
func (b *bridge) Teardown(ctx context.Context, id, doName string) error {
	return b.do(ctx, http.MethodPost, "/internal/clusters/teardown/"+id, map[string]string{"doName": doName}, nil)
}
